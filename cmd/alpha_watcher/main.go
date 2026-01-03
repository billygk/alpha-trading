package main

import (
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	// Import our local internal packages
	"alpha_trading/internal/config"
	"alpha_trading/internal/market"
	"alpha_trading/internal/notifications"
	"alpha_trading/internal/storage"
	"alpha_trading/internal/watcher"
)

const LogFile = "watcher.log"

// main is the entry point of the application.
func main() {
	// 1. Initialization
	setupLogging()
	config.Load() // Load env vars

	// 2. Setup Signal Handling (Graceful Shutdown)
	// We create a channel to listen for OS signals (like Ctrl+C or kill).
	// 'chan' is a typed conduit for sending/receiving values.
	c := make(chan os.Signal, 1)

	// Notify causes package signal to relay incoming signals to c.
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	// Start a goroutine (lightweight thread) to handle the shutdown.
	// This runs in the background while main() continues.
	go func() {
		<-c // Block here until a value is received from the channel
		log.Println("⚠️ Watcher Shutting Down: System signal received.")

		// Perform cleanup: Save state one last time
		state, err := storage.LoadState()
		if err == nil {
			state.LastSync = time.Now().In(config.CetLoc).Format(time.RFC3339)
			storage.SaveState(state)
			log.Println("Final state saved successfully.")
		} else {
			log.Printf("Error loading state during shutdown: %v", err)
		}

		notifications.Notify("⚠️ Watcher Shutting Down: System signal received.")
		os.Exit(0) // Exit the program immediately
	}()

	// 3. Setup Dependencies
	// Initialize the market provider (Alpaca)
	provider := market.NewAlpacaProvider()

	// Initialize the Watcher service with the provider
	w := watcher.New(provider)

	log.Println("Alpha Watcher v1.9.0-Refactored Initialized")

	// 4. Main Loop
	// 'for {}' is an infinite loop in Go (like while(true)).
	for {
		w.Poll() // Do one check cycle

		// Calculate next run time for logging purposes
		nextTick := time.Now().In(config.CetLoc).Add(1 * time.Hour)
		log.Printf("Next check scheduled for: %s", nextTick.Format("2006-01-02 15:04:05 MST"))

		// Sleep pauses the current goroutine (main thread) for the duration.
		time.Sleep(1 * time.Hour)
	}
}

// --- LOGGING ---

// setupLogging configures logs to write to both the file and the console.
func setupLogging() {
	// Open (or create) the log file for appending.
	f, err := os.OpenFile(LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("Failed to open log file: %v", err)
		return
	}

	// MultiWriter allows writing to multiple destinations at once.
	mw := io.MultiWriter(os.Stdout, f)
	log.SetOutput(mw)

	// SetFlags(0) removes the default timestamp from the log package,
	// because we might want to control the timestamp format ourselves elsewhere,
	// or in this case, we rely on the system or just simple messages.
	// (Note: standard log.Printf usually adds timestamps if flags aren't 0)
	log.SetFlags(0)
}
