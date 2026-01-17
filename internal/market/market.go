package market

import (
	"strings"
	"time"

	"github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
	"github.com/shopspring/decimal"
)

// MarketProvider is an Interface.
// Interfaces define *behavior*. Any struct that implements these methods
// satisfies the interface. This allows us to swap out Alpaca for Kraken,
// or a Mock for testing, without changing the code that *uses* the provider.
type MarketProvider interface {
	GetPrice(ticker string) (decimal.Decimal, error)
	GetQuote(ticker string) (*marketdata.Quote, error)
	GetEquity() (decimal.Decimal, error)
	GetClock() (*alpaca.Clock, error)
	SearchAssets(query string) ([]alpaca.Asset, error)
	PlaceOrder(ticker string, qty decimal.Decimal, side string, slPrice decimal.Decimal, tpPrice decimal.Decimal) (*alpaca.Order, error)
	GetOrder(orderID string) (*alpaca.Order, error)
	ListOrders(status string) ([]alpaca.Order, error)
	ListPositions() ([]alpaca.Position, error)
	CancelOrder(orderID string) error
	GetBuyingPower() (decimal.Decimal, error)
	GetBars(ticker string, limit int) ([]marketdata.Bar, error)
	GetPortfolioHistory(period string, timeframe string) (*alpaca.PortfolioHistory, error)
	GetAccount() (*alpaca.Account, error)
}

// AlpacaProvider is a concrete implementation of MarketProvider for the Alpaca API.
type AlpacaProvider struct {
	mdClient    *marketdata.Client // Client for market data (prices)
	tradeClient *alpaca.Client     // Client for trading data (account equity)
}

// NewAlpacaProvider is a "Constructor" function.
// Go doesn't have classes or constructors, so we use functions that return pointers to new structs.
func NewAlpacaProvider() *AlpacaProvider {
	return &AlpacaProvider{
		// We initialize the clients using the library's NewClient functions.
		// They automatically look for API keys in the environment variables we checked in config.
		mdClient:    marketdata.NewClient(marketdata.ClientOpts{}),
		tradeClient: alpaca.NewClient(alpaca.ClientOpts{}),
	}
}

// GetPrice fetches the latest trade price for a ticker.
// Note the receiver (a *AlpacaProvider) - this makes it a method of the struct.
func (a *AlpacaProvider) GetPrice(ticker string) (decimal.Decimal, error) {
	// We ask for the latest trade.
	trade, err := a.mdClient.GetLatestTrade(ticker, marketdata.GetLatestTradeRequest{})
	if err != nil {
		return decimal.Zero, err // Return 0 and the error if something fails
	}
	if trade == nil {
		return decimal.Zero, nil // Or a specific error like "no trade found"
	}
	return decimal.NewFromFloat(trade.Price), nil // Return the price and nil error if successful
}

// GetQuote fetches the latest quote (Bid/Ask) for a ticker.
func (a *AlpacaProvider) GetQuote(ticker string) (*marketdata.Quote, error) {
	return a.mdClient.GetLatestQuote(ticker, marketdata.GetLatestQuoteRequest{})
}

// GetEquity fetches the current total account equity.
func (a *AlpacaProvider) GetEquity() (decimal.Decimal, error) {
	acct, err := a.tradeClient.GetAccount()
	if err != nil {
		return decimal.Zero, err
	}
	// InexactFloat64 converts the decimal type to a standard float64.
	return acct.Equity, nil
}

// GetBuyingPower fetches the current buying power.
func (a *AlpacaProvider) GetBuyingPower() (decimal.Decimal, error) {
	acct, err := a.tradeClient.GetAccount()
	if err != nil {
		return decimal.Zero, err
	}
	return acct.BuyingPower, nil
}

// GetClock fetches the market clock (open/close status).
func (a *AlpacaProvider) GetClock() (*alpaca.Clock, error) {
	return a.tradeClient.GetClock()
}

// SearchAssets searches for assets matching the query string.
// It fetches active US equities and filters them in memory.
// Returns a maximum of 5 results.
func (a *AlpacaProvider) SearchAssets(query string) ([]alpaca.Asset, error) {
	status := "active"
	class := "us_equity"
	assets, err := a.tradeClient.GetAssets(alpaca.GetAssetsRequest{
		Status:     status,
		AssetClass: class,
	})
	if err != nil {
		return nil, err
	}

	var results []alpaca.Asset
	queryLower := strings.ToLower(query)

	for _, asset := range assets {
		if strings.Contains(strings.ToLower(asset.Symbol), queryLower) ||
			strings.Contains(strings.ToLower(asset.Name), queryLower) {
			results = append(results, asset)
			if len(results) >= 5 {
				break
			}
		}
	}
	return results, nil
}

// GetBars fetches historical bars for a ticker.
func (a *AlpacaProvider) GetBars(ticker string, limit int) ([]marketdata.Bar, error) {
	// Request last 5 days to ensure we get at least one previous close (handling weekends/holidays)
	start := time.Now().AddDate(0, 0, -5)

	bars, err := a.mdClient.GetBars(ticker, marketdata.GetBarsRequest{
		TimeFrame: marketdata.OneDay,
		Start:     start,
	})
	if err != nil {
		return nil, err
	}

	if len(bars) > limit {
		// Return the LAST 'limit' bars (most recent)
		return bars[len(bars)-limit:], nil
	}

	return bars, nil
}

// GetPortfolioHistory fetches the portfolio history for a specific period and timeframe.
func (a *AlpacaProvider) GetPortfolioHistory(period string, timeframe string) (*alpaca.PortfolioHistory, error) {
	return a.tradeClient.GetPortfolioHistory(alpaca.GetPortfolioHistoryRequest{
		Period:    period,
		TimeFrame: alpaca.TimeFrame(timeframe),
	})
}

// GetAccount fetches the full account object.
func (a *AlpacaProvider) GetAccount() (*alpaca.Account, error) {
	return a.tradeClient.GetAccount()
}
