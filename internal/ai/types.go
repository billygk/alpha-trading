package ai

import "github.com/shopspring/decimal"

// AIAnalysis represents the structured output expected from the Gemini model (Spec 59).
type AIAnalysis struct {
	Analysis        string  `json:"analysis"`
	Recommendation  string  `json:"recommendation"` // BUY, SELL, UPDATE, HOLD
	ActionCommand   string  `json:"action_command"`
	ConfidenceScore float64 `json:"confidence_score"`
	RiskAssessment  string  `json:"risk_assessment"` // LOW, MEDIUM, HIGH
}

// PortfolioSnapshot represents the data payload sent to the AI.
type PortfolioSnapshot struct {
	Timestamp       string             `json:"timestamp"`
	MarketStatus    string             `json:"market_status"`
	Capital         decimal.Decimal    `json:"capital_available"` // Buying Power
	Equity          decimal.Decimal    `json:"equity"`
	FiscalLimit     decimal.Decimal    `json:"fiscal_limit"`     // Spec 63 Hard Limit
	AvailableBudget decimal.Decimal    `json:"available_budget"` // Spec 65: FiscalLimit - CurrentExposure
	CurrentExposure decimal.Decimal    `json:"current_exposure"` // Total cost basis of active positions
	Positions       interface{}        `json:"positions"`        // Raw list from state
	MarketContext   string             `json:"market_context"`   // E.g., global trend or sector info if available
	WatchlistPrices map[string]float64 `json:"watchlist_prices"` // Spec 74: Watchlist Prices injection
}
