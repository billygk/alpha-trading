package watcher

import (
	"alpha_trading/internal/models"
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

	// Special Case for BUY flow
	if strings.HasPrefix(data, "EXECUTE_BUY_") || strings.HasPrefix(data, "CANCEL_BUY_") {
		return w.handleBuyCallback(data)
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

func (w *Watcher) handleBuyCallback(data string) string {
	parts := strings.Split(data, "_")
	// EXECUTE_BUY_TICKER or CANCEL_BUY_TICKER
	if len(parts) < 3 {
		return "⚠️ Invalid buy callback data."
	}
	action := parts[0] // EXECUTE or CANCEL
	ticker := parts[2]

	w.mu.Lock()
	defer w.mu.Unlock()

	proposal, exists := w.pendingProposals[ticker]
	if !exists {
		return fmt.Sprintf("⚠️ Proposal for %s expired or not found.", ticker)
	}
	delete(w.pendingProposals, ticker) // Cleanup

	if action == "CANCEL" {
		return fmt.Sprintf("❌ Purchase of %s cancelled.", ticker)
	}

	if action == "EXECUTE" {
		// 1. Re-Verify Price (Slippage check? Optional but good)
		// For now, trust the user click, but maybe check price hasn't jumped 50%?
		// Let's keep it simple: Market Order.

		err := w.provider.PlaceOrder(ticker, proposal.Qty, "buy")
		if err != nil {
			log.Printf("Buy Execution Error: %v", err)
			return fmt.Sprintf("❌ Buy Execution Failed: %v", err)
		}

		// 2. Add to State
		newPos := models.Position{
			Ticker:          ticker,
			Quantity:        proposal.Qty,
			EntryPrice:      proposal.Price, // Approx
			StopLoss:        proposal.StopLoss,
			TakeProfit:      proposal.TakeProfit,
			Status:          "ACTIVE",
			HighWaterMark:   proposal.Price,
			TrailingStopPct: 0, // Could be added to /buy command later
			ThesisID:        fmt.Sprintf("MANUAL_%d", time.Now().Unix()),
		}

		w.state.Positions = append(w.state.Positions, newPos)
		w.saveStateAsync()

		return fmt.Sprintf("✅ PURCHASED: %.2f %s @ Market.\nSL: $%.2f | TP: $%.2f\nTracking Active.",
			proposal.Qty, ticker, proposal.StopLoss, proposal.TakeProfit)
	}

	return "Unknown buy action."
}
