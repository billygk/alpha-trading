package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"alpha_trading/internal/config"
	"alpha_trading/internal/logger"
	"alpha_trading/internal/market"
	"alpha_trading/internal/storage"
	"alpha_trading/internal/telegram" // Replaces internal/notifications
	"alpha_trading/internal/watcher"
)

const LogFile = "watcher.log"

// main is the entry point of the application.
func main() {
	// 1. Initialization
	// Load configuration first to get logger settings
	cfg := config.Load()

	// Setup logging with configured values
	logger.Setup(LogFile, cfg.MaxLogSizeMB, cfg.MaxLogBackups)

	// 2. Setup Dependencies
	// Initialize the market provider (Alpaca)
	provider := market.NewAlpacaProvider()

	// Initialize the Watcher service with the provider
	// This now loads the initial state into memory
	w := watcher.New(provider)

	// 3. Start Telegram Command Listener (Background)
	// We run this in a goroutine so it doesn't block the main loop
	go telegram.StartListener(w.HandleCommand)

	// 4. Setup Signal Handling (Graceful Shutdown)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		log.Println("⚠️ Watcher Shutting Down: System signal received.")

		// Perform cleanup: Save state one last time
		// Note: Since Watcher saves on every Poll, and we don't have pending in-memory changes
		// that aren't on disk (unless we interrupted exactly during a write),
		// loading from disk is generally safe. For a more robust solution,
		// we might ask Watcher to flush its state.
		state, err := storage.LoadState()
		if err == nil {
			state.LastSync = time.Now().In(config.CetLoc).Format(time.RFC3339)
			storage.SaveState(state)
			log.Println("Final state saved successfully.")
		} else {
			log.Printf("Error loading state during shutdown: %v", err)
		}

		telegram.Notify("⚠️ Watcher Shutting Down: System signal received.")
		os.Exit(0)
	}()

	log.Println("Alpha Watcher v1.9.0-Refactored Initialized")

	// 5. Main Loop
	// 'for {}' is an infinite loop in Go (like while(true)).
	for {
		w.Poll() // Do one check cycle

		// Calculate next run time for logging purposes
		// Use Configured Interval
		interval := time.Duration(cfg.PollIntervalMins) * time.Minute
		nextTick := time.Now().In(config.CetLoc).Add(interval)
		log.Printf("Next check scheduled for: %s", nextTick.Format("2006-01-02 15:04:05 MST"))

		// Sleep using configured interval
		time.Sleep(interval)
	}
}

// --- LOGGING ---

// --- LOGGING ---

// setupLogging is replaced by logger.Setup from internal/logger package
// The logger package now handles file rotation (5MB limit, 3 backups)
