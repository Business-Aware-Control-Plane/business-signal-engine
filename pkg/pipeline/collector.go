package pipeline

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/correlation"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/model"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/processor"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/provider"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/publisher"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/storage"
	"github.com/google/uuid"
)

type Collector struct {
	providers  []provider.SignalProvider
	repo       storage.SignalRepository
	validator  *processor.SignalValidator
	aligner    *processor.SlidingWindowAligner
	lkv        *processor.LastKnownValueStore
	corrEngine *correlation.CorrelationEngine
	publisher  publisher.EventPublisher

	// ingest is the persistent fan-in used by RunDaemon: both the per-provider
	// poll producers and any external push source (the webhook ingestor,
	// BSAL-HB-01 §05) write into the same channel, so a webhook delivery is
	// batched and correlated exactly like a polled signal, not a parallel path.
	//
	// ingestMu guards ingestClosed so a concurrent Ingest() and shutdown close
	// can never race — checking "is it closed" and closing it both happen
	// under the same lock, which a bare atomic-bool guard cannot guarantee
	// (a send could still land after the check but during the close).
	ingest       chan model.Signal
	ingestMu     sync.Mutex
	ingestClosed bool
}

func NewCollector(
	repo storage.SignalRepository,
	corrEngine *correlation.CorrelationEngine,
	pub publisher.EventPublisher,
	providers ...provider.SignalProvider,
) *Collector {
	return &Collector{
		providers:  providers,
		repo:       repo,
		validator:  processor.NewSignalValidator(),
		aligner:    processor.NewSlidingWindowAligner(5 * time.Minute),
		lkv:        processor.NewLastKnownValueStore(),
		corrEngine: corrEngine,
		publisher:  pub,
		ingest:     make(chan model.Signal, 500),
	}
}

// Ingest accepts a single push-delivered signal (currently: the webhook
// ingestor) into the same fan-in RunDaemon's poll producers write to. Safe
// to call concurrently, including racing against shutdown: the check and the
// send happen under the same lock a shutdown close also takes, so a signal
// is either delivered or cleanly dropped-and-logged, never sent to a closed
// channel.
func (c *Collector) Ingest(s model.Signal) {
	c.ingestMu.Lock()
	defer c.ingestMu.Unlock()
	if c.ingestClosed {
		log.Printf("[WARN] [Collector] Dropping ingested signal after shutdown: %s:%s", s.Source, s.Type)
		return
	}
	c.ingest <- s
}

// closeIngest is the single place c.ingest is ever closed, guarded so it is
// safe to call even if shutdown logic changes later to call it from more
// than one place.
func (c *Collector) closeIngest() {
	c.ingestMu.Lock()
	defer c.ingestMu.Unlock()
	if !c.ingestClosed {
		c.ingestClosed = true
		close(c.ingest)
	}
}

// providerSourceNames maps each provider's Name() (a display-oriented
// string, not necessarily matching what it puts in model.Signal.Source) to
// the exact Source string its signals actually carry. LastKnownValueStore
// needs this specifically for the empty-fetch case — when a provider polls
// successfully and returns nothing, there is no model.Signal on hand to
// read .Source from, so its identity has to come from somewhere else.
var providerSourceNames = map[string]string{
	"BusinessCalendar":     "business_calendar",
	"Calendar":             "calendar",
	"GoogleAnalytics":      "google_analytics",
	"MetaBusiness":         "meta_business",
	"Prometheus":           "prometheus",
	"SocialMedia":          "social_media",
	"SimulatorStream":      "simulated_google_analytics",
	"Weather":              "weather",
	"StripeReconciliation": "stripe",
}

// fetchAndObserve wraps a single provider's Fetch call, recording the result
// in the last-known-value cache with that provider's own poll frequency
// before the signals ever reach the shared fan-in channel. This is what lets
// a slow-cadence source (weather, calendar) still be carried into a later,
// faster correlation window instead of requiring it to have polled inside
// that exact window (BSAL-HB-01 §03).
func (c *Collector) fetchAndObserve(ctx context.Context, prov provider.SignalProvider) ([]model.Signal, error) {
	signals, err := prov.Fetch(ctx)
	if err != nil {
		return nil, err
	}
	source := providerSourceNames[prov.Name()]
	if source == "" && len(signals) > 0 {
		source = signals[0].Source
	}
	c.lkv.Observe(source, signals, prov.PollFrequency())
	return signals, nil
}

// RunOneShot executes signal collection, validation, correlation, timeline storage, and RabbitMQ publishing once.
func (c *Collector) RunOneShot(ctx context.Context) error {
	log.Printf("[INFO] Starting One-Shot pipeline across %d providers", len(c.providers))

	signalChan := make(chan model.Signal, 200)
	var wg sync.WaitGroup

	for _, p := range c.providers {
		wg.Add(1)
		go func(prov provider.SignalProvider) {
			defer wg.Done()
			log.Printf("[INFO] Executing provider '%s' goroutine...", prov.Name())
			signals, err := c.fetchAndObserve(ctx, prov)
			if err != nil {
				log.Printf("[ERROR] Provider '%s' failed: %v", prov.Name(), err)
				return
			}
			for _, s := range signals {
				signalChan <- s
			}
		}(p)
	}

	go func() {
		wg.Wait()
		close(signalChan)
	}()

	var rawSignals []model.Signal
	for sig := range signalChan {
		rawSignals = append(rawSignals, sig)
	}

	// 1. Validate & Clean Signals
	validSignals := c.validator.ValidateAndClean(rawSignals)

	// 2. Persist Raw Signals to MongoDB
	if len(validSignals) > 0 {
		if err := c.repo.SaveSignals(ctx, validSignals); err != nil {
			log.Printf("[ERROR] Failed to save signals: %v", err)
		}
	}

	// 3. Sliding Window Alignment, carried-forward context, and Correlation Analysis
	now := time.Now()
	window, aligned := c.aligner.AlignToWindow(validSignals, now)
	windowCtx := processor.BuildContext(aligned, c.lkv, now)

	event, err := c.corrEngine.Evaluate(ctx, window, windowCtx)
	if err != nil {
		log.Printf("[WARN] Correlation analysis error: %v", err)
	}

	if event != nil {
		if event.EventID == "" {
			event.EventID = uuid.New().String()
		}

		// 4. Save Event to MongoDB Business Timeline
		if err := c.repo.SaveBusinessEvent(ctx, event); err != nil {
			log.Printf("[ERROR] Failed to save BusinessEvent to timeline: %v", err)
		}

		// 5. Publish Event JSON to RabbitMQ
		if err := c.publisher.PublishBusinessEvent(ctx, event); err != nil {
			log.Printf("[ERROR] Failed to publish BusinessEvent to RabbitMQ: %v", err)
		}
	}

	log.Printf("[INFO] One-Shot pipeline completed successfully. Valid Signals: %d, BusinessEvents Generated: %v", len(validSignals), event != nil)
	return nil
}

// RunDaemon starts continuous polling goroutines per provider, accepts any
// pushed webhook deliveries via Ingest, and streams events to RabbitMQ.
func (c *Collector) RunDaemon(ctx context.Context) error {
	log.Printf("[INFO] Starting Daemon pipeline collector with %d registered providers", len(c.providers))

	var wg sync.WaitGroup

	// Consumer & Correlation Pipeline Goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		var batch []model.Signal
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		processBatch := func() {
			if len(batch) == 0 {
				return
			}
			valid := c.validator.ValidateAndClean(batch)
			if len(valid) > 0 {
				_ = c.repo.SaveSignals(ctx, valid)
				now := time.Now()
				window, aligned := c.aligner.AlignToWindow(valid, now)
				windowCtx := processor.BuildContext(aligned, c.lkv, now)
				evt, err := c.corrEngine.Evaluate(ctx, window, windowCtx)
				if err == nil && evt != nil {
					if evt.EventID == "" {
						evt.EventID = uuid.New().String()
					}
					_ = c.repo.SaveBusinessEvent(ctx, evt)
					_ = c.publisher.PublishBusinessEvent(ctx, evt)
				}
			}
			batch = nil
		}

		for {
			select {
			case sig, ok := <-c.ingest:
				if !ok {
					processBatch()
					return
				}
				batch = append(batch, sig)
				if len(batch) >= 20 {
					processBatch()
				}
			case <-ticker.C:
				processBatch()
			case <-ctx.Done():
				processBatch()
				return
			}
		}
	}()

	// Producer Goroutines
	for _, p := range c.providers {
		wg.Add(1)
		go func(prov provider.SignalProvider) {
			defer wg.Done()

			signals, err := c.fetchAndObserve(ctx, prov)
			if err == nil {
				for _, s := range signals {
					c.Ingest(s)
				}
			}

			ticker := time.NewTicker(prov.PollFrequency())
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					sigs, err := c.fetchAndObserve(ctx, prov)
					if err != nil {
						continue
					}
					for _, s := range sigs {
						c.Ingest(s)
					}
				case <-ctx.Done():
					return
				}
			}
		}(p)
	}

	<-ctx.Done()
	log.Printf("[INFO] Shutdown signal received. Flushing pipeline buffers...")
	c.closeIngest()
	wg.Wait()
	log.Printf("[INFO] Daemon collector pipeline shutdown complete.")
	return nil
}
