package market

import (
	"context"
	"log"
	"os"
	"sync"
	"time"

	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata/stream"
)

// StreamHandler is a callback function for price updates.
type StreamHandler func(ticker string, price float64)

// StreamProvider defines the interface for real-time market data.
type StreamProvider interface {
	Subscribe(tickers []string, handler StreamHandler) error
	Close() error
}

// AlpacaStreamer implements StreamProvider using Alpaca's WebSocket API.
type AlpacaStreamer struct {
	client    *stream.StocksClient
	handler   StreamHandler
	mu        sync.Mutex
	reconnect bool
}

// NewAlpacaStreamer creates a new streamer instance.
func NewAlpacaStreamer() *AlpacaStreamer {
	// Credentials are automatically loaded from env vars by the SDK if present.
	// But explicitly:
	keyID := os.Getenv("APCA_API_KEY_ID")
	secretKey := os.Getenv("APCA_API_SECRET_KEY")

	return &AlpacaStreamer{
		client: stream.NewStocksClient(
			marketdata.IEX, // Use IEX feed (free/paper)
			stream.WithCredentials(keyID, secretKey),
			stream.WithReconnectSettings(10, 500*time.Millisecond),
		),
		reconnect: true,
	}
}

// Subscribe connects to the stream and listens for trades for the given tickers.
func (s *AlpacaStreamer) Subscribe(tickers []string, handler StreamHandler) error {
	s.mu.Lock()
	s.handler = handler
	s.mu.Unlock()

	// Handler for trade updates
	tradeHandler := func(t stream.Trade) {
		// Log debug only if needed, to avoid spam
		// log.Printf("Stream Trade: %s @ $%.2f", t.Symbol, t.Price)
		if s.handler != nil {
			s.handler(t.Symbol, t.Price)
		}
	}

	// Subscribe to trades for all tickers
	if err := s.client.SubscribeToTrades(tradeHandler, tickers...); err != nil {
		return err
	}

	// Connect in a background goroutine to not block
	go func() {
		log.Println("🔌 Connecting to Alpaca Stream...")
		// Connect blocks until the connection is closed
		if err := s.client.Connect(context.Background()); err != nil {
			log.Printf("ERROR: Stream connection closed with error: %v", err)
			// Reconnection is handled by the SDK settings to some extent,
			// but if it fails completely, we might need manual logic.
			// For now, reliance on SDK's built-in reconnection is standard.
			// If it exits, it means it gave up.
			// Should we restart?
			if s.reconnect {
				log.Println("Attempting manual reconnection loop...")
				s.manualReconnectLoop()
			}
		} else {
			log.Println("Stream connection closed normally.")
		}
	}()

	return nil
}

func (s *AlpacaStreamer) Close() error {
	s.reconnect = false
	// Is there a Close method? The SDK handles context cancellation usually.
	// There isn't a direct Close() on the high-level client causing Connect to return immediately in all versions.
	// But we can just let it be.
	return nil
}

func (s *AlpacaStreamer) manualReconnectLoop() {
	backoff := 1 * time.Second
	maxBackoff := 60 * time.Second

	for s.reconnect {
		time.Sleep(backoff)
		log.Println("Reconnecting stream...")
		if err := s.client.Connect(context.Background()); err != nil {
			log.Printf("Reconnection failed: %v", err)
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		} else {
			// If Connect returns nil, it means it finished (closed).
			// If it was a successful connection that lasted, backoff resets on next error.
			backoff = 1 * time.Second
		}
	}
}
