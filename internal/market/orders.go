package market

import (
	"github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
	"github.com/shopspring/decimal"
)

// PlaceOrder executes a market order.
// Side should be "buy" or "sell".
// Side should be "buy" or "sell".
func (a *AlpacaProvider) PlaceOrder(ticker string, qty decimal.Decimal, side string) (*alpaca.Order, error) {
	req := alpaca.PlaceOrderRequest{
		Symbol:      ticker,
		Qty:         &qty,
		Side:        alpaca.Side(side),
		Type:        alpaca.Market,
		TimeInForce: alpaca.Day,
	}
	return a.tradeClient.PlaceOrder(req)
}

// GetOrder fetches a specific order by its ID.
func (a *AlpacaProvider) GetOrder(orderID string) (*alpaca.Order, error) {
	return a.tradeClient.GetOrder(orderID)
}

// ListOrders fetches orders with a specific status (e.g., "open", "all").
func (a *AlpacaProvider) ListOrders(status string) ([]alpaca.Order, error) {
	return a.tradeClient.GetOrders(alpaca.GetOrdersRequest{
		Status: status,
		Limit:  100, // Reasonable limit
	})
}

// ListPositions fetches all open positions.
func (a *AlpacaProvider) ListPositions() ([]alpaca.Position, error) {
	return a.tradeClient.GetPositions()
}

// CancelOrder cancels a specific order by ID.
func (a *AlpacaProvider) CancelOrder(orderID string) error {
	return a.tradeClient.CancelOrder(orderID)
}
