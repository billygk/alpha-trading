package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
	"github.com/joho/godotenv"
)

// --- INTERFACES ---

// MarketProvider abstracts the exchange logic (Alpaca, Kraken, etc.)
type MarketProvider interface {
	GetPrice(ticker string) (float64, error)
}

// AlpacaProvider implements MarketProvider for US Stocks
type AlpacaProvider struct {
	client *marketdata.Client
}

func (a *AlpacaProvider) GetPrice(ticker string) (float64, error) {
	trade, err := a.client.GetLatestTrade(ticker, marketdata.GetLatestTradeRequest{})
	if err != nil {
		return 0, err
	}
	return trade.Price, nil
}

// --- DATA STRUCTURES ---

type Position struct {
	Ticker     string  `json:"ticker"`
	EntryPrice float64 `json:"entry_price"`
	StopLoss   float64 `json:"stop_loss"`
	TakeProfit float64 `json:"take_profit"`
	Status     string  `json:"status"`
	ThesisID   string  `json:"thesis_id"`
}

type PortfolioState struct {
	Version   string     `json:"version"`
	LastSync  string     `json:"last_sync"`
	Positions []Position `json:"positions"`
}

const StateFile = "portfolio_state.json"

func main() {
	// 1. Initialization
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: No .env file found, using system environment variables")
	}

	// Verify required environment variables for Alpaca
	requiredEnvVars := []string{"APCA_API_KEY_ID", "APCA_API_SECRET_KEY", "APCA_API_BASE_URL"}
	missingVars := false
	for _, envVar := range requiredEnvVars {
		if os.Getenv(envVar) == "" {
			log.Printf("CRITICAL: Missing environment variable: %s", envVar)
			missingVars = true
		} else {
			// Masking secret for logs
			val := os.Getenv(envVar)
			masked := val
			if len(val) > 4 {
				masked = "..." + val[len(val)-4:]
			} else {
				masked = "***"
			}
			log.Printf("Env Loaded: %s=%s", envVar, masked)
		}
	}
	if missingVars {
		log.Println("Warning: proceeding but Alpaca client may fail")
	}

	// 2. Setup Alpaca
	// Note: The SDK uses APCA_API_KEY_ID and APCA_API_SECRET_KEY by default
	alpacaClient := marketdata.NewClient(marketdata.ClientOpts{})
	provider := &AlpacaProvider{client: alpacaClient}

	log.Println("Alpha Watcher v1.8.0-GO Initialized [Local Environment]")

	// 3. Main Loop
	for {
		poll(provider)

		// Calculate and log next scheduled check
		nextTick := time.Now().Add(1 * time.Hour)
		log.Printf("Next check scheduled for: %s", nextTick.Format("2006-01-02 15:04:05 MST"))

		// Sleep for exactly 1 hour
		time.Sleep(1 * time.Hour)
	}
}

func poll(p MarketProvider) {
	state, err := loadState()
	if err != nil {
		log.Printf("CRITICAL: Could not load state: %v", err)
		return
	}

	for i, pos := range state.Positions {
		if pos.Status != "ACTIVE" {
			continue
		}

		price, err := p.GetPrice(pos.Ticker)
		if err != nil {
			log.Printf("ERROR: Fetching price for %s: %v", pos.Ticker, err)
			continue
		}

		log.Printf("[%s] Current: $%.2f | SL: $%.2f | TP: $%.2f", pos.Ticker, price, pos.StopLoss, pos.TakeProfit)

		if price <= pos.StopLoss {
			notify(fmt.Sprintf("🛑 *STOP LOSS HIT*\nAsset: %s\nPrice: $%.2f\nAction: SELL REQUIRED", pos.Ticker, price))
			state.Positions[i].Status = "TRIGGERED_SL"
		} else if price >= pos.TakeProfit {
			notify(fmt.Sprintf("💰 *TARGET REACHED*\nAsset: %s\nPrice: $%.2f\nAction: TAKE PROFIT", pos.Ticker, price))
			state.Positions[i].Status = "TRIGGERED_TP"
		}
	}

	state.LastSync = time.Now().Format(time.RFC3339)
	saveState(state)
}

// --- UTILITIES ---

func loadState() (PortfolioState, error) {
	var s PortfolioState
	if _, err := os.Stat(StateFile); os.IsNotExist(err) {
		log.Println("State file missing, generating template...")
		s = PortfolioState{Version: "1.1", Positions: []Position{}}
		// Save the genesis state immediately
		saveState(s)
		return s, nil
	}

	f, err := os.Open(StateFile)
	if err != nil {
		return s, err
	}
	defer f.Close()

	b, err := io.ReadAll(f)
	if err != nil {
		return s, err
	}
	return s, json.Unmarshal(b, &s)
}

func saveState(s PortfolioState) {
	b, _ := json.MarshalIndent(s, "", "  ")
	_ = os.WriteFile(StateFile, b, 0644)
}

func notify(text string) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID := os.Getenv("TELEGRAM_CHAT_ID")

	if token == "" || chatID == "" {
		log.Println("Warning: Telegram credentials missing, skipping notification")
		return
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	payload, _ := json.Marshal(map[string]string{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "Markdown",
	})

	_, err := http.Post(url, "application/json", bytes.NewBuffer(payload))
	if err != nil {
		log.Printf("Telegram Alert Failed: %v", err)
	}
}
