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
   
   # Optional Configuration (Defaults)
   WATCHER_LOG_LEVEL=INFO
   WATCHER_MAX_LOG_SIZE_MB=5
   WATCHER_MAX_LOG_BACKUPS=3
   WATCHER_POLL_INTERVAL=60
   ```

## Configuration Reference

The application can be configured via the `.env` file. If these variables are missing, the defaults below are used.

| Variable | Default | Description |
|----------|---------|-------------|
| `WATCHER_LOG_LEVEL` | `INFO` | Logging verbosity (DEBUG, INFO, ERROR). |
| `WATCHER_MAX_LOG_SIZE_MB` | `5` | Maximum size of `watcher.log` before rotation. |
| `WATCHER_MAX_LOG_BACKUPS` | `3` | Number of old log files to keep. |
| `WATCHER_POLL_INTERVAL` | `60` | Time in minutes between market checks. |
| `CONFIRMATION_TTL_SEC` | `300` | Seconds before a pending confirmation expires (5 mins). |
| `CONFIRMATION_MAX_DEVIATION_PCT` | `0.005` | Max price deviation (0.5%) allowed between trigger and confirmation. |


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

Important: 
- project_log.md should always be updated with the progress of the project.
- Include details comments in code


