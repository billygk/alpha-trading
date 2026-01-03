package market

import (
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
