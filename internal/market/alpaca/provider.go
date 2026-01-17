package alpaca

import (
	"fmt"
	"strings"
	"time"

	"alpha_trading/internal/market"
	"alpha_trading/internal/models"

	"github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
	"github.com/shopspring/decimal"
)

// Provider implements the generic MarketProvider interface for Alpaca.
type Provider struct {
	mdClient    *marketdata.Client
	tradeClient *alpaca.Client
}

// Ensure Provider implements the interface
var _ market.MarketProvider = (*Provider)(nil)

// NewProvider returns a new Alpaca provider.
func NewProvider() *Provider {
	return &Provider{
		mdClient:    marketdata.NewClient(marketdata.ClientOpts{}),
		tradeClient: alpaca.NewClient(alpaca.ClientOpts{}),
	}
}

// --- Market Data ---

func (p *Provider) GetPrice(ticker string) (decimal.Decimal, error) {
	trade, err := p.mdClient.GetLatestTrade(ticker, marketdata.GetLatestTradeRequest{})
	if err != nil {
		return decimal.Zero, err
	}
	if trade == nil {
		return decimal.Zero, nil
	}
	return decimal.NewFromFloat(trade.Price), nil
}

func (p *Provider) GetQuote(ticker string) (*models.Quote, error) {
	q, err := p.mdClient.GetLatestQuote(ticker, marketdata.GetLatestQuoteRequest{})
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, fmt.Errorf("no quote found for %s", ticker)
	}
	return &models.Quote{
		Symbol:    ticker,
		BidPrice:  decimal.NewFromFloat(q.BidPrice),
		AskPrice:  decimal.NewFromFloat(q.AskPrice),
		Timestamp: q.Timestamp,
	}, nil
}

func (p *Provider) GetEquity() (decimal.Decimal, error) {
	acct, err := p.tradeClient.GetAccount()
	if err != nil {
		return decimal.Zero, err
	}
	return acct.Equity, nil
}

func (p *Provider) GetClock() (*models.Clock, error) {
	c, err := p.tradeClient.GetClock()
	if err != nil {
		return nil, err
	}
	return &models.Clock{
		Timestamp: c.Timestamp,
		IsOpen:    c.IsOpen,
		NextOpen:  c.NextOpen,
		NextClose: c.NextClose,
	}, nil
}

func (p *Provider) SearchAssets(query string) ([]models.Asset, error) {
	status := "active"
	class := "us_equity"
	alpacaAssets, err := p.tradeClient.GetAssets(alpaca.GetAssetsRequest{
		Status:     status,
		AssetClass: class,
	})
	if err != nil {
		return nil, err
	}

	var results []models.Asset
	queryLower := strings.ToLower(query)

	for _, a := range alpacaAssets {
		if strings.Contains(strings.ToLower(a.Symbol), queryLower) ||
			strings.Contains(strings.ToLower(a.Name), queryLower) {
			results = append(results, models.Asset{
				ID:       a.ID,
				Symbol:   a.Symbol,
				Name:     a.Name,
				Class:    string(a.Class),
				Exchange: a.Exchange,
				Status:   string(a.Status),
				Tradable: a.Tradable,
			})
			if len(results) >= 5 {
				break
			}
		}
	}
	return results, nil
}

// --- Execution ---

func (p *Provider) PlaceOrder(ticker string, qty decimal.Decimal, side string, slPrice decimal.Decimal, tpPrice decimal.Decimal) (*models.Order, error) {
	req := alpaca.PlaceOrderRequest{
		Symbol:      ticker,
		Qty:         &qty,
		Side:        alpaca.Side(side),
		Type:        alpaca.Market, // We only support Market for now in this wrapper
		TimeInForce: alpaca.Day,
	}

	if side == "buy" && (!slPrice.IsZero() || !tpPrice.IsZero()) {
		req.OrderClass = alpaca.Bracket
		if !tpPrice.IsZero() {
			req.TakeProfit = &alpaca.TakeProfit{
				LimitPrice: &tpPrice,
			}
		}
		if !slPrice.IsZero() {
			req.StopLoss = &alpaca.StopLoss{
				StopPrice: &slPrice,
			}
		}
	}

	o, err := p.tradeClient.PlaceOrder(req)
	if err != nil {
		return nil, err
	}
	return mapOrder(o), nil
}

func (p *Provider) GetOrder(orderID string) (*models.Order, error) {
	o, err := p.tradeClient.GetOrder(orderID)
	if err != nil {
		return nil, err
	}
	return mapOrder(o), nil
}

func (p *Provider) ListOrders(status string) ([]models.Order, error) {
	orders, err := p.tradeClient.GetOrders(alpaca.GetOrdersRequest{
		Status: status,
		Limit:  100,
	})
	if err != nil {
		return nil, err
	}

	var result []models.Order
	for _, o := range orders {
		result = append(result, *mapOrder(&o)) // mapOrder returns pointer, we dereference for slice? Or slice should be pointers?
		// Interface says []models.Order (structs).
	}
	return result, nil
}

func (p *Provider) CancelOrder(orderID string) error {
	return p.tradeClient.CancelOrder(orderID)
}

// --- Helpers ---

func (p *Provider) GetBuyingPower() (decimal.Decimal, error) {
	acct, err := p.tradeClient.GetAccount()
	if err != nil {
		return decimal.Zero, err
	}
	return acct.BuyingPower, nil
}

func (p *Provider) ListPositions() ([]models.BrokerPosition, error) {
	alpacaPositions, err := p.tradeClient.GetPositions()
	if err != nil {
		return nil, err
	}

	var result []models.BrokerPosition
	for _, x := range alpacaPositions {
		// Helper to safely dereference decimal pointers from Alpaca SDK
		current := decimal.Zero
		if x.CurrentPrice != nil {
			current = *x.CurrentPrice
		}
		change := decimal.Zero
		if x.ChangeToday != nil {
			change = *x.ChangeToday
		}
		marketValue := decimal.Zero
		if x.MarketValue != nil {
			marketValue = *x.MarketValue
		}
		costBasis := x.CostBasis
		unrealizedPL := decimal.Zero
		if x.UnrealizedPL != nil {
			unrealizedPL = *x.UnrealizedPL
		}
		unrealizedPLPC := decimal.Zero
		if x.UnrealizedPLPC != nil {
			unrealizedPLPC = *x.UnrealizedPLPC
		}

		result = append(result, models.BrokerPosition{
			Symbol:         x.Symbol,
			Qty:            x.Qty,           // Alpaca SDK v3 is decimal.Decimal (value)
			AvgEntryPrice:  x.AvgEntryPrice, // decimal.Decimal (value)
			CurrentPrice:   current,
			MarketValue:    marketValue,
			CostBasis:      costBasis,
			UnrealizedPL:   unrealizedPL,
			UnrealizedPLPC: unrealizedPLPC,
			ChangeToday:    change,
		})
	}
	return result, nil
}

func (p *Provider) GetBars(ticker string, limit int) ([]models.Bar, error) {
	start := time.Now().AddDate(0, 0, -5) // 5 days back
	bars, err := p.mdClient.GetBars(ticker, marketdata.GetBarsRequest{
		TimeFrame: marketdata.OneDay,
		Start:     start,
	})
	if err != nil {
		return nil, err
	}

	if len(bars) > limit {
		bars = bars[len(bars)-limit:]
	}

	var result []models.Bar
	for _, b := range bars {
		result = append(result, models.Bar{
			Time:   b.Timestamp,
			Open:   decimal.NewFromFloat(b.Open),
			High:   decimal.NewFromFloat(b.High),
			Low:    decimal.NewFromFloat(b.Low),
			Close:  decimal.NewFromFloat(b.Close),
			Volume: int64(b.Volume),
		})
	}
	return result, nil
}

func (p *Provider) GetPortfolioHistory(period string, timeframe string) (*models.PortfolioHistory, error) {
	h, err := p.tradeClient.GetPortfolioHistory(alpaca.GetPortfolioHistoryRequest{
		Period:    period,
		TimeFrame: alpaca.TimeFrame(timeframe),
	})
	if err != nil {
		return nil, err
	}

	// Map
	timestamps := h.Timestamp
	equity := h.Equity
	pl := h.ProfitLoss
	plpct := h.ProfitLossPct

	return &models.PortfolioHistory{
		Timestamps:    timestamps,
		Equity:        equity,
		ProfitLoss:    pl,
		ProfitLossPct: plpct,
	}, nil
}

func (p *Provider) GetAccount() (*models.Account, error) {
	a, err := p.tradeClient.GetAccount()
	if err != nil {
		return nil, err
	}
	return &models.Account{
		ID:               a.ID,
		Currency:         a.Currency,
		Equity:           a.Equity,
		BuyingPower:      a.BuyingPower,
		Cash:             a.Cash,
		PortfolioValue:   a.PortfolioValue,
		DaytradeCount:    int(a.DaytradeCount),
		IsDayTrader:      a.DaytradeCount > 3, // Simplification or check generic logic
		IsAccountBlocked: a.AccountBlocked,
	}, nil
}

// Helpers

func mapOrder(o *alpaca.Order) *models.Order {
	if o == nil {
		return nil
	}

	// mapOrder helper
	qty := o.Qty             // Value
	filledQty := o.FilledQty // Value

	var filledAvgPrice decimal.Decimal
	if o.FilledAvgPrice != nil {
		filledAvgPrice = *o.FilledAvgPrice
	}

	status := o.Status

	res := &models.Order{
		ID:             o.ID,
		ClientOrderID:  o.ClientOrderID,
		Symbol:         o.Symbol,
		Qty:            *qty,
		FilledQty:      filledQty,
		Type:           string(o.Type),
		Side:           string(o.Side),
		Status:         status,
		FilledAvgPrice: filledAvgPrice,
		CreatedAt:      o.CreatedAt,
		// FilledAt handled below
		FailReason: "",
	}

	if o.FilledAt != nil {
		res.FilledAt = o.FilledAt
	}

	return res
}
