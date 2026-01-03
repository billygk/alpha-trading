package models

// Position represents a single trading position.
//
// In Go, structs are collections of fields.
// The text inside the backticks (e.g. `json:"ticker"`) are "struct tags".
// They tell the JSON encoder/decoder which keys to map to these fields.
type Position struct {
	Ticker     string  `json:"ticker"`      // The stock symbol (e.g., "AAPL")
	EntryPrice float64 `json:"entry_price"` // Price at which we bought
	StopLoss   float64 `json:"stop_loss"`   // Price at which we sell to limit loss
	TakeProfit float64 `json:"take_profit"` // Price at which we sell to take profit
	Status     string  `json:"status"`      // e.g., "ACTIVE", "TRIGGERED_SL"
	ThesisID   string  `json:"thesis_id"`   // ID linking to the trade thesis
}

// PortfolioState tracks the state of the portfolio and system.
// This struct matches the structure of our JSON storage file.
type PortfolioState struct {
	Version       string     `json:"version"`        // Schema version for future compatibility
	LastSync      string     `json:"last_sync"`      // Timestamp of last file save
	LastHeartbeat string     `json:"last_heartbeat"` // Timestamp of last "I'm alive" message
	Positions     []Position `json:"positions"`      // A slice (variable-length array) of Positions
}
