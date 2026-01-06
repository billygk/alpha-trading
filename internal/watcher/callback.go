package watcher

import (
	"alpha_trading/internal/models"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/shopspring/decimal"
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

	action := parts[0]  // CONFIRM or CANCEL
	trigger := parts[1] // SL, TP, TS
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

		// 1.5 Find Position (Used for TP Guardrail & Execution)
		posIndex := -1
		var position models.Position
		activeFound := false

		for i, p := range w.state.Positions {
			if p.Ticker == ticker && p.Status == "ACTIVE" {
				position = p
				posIndex = i
				activeFound = true
				break
			}
		}

		if !activeFound {
			msg := fmt.Sprintf("❌ Execution Failed: Could not find active position for %s.", ticker)
			log.Printf("[FATAL_TRADE_ERROR] %s", msg)
			return msg
		}

		// 2. Refresh Price
		currentPrice, err := w.provider.GetPrice(ticker)
		if err != nil {
			log.Printf("Error fetching price for checks: %v", err)
			return fmt.Sprintf("⚠️ Error fetching current price for %s. Aborted.", ticker)
		}

		// 3. TP Price Protection Guardrail (Spec 36)
		if trigger == "TP" {
			// Gate: FreshPrice < (Position.TP * 0.995)
			// Guardrail: 0.5% slippage below Target
			thresholdRatio := decimal.NewFromFloat(0.995)
			thresholdPrice := position.TakeProfit.Mul(thresholdRatio)

			if currentPrice.LessThan(thresholdPrice) {
				return fmt.Sprintf("⚠️ TP GUARDRAIL: Price $%s has slipped below 99.5%% of TP ($%s). Manual review required.", currentPrice.StringFixed(2), position.TakeProfit.StringFixed(2))
			}
		}

		// 4. Standard Deviation Gate (Spec 18)
		// deviation = abs(current - trigger_from_pending) / trigger_from_pending
		deviation := currentPrice.Sub(pending.TriggerPrice).Div(pending.TriggerPrice)
		if deviation.IsNegative() {
			deviation = deviation.Neg() // Abs
		}

		maxDev := decimal.NewFromFloat(w.config.ConfirmationMaxDeviationPct)
		if deviation.GreaterThan(maxDev) {
			displayDev := deviation.Mul(decimal.NewFromInt(100)).StringFixed(2)
			displayMax := maxDev.Mul(decimal.NewFromInt(100)).StringFixed(2)
			return fmt.Sprintf("⚠️ PRICE DEVIATION: Price changed by %s%% (Max %s%%). Action aborted for safety.", displayDev, displayMax)
		}

		// 5. Execution (Sell)
		qty := position.Quantity
		if qty.IsZero() {
			msg := fmt.Sprintf("❌ Execution Failed: Quantity is zero for %s.", ticker)
			return msg
		}

		order, err := w.provider.PlaceOrder(ticker, qty, "sell")
		if err != nil {
			msg := fmt.Sprintf("❌ Execution Failed for %s: %v", ticker, err)
			log.Printf("[FATAL_TRADE_ERROR] %s", msg)
			return msg
		}

		// 4. Verification Check
		time.Sleep(2 * time.Second)
		verifiedOrder, err := w.provider.GetOrder(order.ID)
		if err != nil {
			msg := fmt.Sprintf("⚠️ Order Placed but Verification Failed: %v. Please check Alpaca dashboard.", err)
			log.Printf("[FATAL_TRADE_ERROR] Verification failed for order %s: %v", order.ID, err)
			// We optimize for safety: do not mark EXECUTED if we can't verify?
			// OR mark executed but warn. Let's Warn.
			return msg
		}

		status := strings.ToLower(verifiedOrder.Status)
		// Valid statuses for a "working" or "filled" order
		validStatuses := []string{"filled", "new", "accepted", "partially_filled", "calculated", "pending_new"}
		isValid := false
		for _, s := range validStatuses {
			if status == s {
				isValid = true
				break
			}
		}

		if !isValid {
			msg := fmt.Sprintf("❌ Execution Failed: Order Status is '%s' (Reason: %s). position remains ACTIVE.", status, verifiedOrder.FailedAt)
			log.Printf("[FATAL_TRADE_ERROR] Order %s failed with status %s", order.ID, status)
			return msg
		}

		// 5. Update State
		if posIndex != -1 {
			w.state.Positions[posIndex].Status = "EXECUTED"
			w.saveStateAsync()
		}

		return fmt.Sprintf("✅ ORDER PLACED: Sold %s at Market (Status: %s).", ticker, status)
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
		// 1. Execute Buy
		order, err := w.provider.PlaceOrder(ticker, proposal.Qty, "buy")
		if err != nil {
			msg := fmt.Sprintf("❌ Buy Execution Failed: %v", err)
			log.Printf("[FATAL_TRADE_ERROR] %s", msg)
			return msg
		}

		// 2. Verification
		time.Sleep(2 * time.Second)
		verifiedOrder, err := w.provider.GetOrder(order.ID)
		if err != nil {
			msg := fmt.Sprintf("⚠️ Buy Placed but Verification Failed: %v. Check Dashboard.", err)
			log.Printf("[FATAL_TRADE_ERROR] Buy verification failed for %s: %v", order.ID, err)
			return msg
		}

		status := strings.ToLower(verifiedOrder.Status)
		validStatuses := []string{"filled", "new", "accepted", "partially_filled", "calculated", "pending_new"}
		isValid := false
		for _, s := range validStatuses {
			if status == s {
				isValid = true
				break
			}
		}

		if !isValid {
			msg := fmt.Sprintf("❌ Buy Failed: Order Status '%s'.", status)
			log.Printf("[FATAL_TRADE_ERROR] Buy Order %s failed: status %s", order.ID, status)
			return msg
		}

		// 3. Add to State
		newPos := models.Position{
			Ticker:          ticker,
			Quantity:        proposal.Qty,
			EntryPrice:      proposal.Price, // Approx
			StopLoss:        proposal.StopLoss,
			TakeProfit:      proposal.TakeProfit,
			Status:          "ACTIVE",
			HighWaterMark:   proposal.Price,
			TrailingStopPct: proposal.TrailingStopPct,
			ThesisID:        fmt.Sprintf("MANUAL_%d", time.Now().Unix()),
		}

		w.state.Positions = append(w.state.Positions, newPos)
		w.saveStateAsync()

		return fmt.Sprintf("✅ PURCHASED: %s %s @ Market.\nStatus: %s\nSL: $%s | TP: $%s\nTracking Active.",
			proposal.Qty.StringFixed(2), ticker, status, proposal.StopLoss.StringFixed(2), proposal.TakeProfit.StringFixed(2))
	}

	return "Unknown buy action."
}
