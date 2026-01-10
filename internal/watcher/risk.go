package watcher

import (
	"fmt"
	"log"
	"strings"
	"time"

	"alpha_trading/internal/config"
	"alpha_trading/internal/storage"
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
	defer w.mu.Unlock()

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
	storage.SaveState(w.state)
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
