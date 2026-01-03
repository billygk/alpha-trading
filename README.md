# Alpha Watcher

A Go-based trading bot that watches positions and manages stops/monitoring.

## Setup

1. Ensure you have Go installed (1.22+).
2. Create a `.env` file in the root directory with your Alpaca credentials:
   ```env
   APCA_API_KEY_ID=your_key
   APCA_API_SECRET_KEY=your_secret
   APCA_API_BASE_URL=https://paper-api.alpaca.markets
   TELEGRAM_BOT_TOKEN=your_token
   TELEGRAM_CHAT_ID=your_chat_id
   ```

## Running

To run the watcher directly:

```bash
go run ./cmd/alpha_watcher
```

## Building

To build a binary:

```bash
go build -o alpha_watcher.exe ./cmd/alpha_watcher
```

Then run it:

```bash
.\alpha_watcher.exe
```

## Structure

- `cmd/alpha_watcher`: Entry point (`main.go`).
- `internal/`: Core logic packages.
  - `config`: Environment setup.
  - `market`: Market data providers.
  - `watcher`: Main polling loop.
  - `models`: Data structures.
