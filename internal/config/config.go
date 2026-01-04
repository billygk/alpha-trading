package config

import (
	"log"
	"strconv"
)

// CetLoc is a public variable (indicated by the Capitalized name) holding the Time Zone info.
// ... (rest of file) ...

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
