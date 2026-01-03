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