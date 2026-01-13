package watcher

import (
	"fmt"
	"log"
	"strings"
	"time"

	"alpha_trading/internal/ai"
	"alpha_trading/internal/config"
	"alpha_trading/internal/telegram"

	"github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
	"github.com/shopspring/decimal"
)

type PendingAction struct {
	Ticker       string
	Action       string // "SELL" (for now)
	TriggerPrice decimal.Decimal
	Timestamp    time.Time
}

type PendingProposal struct {
	Ticker          string
	Qty             decimal.Decimal
	Price           decimal.Decimal
	TotalCost       decimal.Decimal
	StopLoss        decimal.Decimal
	TakeProfit      decimal.Decimal
	TrailingStopPct decimal.Decimal
	Timestamp       time.Time
}

// checkRisk iterates positions and checks for triggers.
func (w *Watcher) checkRisk() {
	w.mu.Lock()
	// defer w.mu.Unlock() removed to prevent double-unlock with manual Unlock() below

	// --- QUEUED ORDER CHECK (Empty Portfolio) ---
	if len(w.state.Positions) == 0 {
		openOrders, err := w.provider.ListOrders("open")
		if err == nil && len(openOrders) > 0 {
			var sb strings.Builder
			sb.WriteString("⏳ *WAITING FOR MARKET OPEN*\n")
			for _, o := range openOrders {
				qtyStr := "0"
				if o.Qty != nil {
					qtyStr = o.Qty.String()
				}
				sb.WriteString(fmt.Sprintf("• %s %s shares of %s are queued.\n", o.Side, qtyStr, o.Symbol))
			}
			telegram.Notify(sb.String())
		}
	}

	// --- PENDING ACTION CLEANUP ---
	// Remove expired actions so we don't block new alerts forever if user ignores them.
	ttl := time.Duration(w.config.ConfirmationTTLSec) * time.Second
	for ticker, action := range w.pendingActions {
		if time.Since(action.Timestamp) > ttl {
			delete(w.pendingActions, ticker)
			// Optional: Log or notify?
			// log.Printf("Expired pending action for %s", ticker)
		}
	}

	// --- POSITION CHECK LOGIC ---
	for i, pos := range w.state.Positions {
		if pos.Status != "ACTIVE" {
			continue
		}

		price, err := w.provider.GetPrice(pos.Ticker)
		if err != nil {
			log.Printf("ERROR: Fetching price for %s: %v", pos.Ticker, err)
			continue
		}

		// Update High Water Mark if applicable
		// Spec 52: HWM Monotonicity: HWM = max(stored_HWM, current_price)
		if pos.HighWaterMark.IsZero() || price.GreaterThan(pos.HighWaterMark) {
			log.Printf("[%s] New High Water Mark: $%s (Old: $%s)", pos.Ticker, price.StringFixed(2), pos.HighWaterMark.StringFixed(2))
			w.state.Positions[i].HighWaterMark = price
			pos.HighWaterMark = price // Update local copy for calculations below
		}

		log.Printf("[%s] Current: $%s | SL: $%s | TP: $%s | HWM: $%s", pos.Ticker, price.StringFixed(2), pos.StopLoss.StringFixed(2), pos.TakeProfit.StringFixed(2), pos.HighWaterMark.StringFixed(2))

		// Check Trailing Stop
		triggeredTS := false
		if pos.TrailingStopPct.GreaterThan(decimal.Zero) && pos.HighWaterMark.GreaterThan(decimal.Zero) {
			// trailingTrigger = HWM * (1 - pct/100)
			multiplier := decimal.NewFromInt(100).Sub(pos.TrailingStopPct).Div(decimal.NewFromInt(100))
			trailingTriggerPrice := pos.HighWaterMark.Mul(multiplier)

			if price.LessThanOrEqual(trailingTriggerPrice) {
				triggeredTS = true
				log.Printf("[%s] Trailing Stop Triggered! Price $%s <= Trigger $%s", pos.Ticker, price.StringFixed(2), trailingTriggerPrice.StringFixed(2))
			}
		}

		triggeredSL := !pos.StopLoss.IsZero() && price.LessThanOrEqual(pos.StopLoss)
		triggeredTP := !pos.TakeProfit.IsZero() && price.GreaterThanOrEqual(pos.TakeProfit)

		// Check triggers (Stop Loss / Take Profit / Trailing Stop)
		if triggeredSL || triggeredTP || triggeredTS {
			// 1. Debounce (Pending Action)
			if _, exists := w.pendingActions[pos.Ticker]; exists {
				continue
			}

			// 2. Alert Fatigue (Spec 38)
			// Don't re-alert if we alerted recently (e.g., within 15 mins)
			// Since PollInterval is usually 60m, this effectively limits to once per poll.
			// But if Interval is small, this helps.
			if lastAlert, ok := w.lastAlerts[pos.Ticker]; ok {
				if time.Since(lastAlert) < 15*time.Minute {
					continue
				}
			}

			// 3. Precedence Logic (Spec 36)
			// TP > SL > TS (SL is hard stop, usually takes precedence over TS if both hit)
			actionType := "STOP LOSS"
			triggerType := "SL"

			if triggeredTP {
				actionType = "TAKE PROFIT"
				triggerType = "TP"
			} else if triggeredSL {
				actionType = "STOP LOSS"
				triggerType = "SL"
			} else if triggeredTS {
				actionType = "TRAILING STOP"
				triggerType = "TS"
			}

			// Create Pending Action
			w.pendingActions[pos.Ticker] = PendingAction{
				Ticker:       pos.Ticker,
				Action:       "SELL", // Always sell for TP/SL/TS
				TriggerPrice: price,
				Timestamp:    time.Now(),
			}

			// Update Last Alert
			w.lastAlerts[pos.Ticker] = time.Now()

			// Send Interactive Message
			msg := fmt.Sprintf("🚨 *POLL ALERT: %s*\nAsset: %s\nPrice: $%s\nAction: SELL REQUIRED\n\n⏱️ Valid for %d seconds.",
				actionType, pos.Ticker, price.StringFixed(2), w.config.ConfirmationTTLSec)

			buttons := []telegram.Button{
				{Text: "✅ CONFIRM", CallbackData: fmt.Sprintf("CONFIRM_%s_%s", triggerType, pos.Ticker)},
				{Text: "❌ CANCEL", CallbackData: fmt.Sprintf("CANCEL_%s_%s", triggerType, pos.Ticker)},
			}

			telegram.SendInteractiveMessage(msg, buttons)
		}
	}

	// Spec 32: Automated Operational Awareness

	w.state.LastSync = time.Now().In(config.CetLoc).Format(time.RFC3339)
	w.mu.Unlock() // Unlock before save to prevent deadlock if saveState acquires lock
	w.saveState()
}

// ensureSequentialClearance ensures all open orders for a ticker are canceled and cleared (Spec 54).
func (w *Watcher) ensureSequentialClearance(ticker string) error {
	// 1. Initial Check
	orders, err := w.provider.ListOrders("open")
	if err != nil {
		return fmt.Errorf("failed to list orders: %v", err)
	}

	hasOrders := false
	for _, o := range orders {
		if o.Symbol == ticker {
			hasOrders = true
			if err := w.provider.CancelOrder(o.ID); err != nil {
				log.Printf("Warning: Failed to cancel order %s: %v", o.ID, err)
			}
		}
	}

	if !hasOrders {
		return nil
	}

	// 2. Poll until cleared (Max 5 retries, 500ms apart)
	for i := 0; i < 5; i++ {
		time.Sleep(500 * time.Millisecond)
		orders, err = w.provider.ListOrders("open")
		if err != nil {
			continue
		}

		found := false
		for _, o := range orders {
			if o.Symbol == ticker {
				found = true
				break
			}
		}

		if !found {
			return nil // Cleared
		}
	}

	return fmt.Errorf("timeout waiting for orders to clear for %s", ticker)
}

// verifyOrderExecution polls for order status validation (Spec 53).
func (w *Watcher) verifyOrderExecution(orderID string) (*alpaca.Order, error) {
	// Query every 1 second for 5 seconds
	for i := 0; i < 5; i++ {
		time.Sleep(1 * time.Second)
		order, err := w.provider.GetOrder(orderID)
		if err != nil {
			log.Printf("Verification poll failed: %v", err)
			continue
		}

		status := strings.ToLower(order.Status)
		if status == "filled" {
			return order, nil
		}

		if status == "canceled" || status == "rejected" || status == "expired" {
			// Spec 56: Re-Sync Enforcement on Execution Failure
			log.Printf("🚨 Order %s failed with status %s. Triggering Re-Sync.", orderID, status)
			if count, _, syncErr := w.syncState(); syncErr != nil {
				log.Printf("CRITICAL: Re-Sync failed after order failure: %v", syncErr)
			} else {
				log.Printf("Re-Sync complete. active positions: %d", count)
			}
			return order, fmt.Errorf("order terminated with status: %s", status)
		}
	}

	// If we get here, it's still pending/accepted/new.
	// We return the last known state.
	return w.provider.GetOrder(orderID)
}

// handleAIResult processes the AI analysis (Spec 60, 61, 62).
func (w *Watcher) handleAIResult(analysis *ai.AIAnalysis, snapshot *ai.PortfolioSnapshot, isManual bool) {
	log.Printf("🤖 AI Analysis: Recommends %s (Confidence: %.2f)", analysis.Recommendation, analysis.ConfidenceScore)

	// Tier 3: Low Priority (Log only)
	if analysis.ConfidenceScore < 0.70 { // Spec 59 Guardrail
		log.Printf("AI Recommendation Ignored due to low confidence (%.2f < 0.70).", analysis.ConfidenceScore)
		if isManual {
			telegram.Notify(fmt.Sprintf("🤖 AI Analysis: Recommends %s (Confidence: %.2f)\n⚠️ Recommendation Ignored due to low confidence (%.2f < 0.70).", analysis.Recommendation, analysis.ConfidenceScore, analysis.ConfidenceScore))
		}
		return
	}

	ticker := ""
	parts := strings.Fields(analysis.ActionCommand)
	if len(parts) > 1 {
		ticker = strings.ToUpper(parts[1])
	}

	// Spec 62: Telemetry
	// "Tier 1: Trade Proposals ... Notification: ON"
	// "Tier 2: Confidence > 0.7 but HOLD ... Notification: SILENT"

	if analysis.Recommendation == "HOLD" {
		// Tier 2: Silent Notification (or just log per user preference, Spec says Silent Notification? Telemetry?
		// "Notification: SILENT" usually means send without sound or just log if no silent mode implemented.
		// "Tier 3: Low Priority ... (LOG ONLY)".
		// Let's just log HOLDs with high confidence for now to avoid spam, unless user wants debug.
		log.Printf("AI STRATEGY: HOLD %s. Critique: %s", ticker, analysis.Analysis)
		if isManual {
			telegram.Notify(fmt.Sprintf("🤖 AI Analysis: Recommends HOLD (Confidence: %.2f)\nCritique: %s", analysis.ConfidenceScore, analysis.Analysis))
		}
		return
	}

	// Tier 1: Actionable
	msg := fmt.Sprintf("🤖 *AI STRATEGY REPORT: %s*\n"+
		"Conviction: %.2f | Risk: %s\n"+
		"Critique: %s\n"+
		"Recommendation: %s\n"+
		"Command: `%s`",
		ticker, analysis.ConfidenceScore, analysis.RiskAssessment, analysis.Analysis, analysis.Recommendation, analysis.ActionCommand)

	// Route based on Recommendation
	switch analysis.Recommendation {
	case "BUY", "SELL":
		// Spec 60: Semi-Autonomous Gate
		// Proposals require button click.

		// We format a proposal message with EXECUTE button.
		// Use the command string as data?
		// We can use a special callback "AI_EXECUTE_<BASE64_CMD>" or just parse it here and create a specific proposal.
		// For simplicity, we just send the message with a "COPY COMMAND" suggestion or a button if we can parse it easily.
		// Spec 60 says "Proposals ... require a [ ✅ EXECUTE ] button".
		// We need to parse the command to know what to execute.

		// If command is valid, show button.
		// We use a generic AI_EXECUTE_ callback and store the command in memory?
		// Or we trust the command string if signed? No signing.
		// Let's store in PendingActions or similar?
		// PendingProposals is for /buy.
		// Let's just output the message and ask user to copy-paste or click a button that runs it?
		// "buttons expire after 300s".

		// Implementation: Store the command payload mapped to a unique ID.
		actionID := fmt.Sprintf("AI_%d_%s", time.Now().UnixNano(), ticker)

		w.mu.Lock()
		w.pendingActions[actionID] = PendingAction{
			Ticker:    ticker,
			Action:    analysis.ActionCommand, // Hijacking Action field to store command
			Timestamp: time.Now(),
		}
		w.mu.Unlock()

		buttons := []telegram.Button{
			{Text: "✅ EXECUTE AI", CallbackData: fmt.Sprintf("AI_EXEC_%s", actionID)},
			{Text: "❌ DISMISS", CallbackData: fmt.Sprintf("AI_DISMISS_%s", actionID)},
		}
		telegram.SendInteractiveMessage(msg, buttons)

	case "UPDATE":
		// Spec 61: Protected Autonomous Ratchet
		// Logic: Auto-execute if:
		// 1. new_sl > current_sl (Monotonic)
		// 2. new_sl < market_price * 0.985 (Buffer > 1.5%) - Spec says "at least 1.5% below current"
		// 3. Max 1 update per 4h per ticker.

		// Parse params from "/update <ticker> <sl> <tp>"
		// parts[0]=/update, parts[1]=ticker, parts[2]=sl, parts[3]=tp
		if len(parts) >= 4 {
			newSL, _ := decimal.NewFromString(parts[2])
			// newTP, _ := decimal.NewFromString(parts[3])

			// Check Constraints
			safe := false
			reason := ""

			// Fetch current state
			var currentSL decimal.Decimal
			for _, p := range w.state.Positions {
				if p.Ticker == ticker {
					currentSL = p.StopLoss
					break
				}
			}

			currentPrice, _ := w.provider.GetPrice(ticker)

			// 1. Monotonicity
			if newSL.GreaterThan(currentSL) {
				// 2. Buffer
				bufferPrice := currentPrice.Mul(decimal.NewFromFloat(0.985))
				if newSL.LessThan(bufferPrice) {
					// 3. Frequency
					lastUpd, ok := w.lastAlerts[ticker+"_UPDATE"]
					if !ok || time.Since(lastUpd) > 4*time.Hour {
						safe = true
					} else {
						reason = "Frequency Limit (4h)"
					}
				} else {
					reason = "Buffer Violation (<1.5% gap)"
				}
			} else {
				reason = "Not Monotonic (New SL <= Old SL)"
			}

			// Override: Force Manual Confirmation (User Request)
			if safe {
				safe = false
				reason = "Manual Confirmation Enforced"
			}

			if safe {
				// Unreachable
			} else {
				// Downgrade to Manual
				msg += fmt.Sprintf("\n\n⚠️ Auto-Update Blocked: %s. Manual Confirmation Required.", reason)
				actionID := fmt.Sprintf("AI_%d_%s", time.Now().UnixNano(), ticker)

				w.mu.Lock()
				w.pendingActions[actionID] = PendingAction{
					Ticker:    ticker,
					Action:    analysis.ActionCommand,
					Timestamp: time.Now(),
				}
				w.mu.Unlock()
				buttons := []telegram.Button{
					{Text: "✅ EXECUTE", CallbackData: fmt.Sprintf("AI_EXEC_%s", actionID)},
					{Text: "❌ DISMISS", CallbackData: fmt.Sprintf("AI_DISMISS_%s", actionID)},
				}
				telegram.SendInteractiveMessage(msg, buttons)
			}
		}
	}
}
