package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/config"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/pipeline"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/provider"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/storage"
)

func main() {
	cfg := config.LoadConfig()

	modeFlag := flag.String("mode", cfg.RunMode, "Execution mode: 'daemon' (continuous ticker), 'oneshot' (single pass & exit), or 'simulation'")
	simulateFlag := flag.Bool("simulate", cfg.EnableSimulator, "Enable test stream simulator module for testing/demo")
	flag.Parse()

	if *simulateFlag || *modeFlag == "simulation" {
		cfg.EnableSimulator = true
	}

	log.Printf("=================================================================")
	log.Printf("   Business Signal Abstraction Layer (BSAL) Engine Starting      ")
	log.Printf("   Mode: %s | Target Country: %s | DB: MongoDB                   ", *modeFlag, cfg.CountryCode)
	log.Printf("   Simulator Active: %v                                           ", cfg.EnableSimulator)
	log.Printf("=================================================================")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		log.Printf("[INFO] Received OS signal (%v). Initiating shutdown...", sig)
		cancel()
	}()

	// Initialize MongoDB repository
	repo, err := storage.NewMongoRepository(ctx, cfg)
	if err != nil {
		log.Fatalf("[FATAL] Database initialization failed: %v", err)
	}
	defer func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer closeCancel()
		_ = repo.Close(closeCtx)
	}()

	// Instantiate production providers
	providersList := []provider.SignalProvider{
		provider.NewGoogleAnalyticsProvider(cfg),
		provider.NewMetaBusinessProvider(cfg),
		provider.NewWeatherProvider(cfg),
		provider.NewCalendarProvider(cfg),
	}

	// Register dedicated Test Stream Simulator if enabled
	if cfg.EnableSimulator {
		log.Printf("[INFO] Registering dedicated Test Stream Simulator module...")
		providersList = append(providersList, provider.NewSimulatorProvider(cfg))
	}

	// Create pipeline collector
	collector := pipeline.NewCollector(repo, providersList...)

	if *modeFlag == "oneshot" {
		if err := collector.RunOneShot(ctx); err != nil {
			log.Fatalf("[FATAL] One-Shot collection failed: %v", err)
		}
		log.Println("[INFO] BSAL One-Shot extraction completed cleanly.")
	} else {
		if err := collector.RunDaemon(ctx); err != nil {
			log.Fatalf("[FATAL] Daemon collector error: %v", err)
		}
	}
}
