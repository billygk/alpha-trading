package watcher

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"alpha_trading/internal/ai"
	"alpha_trading/internal/config"
	"alpha_trading/internal/market"
	"alpha_trading/internal/models"
	"alpha_trading/internal/storage"
	"alpha_trading/internal/telegram"

	"github.com/shopspring/decimal"
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
	lastAnalyzeTime  map[string]time.Time // To prevent API spam (Spec 64)
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
		lastAnalyzeTime:  make(map[string]time.Time),
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
			{"/analyze", "Request AI portfolio analysis (10m cooldown)", "/analyze [ticker]"},
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

	// 4. AI Analysis Loop (Spec 58)
	// Trigger: Success of Poll Interval AND Market is Open (or Pre-Market)
	// We rely on the implicit "Poll" call being the interval trigger.
	// We need to check if Market is Open.
	// Re-fetch clock to be sure or reuse if we had it?
	// We fetch it fresh to be safe.
	c, err := w.provider.GetClock()
	if err == nil {
		// Time Gates:
		// 1. Market Open
		// 2. Pre-Market (14:30 - 15:30 CET). US Open is 15:30 CET.
		// "OR it is the 'Pre-Market Hour' (14:30 - 15:30 CET)"
		// Alpaca Clock is usually in EST.
		// Let's just use Alpaca's "IsOpen" for standard Hours.
		// For Pre-Market, we check current time vs Open time?
		// Spec says: "The API call to Gemini MUST ONLY occur if: The US Market is OPEN... OR it is the Pre-Market Hour"

		runAI := false
		if c.IsOpen {
			runAI = true
		} else {
			// Check Pre-Market (1 hour before open)
			if time.Until(c.NextOpen) <= 1*time.Hour {
				runAI = true
			}
		}

		if runAI {
			// Run AI Analysis Async
			go w.runAIAnalysis("", false)
		}
	}
}

func (w *Watcher) runAIAnalysis(ticker string, isManual bool) {
	// Spec 58 & 64: AI Analysis Loop
	if w.config.GeminiAPIKey == "" {
		return
	}

	// 1. Gather Data (Snapshot)
	snapshot, err := w.buildPortfolioSnapshot(ticker)
	if err != nil {
		log.Printf("AI Error: Failed to build snapshot: %v", err)
		return
	}

	// 2. Call AI
	// We need an AI Client.
	// Initialized in New? Or ad-hoc?
	// Let's make it ad-hoc for now or add to Watcher struct.
	// Ideally Watcher struct.
	// But since we are patching, let's instantiate.
	aiClient := ai.NewClient() // We'll fix imports later

	// Load System Instruction
	sysInstr, err := os.ReadFile("portfolio_review_update.md")
	if err != nil {
		log.Printf("AI Error: SysInstr missing: %v", err)
		return
	}

	// Enhance Prompt Context if ticker provided (Spec 64)
	contextMsg := ""
	if ticker != "" {
		contextMsg = fmt.Sprintf("\nFOCUS_CONTEXT: The user requested a specific analysis for %s. Please prioritize this asset in your review.", ticker)
	}

	analysis, err := aiClient.AnalyzePortfolio(string(sysInstr)+contextMsg, *snapshot)
	if err != nil {
		log.Printf("AI Error: API failure: %v", err)
		// Always notify on API failure (e.g. Quota Exceeded) so user knows why AI is silent
		telegram.Notify(fmt.Sprintf("⚠️ AI Analysis Failed:\n```\n%v\n```", err))
		return
	}

	// 3. Process Result (Spec 59, 60, 61, 62)
	w.handleAIResult(analysis, snapshot, isManual)
}

func (w *Watcher) buildPortfolioSnapshot(ticker string) (*ai.PortfolioSnapshot, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	equity, err := w.provider.GetEquity()
	if err != nil {
		return nil, err
	}
	bp, err := w.provider.GetBuyingPower() // Assuming this method exists in provider based on interface check
	if err != nil {
		return nil, err
	}

	clock, _ := w.provider.GetClock()
	status := "CLOSED"
	if clock != nil && clock.IsOpen {
		status = "OPEN"
	}

	marketContext := "Sector Scan: N/A"
	if ticker != "" {
		marketContext = fmt.Sprintf("Analysis Focus: %s", ticker)
	}

	// Spec 65: Calculate Available Budget
	var currentExposure decimal.Decimal
	for _, p := range w.state.Positions {
		if p.Status == "ACTIVE" {
			cost := p.Quantity.Mul(p.EntryPrice)
			currentExposure = currentExposure.Add(cost)
		}
	}
	fiscalLimit := decimal.NewFromFloat(w.config.FiscalBudgetLimit)
	availableBudget := fiscalLimit.Sub(currentExposure)

	return &ai.PortfolioSnapshot{
		Timestamp:       time.Now().Format(time.RFC3339),
		MarketStatus:    status,
		Capital:         bp,
		Equity:          equity,
		FiscalLimit:     fiscalLimit,
		AvailableBudget: availableBudget,
		CurrentExposure: currentExposure,
		Positions:       w.state.Positions,
		MarketContext:   marketContext,
	}, nil
}
