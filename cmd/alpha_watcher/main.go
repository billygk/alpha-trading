package main

import (
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"alpha_trading/internal/config"
	"alpha_trading/internal/market"
	"alpha_trading/internal/storage"
	"alpha_trading/internal/telegram" // Replaces internal/notifications
	"alpha_trading/internal/watcher"
)

const LogFile = "watcher.log"

// main is the entry point of the application.
func main() {
	// 1. Initialization
	setupLogging()
	config.Load() // Load env vars

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
		nextTick := time.Now().In(config.CetLoc).Add(1 * time.Hour)
		log.Printf("Next check scheduled for: %s", nextTick.Format("2006-01-02 15:04:05 MST"))

		// Sleep pauses the main thread.
		// The Telegram listener continues to run in its own goroutine.
		time.Sleep(1 * time.Hour)
	}
}

// --- LOGGING ---

// setupLogging configures logs to write to both the file and the console.
func setupLogging() {
	f, err := os.OpenFile(LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("Failed to open log file: %v", err)
		return
	}

	mw := io.MultiWriter(os.Stdout, f)
	log.SetOutput(mw)
	log.SetFlags(0)
}
