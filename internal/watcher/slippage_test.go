package watcher

import (
	"testing"
	"time"

	"alpha_trading/internal/ai"
	"alpha_trading/internal/config"
	"alpha_trading/internal/models"

	"github.com/shopspring/decimal"
)

// SpyMarketProvider tracks calls for testing
type SpyMarketProvider struct {
	quotes           map[string]models.Quote
	placeOrderCalled bool
}

// Implement MarketProvider interface
func (m *SpyMarketProvider) GetPrice(ticker string) (decimal.Decimal, error) {
	return decimal.NewFromFloat(100.0), nil
}
func (m *SpyMarketProvider) GetQuote(ticker string) (*models.Quote, error) {
	if q, ok := m.quotes[ticker]; ok {
		return &q, nil
	}
	return &models.Quote{BidPrice: decimal.NewFromFloat(100), AskPrice: decimal.NewFromFloat(100.1)}, nil
}
func (m *SpyMarketProvider) PlaceOrder(ticker string, qty decimal.Decimal, side string, sl, tp decimal.Decimal) (*models.Order, error) {
	m.placeOrderCalled = true
	return &models.Order{ID: "spy_order_id"}, nil
}
func (m *SpyMarketProvider) GetOrder(orderID string) (*models.Order, error) {
	// Return filled to avoid wait loop
	return &models.Order{ID: orderID, Status: "filled", FilledAvgPrice: decimal.NewFromFloat(100.0)}, nil
}

// Stubs
func (m *SpyMarketProvider) GetBuyingPower() (decimal.Decimal, error) {
	return decimal.NewFromFloat(10000), nil
}
func (m *SpyMarketProvider) ListOrders(status string) ([]models.Order, error)         { return nil, nil }
func (m *SpyMarketProvider) GetEquity() (decimal.Decimal, error)                      { return decimal.Zero, nil }
func (m *SpyMarketProvider) GetClock() (*models.Clock, error)                         { return &models.Clock{IsOpen: true}, nil }
func (m *SpyMarketProvider) SearchAssets(query string) ([]models.Asset, error)        { return nil, nil }
func (m *SpyMarketProvider) UpdatePositionRisk(ticker string, sl, tp decimal.Decimal) error { return nil }
func (m *SpyMarketProvider) CancelOrder(orderID string) error                         { return nil }
func (m *SpyMarketProvider) ListPositions() ([]models.BrokerPosition, error)          { return nil, nil }
func (m *SpyMarketProvider) GetBars(ticker string, limit int) ([]models.Bar, error)   { return nil, nil }
func (m *SpyMarketProvider) GetPortfolioHistory(period, timeframe string) (*models.PortfolioHistory, error) {
	return nil, nil
}
func (m *SpyMarketProvider) GetAccount() (*models.Account, error) { return nil, nil }

func TestAutonomousSlippageGate(t *testing.T) {
	// 1. Setup High Spread
	// Bid 100, Ask 101. Spread = 1/100 = 1% > 0.5% limit.
	highSpreadQuote := models.Quote{
		Symbol:    "VOLA",
		BidPrice:  decimal.NewFromFloat(100.00),
		AskPrice:  decimal.NewFromFloat(101.00),
		Timestamp: time.Now(),
	}

	provider := &SpyMarketProvider{
		quotes: map[string]models.Quote{
			"VOLA": highSpreadQuote,
		},
	}

	w := &Watcher{
		config: &config.Config{
			DefaultStopLossPct:   5.0,
			DefaultTakeProfitPct: 15.0,
		},
		provider: provider,
		state: models.PortfolioState{
			AutonomousEnabled: true,
		},
	}

	// 2. Prepare AI Analysis
	analysis := &ai.AIAnalysis{
		Recommendation:  "BUY",
		ActionCommand:   "/buy VOLA 1",
		ConfidenceScore: 0.90, // High conviction
		RiskAssessment:  "MEDIUM",
		Analysis:        "Testing Slippage",
	}

	snapshot := &ai.PortfolioSnapshot{
		Capital: decimal.NewFromFloat(1000.0),
	}

	// 3. Execute
	w.handleAIResult(analysis, snapshot, false)

	// 4. Verify PlaceOrder NOT called
	if provider.placeOrderCalled {
		t.Error("Slippage Gate Failed: PlaceOrder was called despite 1% spread")
	}

	// 5. Setup Low Spread (Pass Case)
	// Bid 100, Ask 100.1. Spread = 0.1% < 0.5%.
	lowSpreadQuote := models.Quote{
		Symbol:    "SAFE",
		BidPrice:  decimal.NewFromFloat(100.00),
		AskPrice:  decimal.NewFromFloat(100.10),
		Timestamp: time.Now(),
	}
	provider.quotes["SAFE"] = lowSpreadQuote
	provider.placeOrderCalled = false // Reset

	analysis.ActionCommand = "/buy SAFE 1"

	// Execute
	w.handleAIResult(analysis, snapshot, false)

	// Verify PlaceOrder CALLED
	if !provider.placeOrderCalled {
		t.Error("Slippage Gate Error: PlaceOrder NOT called for low spread")
	}
}
