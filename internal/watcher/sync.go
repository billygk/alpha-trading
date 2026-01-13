package watcher

import (
	"fmt"
	"log"
	"strings"
	"time"

	"alpha_trading/internal/models"
	"alpha_trading/internal/storage"

	"github.com/shopspring/decimal"
)

// saveStateAsync saves without blocking, or just call storage?
// For simplicity and safety, we just call storage.SaveState since it's fast enough on low volume.
// saveState persists the current state to disk with updated metrics.
func (w *Watcher) saveState() {
	// Spec 65: Update Budget Metrics before save
	w.mu.RLock()
	// Calculate Exposure
	currentExposure := decimal.Zero
	for _, p := range w.state.Positions {
		if p.Status == "ACTIVE" {
			cost := p.Quantity.Mul(p.EntryPrice)
			currentExposure = currentExposure.Add(cost)
		}
	}
	w.mu.RUnlock()

	w.mu.Lock()
	w.state.CurrentExposure = currentExposure
	w.state.FiscalLimit = decimal.NewFromFloat(w.config.FiscalBudgetLimit)
	w.state.AvailableBudget = w.state.FiscalLimit.Sub(currentExposure)
	w.mu.Unlock()

	storage.SaveState(w.state)
}

func (w *Watcher) searchAssets(query string) string {
	assets, err := w.provider.SearchAssets(query)
	if err != nil {
		log.Printf("Error searching assets: %v", err)
		return "⚠️ Error: Could not search assets."
	}

	if len(assets) == 0 {
		return fmt.Sprintf("🔍 No results found for '%s'.", query)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔍 *Results for '%s'*\n", query))
	for _, asset := range assets {
		sb.WriteString(fmt.Sprintf("- *%s*: %s\n", asset.Symbol, asset.Name))
	}
	return sb.String()
}

func (w *Watcher) getPrice(ticker string) string {
	price, err := w.provider.GetPrice(ticker)

	if err != nil || price.IsZero() {
		log.Printf("Price lookup failed for %s (err: %v, price: %v). Falling back to search.", ticker, err, price)
		searchResult := w.searchAssets(ticker)
		return fmt.Sprintf("⚠️ Price not found for '%s'. Did you mean:\n\n%s", ticker, searchResult)
	}
	return fmt.Sprintf("💲 *%s*: $%s", ticker, price.StringFixed(2))
}

// syncState performs the core logic of synchronization (Spec 29 & 40).
// Returns count of active positions, list of discovered tickers, and error.
func (w *Watcher) syncState() (int, []string, error) {
	// Provider usage outside lock to avoid blocking
	positions, err := w.provider.ListPositions()
	if err != nil {
		return 0, nil, err
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// Rebuild State
	newPositions := []models.Position{}

	// Create a map of existing HighWaterMarks to preserve them
	existsMap := make(map[string]bool)

	for _, p := range w.state.Positions {
		if p.Status == "ACTIVE" {
			existsMap[p.Ticker] = true
		}
	}

	var discoveredTickers []string

	for _, p := range positions {
		ticker := p.Symbol
		qty := p.Qty
		avgEntry := p.AvgEntryPrice

		var currentPrice decimal.Decimal
		if p.CurrentPrice != nil {
			currentPrice = *p.CurrentPrice
		}

		// Spec 40: HWM = max(AvgEntry, Current)
		hwm := avgEntry
		if currentPrice.GreaterThan(hwm) {
			hwm = currentPrice
		}

		// Defaults
		sl := decimal.Zero
		tp := decimal.Zero
		tsPct := decimal.NewFromFloat(w.config.DefaultTrailingStopPct)
		thesisID := fmt.Sprintf("IMPORTED_%d", time.Now().Unix())

		// If exists in local state, preserve SL/TP/TS
		if existsMap[ticker] {
			for _, oldP := range w.state.Positions {
				if oldP.Ticker == ticker && oldP.Status == "ACTIVE" {
					sl = oldP.StopLoss
					tp = oldP.TakeProfit
					tsPct = oldP.TrailingStopPct
					thesisID = oldP.ThesisID

					// Backfill Defaults if Zero
					if sl.IsZero() {
						slMult := decimal.NewFromInt(1).Sub(decimal.NewFromFloat(w.config.DefaultStopLossPct).Div(decimal.NewFromInt(100)))
						sl = avgEntry.Mul(slMult)
					}
					if tp.IsZero() {
						tpMult := decimal.NewFromInt(1).Add(decimal.NewFromFloat(w.config.DefaultTakeProfitPct).Div(decimal.NewFromInt(100)))
						tp = avgEntry.Mul(tpMult)
					}
					if tsPct.IsZero() {
						tsPct = decimal.NewFromFloat(w.config.DefaultTrailingStopPct)
					}
					break
				}
			}
		} else {
			// Spec 42: Apply Defaults to Discovered Positions
			slMult := decimal.NewFromInt(1).Sub(decimal.NewFromFloat(w.config.DefaultStopLossPct).Div(decimal.NewFromInt(100)))
			sl = avgEntry.Mul(slMult)

			tpMult := decimal.NewFromInt(1).Add(decimal.NewFromFloat(w.config.DefaultTakeProfitPct).Div(decimal.NewFromInt(100)))
			tp = avgEntry.Mul(tpMult)

			discoveredTickers = append(discoveredTickers, ticker)
			log.Printf("ℹ️ Position discovered: %s. Applied Default SL ($%s) & TP ($%s).", ticker, sl.StringFixed(2), tp.StringFixed(2))
		}

		// Spec 57: State Purity Enforcement (Implicit)
		// By rebuilding newPositions purely from the Alpaca list, any local position
		// that is NOT in the Alpaca response (e.g., closed/sold externally) is automatically dropped.
		// This satisfies the "Reconciliation Safeguard" requirement.

		newPos := models.Position{
			Ticker:          ticker,
			Quantity:        qty,
			EntryPrice:      avgEntry,
			StopLoss:        sl,
			TakeProfit:      tp,
			Status:          "ACTIVE",
			HighWaterMark:   hwm,
			TrailingStopPct: tsPct,
			ThesisID:        thesisID,
		}
		newPositions = append(newPositions, newPos)
	}

	w.state.Positions = newPositions
	w.saveState()

	return len(newPositions), discoveredTickers, nil
}
