package models

import (
	"time"

	"github.com/shopspring/decimal"
)

// Order represents a generic order found in any broker.
type Order struct {
	ID             string          `json:"id"`
	ClientOrderID  string          `json:"client_order_id"`
	Symbol         string          `json:"symbol"`
	Qty            decimal.Decimal `json:"qty"`
	FilledQty      decimal.Decimal `json:"filled_qty"`
	Type           string          `json:"type"`   // market, limit, stop, etc.
	Side           string          `json:"side"`   // buy, sell
	Status         string          `json:"status"` // new, filled, canceled, expired, rejected
	FilledAvgPrice decimal.Decimal `json:"filled_avg_price"`
	CreatedAt      time.Time       `json:"created_at"`
	FilledAt       *time.Time      `json:"filled_at,omitempty"`
	FailReason     string          `json:"fail_reason,omitempty"`
}

// Quote represents a generic bid/ask quote.
type Quote struct {
	Symbol    string
	BidPrice  decimal.Decimal
	AskPrice  decimal.Decimal
	Timestamp time.Time
}

// Account represents the generic account state.
type Account struct {
	ID               string
	Currency         string
	Equity           decimal.Decimal
	BuyingPower      decimal.Decimal
	Cash             decimal.Decimal
	PortfolioValue   decimal.Decimal
	DaytradeCount    int
	IsDayTrader      bool
	IsAccountBlocked bool
}

// Clock represents the market status.
type Clock struct {
	Timestamp time.Time
	IsOpen    bool
	NextOpen  time.Time
	NextClose time.Time
}

// Asset represents a tradable instrument.
type Asset struct {
	ID       string
	Symbol   string
	Name     string
	Class    string // us_equity, crypto, etc.
	Exchange string
	Status   string // active, inactive
	Tradable bool
}

// Bar represents a candlestick for a timeframe.
type Bar struct {
	Time   time.Time
	Open   decimal.Decimal
	High   decimal.Decimal
	Low    decimal.Decimal
	Close  decimal.Decimal
	Volume int64
}

// PortfolioHistory represents the equity curve over time.
type PortfolioHistory struct {
	Timestamps    []int64           `json:"timestamp"`
	Equity        []decimal.Decimal `json:"equity"`
	ProfitLoss    []decimal.Decimal `json:"profit_loss"`
	ProfitLossPct []decimal.Decimal `json:"profit_loss_pct"`
}

// BrokerPosition represents a position held at the broker.
type BrokerPosition struct {
	Symbol         string          `json:"symbol"`
	Qty            decimal.Decimal `json:"qty"`
	AvgEntryPrice  decimal.Decimal `json:"avg_entry_price"`
	CurrentPrice   decimal.Decimal `json:"current_price"`
	MarketValue    decimal.Decimal `json:"market_value"`
	CostBasis      decimal.Decimal `json:"cost_basis"`
	UnrealizedPL   decimal.Decimal `json:"unrealized_pl"`
	UnrealizedPLPC decimal.Decimal `json:"unrealized_plpc"`
	ChangeToday    decimal.Decimal `json:"change_today"`
}

// MarketProvider defines the interface for interacting with a brokerage.
// (Defined in internal/market/market.go, but models are here)
