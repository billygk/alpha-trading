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
			{"/ping", "Connectivity check", "/ping"},
			{"/status", "Detailed broker-native dashboard", "/status"},
			{"/buy", "Manual entry (native bracket)", "/buy <ticker> <qty> [sl] [tp]"},
			{"/sell", "Universal exit (cancels orders + liquidates)", "/sell <ticker>"},
			{"/update", "Mutate native risk parameters", "/update <ticker> <sl> <tp>"},
			{"/scan", "Trigger AI Portfolio Review & Autonomous Rotation", "/scan"},
			{"/stop", "Killswitch. Disables all autonomous execution", "/stop"},
			{"/start", "Enable autonomous execution", "/start"},
			{"/config", "Inspect system parameters", "/config"},
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

func (w *Watcher) SendStartupNotification() {
	equity, err := w.provider.GetEquity()
	if err != nil {
		log.Printf("Startup Warning: Could not fetch equity: %v", err)
	}
	bp, err := w.provider.GetBuyingPower()
	if err != nil {
		log.Printf("Startup Warning: Could not fetch BP: %v", err)
	}

	mode := "MANUAL"
	if w.IsAutonomousEnabled() {
		mode = "AUTONOMOUS"
	}

	msg := fmt.Sprintf("🚀 *SYSTEM START: Alpha Watcher %s online*\nMode: [%s]\nEquity: $%s | BP: $%s",
		w.config.Version, mode, equity.StringFixed(2), bp.StringFixed(2))
	telegram.Notify(msg)
}

func (w *Watcher) SendShutdownNotification() {
	w.mu.Lock()
	w.saveStateLocked()
	w.mu.Unlock()
	telegram.Notify("🛑 SYSTEM SHUTDOWN: Signal received. State saved successfully.")
}

func (w *Watcher) IsAutonomousEnabled() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.state.AutonomousEnabled
}

func (w *Watcher) runAIAnalysis(ticker string, isManual bool) {
	// Spec 96: Logic Guard for Autonomous Execution
	if !isManual && !w.IsAutonomousEnabled() {
		return
	}

	// Spec 58 & 64: AI Analysis Loop
	if w.config.GeminiAPIKey == "" {
		return
	}

	// Spec 99.2: Analysis Start Heartbeat
	if !isManual {
		telegram.Notify("🔍 SCAN INITIATED: Performing portfolio review and sector check...")
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

	// Spec 99.2: AI Interaction Heartbeat
	telegram.Notify("🤖 CONSULTING AI: Sending context to Gemini 2.5 Flash...")

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
	// Spec 70: Use JIT Sync to populate budget/exposure
	// This also populates WatchlistPrices (Spec 72)
	if _, err := w.SyncWithBroker(); err != nil {
		log.Printf("Snapshot Warning: JIT Sync failed: %v", err)
	}

	w.mu.RLock()
	defer w.mu.RUnlock()

	// Spec 78: Priority Watchlist Price Guardrail
	// Ensure WatchlistPrices is populated if triggers are configured.
	if len(w.config.WatchlistTickers) > 0 {
		if len(w.state.WatchlistPrices) == 0 {
			// CRITICAL: Data Missing. Try forced refresh?
			// SyncWithBroker just ran. If it's still empty, it means API failure or configuration mismatch.
			log.Printf("[CRITICAL_DATA_MISSING] Watchlist defined but prices are empty. AI may hallucinate HOLDs.")

			// We can try one more specific fetch for the first ticker to see if it's a connectivity issue?
			// Or just log as per spec.
			// "attempt a forced price refresh before proceeding"
			// SyncWithBroker WAS the forced refresh. If it failed to populate, we might be blocked.
			// Let's explicitly try to re-fetch one last time just for the watchlist if it's empty?
			// Actually, SyncWithBroker is the mechanism. If it fails, we shouldn't infinite loop.
			// We just log the critical error.
		}
	}

	equity, err := w.provider.GetEquity()
	if err != nil {
		return nil, err
	}
	bp, err := w.provider.GetBuyingPower()
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

	// Calculate Current Exposure Dynamically (Spec 92: Runtime-only)
	// Exposure = Sum(Qty * EntryPrice)
	currentExposure := decimal.Zero
	// Use read lock for safe access to Positions (already held)
	for _, p := range w.state.Positions {
		if p.Status == "ACTIVE" {
			cost := p.Quantity.Mul(p.EntryPrice)
			currentExposure = currentExposure.Add(cost)
		}
	}

	return &ai.PortfolioSnapshot{
		Timestamp:       time.Now().Format(time.RFC3339),
		MarketStatus:    status,
		Capital:         bp,
		Equity:          equity,
		CurrentExposure: currentExposure,
		Positions:       w.state.Positions,
		MarketContext:   marketContext,
		WatchlistPrices: w.state.WatchlistPrices, // Spec 74
	}, nil
}
