package watcher

import (
	"alpha_trading/internal/config"
	"alpha_trading/internal/models"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// TestHandleCommand_Update_Fractional verifies that we can update SL/TP for fractional positions
// internally, even though the broker doesn't support fractional limit orders.
func TestHandleCommand_Update_Fractional(t *testing.T) {
	// 1. Setup
	cfg := &config.Config{
		DefaultStopLossPct:     5.0,
		DefaultTakeProfitPct:   15.0,
		DefaultTrailingStopPct: 3.0,
	}

	// Mock Provider with Price for Validation
	mockProvider := &MockProvider{
		prices: map[string]decimal.Decimal{
			"NVDA": decimal.NewFromFloat(500.00),
		},
	}

	w := &Watcher{
		config:   cfg,
		provider: mockProvider,
		state: models.PortfolioState{
			Positions: []models.Position{
				{
					Ticker:       "NVDA",
					Status:       "ACTIVE",
					Quantity:     decimal.NewFromFloat(0.5), // Fractional
					IsFractional: true,
					EntryPrice:   decimal.NewFromFloat(450.00),
					StopLoss:     decimal.NewFromFloat(400.00),
					TakeProfit:   decimal.NewFromFloat(600.00),
					OpenedAt:     time.Now(),
				},
			},
		},
	}

	// 2. Execute Command: /update NVDA <SL> <TP>
	// Current Price: 500
	// New SL: 480 (< 500) -> Safe
	// New TP: 550 (> 500) -> Safe
	// TP > SL -> Safe
	// New SL (480) >= Current SL (400) -> Safe (Monotonicity)
	cmd := "/update NVDA 480 550"
	resp := w.HandleCommand(cmd)

	// 3. Verify Response
	// Should Success, NOT "Fractional positions ... do not support..."
	expected := "🛡️ RISK UPDATED: NVDA | New SL: $480.00 | New TP: $550.00"
	if resp != expected {
		t.Errorf("Expected success message '%s', got: '%s'", expected, resp)
	}

	// 4. Verify Internal State
	w.mu.RLock()
	pos := w.state.Positions[0]
	w.mu.RUnlock()

	if !pos.StopLoss.Equal(decimal.NewFromFloat(480)) {
		t.Errorf("Expected SL to be updated t 480, got %s", pos.StopLoss)
	}
	if !pos.TakeProfit.Equal(decimal.NewFromFloat(550)) {
		t.Errorf("Expected TP to be updated to 550, got %s", pos.TakeProfit)
	}
}
