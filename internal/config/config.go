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
func Load() {
	// godotenv.Load() reads the .env file and sets variables in the process environment.
	// We capture the error in 'err'. In Go, errors are values, not exceptions.
	// if err != nil checks if an error occurred.
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: No .env file found, using system environment variables")
	}

	// Define which variables are critical for the application to run.
	requiredEnvVars := []string{"APCA_API_KEY_ID", "APCA_API_SECRET_KEY", "APCA_API_BASE_URL"}
	missingVars := false

	// Underscore (_) is the "blank identifier". We use it here to ignore the index of the slice loop.
	// we only care about 'envVar', which is the value.
	for _, envVar := range requiredEnvVars {
		// os.Getenv returns the value of the environment variable, or empty string if not set.
		if os.Getenv(envVar) == "" {
			log.Printf("CRITICAL: Missing environment variable: %s", envVar)
			missingVars = true
		} else {
			// Security best practice: Never log full secrets!
			// Here we implement simple masking to show the variable is present without revealing it.
			val := os.Getenv(envVar)
			masked := val
			if len(val) > 4 {
				masked = "..." + val[len(val)-4:] // Slice syntax: take characters from length-4 to the end
			} else {
				masked = "***"
			}
			log.Printf("Env Loaded: %s=%s", envVar, masked)
		}
	}

	// We don't panic here to allow for testing or partial runs, but warn the user.
	if missingVars {
		log.Println("Warning: proceeding but Alpaca client may fail")
	}
}
