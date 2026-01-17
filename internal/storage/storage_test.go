package storage

import (
	"os"
	"testing"

	"github.com/shopspring/decimal"
)

func TestMigrateState(t *testing.T) {
	// 1. Setup Temp Dir to avoid touching real state file
	tmpDir := t.TempDir()
	originalWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to chdir: %v", err)
	}
	defer os.Chdir(originalWd)

	// 2. Create Legacy State (v1.1)
	legacyJSON := `{
		"version": "1.1",
		"positions": [
			{
				"ticker": "AAPL",
				"entry_price": "150.00",
				"quantity": "10",
				"status": "ACTIVE"
			}
		]
	}`

	if err := os.WriteFile(StateFile, []byte(legacyJSON), 0644); err != nil {
		t.Fatalf("Failed to write legacy state: %v", err)
	}

	// 3. Load State (Trigger Migration)
	s, err := LoadState()
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	// 4. Verify Version Upgrade
	if s.Version != "1.3" {
		t.Errorf("Expected version 1.3, got %s", s.Version)
	}

	// 5. Verify Backfill (Spec 24)
	// "Initialize HighWaterMark to the current EntryPrice"
	if len(s.Positions) != 1 {
		t.Fatalf("Expected 1 position, got %d", len(s.Positions))
	}

	pos := s.Positions[0]
	if pos.Ticker != "AAPL" {
		t.Errorf("Expected AAPL, got %s", pos.Ticker)
	}

	expectedEntry := decimal.NewFromFloat(150.00)
	if !pos.EntryPrice.Equal(expectedEntry) {
		t.Errorf("EntryPrice mismatch: got %s", pos.EntryPrice)
	}

	if !pos.HighWaterMark.Equal(expectedEntry) {
		t.Errorf("HighWaterMark mismatch: expected %s, got %s", expectedEntry, pos.HighWaterMark)
	}

	// Verify persistence (Load again)
	s2, err := LoadState()
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}
	if s2.Version != "1.3" {
		t.Errorf("Persisted version mismatch: got %s", s2.Version)
	}
}
