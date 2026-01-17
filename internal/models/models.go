package models

import (
	"time"

	"github.com/shopspring/decimal"
)

// Position represents a single trading position.
//
// In Go, structs are collections of fields.
// The text inside the backticks (e.g. `json:"ticker"`) are "struct tags".
// They tell the JSON encoder/decoder which keys to map to these fields.
type Position struct {
	Ticker          string          `json:"ticker"`            // The stock symbol (e.g., "AAPL")
	Quantity        decimal.Decimal `json:"quantity"`          // Number of shares held
	EntryPrice      decimal.Decimal `json:"entry_price"`       // Price at which we bought
	StopLoss        decimal.Decimal `json:"stop_loss"`         // Price at which we sell to limit loss
	TakeProfit      decimal.Decimal `json:"take_profit"`       // Price at which we sell to take profit
	Status          string          `json:"status"`            // e.g., "ACTIVE", "TRIGGERED_SL", "TRIGGERED_TS"
	ThesisID        string          `json:"thesis_id"`         // ID linking to the trade thesis
	HighWaterMark   decimal.Decimal `json:"high_water_mark"`   // Highest price reached since entry
	TrailingStopPct decimal.Decimal `json:"trailing_stop_pct"` // Trailing Stop percentage (e.g., 5.0 for 5%)
	OpenedAt        time.Time       `json:"opened_at"`         // Spec 66: Timestamp when position was opened
}

// PortfolioState tracks the state of the portfolio and system.
// This struct matches the structure of our JSON storage file.
type PortfolioState struct {
	Version           string             `json:"version"`            // Schema version for future compatibility
	LastSync          string             `json:"last_sync"`          // Timestamp of last file save
	LastHeartbeat     string             `json:"last_heartbeat"`     // Timestamp of last "I'm alive" message
	LastEODDate       string             `json:"last_eod_date"`      // Spec 49: Date of last EOD report (YYYY-MM-DD, NY Time)
	Positions         []Position         `json:"positions"`          // A slice (variable-length array) of Positions
	WatchlistPrices   map[string]float64 `json:"watchlist_prices"`   // Spec 72: Watchlist Prices
	AutonomousEnabled bool               `json:"autonomous_enabled"` // Spec 86: Global Killswitch
}
