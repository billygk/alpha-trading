package watcher

import (
	"fmt"
	"strings"
	"testing"

	"alpha_trading/internal/config"
	"alpha_trading/internal/models"

	"github.com/shopspring/decimal"
)

// MockProvider implements MarketProvider for testing
type MockProvider struct {
	prices       map[string]decimal.Decimal
	buyingPower  decimal.Decimal
	orders       []models.Order
	positions    []models.BrokerPosition
	quotes       map[string]models.Quote
}

func (m *MockProvider) GetPrice(ticker string) (decimal.Decimal, error) {
	if p, ok := m.prices[ticker]; ok {
		return p, nil
	}
	return decimal.Zero, fmt.Errorf("price not found for %s", ticker)
}

func (m *MockProvider) GetBuyingPower() (decimal.Decimal, error) {
	return m.buyingPower, nil
}

func (m *MockProvider) ListOrders(status string) ([]models.Order, error) {
	return m.orders, nil
}

func (m *MockProvider) PlaceOrder(ticker string, qty decimal.Decimal, side string, sl, tp decimal.Decimal) (*models.Order, error) {
	return &models.Order{ID: "mock_order_id"}, nil
}

func (m *MockProvider) GetQuote(ticker string) (*models.Quote, error) {
	if q, ok := m.quotes[ticker]; ok {
		return &q, nil
	}
	return nil, fmt.Errorf("quote not found")
}

// Stubs for other interface methods not used in /buy parsing test (or return empty)
func (m *MockProvider) GetEquity() (decimal.Decimal, error) { return decimal.Zero, nil }
func (m *MockProvider) GetClock() (*models.Clock, error)   { return &models.Clock{IsOpen: true}, nil }
func (m *MockProvider) SearchAssets(query string) ([]models.Asset, error) {
	return nil, nil
}
func (m *MockProvider) UpdatePositionRisk(ticker string, sl, tp decimal.Decimal) error { return nil }
func (m *MockProvider) GetOrder(orderID string) (*models.Order, error)                { return nil, nil }
func (m *MockProvider) CancelOrder(orderID string) error                              { return nil }
func (m *MockProvider) ListPositions() ([]models.BrokerPosition, error)               { return m.positions, nil }
func (m *MockProvider) GetBars(ticker string, limit int) ([]models.Bar, error)        { return nil, nil }
func (m *MockProvider) GetPortfolioHistory(period, timeframe string) (*models.PortfolioHistory, error) {
	return nil, nil
}
func (m *MockProvider) GetAccount() (*models.Account, error) { return nil, nil }

func TestHandleCommand_Buy(t *testing.T) {
	// 1. Setup
	cfg := &config.Config{
		DefaultStopLossPct:     5.0,
		DefaultTakeProfitPct:   15.0,
		DefaultTrailingStopPct: 3.0,
		ConfirmationTTLSec:     300,
	}

	mockProvider := &MockProvider{
		prices: map[string]decimal.Decimal{
			"AAPL": decimal.NewFromFloat(150.00),
		},
		buyingPower: decimal.NewFromFloat(1000.00),
		orders:      []models.Order{}, // No pending orders
	}

	w := &Watcher{
		config:           cfg,
		provider:         mockProvider,
		pendingProposals: make(map[string]PendingProposal),
	}

	// 2. Execute Command: /buy AAPL 2 (Implicit SL/TP)
	// Price 150. Qty 2. Cost 300.
	// Default SL: 150 * (1 - 0.05) = 142.50
	// Default TP: 150 * (1 + 0.15) = 172.50
	cmd := "/buy AAPL 2"
	_ = w.HandleCommand(cmd) // Returns empty string (interactive msg sent), but we check state.

	// 3. Verify Pending Proposal
	w.mu.RLock() // Although test is single threaded, good practice
	prop, exists := w.pendingProposals["AAPL"]
	w.mu.RUnlock()

	if !exists {
		t.Fatal("Expected pending proposal for AAPL, found none")
	}

	if !prop.Qty.Equal(decimal.NewFromInt(2)) {
		t.Errorf("Expected Qty 2, got %s", prop.Qty)
	}

	expectedSL := decimal.NewFromFloat(142.50)
	if !prop.StopLoss.Equal(expectedSL) {
		t.Errorf("Expected SL %s, got %s", expectedSL, prop.StopLoss)
	}

	expectedTP := decimal.NewFromFloat(172.50)
	if !prop.TakeProfit.Equal(expectedTP) {
		t.Errorf("Expected TP %s, got %s", expectedTP, prop.TakeProfit)
	}

	// 4. Test explicit SL/TP
	// /buy AAPL 1 140 180
	cmd = "/buy AAPL 1 140 180"
	w.HandleCommand(cmd)

	w.mu.RLock()
	prop, exists = w.pendingProposals["AAPL"]
	w.mu.RUnlock()

	if !prop.StopLoss.Equal(decimal.NewFromFloat(140)) {
		t.Errorf("Expected explicit SL 140, got %s", prop.StopLoss)
	}
	if !prop.TakeProfit.Equal(decimal.NewFromFloat(180)) {
		t.Errorf("Expected explicit TP 180, got %s", prop.TakeProfit)
	}
}

func TestHandleCommand_Buy_DuplicateOrder(t *testing.T) {
	// Setup
	mockProvider := &MockProvider{
		prices: map[string]decimal.Decimal{
			"AAPL": decimal.NewFromFloat(150.00),
		},
		buyingPower: decimal.NewFromFloat(1000.00),
		orders: []models.Order{
			{Symbol: "AAPL", Status: "new", Side: "buy"},
		},
	}
	w := &Watcher{
		config:   &config.Config{},
		provider: mockProvider,
	}

	cmd := "/buy AAPL 1"
	resp := w.HandleCommand(cmd)

	if !strings.Contains(resp, "Order already pending") {
		t.Errorf("Expected duplicate order warning, got: %s", resp)
	}
}
