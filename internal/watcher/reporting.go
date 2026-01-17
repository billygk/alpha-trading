package watcher

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"alpha_trading/internal/models"
	"alpha_trading/internal/telegram"

	"github.com/shopspring/decimal"
)

func (w *Watcher) getMarketStatus() string {
	clock, err := w.provider.GetClock()
	if err != nil {
		log.Printf("Error fetching market clock: %v", err)
		return "⚠️ Error: Could not fetch market status."
	}

	status := "CLOSED 🔴"
	if clock.IsOpen {
		status = "OPEN 🟢"
	}

	nextSession := "Next Open"
	eventTime := clock.NextOpen
	if clock.IsOpen {
		nextSession = "Closes"
		eventTime = clock.NextClose
	}

	// Format time until next event
	until := time.Until(eventTime.Round(time.Minute)).Round(time.Minute)

	return fmt.Sprintf("🏛️ *MARKET STATUS*\nState: %s\n%s: %s (in %s)",
		status, nextSession, eventTime.Format("15:04 MST"), until)
}

func (w *Watcher) getStatus() string {
	w.mu.RLock()
	// Copy active positions to release lock during network calls
	var activePositions []models.Position
	for _, p := range w.state.Positions {
		if p.Status == "ACTIVE" {
			activePositions = append(activePositions, p)
		}
	}
	w.mu.RUnlock()

	// Parallel Fetching
	var wg sync.WaitGroup
	var mu sync.Mutex // For results map

	type detailedPos struct {
		Ticker    string
		Qty       decimal.Decimal
		Current   decimal.Decimal
		PrevClose decimal.Decimal
		Entry     decimal.Decimal
		SL        decimal.Decimal
		HWM       decimal.Decimal
	}
	posDetails := make(map[string]detailedPos)

	var clock *models.Clock
	var equity decimal.Decimal
	var errClock, errEquity error

	// 1. Fetch System Level Data
	wg.Add(2)
	go func() {
		defer wg.Done()
		clock, errClock = w.provider.GetClock()
	}()
	go func() {
		defer wg.Done()
		equity, errEquity = w.provider.GetEquity()
	}()

	// 2. Fetch Position Data
	for _, p := range activePositions {
		wg.Add(1)
		go func(pos models.Position) {
			defer wg.Done()
			current, _ := w.provider.GetPrice(pos.Ticker)
			bars, _ := w.provider.GetBars(pos.Ticker, 1)

			prevClose := decimal.Zero
			if len(bars) > 0 {
				prevClose = bars[len(bars)-1].Close
			}

			mu.Lock()
			posDetails[pos.Ticker] = detailedPos{
				Ticker:    pos.Ticker,
				Qty:       pos.Quantity,
				Entry:     pos.EntryPrice,
				Current:   current,
				PrevClose: prevClose,
				SL:        pos.StopLoss,
				HWM:       pos.HighWaterMark,
			}
			mu.Unlock()
		}(p)
	}

	wg.Wait()

	// Format Output
	var sb strings.Builder

	// Header: Market Status
	statusIcon := "🔴"
	statusText := "CLOSED"
	timeMsg := ""

	if errClock == nil {
		if clock.IsOpen {
			statusIcon = "🟢"
			statusText = "OPEN"
			until := time.Until(clock.NextClose).Round(time.Minute)
			timeMsg = fmt.Sprintf("Closes in: %s", until)
		} else {
			until := time.Until(clock.NextOpen).Round(time.Minute)
			timeMsg = fmt.Sprintf("Opens in: %s", until)
		}
	} else {
		statusText = "Unknown"
	}

	sb.WriteString(fmt.Sprintf("Market: %s %s\n%s\n\n", statusIcon, statusText, timeMsg))

	// Positions Table
	if len(activePositions) > 0 {
		sb.WriteString("`Ticker | Price | DayP/L | TotP/L`\n")
		sb.WriteString("`--------------------------------`\n")

		totalDayPL := decimal.Zero
		totalUnrealizedPL := decimal.Zero

		for _, p := range activePositions {
			d := posDetails[p.Ticker]
			if d.Current.IsZero() {
				sb.WriteString(fmt.Sprintf("`%-6s | ERR   |   -    |   -   `\n", d.Ticker))
				continue
			}

			// Day P/L
			dayPL := decimal.Zero
			dayPLStr := "   -  "
			if !d.PrevClose.IsZero() {
				// (Current - PrevClose) * Qty
				dayPL = d.Current.Sub(d.PrevClose).Mul(d.Qty)
				totalDayPL = totalDayPL.Add(dayPL)
				icon := "🟢"
				if dayPL.IsNegative() {
					icon = "🔴"
				}
				dayPLStr = fmt.Sprintf("%s%s", icon, dayPL.StringFixed(2))
			}

			// Total P/L
			// (Current - Entry) * Qty
			totPL := d.Current.Sub(d.Entry).Mul(d.Qty)
			totalUnrealizedPL = totalUnrealizedPL.Add(totPL)
			totIcon := "🟢"
			if totPL.IsNegative() {
				totIcon = "🔴"
			}

			sb.WriteString(fmt.Sprintf("`%-6s | %-6s | %s | %s%s`\n",
				d.Ticker, d.Current.StringFixed(2), dayPLStr, totIcon, totPL.StringFixed(2)))

			// Context line
			distSL := "N/A"
			slPriceStr := "N/A"
			if !d.Current.IsZero() && !d.SL.IsZero() {
				// (Current - SL) / Current * 100
				pct := d.Current.Sub(d.SL).Div(d.Current).Mul(decimal.NewFromInt(100))
				distSL = fmt.Sprintf("%s%%", pct.StringFixed(1))
				slPriceStr = "$" + d.SL.StringFixed(2)
			}
			sb.WriteString(fmt.Sprintf("      ↳ SL: %s (%s) | HWM: $%s\n", slPriceStr, distSL, d.HWM.StringFixed(2)))
		}
		sb.WriteString("\n")
	}

	// Footer
	equityStr := "$" + equity.StringFixed(2)
	if errEquity != nil {
		equityStr = "Err"
	}

	uptime := time.Since(startTime).Round(time.Second)

	// Pending Orders (Preserve Spec 26)
	pendingMsg := ""
	openOrders, err := w.provider.ListOrders("open")
	if err == nil && len(openOrders) > 0 {
		pendingMsg = "\n⏳ *PENDING ORDERS*:\n"
		for _, o := range openOrders {
			// Alpaca Order Qty is *decimal.Decimal usually?
			// Let's assume it is String or we use Qty directly if it prints.
			// Actually Alpaca Order struct `Qty` is *decimal.Decimal in v3?
			// Checking orders.go... it returns *alpaca.Order.
			// Spec says use decimal. We used `o.Qty` in handleBuyCommand validation? No, that was `w.provider.ListOrders`.
			// `o.Qty` is *decimal.Decimal.
			qtyStr := "0"
			if !o.Qty.IsZero() {
				qtyStr = o.Qty.String()
			}
			pendingMsg += fmt.Sprintf("• %s %s %s\n", o.Side, qtyStr, o.Symbol)
		}
	}

	buyingPower, errBP := w.provider.GetBuyingPower()

	sb.WriteString(fmt.Sprintf("Equity: %s\n", equityStr))
	if errBP == nil {
		sb.WriteString(fmt.Sprintf("Buying Power: $%s\n", buyingPower.StringFixed(2)))
	} else {
		sb.WriteString("Buying Power: Err\n")
	}
	sb.WriteString(fmt.Sprintf("Uptime: %s%s", uptime, pendingMsg))

	return sb.String()
}

func (w *Watcher) getList() string {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if len(w.state.Positions) == 0 {
		return "No positions tracked."
	}

	// Copy active positions to release lock
	activePositions := []models.Position{}
	for _, pos := range w.state.Positions {
		if pos.Status == "ACTIVE" {
			activePositions = append(activePositions, pos)
		}
	}
	w.mu.RUnlock() // manual unlock to allow network calls

	w.mu.RLock() // Re-acquire (wait, I logic-ed myself into a corner, let's restart the function logic)
	// New logic: Lock, Copy, Unlock, Fetch, Format.

	return w.getListSafe()
}

func (w *Watcher) getListSafe() string {
	w.mu.RLock()
	// Copy positions
	positions := make([]models.Position, len(w.state.Positions))
	copy(positions, w.state.Positions)
	w.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("📋 *POSITIONS*\n")

	var activeFound bool
	for _, pos := range positions {
		if pos.Status != "ACTIVE" {
			continue
		}
		activeFound = true

		price, err := w.provider.GetPrice(pos.Ticker)
		priceStr := fmt.Sprintf("$%s", price.StringFixed(2))
		distSL := "N/A"

		if err != nil {
			priceStr = "Err"
		} else {
			dist := decimal.Zero
			if !price.IsZero() {
				dist = price.Sub(pos.StopLoss).Div(price).Mul(decimal.NewFromInt(100))
			}
			distSL = fmt.Sprintf("%s%%", dist.StringFixed(2))
		}

		sb.WriteString(fmt.Sprintf("\n🔹 *%s*\nPrice: %s\nDist to SL: %s\n",
			pos.Ticker, priceStr, distSL))
	}

	if !activeFound {
		return "No active positions found."
	}

	return sb.String()
}

// checkEOD handles the Market Close detection and Reporting (Spec 49)
func (w *Watcher) checkEOD() {
	clock, err := w.provider.GetClock()
	if err != nil {
		log.Printf("Error fetching market clock: %v", err)
		return
	}

	// Load NY Location for accurate Date tracking
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		log.Printf("Error loading NY location: %v", err)
		return
	}
	nowNY := time.Now().In(loc)
	todayNY := nowNY.Format("2006-01-02")

	shouldSend := false

	// 1. Transition Trigger (Real-time)
	// Only trigger if we mistakenly thought it was open (or tracked it as open) and now it is closed.
	if w.wasMarketOpen && !clock.IsOpen {
		log.Println("📉 MARKET CLOSED (Transition detected).")
		shouldSend = true
	}

	// 2. Resilience Trigger (State-based)
	// If Market is CLOSED, and we haven't sent a report for "Today" yet.
	// We must ensure we are "After Close" (End of Day), not "Before Open" (Start of Day).
	if !clock.IsOpen {
		w.mu.RLock()
		lastDate := w.state.LastEODDate
		w.mu.RUnlock()

		if lastDate != todayNY {
			// Check if we are Before Next Open (i.e. currently Post-Market)
			// If NextOpen is TODAY, then we are Before Open. We should NOT report yet.
			// If NextOpen is TOMORROW (or later), then we are After Close. We SHOULD report.
			nextOpenNY := clock.NextOpen.In(loc)
			isNextOpenToday := nextOpenNY.Format("2006-01-02") == todayNY

			if !isNextOpenToday {
				log.Printf("📉 MARKET CLOSED (Resilience check: Last %s != Today %s).", lastDate, todayNY)
				shouldSend = true
			}
		}
	}

	if shouldSend {
		w.mu.Lock()
		// Double check inside lock
		if w.state.LastEODDate != todayNY {
			w.state.LastEODDate = todayNY
			w.saveStateLocked()
			w.mu.Unlock()

			log.Println("Generating EOD Report (Spec 49)...")
			go w.generateAndSendEODReport()
		} else {
			w.mu.Unlock()
		}
	}

	w.wasMarketOpen = clock.IsOpen
}

// generateAndSendEODReport implements Spec 49
func (w *Watcher) generateAndSendEODReport() {
	// 1. Fetch Data
	// Pillar 1: Current Positions (Unrealized)
	// Note: generic ListPositions returns BrokerPositions (unrealized at broker)
	positions, err := w.provider.ListPositions()
	if err != nil {
		log.Printf("EOD Error: Failed to list positions: %v", err)
		return
	}

	// Pillar 2: Historical (Equity Curve) - Get 1D history
	history, err := w.provider.GetPortfolioHistory("1D", "1Min")
	if err != nil {
		log.Printf("EOD Error: Failed to get history: %v", err)
	}

	// Pillar 3: Realized Today
	closedOrders, err := w.provider.ListOrders("closed")
	if err != nil {
		log.Printf("EOD Error: Failed to list closed orders: %v", err)
	}

	// 2. Calculations
	var startEquity, endEquity decimal.Decimal
	if history != nil && len(history.Equity) > 0 {
		startEquity = history.Equity[0]
		endEquity = history.Equity[len(history.Equity)-1]
	} else {
		// Fallback if history fails
		endEquity, _ = w.provider.GetEquity()
	}

	// Calculate Daily Change
	dailyChangePct := decimal.Zero
	if !startEquity.IsZero() {
		dailyChangePct = endEquity.Sub(startEquity).Div(startEquity).Mul(decimal.NewFromInt(100))
	}

	// Filter Realized Orders (Today Only)
	var realizedToday []string
	loc, _ := time.LoadLocation("Europe/Madrid") // Or use config.CetLoc if exported
	now := time.Now().In(loc)
	y, m, d := now.Date()

	for _, o := range closedOrders {
		if o.FilledAt == nil {
			continue
		}
		// Convert to CET/Target Time
		ft := o.FilledAt.In(loc)
		ty, tm, td := ft.Date()
		if ty == y && tm == m && td == d {
			price := decimal.Zero
			if !o.FilledAvgPrice.IsZero() {
				price = o.FilledAvgPrice
			}
			qty := decimal.Zero
			if !o.Qty.IsZero() {
				qty = o.Qty
			}
			realizedToday = append(realizedToday, fmt.Sprintf("%s %s %s @ $%s", o.Side, o.Symbol, qty.String(), price.StringFixed(2)))
		}
	}

	// 3. Report Formatting
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 *MARKET CLOSE REPORT - %s*\n\n", now.Format("2006-01-02")))

	// Section A: Account
	icon := "🟢"
	if dailyChangePct.IsNegative() {
		icon = "🔴"
	}
	sb.WriteString("*Account Summary*\n")
	sb.WriteString(fmt.Sprintf("End Equity: $%s\n", endEquity.StringFixed(2)))
	sb.WriteString(fmt.Sprintf("Daily Change: %s%s%%\n\n", icon, dailyChangePct.StringFixed(2)))

	// Section B: Per Asset Table (Unrealized)
	if len(positions) > 0 {
		sb.WriteString("`Ticker | Day % | Tot %`\n")
		sb.WriteString("`---------------------`\n")
		for _, p := range positions {
			dayChange := decimal.Zero
			if !p.ChangeToday.IsZero() {
				dayChange = p.ChangeToday.Mul(decimal.NewFromInt(100))
			}
			entry := p.AvgEntryPrice
			current := p.CurrentPrice // Assume safe
			totPct := decimal.Zero
			if !entry.IsZero() {
				totPct = current.Sub(entry).Div(entry).Mul(decimal.NewFromInt(100))
			}

			sb.WriteString(fmt.Sprintf("`%-6s | %5s%%| %5s%%`\n",
				p.Symbol, dayChange.StringFixed(2), totPct.StringFixed(2)))
		}
		sb.WriteString("\n")
	} else {
		sb.WriteString("ℹ️ No active positions carried overnight.\n\n")
	}

	// Section C: Realized
	if len(realizedToday) > 0 {
		sb.WriteString("*Activity Today*\n")
		// Limit length carefully
		if len(realizedToday) > 10 {
			for i := 0; i < 5; i++ {
				sb.WriteString(fmt.Sprintf("• %s\n", realizedToday[i]))
			}
			sb.WriteString(fmt.Sprintf("...and %d more.\n", len(realizedToday)-5))
		} else {
			for _, line := range realizedToday {
				sb.WriteString(fmt.Sprintf("• %s\n", line))
			}
		}
	} else {
		sb.WriteString("ℹ️ No trades closed today.")
	}

	report := sb.String()

	// 4. Send & Persist
	telegram.Notify(report)
	w.saveDailyPerformance(report)
}

func (w *Watcher) saveDailyPerformance(report string) {
	// Append to daily_performance.log
	f, err := os.OpenFile("daily_performance.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("Error opening daily_performance.log: %v", err)
		return
	}
	defer f.Close()

	if _, err := f.WriteString(fmt.Sprintf("\n--- %s ---\n%s\n", time.Now().Format("2006-01-02 15:04:05"), report)); err != nil {
		log.Printf("Error writing to daily log: %v", err)
	}
}
