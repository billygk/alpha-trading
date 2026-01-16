package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"alpha_trading/internal/config"
	"alpha_trading/internal/logger"
	"alpha_trading/internal/market"
	"alpha_trading/internal/telegram" // Replaces internal/notifications
	"alpha_trading/internal/watcher"
)

const LogFile = "watcher.log"
const VersionFile = "version.latest"

// main is the entry point of the application.
func main() {
	// 1. Initialization
	// Load configuration first to get logger settings
	cfg := config.Load()
	cfg.Version = readVersion()

	// Setup logging with configured values
	logger.Setup(LogFile, cfg.MaxLogSizeMB, cfg.MaxLogBackups)

	// Create a context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Ensure cancel is called eventually

	// 2. Setup Dependencies
	// 4. Initialize Dependency Injection
	// Market Provider (Alpaca)
	marketProvider := market.NewAlpacaProvider()

	// Watcher (The core logic)
	w := watcher.New(cfg, marketProvider)

	// 3. Start Telegram Command Listener (Background)
	// We pass the watcher to the listener so it can query state/uptime
	// Note: We need to expose a method or interface for the Listener to query the Watcher.
	// For now, the listener implementation in internal/telegram/listener.go likely takes specific args.
	// Let's check how we started it before.
	// Previously: go telegram.StartListener(ctx, w.HandleCommand)
	// That remains valid since w.HandleCommand signature hasn't changed.
	go telegram.StartListener(w.HandleCommand, w.HandleCallback)

	// 4. Setup Signal Handling (Graceful Shutdown)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		log.Println("⚠️ Watcher Shutting Down: System signal received.")
		cancel() // Cancel context to stop main loop
	}()

	log.Printf("Alpha Watcher %s Initialized", cfg.Version)
	log.Printf("Polling Interval: %d mins (Fallback)", cfg.PollIntervalMins)

	// 5. Main Loop
	// Listen for context cancellation or ticker
	w.Poll() // Run once immediately on start

	ticker := time.NewTicker(time.Duration(cfg.PollIntervalMins) * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("🛑 Main loop stopping...")
			return
		case <-ticker.C:
			// Calculate next run time for logging purposes
			nextTick := time.Now().In(config.CetLoc).Add(time.Duration(cfg.PollIntervalMins) * time.Minute)
			log.Printf("Next check scheduled for: %s", nextTick.Format("2006-01-02 15:04:05 MST"))
			w.Poll()
		}
	}
}

func readVersion() string {
	// read version from VersionFile file
	version, err := os.ReadFile(VersionFile)
	if err != nil {
		return "v0.0.0-dev"
	}
	return string(version)
}
