package storage

import (
	"encoding/json"
	"io"
	"log"
	"os"

	"alpha_trading/internal/models"
)

// StateFile defines where we save our data on disk.
const StateFile = "portfolio_state.json"

// LoadState reads the portfolio state from disk.
// It returns the PortfolioState struct and an error if one occurred.
func LoadState() (models.PortfolioState, error) {
	var s models.PortfolioState

	// os.Stat checks if a file exists.
	if _, err := os.Stat(StateFile); os.IsNotExist(err) {
		log.Println("State file missing, generating template...")
		// Create a default initial state
		s = models.PortfolioState{Version: "1.1", Positions: []models.Position{}}
		// Save it immediately so next time we find it
		SaveState(s)
		return s, nil
	}

	// Open the file for reading.
	f, err := os.Open(StateFile)
	if err != nil {
		return s, err
	}
	// defer ensures f.Close() is called when this function exits,
	// regardless of whether it returns normally or errors out.
	defer f.Close()

	// Read all bytes from the file.
	b, err := io.ReadAll(f)
	if err != nil {
		return s, err
	}

	// Unmarshal converts JSON bytes into our Go struct.
	// We pass &s (pointer to s) so Unmarshal can modify s directly.
	return s, json.Unmarshal(b, &s)
}

// SaveState writes the current state to disk.
func SaveState(s models.PortfolioState) {
	// MarshalIndent makes the JSON human-readable (pretty-printed) with 2-space indentation.
	b, _ := json.MarshalIndent(s, "", "  ")

	// WriteFile writes the bytes to the file, creating it if needed.
	// 0644 is the file permission (Read/Write for owner, Read-only for others).
	_ = os.WriteFile(StateFile, b, 0644)
}
