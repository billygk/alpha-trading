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
	CallbackQuery struct {
		ID      string `json:"id"`
		Data    string `json:"data"`
		Message struct {
			Chat struct {
				ID int64 `json:"id"`
			} `json:"chat"`
		} `json:"message"`
		From struct {
			Username string `json:"username"`
		} `json:"from"`
	} `json:"callback_query"`
}

type UpdateResponse struct {
	Ok          bool     `json:"ok"`
	Result      []Update `json:"result"`
	Description string   `json:"description"`
	ErrorCode   int      `json:"error_code"`
}

// CommandHandler defines the callback signature for processing commands
type CommandHandler func(command string) string

// StartListener begins long-polling for updates.
func StartListener(cmdHandler CommandHandler, cbHandler CallbackHandler) {
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

			// Check for Message or Callback
			var chatID int64
			var username string
			var text string
			var isCallback bool

			if update.Message.Chat.ID != 0 {
				chatID = update.Message.Chat.ID
				username = update.Message.From.Username
				text = update.Message.Text
			} else if update.CallbackQuery.ID != "" {
				isCallback = true
				chatID = update.CallbackQuery.Message.Chat.ID
				username = update.CallbackQuery.From.Username
				text = update.CallbackQuery.Data
			}

			// Access Control
			if chatID != authChatID {
				log.Printf("⚠️ UNAUTHORIZED ACCESS ATTEMPT: User %s (ID: %d)", username, chatID)
				continue
			}

			if isCallback {
				log.Printf("Callback received: %s", text)
				response := cbHandler(update.CallbackQuery.ID, text)
				Notify(response) // Or handle specific answerCallback logic
			} else {
				// Process Command
				text = strings.TrimSpace(text)
				if strings.HasPrefix(text, "/") {
					log.Printf("Command received: %s", text)
					response := cmdHandler(text)
					Notify(response)
				}
			}
		}
	}
}
