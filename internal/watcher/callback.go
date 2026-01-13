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

	// Special Case for AI flow (Spec 64)
	if strings.HasPrefix(data, "AI_") {
		return w.handleAICallback(data)
	}

	action := parts[0]  // CONFIRM or CANCEL
	trigger := parts[1] // SL, TP, TS
	ticker := parts[2]

	w.mu.Lock()
	pending, exists := w.pendingActions[ticker]
	if !exists {
		w.mu.Unlock()
		return fmt.Sprintf("⚠️ Action for %s expired or not found.", ticker)
	}

	// Always cleanup pending action at end (Point 6)
	delete(w.pendingActions, ticker)

	// 1.5 Find Position (Used for TP Guardrail & Execution)
	// Make a copy for validation outside lock
	var position models.Position
	activeFound := false

	for _, p := range w.state.Positions {
		if p.Ticker == ticker && p.Status == "ACTIVE" {
			position = p
			activeFound = true
			break
		}
	}
	w.mu.Unlock()

	if action == "CANCEL" {
		return fmt.Sprintf("❌ Action for %s cancelled by user.", ticker)
	}

	if action == "CONFIRM" {
		// 1. Temporal Gate
		ttl := time.Duration(w.config.ConfirmationTTLSec) * time.Second
		if time.Since(pending.Timestamp) > ttl {
			return fmt.Sprintf("⏳ TIMEOUT: Confirmation for %s is too old (> %ds). Action aborted.", ticker, w.config.ConfirmationTTLSec)
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

		// Spec 54: Sequential Order Clearance
		if err := w.ensureSequentialClearance(ticker); err != nil {
			log.Printf("Warning: Sequential clearance failed for %s: %v", ticker, err)
			// Proceed but warn? Or abort? Spec says "ONLY then is the bot permitted".
			// But if it times out, we might be stuck. Let's abort to be safe strict compliance.
			return fmt.Sprintf("❌ Execution Aborted: Could not clear pending orders for %s (Timeout).", ticker)
		}

		order, err := w.provider.PlaceOrder(ticker, qty, "sell")
		if err != nil {
			msg := fmt.Sprintf("❌ Execution Failed for %s: %v", ticker, err)
			log.Printf("[FATAL_TRADE_ERROR] %s", msg)
			return msg
		}

		// Spec 53: Execution Verification
		verifiedOrder, err := w.verifyOrderExecution(order.ID)
		if err != nil {
			// Spec 53 says: Send [CRITICAL] alert.
			// Re-sync is already triggered inside verifyOrderExecution if status was fail.
			msg := fmt.Sprintf("🚨 Critical: Order Verification Failed: %v", err)
			log.Printf("[FATAL_TRADE_ERROR] %s", msg)
			return msg
		}

		status := strings.ToLower(verifiedOrder.Status)
		// Double check status just in case
		if status == "canceled" || status == "rejected" || status == "expired" {
			return fmt.Sprintf("❌ Execution Failed: Order Status %s.", status)
		}

		// 5. Update State (Only if we are confident)
		if status == "filled" {
			w.mu.Lock()
			// Find position again by Ticker (index might have shifted if other things happened)
			foundIndex := -1
			for i, p := range w.state.Positions {
				if p.Ticker == ticker && p.Status == "ACTIVE" {
					foundIndex = i
					break
				}
			}

			if foundIndex != -1 {
				w.state.Positions[foundIndex].Status = "EXECUTED"
				w.saveStateLocked()
			}
			w.mu.Unlock()

			return fmt.Sprintf("✅ ORDER PLACED: Sold %s at Market (Filled).", ticker)
		}

		return fmt.Sprintf("⚠️ Order Placed but not yet Filled (Status: %s). Position remains ACTIVE.", status)
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
	proposal, exists := w.pendingProposals[ticker]
	if !exists {
		w.mu.Unlock()
		return fmt.Sprintf("⚠️ Proposal for %s expired or not found.", ticker)
	}
	delete(w.pendingProposals, ticker) // Cleanup
	w.mu.Unlock()

	// 1. Temporal Gate (Spec 39)
	ttl := time.Duration(w.config.ConfirmationTTLSec) * time.Second
	if time.Since(proposal.Timestamp) > ttl {
		return fmt.Sprintf("⏳ TIMEOUT: Proposal for %s expired (> %ds). Action aborted.", ticker, w.config.ConfirmationTTLSec)
	}

	if action == "CANCEL" {
		return fmt.Sprintf("❌ Purchase of %s cancelled.", ticker)
	}

	if action == "EXECUTE" {
		// Spec 54: Sequential Order Clearance (Safeguard)
		if err := w.ensureSequentialClearance(ticker); err != nil {
			return fmt.Sprintf("❌ Buy Aborted: Could not clear pending orders for %s.", ticker)
		}

		// 1. Execute Buy
		order, err := w.provider.PlaceOrder(ticker, proposal.Qty, "buy")
		if err != nil {
			msg := fmt.Sprintf("❌ Buy Execution Failed: %v", err)
			log.Printf("[FATAL_TRADE_ERROR] %s", msg)
			return msg
		}

		// Spec 53: Execution Verification
		verifiedOrder, err := w.verifyOrderExecution(order.ID)
		if err != nil {
			msg := fmt.Sprintf("🚨 Critical: Buy Verification Failed: %v", err)
			log.Printf("[FATAL_TRADE_ERROR] %s", msg)
			return msg
		}

		status := strings.ToLower(verifiedOrder.Status)
		if status == "canceled" || status == "rejected" {
			return fmt.Sprintf("❌ Buy Failed: Order Status '%s'.", status)
		}

		if status == "filled" {
			// 3. Add to State
			newPos := models.Position{
				Ticker:          ticker,
				Quantity:        proposal.Qty,
				EntryPrice:      proposal.Price, // Approx, ideally use verifiedOrder.FilledAvgPrice if available
				StopLoss:        proposal.StopLoss,
				TakeProfit:      proposal.TakeProfit,
				Status:          "ACTIVE",
				HighWaterMark:   proposal.Price,
				TrailingStopPct: proposal.TrailingStopPct,
				ThesisID:        fmt.Sprintf("MANUAL_%d", time.Now().Unix()),
			}

			// Refine EntryPrice if available
			if verifiedOrder.FilledAvgPrice != nil {
				newPos.EntryPrice = *verifiedOrder.FilledAvgPrice
				newPos.HighWaterMark = *verifiedOrder.FilledAvgPrice
			}

			w.mu.Lock()
			w.state.Positions = append(w.state.Positions, newPos)
			w.saveStateLocked()
			w.mu.Unlock()

			return fmt.Sprintf("✅ PURCHASED: %s %s @ Market (Filled).\nStatus: %s\nSL: $%s | TP: $%s\nTracking Active.",
				proposal.Qty.StringFixed(2), ticker, status, proposal.StopLoss.StringFixed(2), proposal.TakeProfit.StringFixed(2))
		}

		return fmt.Sprintf("⚠️ Buy Order Placed but not yet Filled (Status: %s). Position NOT yet tracked. Check /refresh later.", status)
	}

	return "Unknown buy action."
}

// handleAICallback processes AI_EXEC_ and AI_DISMISS_ buttons.
func (w *Watcher) handleAICallback(data string) string {
	// Format: AI_EXEC_AI_<Nano>_<Ticker> or AI_DISMISS_...
	// We need to extract the ActionID: AI_<Nano>_<Ticker>
	// Prefix is 8 chars "AI_EXEC_" or 11 chars "AI_DISMISS_"

	var actionID string
	var isExec bool

	if strings.HasPrefix(data, "AI_EXEC_") {
		actionID = strings.TrimPrefix(data, "AI_EXEC_")
		isExec = true
	} else if strings.HasPrefix(data, "AI_DISMISS_") {
		actionID = strings.TrimPrefix(data, "AI_DISMISS_")
		isExec = false
	} else {
		return "⚠️ Invalid AI callback format."
	}

	w.mu.Lock()
	pending, exists := w.pendingActions[actionID]
	if !exists {
		w.mu.Unlock()
		return "⚠️ AI Action expired or already processed."
	}
	delete(w.pendingActions, actionID) // Cleanup
	w.mu.Unlock()

	if !isExec {
		return fmt.Sprintf("❌ AI Proposal for %s dismissed.", pending.Ticker)
	}

	// EXECUTE
	// The pending.Action field holds the command string, e.g., "/update XBI 121.399 133.034 95"
	cmd := pending.Action
	log.Printf("Executing AI Command: %s", cmd)

	// Recursively execute the stored command
	// HandleCommand handles its own locking if needed.
	result := w.HandleCommand(cmd)

	return fmt.Sprintf("🤖⚡ **AI EXECUTION**\nCommand: `%s`\nResult: %s", cmd, result)
}
