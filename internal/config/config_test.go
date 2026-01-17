package config

import (
	"os"
	"testing"
)

func TestLoadConfig_Defaults(t *testing.T) {
	// 1. Setup Required Envs (to bypass validation)
	required := map[string]string{
		"APCA_API_KEY_ID":     "test_key",
		"APCA_API_SECRET_KEY": "test_secret",
		"APCA_API_BASE_URL":   "https://paper-api.alpaca.markets",
		"TELEGRAM_BOT_TOKEN":  "test_token",
		"TELEGRAM_CHAT_ID":    "123456",
		"GEMINI_API_KEY":      "test_gemini",
		// FISCAL_BUDGET_LIMIT is optional, default 300
	}

	for k, v := range required {
		os.Setenv(k, v)
		defer os.Unsetenv(k) // Clean up
	}

	// 2. Ensure Optional Envs are Unset
	optionals := []string{
		"WATCHER_LOG_LEVEL",
		"WATCHER_POLL_INTERVAL",
		"CONFIRMATION_TTL_SEC",
		"DEFAULT_TAKE_PROFIT_PCT",
	}

	for _, k := range optionals {
		os.Unsetenv(k)
	}

	// 3. Load Config
	cfg := Load()

	// 4. Verify Defaults
	if cfg.LogLevel != "INFO" {
		t.Errorf("Expected LogLevel 'INFO', got '%s'", cfg.LogLevel)
	}

	if cfg.PollIntervalMins != 60 {
		t.Errorf("Expected PollIntervalMins 60, got %d", cfg.PollIntervalMins)
	}

	if cfg.ConfirmationTTLSec != 300 {
		t.Errorf("Expected ConfirmationTTLSec 300, got %d", cfg.ConfirmationTTLSec)
	}

	if cfg.DefaultTakeProfitPct != 15.0 {
		t.Errorf("Expected DefaultTakeProfitPct 15.0, got %f", cfg.DefaultTakeProfitPct)
	}

	if cfg.FiscalBudgetLimit != 300.0 {
		t.Errorf("Expected FiscalBudgetLimit 300.0, got %f", cfg.FiscalBudgetLimit)
	}
}
