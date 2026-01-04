package models

// Position represents a single trading position.
//
// In Go, structs are collections of fields.
// The text inside the backticks (e.g. `json:"ticker"`) are "struct tags".
// They tell the JSON encoder/decoder which keys to map to these fields.
type Position struct {
	Ticker          string  `json:"ticker"`            // The stock symbol (e.g., "AAPL")
	Quantity        float64 `json:"quantity"`          // Number of shares held
	EntryPrice      float64 `json:"entry_price"`       // Price at which we bought
	StopLoss        float64 `json:"stop_loss"`         // Price at which we sell to limit loss
	TakeProfit      float64 `json:"take_profit"`       // Price at which we sell to take profit
	Status          string  `json:"status"`            // e.g., "ACTIVE", "TRIGGERED_SL", "TRIGGERED_TS"
	ThesisID        string  `json:"thesis_id"`         // ID linking to the trade thesis
	HighWaterMark   float64 `json:"high_water_mark"`   // Highest price reached since entry
	TrailingStopPct float64 `json:"trailing_stop_pct"` // Trailing Stop percentage (e.g., 5.0 for 5%)
}

// PortfolioState tracks the state of the portfolio and system.
// This struct matches the structure of our JSON storage file.
type PortfolioState struct {
	Version       string     `json:"version"`        // Schema version for future compatibility
	LastSync      string     `json:"last_sync"`      // Timestamp of last file save
	LastHeartbeat string     `json:"last_heartbeat"` // Timestamp of last "I'm alive" message
	Positions     []Position `json:"positions"`      // A slice (variable-length array) of Positions
}
