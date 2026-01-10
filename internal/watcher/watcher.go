package watcher

import (
	"log"
	"sync"
	"time"

	"alpha_trading/internal/config"
	"alpha_trading/internal/market"
	"alpha_trading/internal/models"
	"alpha_trading/internal/storage"
	"alpha_trading/internal/telegram"
)

var startTime = time.Now()

var sectors = map[string][]string{
	"biotech": {"XBI", "VRTX", "AMGN"},
	"metals":  {"GLD", "SLV", "COPX"},
	"energy":  {"URA", "CCJ", "XLE"},
	"defense": {"ITA", "LMT", "RTX"},
}

type Watcher struct {
	provider         market.MarketProvider
	state            models.PortfolioState
	mu               sync.RWMutex
	commands         []CommandDoc
	pendingActions   map[string]PendingAction
	pendingProposals map[string]PendingProposal
	lastAlerts       map[string]time.Time // To prevent alert fatigue (Spec 38)
	wasMarketOpen    bool                 // For EOD trigger (Spec 49)
	config           *config.Config
}

func New(cfg *config.Config, provider market.MarketProvider) *Watcher {
	// Load initial state into memory
	s, err := storage.LoadState()
	if err != nil {
		log.Printf("CRITICAL: Could not load initial state: %v", err)
	}

	w := &Watcher{
		provider:         provider,
		state:            s,
		pendingActions:   make(map[string]PendingAction),
		pendingProposals: make(map[string]PendingProposal),
		lastAlerts:       make(map[string]time.Time),
		config:           cfg,
		wasMarketOpen:    false, // Default to false, will sync on first poll
		commands: []CommandDoc{
			{"/buy", "Propose a new trade", "/buy <ticker> <qty> [sl] [tp]"},
			{"/sell", "Liquidate and clean state", "/sell <ticker>"},
			{"/refresh", "Sync local state with Alpaca truth", "/refresh"},
			{"/status", "Immediate Rich Dashboard", "/status"},
			{"/list", "List active positions", "/list"},
			{"/price", "Get real-time price for a ticker", "/price AAPL"},
			{"/market", "Check market status", "/market"},
			{"/search", "Search for assets by name/ticker", "/search Apple"},
			{"/ping", "Check bot latency", "/ping"},
			{"/update", "Update SL/TP for active position", "/update <ticker> <sl> <tp> [ts-pct]"},
			{"/scan", "Scan sector health (biotech, metals, energy, defense)", "/scan <sector>"},
			{"/portfolio", "Dump raw portfolio state for debugging", "/portfolio"},
			{"/help", "Show this help message", "/help"},
		},
	}

	return w
}

func (w *Watcher) Poll() {
	w.checkEOD()

	var sendDashboard bool

	// 1. Critical Section: State Management & Risk Checks
	func() {
		w.mu.Lock()
		defer w.mu.Unlock()

		// --- AUTO-STATUS / HEARTBEAT ---
		// Spec 34: If market is OPEN and PollInterval elapsed (implied by Poll call), send dashboard.
		// We verify market status inside lock? Or outside? Outside is better for latency, but we need config.
		// Config is constant? Yes.
		// Let's do a quick clock check here or assume caller behavior?
		// Actually, let's just use the boolean flag logic.

		// Logic:
		// 1. Always check risk (Stop Loss, etc) - This is already done below in this function (lines 800+).
		// 2. Decide if we send the dashboard.

		// For now, we just mark it. We can't call GetClock inside here effectively if we want to minimize lock time,
		// but risk checks do network calls anyway.

		// Spec 43: Auto-Status during market hours
		// If AUTO_STATUS_ENABLED is true, we verify market status here (inside lock mainly for variable access, but network call is better outside).
		// We use the 'sendDashboard' flag.
		if w.config.AutoStatusEnabled {
			// Logic handled below outside lock
			sendDashboard = true
		} else {
			// Standard 24h Heartbeat for fallback
			if w.state.LastHeartbeat == "" {
				sendDashboard = true
			} else {
				lastHB, _ := time.Parse(time.RFC3339, w.state.LastHeartbeat)
				if time.Since(lastHB) >= 24*time.Hour {
					sendDashboard = true
				}
			}
		}

		if sendDashboard {
			w.state.LastHeartbeat = time.Now().In(config.CetLoc).Format(time.RFC3339)
		}
	}()

	// 2. Dashboard Delivery (Outside Lock)
	if sendDashboard {
		// Spec 43: Check Market Status
		clock, err := w.provider.GetClock()
		isMarketOpen := err == nil && clock.IsOpen

		shouldSend := false
		if w.config.AutoStatusEnabled {
			if isMarketOpen {
				shouldSend = true // Only send if Market is OPEN
			}
		} else {
			shouldSend = true // Fallback 24h heartbeat
		}

		if shouldSend {
			msg := w.getStatus()
			telegram.Notify(msg)
		}
	}

	// 3. Re-acquire lock for Risk Logic (legacy structure compatibility)
	// The original code had Risk Logic INSIDE the functionality.
	// We split it. We need to run risk check logic.
	// We can put Risk Logic in its own method `checkRisk()`?
	// Or just do it here. The original function was monolithic.
	// Let's run risk check here.
	w.checkRisk()
}
