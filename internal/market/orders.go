package market

import (
	"github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
	"github.com/shopspring/decimal"
)

// PlaceOrder executes a market order.
// Side should be "buy" or "sell".
func (a *AlpacaProvider) PlaceOrder(ticker string, qty float64, side string) error {
	qtyDec := decimal.NewFromFloat(qty)
	req := alpaca.PlaceOrderRequest{
		Symbol:      ticker,
		Qty:         &qtyDec,
		Side:        alpaca.Side(side),
		Type:        alpaca.Market,
		TimeInForce: alpaca.GTC,
	}
	_, err := a.tradeClient.PlaceOrder(req)
	return err
}
