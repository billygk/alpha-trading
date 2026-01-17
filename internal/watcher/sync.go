package watcher

import (
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"alpha_trading/internal/models"
	"alpha_trading/internal/storage"
	"alpha_trading/internal/telegram"

	"github.com/shopspring/decimal"
)

// saveStateAsync saves without blocking, or just call storage?
// For simplicity and safety, we just call storage.SaveState since it's fast enough on low volume.
// saveState persists the current state to disk with updated metrics.
// It acquires the lock internally.
func (w *Watcher) saveState() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.saveStateLocked()
}

// saveStateLocked persists the current state to disk with updated metrics.
// It assumes w.mu is ALREADY LOCKED by the caller.
func (w *Watcher) saveStateLocked() {
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

// SyncWithBroker implements Spec 68: Just-In-Time Broker Reconciliation.
// It fetches fresh account data and positions from Alpaca, updates the local state,
// and strictly enforces the Dynamic Budget rules (Spec 69).
// Returns the updated portfolio state.
func (w *Watcher) SyncWithBroker() (models.PortfolioState, error) {
	// 1. Fetch Data in Parallel (could use goroutines, but sequential is safer/easier for now)
	positions, err := w.provider.ListPositions()
	if err != nil {
		return w.state, fmt.Errorf("JIT Sync: Failed to list positions: %v", err)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// 2. Reconcile Positions (Spec 42 & 29 Logic)
	// We reuse the logic from syncState but adapt it here or call a helper.
	// Since syncState logic is complex (HWM preservation), let's inline/refactor the core here.

	// Map existing HWMs
	existsMap := make(map[string]models.Position)
	for _, p := range w.state.Positions {
		if p.Status == "ACTIVE" {
			existsMap[p.Ticker] = p
		}
	}

	newPositions := []models.Position{}

	for _, p := range positions {
		ticker := p.Symbol
		qty := p.Qty
		avgEntry := p.AvgEntryPrice

		// Check BrokerPosition fields
		// In models.BrokerPosition, Qty and AvgEntryPrice are values.
		currentPrice := p.CurrentPrice // Already value in BrokerPosition

		// HWM Logic
		hwm := avgEntry
		if currentPrice.GreaterThan(hwm) {
			hwm = currentPrice
		}

		// Defaults
		sl := decimal.Zero
		tp := decimal.Zero
		tsPct := decimal.NewFromFloat(w.config.DefaultTrailingStopPct)
		thesisID := fmt.Sprintf("IMPORTED_%d", time.Now().Unix())
		var openedAt time.Time // Default zero

		// Check local state for overrides
		if oldP, ok := existsMap[ticker]; ok {
			sl = oldP.StopLoss
			tp = oldP.TakeProfit
			tsPct = oldP.TrailingStopPct
			thesisID = oldP.ThesisID

			// Spec 66: Stagnation Timer - Persist OpenedAt
			if !oldP.OpenedAt.IsZero() {
				openedAt = oldP.OpenedAt
			}

			// Spec 52: Monotonicity - Preserve HWM if local is higher
			if oldP.HighWaterMark.GreaterThan(hwm) {
				hwm = oldP.HighWaterMark
			}
		} else {
			// New Position Discovery
			openedAt = time.Now()
			log.Printf("ℹ️ Position discovered: %s", ticker)
		}

		// Ensure defaults if missing or zero (Spec 42)
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
			OpenedAt:        openedAt,
		}

		newPositions = append(newPositions, newPos)
	}

	w.state.Positions = newPositions

	// Spec 72: Watchlist Price Grounding (Env & State)
	// Refresh Logic: Fetch LatestTrade for all tickers in WATCHLIST_TICKERS and update the local state.
	// We do this AFTER reconciling positions, but before saving.
	// Spec 99.2: Price Updates Consolidated Block
	if len(w.config.WatchlistTickers) > 0 {
		if w.state.WatchlistPrices == nil {
			w.state.WatchlistPrices = make(map[string]float64)
		}

		var snapshotMsg strings.Builder
		hasSignificantChange := false

		for _, ticker := range w.config.WatchlistTickers {
			ticker = strings.ToUpper(strings.TrimSpace(ticker))
			if ticker == "" {
				continue
			}
			// Use GetPrice (returns decimal) -> float64
			priceDec, err := w.provider.GetPrice(ticker)
			if err != nil {
				log.Printf("Watchlist Warning: Could not fetch price for %s: %v", ticker, err)
				continue
			}
			newPrice, _ := priceDec.Float64()
			oldPrice := w.state.WatchlistPrices[ticker]

			// Calculate change
			if oldPrice > 0 {
				pctChange := (newPrice - oldPrice) / oldPrice * 100.0
				if math.Abs(pctChange) > 0.5 {
					hasSignificantChange = true
					icon := "▲"
					if pctChange < 0 {
						icon = "▼"
					}
					snapshotMsg.WriteString(fmt.Sprintf("- %s: $%s (%s%.1f%%)\n", ticker, priceDec.StringFixed(2), icon, math.Abs(pctChange)))
				}
			}

			w.state.WatchlistPrices[ticker] = newPrice
		}

		if hasSignificantChange {
			telegram.Notify(fmt.Sprintf("📊 MARKET SNAPSHOT:\n%s", snapshotMsg.String()))
		}
	}

	// LastSync updated in SaveState
	w.saveStateLocked()

	return w.state, nil
}

// syncState passes through to SyncWithBroker now to unify logic.
// Returns count, discovered (empty if sync works generally), error.
func (w *Watcher) syncState() (int, []string, error) {
	state, err := w.SyncWithBroker()
	if err != nil {
		return 0, nil, err
	}
	return len(state.Positions), []string{}, nil
}
