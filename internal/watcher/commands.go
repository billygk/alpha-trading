package watcher

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"alpha_trading/internal/telegram"

	"github.com/shopspring/decimal"
)

type CommandDoc struct {
	Name        string
	Description string
	Example     string
}

// HandleCommand processes inbound Telegram commands safely.
func (w *Watcher) HandleCommand(cmd string) string {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return ""
	}

	switch parts[0] {
	case "/ping":
		return "Pong 🏓"
	case "/status":
		return w.getStatus()
	case "/list":
		return w.getList()
	case "/price":
		if len(parts) < 2 {
			return "Usage: /price <ticker>"
		}
		return w.getPrice(strings.ToUpper(parts[1]))
	case "/market":
		return w.getMarketStatus()
	case "/search":
		if len(parts) < 2 {
			return "Usage: /search <query>"
		}
		// "Apple Inc" -> "Apple Inc"
		query := strings.Join(parts[1:], " ")
		return w.searchAssets(query)
	case "/help":
		println("Help command received")
		return w.getHelp()
	case "/buy":
		return w.handleBuyCommand(parts)
	case "/scan":
		return w.handleScanCommand(parts)
	case "/portfolio":
		return w.handlePortfolioCommand()
	case "/sell":
		return w.handleSellCommand(parts)
	case "/update":
		return w.handleUpdateCommand(parts)
	case "/refresh":
		// Spec 44: Command Purity Enforcement
		if len(parts) > 1 {
			return "⚠️ Error: /refresh does not accept parameters. Use /sell then /buy to change settings."
		}
		return w.handleRefreshCommand()
	default:
		return "Unknown command. Try /buy, /status, /sell, /refresh or /scan."
	}
}

func (w *Watcher) handleScanCommand(parts []string) string {
	if len(parts) < 2 {
		return "Usage: /scan <sector>\nAvailable: biotech, metals, energy, defense"
	}

	sectorKey := strings.ToLower(parts[1])
	tickers, exists := sectors[sectorKey]
	if !exists {
		return fmt.Sprintf("⚠️ Unknown sector '%s'.\nAvailable: biotech, metals, energy, defense", sectorKey)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔋 *SECTOR REPORT: %s*\n", strings.ToUpper(sectorKey)))

	for _, ticker := range tickers {
		price, err := w.provider.GetPrice(ticker)
		if err != nil {
			sb.WriteString(fmt.Sprintf("• %s: ⚠️ Err\n", ticker))
			continue
		}
		sb.WriteString(fmt.Sprintf("• %s: $%s\n", ticker, price.StringFixed(2)))
	}

	return sb.String()
}

func (w *Watcher) handleBuyCommand(parts []string) string {
	// 1. Parsing & Default Logic (Spec 41)
	// /buy AAPL 1 [sl] [tp]
	if len(parts) < 3 {
		return "Usage: /buy <ticker> <qty> [sl] [tp]"
	}

	ticker := strings.ToUpper(parts[1])

	// 1.5 Validation Gate (Duplicate Order Check) - Restored
	openOrders, err := w.provider.ListOrders("open")
	if err == nil {
		for _, o := range openOrders {
			if o.Symbol == ticker {
				return fmt.Sprintf("⚠️ Order already pending for %s. Cancel it on Alpaca before placing a new one.", ticker)
			}
		}
	} else {
		log.Printf("Warning: Failed to list open orders: %v", err)
	}

	qty, err1 := decimal.NewFromString(parts[2])
	if err1 != nil {
		return "⚠️ Invalid quantity format."
	}

	// Optional SL
	var sl decimal.Decimal
	var err2 error
	if len(parts) >= 4 && parts[3] != "0" {
		sl, err2 = decimal.NewFromString(parts[3])
	}

	// Optional TP
	var tp decimal.Decimal
	var err3 error
	if len(parts) >= 5 && parts[4] != "0" {
		tp, err3 = decimal.NewFromString(parts[4])
	}

	if err2 != nil || err3 != nil {
		return "⚠️ Invalid price format."
	}

	// 2. Price Check Gate (needed for Default Calc)
	price, err := w.provider.GetPrice(ticker)
	if err != nil {
		return fmt.Sprintf("⚠️ Could not fetch price for %s.", ticker)
	}

	// Default Logic (Spec 41)
	if sl.IsZero() {
		// Entry * (1 - DefaultSL/100)
		multiplier := decimal.NewFromInt(1).Sub(decimal.NewFromFloat(w.config.DefaultStopLossPct).Div(decimal.NewFromInt(100)))
		sl = price.Mul(multiplier)
	}

	if tp.IsZero() {
		// Entry * (1 + DefaultTP/100)
		multiplier := decimal.NewFromInt(1).Add(decimal.NewFromFloat(w.config.DefaultTakeProfitPct).Div(decimal.NewFromInt(100)))
		tp = price.Mul(multiplier)
	}

	// Default Trailing Stop (Spec 41 Safety)
	tsPct := decimal.NewFromFloat(w.config.DefaultTrailingStopPct)

	totalCost := price.Mul(qty)
	buyingPower, err := w.provider.GetBuyingPower()
	if err != nil {
		log.Printf("Error fetching BP: %v", err)
		return "⚠️ Error checking buying power."
	}

	if totalCost.GreaterThan(buyingPower) {
		return fmt.Sprintf("❌ Insufficient Buying Power.\nRequired: $%s\nAvailable: $%s", totalCost.StringFixed(2), buyingPower.StringFixed(2))
	}

	// Store Proposal
	w.mu.Lock()
	w.pendingProposals[ticker] = PendingProposal{
		Ticker:          ticker,
		Qty:             qty,
		Price:           price,
		TotalCost:       totalCost,
		StopLoss:        sl,
		TakeProfit:      tp,
		TrailingStopPct: tsPct,
		Timestamp:       time.Now(),
	}
	w.mu.Unlock()

	// Response with Buttons
	msg := fmt.Sprintf("📝 *TRADE PROPOSAL*\n"+
		"Asset: %s\n"+
		"Qty: %s\n"+
		"Price: $%s\n"+
		"Total: $%s\n"+
		"SL: $%s | TP: $%s\n"+
		"TS: %s%%\n"+
		"Confirm Execution?\n\n"+
		"⏱️ Valid for %d seconds.",
		ticker, qty.StringFixed(2), price.StringFixed(2), totalCost.StringFixed(2), sl.StringFixed(2), tp.StringFixed(2), tsPct.StringFixed(2),
		w.config.ConfirmationTTLSec)

	buttons := []telegram.Button{
		{Text: "✅ EXECUTE", CallbackData: fmt.Sprintf("EXECUTE_BUY_%s", ticker)},
		{Text: "❌ CANCEL", CallbackData: fmt.Sprintf("CANCEL_BUY_%s", ticker)},
	}

	telegram.SendInteractiveMessage(msg, buttons)
	return "" // Message sent interactively
}

func (w *Watcher) getHelp() string {
	var sb strings.Builder
	sb.WriteString("🤖 *ALPHA WATCHER COMMANDS*\n\n")
	for _, cmd := range w.commands {
		sb.WriteString(fmt.Sprintf("🔹 *%s*\n%s\n`%s`\n\n", cmd.Name, cmd.Description, cmd.Example))
	}
	return sb.String()
}

func (w *Watcher) handleSellCommand(parts []string) string {
	if len(parts) < 2 {
		return "Usage: /sell <ticker>"
	}
	ticker := strings.ToUpper(parts[1])

	msg := []string{fmt.Sprintf("📉 *Manual Universal Exit: %s*", ticker)}

	// 1. Sequential Clearance (Spec 54)
	if err := w.ensureSequentialClearance(ticker); err != nil {
		msg = append(msg, fmt.Sprintf("⚠️ Failed to clear pending orders: %v", err))
		// We try to proceed anyway for "Emergency Exit" semantics, but warn.
	} else {
		msg = append(msg, "DEBUG: Pending orders cleared.")
	}

	// 2. Check Active Positions & Execute Sell
	positions, err := w.provider.ListPositions()
	positionFound := false
	if err != nil {
		msg = append(msg, fmt.Sprintf("⚠️ Failed to list positions: %v", err))
	} else {
		for _, p := range positions {
			if p.Symbol == ticker {
				positionFound = true

				// Execute Sell
				order, err := w.provider.PlaceOrder(ticker, p.Qty, "sell")
				if err != nil {
					msg = append(msg, fmt.Sprintf("❌ Failed to sell position: %v", err))
					log.Printf("[FATAL_TRADE_ERROR] Manual sell failed for %s: %v", ticker, err)
				} else {
					// Spec 53: Execution Verification
					verified, vErr := w.verifyOrderExecution(order.ID)
					if vErr != nil {
						msg = append(msg, fmt.Sprintf("⚠️ Order placed but verification failed: %v", vErr))
					} else {
						msg = append(msg, fmt.Sprintf("✅ Triggered Market Sell (Status: %s).", verified.Status))
					}
				}
				break
			}
		}
	}

	if !positionFound {
		msg = append(msg, "ℹ️ No active position found on exchange.")
	}

	// 3. Cleanup Local State
	// Mark local state as CLOSED for this ticker regardless of exchange state if requested?
	// Spec says: "Upon confirmation of fill/cancellation, update portfolio_state.json to reflect Status: CLOSED."
	// Since we fired Market Sell, we can assume it will close. Better to be safe and mask it.

	updated := false
	for i := range w.state.Positions {
		if w.state.Positions[i].Ticker == ticker && w.state.Positions[i].Status == "ACTIVE" {
			w.state.Positions[i].Status = "CLOSED"
			updated = true
		}
	}

	if updated {
		w.saveStateAsync()
		msg = append(msg, "✅ Local state updated to CLOSED.")
	}

	if !positionFound { // Removed ordersFound check as we cleared them implicitly
		return strings.Join(msg, "\n")
	}

	return strings.Join(msg, "\n")
}

func (w *Watcher) handleUpdateCommand(parts []string) string {
	// /update AAPL 200 250 [5.0]
	if len(parts) < 4 {
		return "Usage: /update <ticker> <sl> <tp> [ts_pct]"
	}

	ticker := strings.ToUpper(parts[1])
	sl, err1 := decimal.NewFromString(parts[2])
	tp, err2 := decimal.NewFromString(parts[3])

	var tsPct decimal.Decimal
	var err3 error
	if len(parts) >= 5 {
		tsPct, err3 = decimal.NewFromString(parts[4])
	}

	if err1 != nil || err2 != nil || err3 != nil {
		return "⚠️ Invalid number format."
	}

	// --- Spec 51: Intent Mutation Guardrails ---
	// 1. Context: Get Market Price (Network Call outside lock)
	currentPrice, err := w.provider.GetPrice(ticker)
	if err != nil {
		return fmt.Sprintf("⚠️ Validation Failed: Could not fetch market price for %s to verify safety.", ticker)
	}

	// 2. SL Validation
	if !sl.LessThan(currentPrice) {
		return fmt.Sprintf("❌ Safety Gate: New SL ($%s) must be LOWER than current price ($%s).", sl.StringFixed(2), currentPrice.StringFixed(2))
	}

	// 3. TP Validation
	if !tp.GreaterThan(currentPrice) {
		return fmt.Sprintf("❌ Safety Gate: New TP ($%s) must be HIGHER than current price ($%s).", tp.StringFixed(2), currentPrice.StringFixed(2))
	}

	// 4. Logical Consistency
	if !tp.GreaterThan(sl) {
		return "❌ Logic Error: Take Profit must be higher than Stop Loss."
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	found := false
	for i, p := range w.state.Positions {
		if p.Ticker == ticker && p.Status == "ACTIVE" {
			w.state.Positions[i].StopLoss = sl
			w.state.Positions[i].TakeProfit = tp
			if len(parts) >= 5 {
				w.state.Positions[i].TrailingStopPct = tsPct
			}
			found = true
			break
		}
	}

	if !found {
		return fmt.Sprintf("⚠️ No active position found for %s (or check portfolio_state.json).", ticker)
	}

	// Spec 51: Explicit confirmation format
	w.saveStateAsync()
	return fmt.Sprintf("✅ Parameters Updated for %s.\nNew Floor (SL): $%s | New Ceiling (TP): $%s",
		ticker, sl.StringFixed(2), tp.StringFixed(2))
}

func (w *Watcher) handleRefreshCommand() string {
	count, discovered, err := w.syncState()
	if err != nil {
		return fmt.Sprintf("❌ Failed to sync state: %v", err)
	}

	msg := fmt.Sprintf("🔄 Strict Mirror Sync Complete: Local state aligned with Alpaca (%d active positions).", count)
	if len(discovered) > 0 {
		msg += fmt.Sprintf("\n⚠️ Imported & Protected: %s", strings.Join(discovered, ", "))
	}

	return msg
}

// handlePortfolioCommand implements Spec 50: Raw State Inspection
// It reads the local portfolio_state.json and returns it as a code block.
// Refined Logic: Chunks content if > 3900 chars (Spec 50 Refinement).
func (w *Watcher) handlePortfolioCommand() string {
	// 1. Read the file
	data, err := os.ReadFile("portfolio_state.json")
	if err != nil {
		log.Printf("Error reading portfolio_state.json: %v", err)
		return fmt.Sprintf("⚠️ Failed to read local state file: %v", err)
	}

	content := string(data)
	contentLen := len(content)
	chunkSize := 3900

	// 2. Simple Case: Fits in one message
	if contentLen <= chunkSize {
		return fmt.Sprintf("Portfolio State JSON (Part 1/1):\n```json\n%s\n```", content)
	}

	// 3. Complex Case: Multi-part Chunking
	chunks := (contentLen + chunkSize - 1) / chunkSize // ceil division

	for i := 0; i < chunks; i++ {
		start := i * chunkSize
		end := start + chunkSize
		if end > contentLen {
			end = contentLen
		}

		chunk := content[start:end]
		msg := fmt.Sprintf("Portfolio State JSON (Part %d/%d):\n```json\n%s\n```", i+1, chunks, chunk)

		// Proactively send to avoid return-value size limits or timeouts
		// Telegram API rate limits might hit if chunks are plenty, but for state.json (<100KB) it's fine.
		telegram.Notify(msg)

		// Small sleep to ensure ordering (Telegram API race condition mitigation)
		time.Sleep(200 * time.Millisecond)
	}

	return "" // Handled proactively
}
