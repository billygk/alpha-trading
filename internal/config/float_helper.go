package config

import (
	"log"
	"os"
	"strconv"
)

// Helper to get float64 env with default
func getEnvAsFloat64(key string, fallback float64) float64 {
	valueStr, exists := os.LookupEnv(key)
	if !exists {
		return fallback
	}
	val, err := strconv.ParseFloat(valueStr, 64)
	if err != nil {
		log.Printf("Warning: Invalid float64 for config %s, using default %f", valueStr, fallback)
		return fallback
	}
	return val
}
