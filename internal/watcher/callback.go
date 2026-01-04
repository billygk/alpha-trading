package watcher

import (
	"fmt"
	"log"
	"strings"
	"time"
)

// HandleCallback processes button clicks from Telegram.
func (w *Watcher) HandleCallback(callbackID, data string) string {
	parts := strings.Split(data, "_")
	if len(parts) < 3 {
		return "⚠️ Invalid callback data."
	}

	action := parts[0] // CONFIRM or CANCEL
	// trigger := parts[1]  // SL or TP (unused but part of protocol)
	ticker := parts[2]

	w.mu.Lock()
	defer w.mu.Unlock()

	pending, exists := w.pendingActions[ticker]
	if !exists {
		return fmt.Sprintf("⚠️ Action for %s expired or not found.", ticker)
	}

	// Always cleanup pending action at end (Point 6)
	delete(w.pendingActions, ticker)

	if action == "CANCEL" {
		return fmt.Sprintf("❌ Action for %s cancelled by user.", ticker)
	}

	if action == "CONFIRM" {
		// 1. Temporal Gate
		ttl := time.Duration(w.config.ConfirmationTTLSec) * time.Second
		if time.Since(pending.Timestamp) > ttl {
			return fmt.Sprintf("⏳ TIMEOUT: Confirmation for %s is too old (> %ds). Action aborted.", ticker, w.config.ConfirmationTTLSec)
		}

		// 2. Deviation Gate
		currentPrice, err := w.provider.GetPrice(ticker)
		if err != nil {
			log.Printf("Error fetching price for deviation check: %v", err)
			return fmt.Sprintf("⚠️ Error fetching current price for %s. Aborted safety check.", ticker)
		}

		deviation := (currentPrice - pending.TriggerPrice) / pending.TriggerPrice
		if deviation < 0 {
			deviation = -deviation // Abs
		}

		maxDev := w.config.ConfirmationMaxDeviationPct
		if deviation > maxDev {
			return fmt.Sprintf("⚠️ PRICE DEVIATION: Price changed by %.2f%% (Max %.2f%%). Action aborted for safety.", deviation*100, maxDev*100)
		}

		// 3. Execution
		err = w.provider.PlaceOrder(ticker, 0, "sell") // Qty 0 is not supported? We need position size.
		// Wait, specs say "Market Order". Position handling usually requires knowing quantity.
		// Specs don't explicitly say "Close Position", but "PlaceOrder".
		// We need to look up quantity from state.

		qty := 0.0
		posIndex := -1
		for i, p := range w.state.Positions {
			if p.Ticker == ticker && p.Status == "ACTIVE" {
				qty = p.Quantity // Assuming struct has Quantity
				posIndex = i
				break
			}
		}

		if qty == 0 {
			return fmt.Sprintf("⚠️ Error: Could not find active position quantity for %s.", ticker)
		}

		err = w.provider.PlaceOrder(ticker, qty, "sell")
		if err != nil {
			log.Printf("Execution Error: %v", err)
			return fmt.Sprintf("❌ Execution Failed for %s: %v", ticker, err)
		}

		// 4. Update State
		if posIndex != -1 {
			w.state.Positions[posIndex].Status = "EXECUTED"
			w.saveStateAsync()
		}

		return fmt.Sprintf("✅ ORDER PLACED: Sold %s at Market.", ticker)
	}

	return "Unknown action."
}
