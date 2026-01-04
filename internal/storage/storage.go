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

// SaveState writes the current state to disk using an atomic write pattern.
// 1. Write to a temporary file.
// 2. Sync to ensure data is on disk.
// 3. Rename temporary file to destination (atomic operation).
func SaveState(s models.PortfolioState) {
	// MarshalIndent makes the JSON human-readable (pretty-printed) with 2-space indentation.
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		log.Printf("ERROR: Failed to marshal state: %v", err)
		return
	}

	// Create a temporary file in the same directory to ensure atomic rename works across filesystems
	// "portfolio_state.json.tmp"
	tmpFile := StateFile + ".tmp"
	f, err := os.Create(tmpFile)
	if err != nil {
		log.Printf("ERROR: Failed to create temp state file: %v", err)
		return
	}

	// Ensure we close the file, even if writing fails
	defer f.Close()

	// Write the JSON data
	if _, err := f.Write(b); err != nil {
		log.Printf("ERROR: Failed to write to temp state file: %v", err)
		return
	}

	// Force sync to disk to prevent data loss on power failure before rename
	if err := f.Sync(); err != nil {
		log.Printf("ERROR: Failed to sync temp state file: %v", err)
		return
	}

	// Close explicitly before renaming (essential on Windows)
	f.Close()

	// Atomic Rename
	if err := os.Rename(tmpFile, StateFile); err != nil {
		log.Printf("ERROR: Failed to replace state file (atomic rename): %v", err)
	}
}
