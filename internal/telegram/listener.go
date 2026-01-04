package telegram

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Update represents a Telegram Update object (partial schema)
type Update struct {
	UpdateID int `json:"update_id"`
	Message  struct {
		Text string `json:"text"`
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		From struct {
			Username string `json:"username"`
		} `json:"from"`
	} `json:"message"`
}

type UpdateResponse struct {
	Ok          bool     `json:"ok"`
	Result      []Update `json:"result"`
	Description string   `json:"description"`
	ErrorCode   int      `json:"error_code"`
}

// CommandHandler defines the callback signature for processing commands
type CommandHandler func(command string) string

// StartListener begins long-polling for commands.
// It runs blocking, so it should be called in a goroutine.
func StartListener(handler CommandHandler) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	authChatIDStr := os.Getenv("TELEGRAM_CHAT_ID")

	if token == "" || authChatIDStr == "" {
		log.Println("Telegram Listener: Credentials missing, disabled.")
		return
	}

	authChatID, _ := strconv.ParseInt(authChatIDStr, 10, 64)
	offset := 0

	log.Println("Telegram Listener: Started")

	for {
		url := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=60", token, offset)
		resp, err := http.Get(url)
		if err != nil {
			log.Printf("Telegram Listener Error: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		var result UpdateResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			log.Printf("Telegram Decode Error: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}
		resp.Body.Close()

		if !result.Ok {
			log.Printf("Telegram API Error: %s (Code: %d)", result.Description, result.ErrorCode)
			time.Sleep(5 * time.Second)
			continue
		}

		for _, update := range result.Result {
			offset = update.UpdateID + 1

			// Access Control
			if update.Message.Chat.ID != authChatID {
				log.Printf("⚠️ UNAUTHORIZED CODE ACCESS ATTEMPT: User %s (ID: %d) tried: %s",
					update.Message.From.Username, update.Message.Chat.ID, update.Message.Text)
				// We do NOT reply to unauthorized users to avoid leaking bot existence/logic
				continue
			}

			// Process Command
			text := strings.TrimSpace(update.Message.Text)
			if strings.HasPrefix(text, "/") {
				log.Printf("Command received: %s", text)
				response := handler(text)
				Notify(response)
			}
		}
	}
}
