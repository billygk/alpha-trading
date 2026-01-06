package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
)

// Notify sends a message to the configured Telegram chat.
func Notify(text string) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID := os.Getenv("TELEGRAM_CHAT_ID")

	if token == "" || chatID == "" {
		log.Println("Warning: Telegram credentials missing, skipping notification")
		return
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)

	payload := map[string]string{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "Markdown",
	}

	// Debug Logging
	if os.Getenv("WATCHER_LOG_LEVEL") == "DEBUG" {
		log.Printf("[DEBUG] Telegram Notify: %s", text)
	}

	body, _ := json.Marshal(payload)

	_, err := http.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		log.Printf("Telegram Alert Failed: %v", err)
	}
}
