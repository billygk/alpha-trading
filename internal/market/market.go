package market

import (
	"strings"

	"github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
)

// MarketProvider is an Interface.
// Interfaces define *behavior*. Any struct that implements these methods
// satisfies the interface. This allows us to swap out Alpaca for Kraken,
// or a Mock for testing, without changing the code that *uses* the provider.
type MarketProvider interface {
	GetPrice(ticker string) (float64, error)
	GetEquity() (float64, error)
	GetClock() (*alpaca.Clock, error)
	SearchAssets(query string) ([]alpaca.Asset, error)
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
func (a *AlpacaProvider) GetPrice(ticker string) (float64, error) {
	// We ask for the latest trade.
	trade, err := a.mdClient.GetLatestTrade(ticker, marketdata.GetLatestTradeRequest{})
	if err != nil {
		return 0, err // Return 0 and the error if something fails
	}
	if trade == nil {
		return 0, nil // Or a specific error like "no trade found"
	}
	return trade.Price, nil // Return the price and nil error if successful
}

// GetEquity fetches the current total account equity.
func (a *AlpacaProvider) GetEquity() (float64, error) {
	acct, err := a.tradeClient.GetAccount()
	if err != nil {
		return 0, err
	}
	// InexactFloat64 converts the decimal type to a standard float64.
	return acct.Equity.InexactFloat64(), nil
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
