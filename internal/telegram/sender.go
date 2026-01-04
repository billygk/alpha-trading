package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
)

// Button represents an inline keyboard button.
type Button struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data"`
}

// SendInteractiveMessage sends a message with inline buttons.
func SendInteractiveMessage(text string, buttons []Button) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID := os.Getenv("TELEGRAM_CHAT_ID")

	if token == "" || chatID == "" {
		return
	}

	// Construct Inline Keyboard
	var inlineKeyboard [][]Button
	// For now, we put all buttons in one row (slice of slice)
	inlineKeyboard = append(inlineKeyboard, buttons)

	keyboardPayload := map[string]interface{}{
		"inline_keyboard": inlineKeyboard,
	}

	keyboardJSON, _ := json.Marshal(keyboardPayload)

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	data := map[string]string{
		"chat_id":      chatID,
		"text":         text,
		"parse_mode":   "Markdown",
		"reply_markup": string(keyboardJSON),
	}

	jsonData, _ := json.Marshal(data)
	resp, err := http.Post(apiURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("Telegram Error: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Telegram API Error: Status %s", resp.Status)
	}
}
