package notifications

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
)

// Notify sends a message to a Telegram chat.
// It is a standalone function because it doesn't need to stash any state.
func Notify(text string) {
	// Retrieve credentials from environment.
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID := os.Getenv("TELEGRAM_CHAT_ID")

	// If missing, we log a warning but don't crash.
	if token == "" || chatID == "" {
		log.Println("Warning: Telegram credentials missing, skipping notification")
		return // Exit function early
	}

	// Construct the URL for the Telegram API.
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)

	// Create the payload map.
	// map[string]string means keys are strings and values are strings.
	payloadMap := map[string]string{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "Markdown", // Allows us to use bold/italic in messages
	}

	// Marshal converts the map to a JSON byte slice.
	payloadBytes, _ := json.Marshal(payloadMap)

	// Send the HTTP POST request.
	// We wrap the byte slice in a new buffer to satisfy the Reader interface required by http.Post.
	_, err := http.Post(url, "application/json", bytes.NewBuffer(payloadBytes))
	if err != nil {
		log.Printf("Telegram Alert Failed: %v", err)
	}
}
