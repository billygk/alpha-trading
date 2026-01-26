package watcher

import (
	"alpha_trading/internal/config"
	"alpha_trading/internal/models"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// TestSyncWithBroker_SLRetention_CaseMismatch reproduces the issue where
// a mismatch in ticker string (case/whitespace) causes the sync logic
// to treat an existing position as new, resetting its Stop Loss.
func TestSyncWithBroker_SLRetention_CaseMismatch(t *testing.T) {
	// 1. Setup
	cfg := &config.Config{
		DefaultStopLossPct:     5.0,
		DefaultTakeProfitPct:   15.0,
		DefaultTrailingStopPct: 3.0,
	}

	// 2. Pre-seed Local State with a custom SL that MUST be preserved
	// Ticker is "AAPL" (Uppercase)
	customSL := decimal.NewFromFloat(200.00)
	initialState := models.PortfolioState{
		Positions: []models.Position{
			{
				Ticker:        "AAPL",
				Status:        "ACTIVE",
				Quantity:      decimal.NewFromInt(10),
				EntryPrice:    decimal.NewFromFloat(150.00),
				StopLoss:      customSL, // High SL (User set this)
				TakeProfit:    decimal.NewFromFloat(300.00),
				HighWaterMark: decimal.NewFromFloat(160.00),
				OpenedAt:      time.Now(),
			},
		},
	}

	// 3. Mock Broker with "Dirty" Ticker
	// Broker returns "aapl " (Lowercase + Space) for the same position
	mockPositions := []models.BrokerPosition{
		{
			Symbol:        "aapl ", // dirty ticker mismatch
			Qty:           decimal.NewFromInt(10),
			AvgEntryPrice: decimal.NewFromFloat(150.00),
			CurrentPrice:  decimal.NewFromFloat(155.00),
		},
	}

	// Reuse MockProvider from commands_test.go (same package)
	mockProvider := &MockProvider{
		positions: mockPositions,
		prices: map[string]decimal.Decimal{
			"AAPL": decimal.NewFromFloat(155.00),
		},
	}

	w := &Watcher{
		config:   cfg,
		state:    initialState,
		provider: mockProvider,
	}

	// 4. Execute Sync
	// This calls SyncWithBroker internally (or we call it directly)
	_, err := w.SyncWithBroker()
	if err != nil {
		t.Fatalf("SyncWithBroker failed: %v", err)
	}

	// 5. Verify SL Retention
	if len(w.state.Positions) == 0 {
		t.Fatal("Position lost during sync")
	}

	pos := w.state.Positions[0]
	// Broker ticker was used (likely), but we want to check if SL was preserved.
	// If bug is present, SL will be reset to Default (150 * 0.95 = 142.50)
	// If fixed, SL matches customSL (200.00)

	if !pos.StopLoss.Equal(customSL) {
		t.Errorf("SL Monotonicity Fail: SL was reset to %s. Expected %s (Custom User Value).", pos.StopLoss, customSL)
	}
}
