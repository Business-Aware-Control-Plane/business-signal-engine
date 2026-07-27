package pipeline

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/model"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/provider"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/storage"
)

type Collector struct {
	providers []provider.SignalProvider
	repo      storage.SignalRepository
}

func NewCollector(repo storage.SignalRepository, providers ...provider.SignalProvider) *Collector {
	return &Collector{
		providers: providers,
		repo:      repo,
	}
}

// RunOneShot executes all registered providers concurrently once, persists all returned signals into MongoDB, and exits.
func (c *Collector) RunOneShot(ctx context.Context) error {
	log.Printf("[INFO] Starting One-Shot signal extraction across %d providers", len(c.providers))

	signalChan := make(chan model.Signal, 200)
	var wg sync.WaitGroup

	// Concurrently trigger each provider in its own goroutine
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

	// Wait for all provider goroutines to finish then close channel
	go func() {
		wg.Wait()
		close(signalChan)
	}()

	var allSignals []model.Signal
	for sig := range signalChan {
		allSignals = append(allSignals, sig)
	}

	if len(allSignals) > 0 {
		if err := c.repo.SaveSignals(ctx, allSignals); err != nil {
			return err
		}
	}

	log.Printf("[INFO] One-Shot collection completed successfully. Total signals persisted: %d", len(allSignals))
	return nil
}

// RunDaemon starts continuous polling goroutines per provider and streams signals to MongoDB.
func (c *Collector) RunDaemon(ctx context.Context) error {
	log.Printf("[INFO] Starting Daemon mode collector with %d registered providers", len(c.providers))

	signalChan := make(chan model.Signal, 500)
	var wg sync.WaitGroup

	// Consumer goroutine: receives signals from channel and saves to MongoDB
	wg.Add(1)
	go func() {
		defer wg.Done()
		var batch []model.Signal
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case sig, ok := <-signalChan:
				if !ok {
					if len(batch) > 0 {
						_ = c.repo.SaveSignals(context.Background(), batch)
					}
					return
				}
				batch = append(batch, sig)
				if len(batch) >= 20 {
					_ = c.repo.SaveSignals(ctx, batch)
					batch = nil
				}
			case <-ticker.C:
				if len(batch) > 0 {
					_ = c.repo.SaveSignals(ctx, batch)
					batch = nil
				}
			case <-ctx.Done():
				if len(batch) > 0 {
					_ = c.repo.SaveSignals(context.Background(), batch)
				}
				return
			}
		}
	}()

	// Producer goroutines: per provider ticker loop
	for _, p := range c.providers {
		wg.Add(1)
		go func(prov provider.SignalProvider) {
			defer wg.Done()

			// Run immediately on start
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
					log.Printf("[INFO] Ticker triggered for provider '%s'", prov.Name())
					sigs, err := prov.Fetch(ctx)
					if err != nil {
						log.Printf("[ERROR] Ticker fetch error for '%s': %v", prov.Name(), err)
						continue
					}
					for _, s := range sigs {
						signalChan <- s
					}
				case <-ctx.Done():
					log.Printf("[INFO] Stopping ticker loop for provider '%s'", prov.Name())
					return
				}
			}
		}(p)
	}

	<-ctx.Done()
	log.Printf("[INFO] Graceful shutdown initiated. Waiting for worker goroutines...")
	close(signalChan)
	wg.Wait()
	log.Printf("[INFO] Daemon collector shutdown complete.")
	return nil
}
