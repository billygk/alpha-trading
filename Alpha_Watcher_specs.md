# AI Coding Agent Specification: Alpha Watcher (Go)

## 1. Role & Context
You are an expert Golang Developer and System Architect. You are building a high-availability trading "Watcher" for a user with deep expertise in Linux, security, and networking.

## 2. Technical Constraints
- **Runtime**: Golang 1.22+.
- **Target OS**: Ubuntu 24.04 (Linux AMD64).
- **Hardware**: 1 GB RAM (GCP e2-micro). Code must be highly memory-efficient.
- **Secrets**: Use `joho/godotenv`. NEVER hardcode credentials or include them in Git commits.
- **Broker**: Alpaca Markets (v3 Go SDK).
- **Alerts**: Raw HTTP calls to Telegram Bot API.

## 3. Implementation Guardrails
- **No Floating Point Errors**: Financial math must eventually use high-precision decimals.
- **Error Handling**: All errors must be logged with file/line context (`log.Lshortfile`).
- **State Persistence**: Maintain `portfolio_state.json` as the source of truth.
- **Deployment Model**: Compile locally on Windows for Linux target: `GOOS=linux GOARCH=amd64 go build`.

## 4. Current Prompt for Agent
"Analyze the existing alpha_watcher.go and portfolio_state.json. Ensure the environment variables for APCA_API_KEY_ID, APCA_API_SECRET_KEY, and APCA_API_BASE_URL are correctly mapped to the Alpaca Client initialization. Implement a 'Genesis State' check that creates a valid template JSON if the file is missing. Ensure the polling loop sleeps for exactly 1 hour and logs the next scheduled check time."

## 5. Security Protocol
- **Identity**: Spanish/EU context. Use local time for logging (CET).
- **Firewall Awareness**: The code only makes outbound HTTPS requests (Port 443). Do not attempt to open listening ports.

## 6. CI/CD Automation using Github Action

1. Create a GitHub Actions workflow at .github/workflows/release.yml.
2. The workflow must trigger on pushing tags matching v*.
3. Use ubuntu-latest to compile a STATIC Linux binary: env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags='-s -w' -o alpha_watcher_linux_amd64 ..
4. Generate a SHA256 checksum of the binary.
5. Use softprops/action-gh-release to create a release and upload both the binary and the checksum.

## 7. Graceful Shutdown & Signal Handling

1. Implement a signal listener in main.go to catch os.Interrupt and syscall.SIGTERM.
2. On receiving a signal, the bot must:
- Log the shutdown request.
- Perform a final saveState() to ensure the LastSync and any status changes are persisted.
- Send a Telegram alert: "⚠️ Watcher Shutting Down: System signal received."
- Exit with code 0.

## 8. Robust File Logging

1. Update the logging logic to output to BOTH os.Stdout and a file named watcher.log.
2. Ensure the log file is opened in "Append" mode.
3. Use a custom logger or io.MultiWriter to ensure all log.Printf statements appear in the file.
4. Add a timestamp to every log entry in CET (Spanish) time format.

## 9. Heartbeat Logic (Observability)

1. Implement a "Heartbeat" feature. Every 24 hours (or after a configurable number of polls), the bot should send a Telegram message with a status summary.
2. Summary should include:
- Total Active Positions being watched.
- System Uptime (calculate since start).
- Current Alpaca Account Equity (fetch from client.GetAccount()).
3. Ensure the heartbeat does not block the main polling loop (use a goroutine or check a timestamp within the main loop).

## 10. Systemd Service Configuration (Daemonization)

1. Generate a Linux systemd unit file named alpha-watcher.service.
2. The service should:
- Run under the ubuntu user.
- Set the WorkingDirectory to /home/ubuntu/alpha_trading.
- Use Restart=always and RestartSec=10.
- Capture StandardOutput and StandardError to syslog.
3. Provide the terminal commands to install, enable, and start this service on the GCP box.

## 11. Interactive Telegram Commands (Inbound Integration)

1. Implement a Telegram "Command Listener" using long-polling (/getUpdates).
2. Use a dedicated goroutine for the listener to prevent blocking the market polling loop.
3. Use a sync.RWMutex to protect the PortfolioState struct during concurrent read/write operations.
4. Implement the following commands (strictly restricted to the TELEGRAM_CHAT_ID):
- /status: Return the current uptime and account equity.
- /list: List all active positions and their current distance from Stop Loss.
- /ping: Simple connectivity check.
5. Ensure the bot ignores messages from any other chat_id.

## 12. Atomic State Persistence

1. Refactor saveState() to use an "Atomic Write" pattern.
2. Logic: Write the JSON to a temporary file (e.g., portfolio_state.json.tmp), then use os.Rename to overwrite the original file.
3. This ensures that even if the process crashes or the disk is full during the write, the original portfolio_state.json remains intact and uncorrupted.

## 13. Log Rotation & Management

1. Implement basic internal log rotation or provide a logrotate configuration file for the GCP box.
2. If internal: When watcher.log reaches 5MB, rename it to watcher.log.1 and start a new file. Keep a maximum of 3 old log files.
3. This prevents the application from consuming all available disk space over months of operation.

## 13.1. Environment-Only Configuration System
1. Create a Config struct to centralize all "tweakable" parameters.
2. Parameters to include:
- LogLevel (INFO, DEBUG, ERROR).
- MaxLogSizeMB (Default 5).
- MaxLogBackups (Default 3).
- PollIntervalMins (Default 60).
3. Implementation:
- The bot must use the .env file (via godotenv) as the only external source of configuration.
- No JSON or YAML fallback files are permitted.
- The Agent must implement a "LoadConfig" function that checks for environment variables (e.g., WATCHER_LOG_LEVEL, WATCHER_POLL_INTERVAL).
- MANDATORY: If an environment variable is missing, the code must fall back to hardcoded "Sensible Defaults" defined within the Go struct.
4. Refactor the existing Logger and Polling logic to consume values from this centralized Config object.

## 14. Real-time Market Data (Alpaca WebSockets)

1. Transition the core monitoring logic from 1-hour polling to Alpaca's Streaming SDK (WebSockets).
2. The bot must subscribe to trades for all tickers listed in portfolio_state.json.
3. Implement a "Stream Reconnection" logic: if the WebSocket drops, the bot must log the event and attempt to reconnect with exponential backoff.
4. Maintain the "Polling" logic as a fallback: if the WebSocket is down for more than 5 minutes, perform a manual REST poll of all positions.

## 15. Market Query Engine (Telegram)

1. Extend the Telegram command handler to support:
2. /price <ticker>: The bot must query Alpaca's GetLatestTrade for any provided ticker (even if not in the state file) and return the current price.
3. /market: Query Alpaca's GetClock and return if the market is currently OPEN or CLOSED, and the time until the next state change.
4. Implementation Note: Must use the existing Telegram goroutine and respect the sync.RWMutex for state-related queries.
5. Verification: Ensure that if a ticker is invalid, the bot returns a clean "Ticker not found" message instead of crashing.

## 16. Search & Discovery (Minimalist)

1. Implement /search <query>:
2. The bot should use Alpaca's GetAssets with a filter to find symbols matching the query string.
3. Return a maximum of 5 results (Ticker - Name) to prevent Telegram character overflow.
4. Constraint: This must be a memory-efficient call. Do not cache the entire asset list in RAM. Use alpaca.GetAssets with the status=active and asset_class=us_equity parameters.

## 17. Interactive Help System (Self-Documentation)

1. Implement a /help command in the Telegram handler.
2. The command must return a comprehensive list of all active commands: /status, /list, /ping, /price, /market, and /search.
3. Each command must include a one-line description and example usage (e.g., /price AAPL).
4. Implementation Requirement: Store these descriptions in a structured way (e.g., a map or a slice of structs) within the code so that the help system is easy to update in future tasks.
5. Formatting: Use Markdown formatting for the response to ensure triggers and descriptions are clearly separated.

## 18. Attended Automation & Manual Confirmation (ACTIVE)

1. Monitor real-time prices: Utilize the WebSocket stream (Point 14).
2. On SL/TP trigger: - Capture the trigger_price and timestamp.
- Store the pending trade in a thread-safe map (sync.Mutex protected).
3. Send Telegram message: Include ticker, side, trigger price, and inline [✅ CONFIRM] / [❌ CANCEL] buttons.
4. On Callback (User clicks Confirm):
- Temporal Gate: Validate that now - timestamp <= CONFIRMATION_TTL_SEC (from .env).
- Deviation Gate: Fetch the latest price (REST call for accuracy) and validate that abs(current_price - trigger_price) / trigger_price <= CONFIRMATION_MAX_DEVIATION_PCT (from .env).
- If either gate fails: Notify user of the specific failure and purge the action.
5. Execution: If both gates pass, execute alpaca.PlaceOrder (Market Order) and update portfolio_state.json to EXECUTED.
6. Cleanup: Purge the pending action from memory regardless of outcome.


## 19. WebSocket Removal & REST Restoration (NEW)

1. Decommission WebSockets: Remove all code related to AlpacaStreamer, stream.Connect(), and IEX/SIP WebSocket subscriptions.
2. Clean Dependencies: Remove any unused WebSocket-related imports or packages (e.g., Alpaca Streaming SDK if not used elsewhere).
3. Core Logic Reversion: Revert the Watcher to be 100% powered by the polling interval defined in WATCHER_POLL_INTERVAL (Point 13.1).
4. Outbound Only: Ensure the application makes only standard HTTPS REST calls to Alpaca.

## 20. Polling-Based Attended Automation (NEW)

1. Integration: Refactor the SL/TP trigger logic from Point 18 to run inside the main polling loop.
2. Batch Processing: During each poll cycle, the bot must:
  - Fetch the latest price for ALL tickers in portfolio_state.json using Alpaca's GetLatestTrade (REST).
  - Compare latest prices against StopLoss and TakeProfit targets in the state file.
3. Trigger Workflow: If a price crosses a threshold during a poll:
  - Initiate the confirmation workflow defined in Point 18.3 (Telegram buttons).
  - Use the price fetched during the poll as the trigger_price.
4. Safety: Ensure the Temporal Gate and Deviation Gate from Point 18.4 still apply, using the time of the poll as the reference.

