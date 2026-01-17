package market

import (
	"alpha_trading/internal/models"

	"github.com/shopspring/decimal"
)

// MarketProvider defines the behavior for interacting with a brokerage.
// It uses generic domain models to allow for multiple implementations (Alpaca, Kraken, etc.).
type MarketProvider interface {
	GetPrice(ticker string) (decimal.Decimal, error)
	GetQuote(ticker string) (*models.Quote, error)
	GetEquity() (decimal.Decimal, error)
	GetClock() (*models.Clock, error)
	SearchAssets(query string) ([]models.Asset, error)

	// Execution
	PlaceOrder(ticker string, qty decimal.Decimal, side string, slPrice decimal.Decimal, tpPrice decimal.Decimal) (*models.Order, error)
	GetOrder(orderID string) (*models.Order, error)
	ListOrders(status string) ([]models.Order, error)
	CancelOrder(orderID string) error

	// Helpers
	GetBuyingPower() (decimal.Decimal, error)
	ListPositions() ([]models.BrokerPosition, error)
	// Actually models.Position is our internal state. The broker returns "Broker Positions".
	// Our Watcher expects []alpaca.Position currently. We need to standardize this too.
	// Let's use `ListPositions() ([]models.BrokerPosition, error)` or map to our internal `models.Position`.
	// For now, let's look at `models.Position` in `internal/models/models.go`. It has stuff like `ThesisID` which broker doesn't have.
	// So we need a `models.BrokerPosition` struct in `market.go`.

	GetBars(ticker string, limit int) ([]models.Bar, error)
	GetPortfolioHistory(period string, timeframe string) (*models.PortfolioHistory, error)
	GetAccount() (*models.Account, error)
}
