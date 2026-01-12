package config

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// CetLoc is a public variable (indicated by the Capitalized name) holding the Time Zone info.
// We use FixedZone here to hardcode UTC+1 for simplicity, but in production, we might load a real location.
var CetLoc = time.FixedZone("CET", 3600)

// Config holds all tweakable application parameters.
// Values are loaded from environment variables or set to sensible defaults.
type Config struct {
	LogLevel                    string  // Environment: WATCHER_LOG_LEVEL
	MaxLogSizeMB                int64   // Environment: WATCHER_MAX_LOG_SIZE_MB
	MaxLogBackups               int     // Environment: WATCHER_MAX_LOG_BACKUPS
	PollIntervalMins            int     // Environment: WATCHER_POLL_INTERVAL
	ConfirmationTTLSec          int     // Environment: CONFIRMATION_TTL_SEC
	ConfirmationMaxDeviationPct float64 // Environment: CONFIRMATION_MAX_DEVIATION_PCT
	DefaultTakeProfitPct        float64 // Environment: DEFAULT_TAKE_PROFIT_PCT
	DefaultStopLossPct          float64 // Environment: DEFAULT_STOP_LOSS_PCT
	DefaultTrailingStopPct      float64 // Environment: DEFAULT_TRAILING_STOP_PCT
	AutoStatusEnabled           bool    // Environment: AUTO_STATUS_ENABLED
	FiscalBudgetLimit           float64 // Environment: FISCAL_BUDGET_LIMIT
	GeminiAPIKey                string  // Environment: GEMINI_API_KEY
}

// Load initializes the configuration.
// It reads .env, checks required secrets, and populates the Config struct.
func Load() *Config {
	// Load .env variables into the process environment without overwriting existing env vars
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: No .env file found, using system environment variables")
	}

	// 1. Validation: Fatal check for required secrets
	requiredSecretVars := map[string]bool{
		"APCA_API_KEY_ID":     true,
		"APCA_API_SECRET_KEY": true,
		"APCA_API_BASE_URL":   true,
		"TELEGRAM_BOT_TOKEN":  true,
		"TELEGRAM_CHAT_ID":    true,
	}

	var missing []string
	for key := range requiredSecretVars {
		if os.Getenv(key) == "" {
			missing = append(missing, key)
		}
	}

	if len(missing) > 0 {
		log.Fatalf("CRITICAL: Missing required environment variables: %v", missing)
	}

	// 2. Print variables explicitly defined in the local .env file (for debugging)
	envMap, err := godotenv.Read()
	if err == nil {
		log.Println("--- .env File Variables ---")
		for key, val := range envMap {
			if requiredSecretVars[key] {
				// Mask secret values (last 4 chars visible)
				masked := "***"
				if len(val) > 4 {
					masked = "***" + val[len(val)-4:]
				}
				log.Printf("%s=%s", key, masked)
			} else {
				log.Printf("%s=%s", key, val)
			}
		}
		log.Println("---------------------------")
	}

	// 3. Populate Config struct with Defaults + Env Overrides
	// Load Fiscal Budget (Spec 63)
	fiscalLimit, err := strconv.ParseFloat(os.Getenv("FISCAL_BUDGET_LIMIT"), 64)
	if err != nil {
		fiscalLimit = 300.0 // Hard default per spec
	}

	cfg := &Config{
		LogLevel:                    getEnv("WATCHER_LOG_LEVEL", "INFO"),
		MaxLogSizeMB:                getEnvAsInt64("WATCHER_MAX_LOG_SIZE_MB", 5),
		MaxLogBackups:               getEnvAsInt("WATCHER_MAX_LOG_BACKUPS", 3),
		PollIntervalMins:            getEnvAsInt("WATCHER_POLL_INTERVAL", 60),
		ConfirmationTTLSec:          getEnvAsInt("CONFIRMATION_TTL_SEC", 300),                 // Default 5 mins
		ConfirmationMaxDeviationPct: getEnvAsFloat64("CONFIRMATION_MAX_DEVIATION_PCT", 0.005), // Default 0.5%
		DefaultTakeProfitPct:        getEnvAsFloat64("DEFAULT_TAKE_PROFIT_PCT", 15.0),         // Default 15.0%
		DefaultStopLossPct:          getEnvAsFloat64("DEFAULT_STOP_LOSS_PCT", 5.0),            // Default 5.0%
		DefaultTrailingStopPct:      getEnvAsFloat64("DEFAULT_TRAILING_STOP_PCT", 3.0),        // Default 3.0%
		AutoStatusEnabled:           getEnvAsBool("AUTO_STATUS_ENABLED", false),               // Default false
		FiscalBudgetLimit:           fiscalLimit,
		GeminiAPIKey:                os.Getenv("GEMINI_API_KEY"),
	}

	log.Printf("Configuration Loaded: LogLevel=%s, MaxSize=%dMB, Backups=%d, PollInterval=%dm",
		cfg.LogLevel, cfg.MaxLogSizeMB, cfg.MaxLogBackups, cfg.PollIntervalMins)

	return cfg
}

// Helper to get string env with default
func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

// Helper to get int env with default
func getEnvAsInt(key string, fallback int) int {
	valueStr, exists := os.LookupEnv(key)
	if !exists {
		return fallback
	}
	return parseInt(valueStr, fallback)
}

func getEnvAsInt64(key string, fallback int64) int64 {
	valueStr, exists := os.LookupEnv(key)
	if !exists {
		return fallback
	}
	return parseInt64(valueStr, fallback)
}

func parseInt(s string, fallback int) int {
	val, err := strconv.Atoi(s)
	if err != nil {
		log.Printf("Warning: Invalid int for config %s, using default %d", s, fallback)
		return fallback
	}
	return val
}

func parseInt64(s string, fallback int64) int64 {
	val, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		log.Printf("Warning: Invalid int64 for config %s, using default %d", s, fallback)
		return fallback
	}
	return val
}

func splitEnv(s string) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s, ""}
}

func getEnvAsBool(key string, fallback bool) bool {
	valStr := os.Getenv(key)
	if valStr == "" {
		return fallback
	}
	val, err := strconv.ParseBool(valStr)
	if err != nil {
		log.Printf("Warning: Invalid bool for config %s, using default %v", key, fallback)
		return fallback
	}
	return val
}
