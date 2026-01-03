package watcher

import (
	"fmt"
	"log"
	"time"

	"alpha_trading/internal/config"
	"alpha_trading/internal/market"
	"alpha_trading/internal/notifications"
	"alpha_trading/internal/storage"
)

// Package-level variable to track when the watcher started running.
var startTime = time.Now()

// Watcher struct holds dependencies.
// By holding an interface (market.MarketProvider), it's decoupled from the specific implementation.
type Watcher struct {
	provider market.MarketProvider
}

// New creates a new Watcher instance.
func New(provider market.MarketProvider) *Watcher {
	return &Watcher{
		provider: provider,
	}
}

// Poll is the main logic of the application.
// It checks prices, manages state, and triggers alerts.
func (w *Watcher) Poll() {
	// 1. Load the current state of our portfolio.
	state, err := storage.LoadState()
	if err != nil {
		log.Printf("CRITICAL: Could not load state: %v", err)
		return
	}

	// --- HEARTBEAT LOGIC ---
	// Determine if we should send a heartbeat (alive) message.
	sendHB := false
	if state.LastHeartbeat == "" {
		sendHB = true // First time ever
	} else {
		// Parse the stored time string back into a Time object
		lastHBTime, parseErr := time.Parse(time.RFC3339, state.LastHeartbeat)
		// Check if it's been more than 24 hours
		if parseErr != nil || time.Since(lastHBTime) >= 24*time.Hour {
			sendHB = true
		}
	}

	if sendHB {
		// Count how many positions are currently active
		activeCount := 0
		for _, pos := range state.Positions {
			if pos.Status == "ACTIVE" {
				activeCount++
			}
		}

		// Fetch current account equity
		equity, eqErr := w.provider.GetEquity()
		equityStr := fmt.Sprintf("$%.2f", equity)
		if eqErr != nil {
			equityStr = "Error fetching"
			log.Printf("Error fetching equity: %v", eqErr)
		}

		// Round uptime to nearest second for cleaner display
		uptimeDuration := time.Since(startTime).Round(time.Second)

		// Construct the message
		hbMsg := fmt.Sprintf("💓 *HEARTBEAT*\n"+
			"Uptime: %s\n"+
			"Active Positions: %d\n"+
			"Equity: %s\n"+
			"System: Nominal",
			uptimeDuration.String(), activeCount, equityStr)

		// Send notification
		notifications.Notify(hbMsg)

		// Update the heartbeat timestamp in our state
		state.LastHeartbeat = time.Now().In(config.CetLoc).Format(time.RFC3339)
	}

	// --- POSITION CHECK LOGIC ---
	// Iterate through all positions to check their status
	for i, pos := range state.Positions {
		if pos.Status != "ACTIVE" {
			continue // Skip inactive positions
		}

		// Get current market price
		price, err := w.provider.GetPrice(pos.Ticker)
		if err != nil {
			log.Printf("ERROR: Fetching price for %s: %v", pos.Ticker, err)
			continue
		}

		log.Printf("[%s] Current: $%.2f | SL: $%.2f | TP: $%.2f", pos.Ticker, price, pos.StopLoss, pos.TakeProfit)

		// Check logic: Stop Loss vs Take Profit
		if price <= pos.StopLoss {
			notifications.Notify(fmt.Sprintf("🛑 *STOP LOSS HIT*\nAsset: %s\nPrice: $%.2f\nAction: SELL REQUIRED", pos.Ticker, price))
			// Direct assignment to slice element updates the state in memory
			state.Positions[i].Status = "TRIGGERED_SL"
		} else if price >= pos.TakeProfit {
			notifications.Notify(fmt.Sprintf("💰 *TARGET REACHED*\nAsset: %s\nPrice: $%.2f\nAction: TAKE PROFIT", pos.Ticker, price))
			state.Positions[i].Status = "TRIGGERED_TP"
		}
	}

	// Update sync timestamp
	state.LastSync = time.Now().In(config.CetLoc).Format(time.RFC3339)

	// Persist the updated state to disk
	storage.SaveState(state)
}
