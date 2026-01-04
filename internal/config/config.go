package config

import (
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
)

// CetLoc is a public variable (indicated by the Capitalized name) holding the Time Zone info.
// We use FixedZone here to hardcode UTC+1 for simplicity, but in production, we might load a real location.
var CetLoc = time.FixedZone("CET", 3600)

// Load initializes the configuration.
// It tries to read a .env file and checks for necessary environment variables.
// Load initializes the configuration.
// It tries to read a .env file and checks for necessary environment variables.
// Load initializes the configuration.
// It tries to read a .env file and checks for necessary environment variables.
func Load() {
	// Load .env variables into the process environment
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: No .env file found, using system environment variables")
	}

	// Define which variables are critical and confidential.
	requiredSecretVars := map[string]bool{
		"APCA_API_KEY_ID":     true,
		"APCA_API_SECRET_KEY": true,
		"APCA_API_BASE_URL":   true,
		"TELEGRAM_BOT_TOKEN":  true,
		"TELEGRAM_CHAT_ID":    true,
	}

	// 1. Check for missing required variables (in actual environment)
	var missing []string
	for key := range requiredSecretVars {
		if os.Getenv(key) == "" {
			missing = append(missing, key)
		}
	}

	if len(missing) > 0 {
		log.Fatalf("CRITICAL: Missing required environment variables: %v", missing)
	}

	// 2. Print variables defined in .env file
	envMap, err := godotenv.Read()
	if err == nil {
		log.Println("--- .env File Variables ---")
		for key, val := range envMap {
			if requiredSecretVars[key] {
				// Mask secret values: show only last 4 chars
				masked := "***"
				if len(val) > 4 {
					masked = "***" + val[len(val)-4:]
				}
				log.Printf("%s=%s", key, masked)
			} else {
				// Print non-secret variables fully
				log.Printf("%s=%s", key, val)
			}
		}
		log.Println("---------------------------")
	}
}

func splitEnv(s string) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s, ""}
}
