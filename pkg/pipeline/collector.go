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
	providers   []provider.SignalProvider
	repo        storage.SignalRepository
	validator   *processor.SignalValidator
	aligner     *processor.SlidingWindowAligner
	corrEngine  *correlation.CorrelationEngine
	publisher   publisher.EventPublisher
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
		corrEngine: corrEngine,
		publisher:  pub,
	}
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
			signals, err := prov.Fetch(ctx)
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

	// 3. Sliding Window Alignment & Correlation Analysis
	now := time.Now()
	window, aligned := c.aligner.AlignToWindow(validSignals, now)

	event, err := c.corrEngine.Evaluate(ctx, window, aligned)
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

// RunDaemon starts continuous polling goroutines per provider and streams events to RabbitMQ.
func (c *Collector) RunDaemon(ctx context.Context) error {
	log.Printf("[INFO] Starting Daemon pipeline collector with %d registered providers", len(c.providers))

	signalChan := make(chan model.Signal, 500)
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
				evt, err := c.corrEngine.Evaluate(ctx, window, aligned)
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
			case sig, ok := <-signalChan:
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

			signals, err := prov.Fetch(ctx)
			if err == nil {
				for _, s := range signals {
					signalChan <- s
				}
			}

			ticker := time.NewTicker(prov.PollFrequency())
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					sigs, err := prov.Fetch(ctx)
					if err != nil {
						continue
					}
					for _, s := range sigs {
						signalChan <- s
					}
				case <-ctx.Done():
					return
				}
			}
		}(p)
	}

	<-ctx.Done()
	log.Printf("[INFO] Shutdown signal received. Flushing pipeline buffers...")
	close(signalChan)
	wg.Wait()
	log.Printf("[INFO] Daemon collector pipeline shutdown complete.")
	return nil
}
