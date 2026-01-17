# Specification: Alpha Watcher (Go)

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

## 4. recheck the code
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
- implement a "LoadConfig" function that checks for environment variables (e.g., WATCHER_LOG_LEVEL, WATCHER_POLL_INTERVAL).
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

## 21. Virtual Trailing Stop Logic (Profit Maximization)
1. State Update: Add HighWaterMark (float64) and TrailingStopPct (float64) to the Position struct.
2. Logic:
  - If CurrentPrice > HighWaterMark, update HighWaterMark = CurrentPrice.
  - Dynamic exit trigger: TriggerPrice = HighWaterMark * (1 - TrailingStopPct/100).
3. Execution: If CurrentPrice <= TriggerPrice, initiate Attended Automation with the label "TRAILING STOP TRIGGERED".

## 22. Trade Proposal & Manual Entry System (NEW)
1. Objective: Allow the user to initiate new trades without manually editing portfolio_state.json.
2. Command: Implement /buy <ticker> <quantity> <sl> <tp>.
  - Example: /buy AAPL 1 210.50 255.00
3. Validation Gate:
  - Fetch latest price via REST.
  - Calculate total cost and ensure it doesn't exceed Alpaca buying_power.
4. Attended Workflow:
  - Respond with: "PROPOSAL: Buy {{qty}} {{ticker}} @ ~${{price}}. Total: ${{cost}}. SL: {{sl}} | TP: {{tp}}. Confirm?"
  - Provide [✅ EXECUTE] and [❌ CANCEL] buttons.
5. Execution:
  - Upon [✅ EXECUTE], call alpaca.PlaceOrder.
  - Update portfolio_state.json with Status: "OPEN", HighWaterMark: price, and EntryPrice: price.

## 23. Market Intelligence Command Handler (Sector Scanning)

1. Objective: Implement a command handler for /scan <sector> to check the health of target equities.
2. Technical Implementation:
  - Create a static map within the market package: map[string][]string where the key is the sector name and values are ticker symbols.
3. Sectors to include:
  - biotech: ["XBI", "VRTX", "AMGN"]
  - metals: ["GLD", "SLV", "COPX"]
  - energy: ["URA", "CCJ", "XLE"]
  - defense: ["ITA", "LMT", "RTX"]
4. When the command is received, iterate through the tickers, fetch GetLatestTrade for each, and format a consolidated Telegram response.
5. Constraint: Do not hardcode prices. Always fetch fresh data via the Alpaca REST client.

## 24. State Schema Migration & Version Management

1. Objective: Update the storage layer to handle schema evolution.
2. Technical Implementation:
  - Update PortfolioState struct Version field to default to 1.2.
  - Add a Migrate() method to the storage package.
3. When loading portfolio_state.json, if the loaded version is < 1.2, the function must:
    - Initialize HighWaterMark to the current EntryPrice for all existing positions.
    - Set TrailingStopPct to a default of 0.0.
    - Update the version in memory and trigger an immediate saveState().
4. Verification: Ensure the migration is atomic and logged as an INFO event.

## 25. Enhanced Execution Feedback & Error Reporting 

Objective: Prevent "Silent Failures" during the Attended Automation workflow.
Technical Implementation:
Refactor the Execute logic in the Telegram handler.
MANDATORY: On ANY failure of alpaca.PlaceOrder or the Deviation Gate, the bot MUST send a specific Telegram alert detailing the error (e.g., "❌ Execution Failed: Insufficient Funds" or "❌ Execution Failed: API Network Timeout").
Order Verification: After a successful PlaceOrder call, the bot must wait 2 seconds and then call alpaca.GetOrder to verify the order status is filled or accepted before updating portfolio_state.json.
Logging: All failed execution attempts must be logged with [FATAL_TRADE_ERROR] prefix for easy grepping in watcher.log.

## 26. Order State Synchronization (The "Queued" Watcher)

Objective: Prevent duplicate orders and track trades that are "Accepted" but not yet "Filled" (e.g., market closed).
Technical Implementation:
Update the /status command to include a "Pending Orders" section.
Before executing a /buy command, the bot MUST check for existing open orders for that ticker via alpaca.ListOrders(status="open").
If an open order exists, the bot must reject the new /buy request with: "⚠️ Order already pending for {{ticker}}. Cancel it on Alpaca before placing a new one."
Polling Update: During the 1-hour polling cycle, if portfolio_state.json is empty but Alpaca has open orders, the bot should send a Telegram notification: "⏳ Waiting for Market Open: {{qty}} shares of {{ticker}} are queued."
Resilience: This prevents the bot from "double-dipping" your €300 capital if you accidentally trigger multiple proposals while the market is closed.

## 27. Universal Exit & Liquidation Handler (/sell)

Objective: A single command to terminate all risk (pending or active) for a specific ticker.
Command: /sell <ticker>.
Logic Flow:
Step 1: Check Active Positions:
Check portfolio_state.json and call alpaca.ListPositions().
If an active position is found: Execute alpaca.PlaceOrder (Side: Sell, Type: Market).
Send Telegram: "📉 Closing Position: Manual Sell for {{ticker}} initiated."
Step 2: Check Pending Orders:
Call alpaca.ListOrders(status="open") for the ticker.
If pending orders exist: Execute alpaca.CancelOrder for each.
Send Telegram: "🚫 Cancelling Orders: Pending orders for {{ticker}} removed."
Step 3: Cleanup:
Upon confirmation of fill/cancellation, update portfolio_state.json to reflect Status: CLOSED.
Safety: If neither a position nor an order is found, return: "❓ No active risk found for {{ticker}}."

## 28. Deep Sync & Recovery (/refresh)

Objective: Force-reconcile local state with Alpaca reality.
Command: /refresh.
Logic:
Fetch ALL current positions from Alpaca.
Overwrite/Update portfolio_state.json to match.
Initialize HighWaterMark to current market price for any newly "discovered" positions.
Send Telegram: "🔄 State Reconciled: Local state now matches Alpaca broker data."

## 29. Manual Sync & State Reconciliation Logic

Objective: Implement the backend logic for the /refresh command to handle state discovery.
Constraint: When a position is found on Alpaca that does not exist in portfolio_state.json, it must be added with a Status: "OPEN" and the current market price assigned to both EntryPrice and HighWaterMark.
Safety: Log a warning for every position that is "Discovered" via sync but was missing from local state, as this indicates a potential local I/O failure.

## 30. Rich Dashboard Implementation (/status)
Objective: Transform the /status command from a simple counter into a detailed financial dashboard.
Market State Integration:
The bot must call alpaca.GetClock().
Header must display: Market: 🟢 OPEN or Market: 🔴 CLOSED.
Calculate time remaining: "Closes in: HH:MM" or "Opens in: HH:MM".
Data Requirements:
For each active position in portfolio_state.json, the bot must:
Fetch GetLatestTrade (Current Price).
Fetch GetBars (daily, limit=1) to get the PreviousClose price.
Calculate Today P/L: (Current - PrevClose) * Qty.
Calculate Total P/L: (Current - EntryPrice) * Qty.
UI Requirements (Telegram):
Use Monospaced formatting for the asset data to ensure alignment.
Visual Indicators: Use "🟢" for positive Total P/L and "🔴" for negative Total P/L.
Strategic Context: Include the "Distance to Stop Loss" (%) and the current "HighWaterMark" for each position.
Performance: Fetch market data (Clock, Account, Prices) in parallel using goroutines to minimize command latency.
Fallbacks: If Alpaca fails to return PreviousClose, skip the "Today P/L" section.

## 31. Unified Global State Version (v1.3)
Requirement: Update state version to 1.3 to ensure the we recognize these UI enhancements.
Logic: No new fields required in the JSON for this, but the version bump ensures the we recompile the notification templates and formatting logic.

## 32. Automated Operational Awareness (Scheduled Status) // NEW POINT
Objective: Implement automated status pushes during active market hours to maintain user synchronization.
Logic:
Integrate with the main Polling Loop (Point 20).
At the end of every successful poll, the bot must call alpaca.GetClock().
Trigger: If clock.IsOpen == true, the bot must automatically invoke the logic defined in Point 30 (Rich Dashboard) and send it to the user.
Environment Control:
Add a new .env variable: AUTO_STATUS_ENABLED=true.
The bot must check this flag before performing the automated push.
Resilience: If the dashboard generation fails during an automated push, the bot must log an [ERROR] but MUST NOT interrupt the primary price-watching loop or trigger a crash.

## 33. Broker-First Dashboard Logic (Overrides Point 30)
Objective: Shift the source of truth for the /status command from the local JSON file to the live Alpaca API.
Data Acquisition Strategy:
The bot MUST execute three parallel calls (using sync.WaitGroup or errgroup):
alpaca.GetAccount(): For total equity, buying power, and account-level P/L.
alpaca.GetClock(): For market status (Open/Closed) and session countdown.
alpaca.ListPositions(): To identify what is currently held in the portfolio.
Reconciliation Logic:
Iterate through results from ListPositions().
Match each Alpaca position against the portfolio_state.json based on the Ticker.
Case A (Matched): Use Alpaca's Qty and CurrentPrice, but retrieve EntryPrice and TrailingStopPct from JSON to calculate SL/TP metrics.
Case B (Unmatched): Display as "⚠️ UNTRACKED". Calculate P/L relative to the CostBasis provided by Alpaca's API since local history is missing.
Enhanced UI Elements:
Header: Equity: $X (Today: +$Y / Z%) where Y is calculated from Equity - LastEquity.
Asset Cards: Include AvgEntry vs Current and a clear "Distance to Virtual Stop" line.

## 34. Scheduled Market-Hours Heartbeat (Overrides Point 32)
Objective: Automate the delivery of the Point 33 Dashboard during active trading windows.
Operational Trigger:
At the conclusion of every successful polling cycle (Point 20), the bot must check the result of alpaca.GetClock().
If clock.IsOpen == true, the bot must immediately execute the Point 33 Dashboard logic and send it to the Telegram chat.
Throttle Control:
Add .env variable: AUTO_STATUS_INTERVAL_POLLS. (Default: 1).
This allows the user to receive a dashboard every N polls.
Execution Safety: If the automated status push fails, it MUST be logged as a [WARNING] but must not affect the LastSync timestamp or the state of the active positions.

## 35. Precision Decimal Transition (Technical Requirement)
Objective: Replace all float64 fields in portfolio_state.json and internal calculations with the shopspring/decimal package.
Reasoning: To prevent IEEE 754 rounding errors in P/L and Trailing Stop calculations.
Migration: The Migrate() function (Point 24) must handle the conversion from float strings to Decimal objects during the v1.3 upgrade.

## 36. Take-Profit (TP) Execution Logic & Guardrails
Objective: Standardize the behavior of the Take-Profit trigger to ensure consistency with the Human-in-the-Loop philosophy.
Trigger Condition:
During the polling cycle (Point 20), if CurrentPrice >= Position.TP:
The bot MUST transition the position to a PENDING_EXIT state in memory.
Send the "TAKE PROFIT" Telegram alert with [✅ CONFIRM] and [❌ CANCEL] buttons.
Price Protection (The TP-Deviation Gate):
Upon clicking [✅ CONFIRM], the bot must fetch a fresh price.
Logic: If FreshPrice < (Position.TP * 0.995), notify the user: "⚠️ Price has slipped below 99.5% of TP. Manual review required."
Interaction with Trailing Stop:
The TP trigger MUST take precedence over the Trailing Stop if both are triggered in the same poll.

## 37. Configurable Default Take-Profit Percentage
Objective: Allow for automated TP calculation based on a global strategy setting while preserving manual overrides.
Environment Variable:
Add DEFAULT_TAKE_PROFIT_PCT to .env. (Example: DEFAULT_TAKE_PROFIT_PCT=10.0).
Logic Integration:
Update the /buy command (Point 22).
If the <tp> parameter is provided as a number (e.g., 58.00), use that absolute price.
NEW: If the <tp> parameter is omitted or passed as 0, the bot must calculate the TP: EntryPrice * (1 + (DEFAULT_TAKE_PROFIT_PCT / 100)).
Validation:
If DEFAULT_TAKE_PROFIT_PCT is missing from .env, fallback to a hardcoded "Sensible Default" of 15.0%.
The bot must confirm the calculated TP price in the Telegram "Proposal" message before the user clicks [✅ EXECUTE].

## 38. Strict "Confirm-to-Sell" Enforcement
Objective: Ensure the bot never autonomously liquidates a position based on Take-Profit triggers without explicit user authorization.
Execution Gate:
The ONLY state that triggers an alpaca.PlaceOrder(Side: Sell) is the successful callback from the Telegram [✅ CONFIRM] button.
Default Behavior (The "Hold" Fallback):
In all other cases—including button expiration (TTL), clicking [❌ CANCEL], bot crashes, or network timeouts—the bot MUST maintain the position and do nothing.
Re-Trigger Management:
If a TP alert is ignored or cancelled, the bot must not re-alert for that position in the same polling cycle.
Implement a LastAlertTime timestamp per position to prevent "Alert Fatigue."

## 39. Universal Temporal Gate (CONFIRMATION_TTL_SEC)
Objective: Prevent the execution of stale trade proposals or exit triggers due to delayed user interaction.
Configuration:
Read CONFIRMATION_TTL_SEC from .env. (Default: 300).
Logic Enforcement:
Every interactive button (Buy, Sell, TP, SL, Trailing) MUST include a Timestamp in its metadata.
Upon callback, calculate: Elapsed = CurrentTime - TriggerTime.
Execution Decision:
IF Elapsed > CONFIRMATION_TTL_SEC: Abort, notify user ("⚠️ Action Expired"), and purge PendingAction.
IF Elapsed <= CONFIRMATION_TTL_SEC: Proceed to Deviation Gate and then execution.
UI Feedback:
Include footer: ⏱️ Valid for {{TTL}} seconds.

## 40. True Cost-Basis Reconciliation (/refresh Overhaul)
Objective: Ensure that a /refresh command results in 100% parity with the Alpaca Broker's calculation of P/L and Equity.
Logic Override (Point 28 & 29):
When running /refresh, the bot MUST NOT use the CurrentPrice as the EntryPrice.
Instead, for every position found in alpaca.ListPositions(), the bot MUST extract the AvgEntryPrice and CostBasis from the API response.
Local State Update:
Position.EntryPrice = alpacaPosition.AvgEntryPrice
Position.HighWaterMark = max(alpacaPosition.AvgEntryPrice, CurrentMarketPrice)
Position.Qty = alpacaPosition.Qty
Decimal Precision Enforcement:
Ensure the AvgEntryPrice (which can be 4 decimal places, e.g., $48.7367) is parsed directly into the shopspring/decimal type to prevent rounding discrepancies between the Bot and the Alpaca Dashboard.
Dashboard Parity:
Update the /status logic to calculate Total P/L using the formula: (CurrentPrice - AvgEntryPriceFromAlpaca) * Qty.
This ensures that if you lose your JSON and refresh, your "Total P/L" in Telegram matches the "Total P/L" in the Alpaca UI exactly.
Safety Gate:
If a /refresh discovers a position with no SL/TP defined in the old JSON, it must set SL: N/A and TP: N/A and notify the user to set them manually using a new /update <ticker> <sl> <tp> command.

## 41. Consolidated Transactional Interface (/buy)
Objective: Implement a streamlined entry command that leverages global defaults for rapid execution.
Command Syntax: /buy <ticker> <qty> [sl_price] [tp_price].
Logic:
The bot MUST parse optional sl_price and tp_price.
If either is 0 or omitted:
Fetch DEFAULT_STOP_LOSS_PCT and DEFAULT_TAKE_PROFIT_PCT from .env.
Calculate absolute prices using shopspring/decimal based on the current market price.
Verification: The bot must respond with a "Proposed Trade" card showing the final calculated SL/TP prices before the user clicks [✅ EXECUTE].
Safety: Ensure DEFAULT_TRAILING_STOP_PCT is also applied as a default if no specific trailing logic is requested.

## 42. Strict Mirror Sync (/refresh)
Objective: Align local state with broker reality while preventing "State Destruction."
Logic:
Step 1: Fetch ListPositions() from Alpaca API.
Step 2: For each position returned by Alpaca:
Update Qty and EntryPrice (AvgEntry) in the local portfolio_state.json.
Conditional SL/TP Assignment:
IF the ticker is already in the JSON: Do not modify existing SL/TP/Trail values.
IF the ticker is MISSING from the JSON (New Discovery): Apply the DEFAULT_STOP_LOSS_PCT and DEFAULT_TAKE_PROFIT_PCT from .env.
Step 3: Remove any tickers from portfolio_state.json that are no longer present in alpaca.ListPositions() (Cleanup).
Result: A clean, synchronized dashboard that respects previous human intent while protecting new discoveries.

## 43. Automated Operational Awareness (The Market-Hours Heartbeat)
Objective: Provide automated transparency without manual querying.
Integration: Connect to the 1-hour polling loop (Point 20).
Trigger:
After every price-watching poll:
Call alpaca.GetClock().
IF is_open == true: Automatically send the Point 33 Dashboard to Telegram.
Configuration: Use AUTO_STATUS_ENABLED=true in .env to toggle this behavior.

## 44. Command Purity Enforcement
Objective: Maintain a strictly transactional interface by banning "Mid-flight Patching."
Logic: The /refresh command MUST NOT accept parameters. It is a read-only reconciliation tool.
Workflow: To change parameters, the user must /sell and then /buy with new values.

## 45. Strategic Entry Guardrails (/buy)
Objective: Ensure that the /buy command calculates stops based on the actual intended risk.
Logic: If SL/TP are omitted, the bot must present a "Trade Proposal" in Telegram showing the calculated prices based on LatestTrade before the user confirms.

## 46. (Reserved for Future Logic Consolidation)

## 47. (Reserved for Future Logic Consolidation)

## 48. Refactored Help System (/help)
Objective: Update the command registry to reflect the simplified, transactional nature of the bot.
Registry:
/buy <ticker> <qty> [sl] [tp]: Deploy capital (0/omitted uses defaults).
/sell <ticker>: Liquidate and clean state.
/refresh: Sync local state with Alpaca truth.
/status: Immediate Rich Dashboard.

## 49. Market Close Performance Report (The EOD Briefing)
Objective: Provide a comprehensive financial summary immediately after the US Market close.
Trigger: Monitor alpaca.GetClock(). Trigger when clock.IsOpen transitions from true to false (approx. 22:01 CET).
Data Acquisition:
Pillar 1 (Current): alpaca.ListPositions() for unrealized stats.
Pillar 2 (Historical): alpaca.GetPortfolioHistory(period="1D") for the daily equity curve.
Pillar 3 (Realized): alpaca.ListOrders(status="closed") filtered by current date to find positions closed during the session.
Report Structure (Telegram):
Header: 📊 MARKET CLOSE REPORT - [YYYY-MM-DD]
Section A (Account Level): Daily Change (%), Total Alpha since start (%), Ending Equity.
Section B (Per Asset Table): Ticker | Day W/L | Total W/L.
Section C (Realized Today): List final P/L for any trades closed since 09:30 EST.
Precision: All calculations MUST use shopspring/decimal.
Persistence: Append the finalized report to daily_performance.log for auditability.

## 50. Raw State Inspection (/portfolio)
Objective: Provide a low-level debugging tool to verify the integrity of the portfolio_state.json file.
Command: /portfolio.
Logic:
The bot MUST read the portfolio_state.json file from the disk.
It MUST handle the content as a string for chunking.
Multi-Message Guardrails (Chunking Strategy):
Logic: If the file content exceeds 3900 characters (leaving room for overhead), the bot MUST split the string into multiple chunks.
Formatting: Each chunk MUST be independently wrapped in ```json and ``` tags.
Sequencing: Include a header in the first message (e.g., Portfolio State (Part 1/N):) and subsequent headers for others.
Implementation:
Use os.ReadFile to get raw data.
Use a loop to iterate through the string in steps of 3900 characters.
Send each segment as a separate sendMessage request to the Telegram API.

## 51. Intent Mutation Guardrails (/update) // NEW POINT
Objective: Allow for the manual adjustment of Stop-Loss (SL) and Take-Profit (TP) levels for active positions without requiring a re-entry.
Command Syntax: /update <ticker> <new_sl> <new_tp>.
Validation Logic (The Safety Gates):
Existence: Check if the ticker exists in portfolio_state.json.
Market Context: Fetch the LatestTrade price from Alpaca.
SL Validation: new_sl must be LOWER than the current market price (prevent immediate trigger).
TP Validation: new_tp must be HIGHER than the current market price.
Consistency: new_tp must be higher than new_sl.
Execution:
Update the fields in the local JSON using shopspring/decimal.
Force an immediate saveState() call.
Send a confirmation message: "✅ Parameters Updated for {{ticker}}. New Floor: ${{sl}} | New Ceiling: ${{tp}}".
HighWaterMark Reset:
If the new SL is part of a trailing strategy, the bot must decide whether to reset the HighWaterMark to the current price or maintain the historical peak.
Decision: Keep historical HWM to maintain trailing integrity unless the user explicitly resets it.
Integration: Update /help (Point 48) to include the new /update command.

## 52. High Water Mark (HWM) Monotonicity Guardrail // NEW POINT
Objective: Prevent "HWM Decay" where the trailing stop floor moves downward.
Mathematical Enforcement:
The HWM update logic MUST be: HWM = max(stored_HWM, current_price).
Under no circumstances (except explicit /buy or /update ... reset) should the HighWaterMark value in portfolio_state.json decrease.
Audit Implementation:
Every time saveState() is called, the bot should verify that for all active positions, NewHWM >= OldHWM.
If a decrease is detected, log a [CRITICAL_STATE_REGRESSION] error with the stack trace.
Serialization Safety:
When using shopspring/decimal, ensure the json.Marshal process does not truncate precision, which could cause "micro-decay" over time.

## 53. Execution Verification & False-Positive Guardrail
Objective: Prevent the bot from reporting "Success" to Telegram when the Broker rejects or cancels an order.
Logic:
Immediately after calling alpaca.PlaceOrder, the bot MUST NOT send a success message or delete the local state.
It MUST initiate an Async Verification Loop:
Query alpaca.GetOrder(orderID) every 1 second for 5 seconds.
IF Status == 'filled': Send "✅ Position Closed/Opened" and perform saveState().
IF Status == 'canceled' or 'rejected': Send a [CRITICAL] alert: "🚨 Order Failed/Canceled by Broker. Position remains in previous state. Reason: {{Alpaca_Reason}}."
ABORT any local state deletions. The JSON must remain untouched to ensure monitoring continues.

## 54. Sequential Order Clearance (Fix for Point 27 Race Condition)
Objective: Resolve the "Locked Shares" race condition where a Market Sell is canceled because a previous Limit Order was still being canceled.
Logic:
In the /sell and /buy logic, the bot MUST call alpaca.ListOrders to find open orders for the ticker.
If found, call alpaca.CancelOrder.
THE BLOCKING WAIT: The bot MUST poll the broker (max 5 retries, 500ms apart) until ListOrders returns an empty set for that ticker.
ONLY then is the bot permitted to call alpaca.PlaceOrder.

## 55. Standardized Time-In-Force (TIF) Override
Objective: Align bot behavior with the successful parameters identified in manual testing (Point 54 manual audit).
Logic:
For all Market Orders (Buy or Sell), the bot MUST explicitly use TimeInForce: "day".
Forbidden: gtc is no longer permitted for Market Orders, as it increases the risk of broker-side rejection or "Zombie Orders" that stay open across sessions.

## 56. Re-Sync Enforcement on Execution Failure
Objective: If an execution failure (Point 53) is detected, the bot must ensure the local state hasn't become corrupted by a partial write.
Logic:
Upon a canceled or rejected order status, the bot MUST automatically trigger the logic of Point 42 (Strict Mirror Sync) to re-validate exactly what is on the broker's books.

## 57. State Purity Enforcement (Auto-Purge Closed Positions)
Objective: Prevent the portfolio_state.json file from bloating and ensure the Watcher iterates only on active risk.
Logic (The "Archive & Delete" Workflow):
When a /sell command results in a filled status (Point 53 verification):
Step 1: Extract the full position object (including thesis_id and final P/L).
Step 2: Append this object to daily_performance.log as a JSON entry for the EOD report.
Step 3: IMMEDIATELY delete the ticker key from the positions array in portfolio_state.json.
Reconciliation Safeguard: During the /refresh sync (Point 42), any position found in the local JSON with a status of CLOSED that no longer exists in Alpaca's active list MUST be purged.

## 58. AI-Directed Analysis Loop (Temporal Gating)
Objective: Trigger AI analysis only when data is actionable to minimize API costs and noise.
Logic:
Trigger: Success of the 1-hour WATCHER_POLL_INTERVAL.
Temporal Gate: The API call to Gemini MUST ONLY occur if:
The US Market is OPEN (15:30 - 22:00 CET).
OR it is the "Pre-Market Hour" (14:30 - 15:30 CET) to prepare the day's bias.
Payload: System Instruction (portfolio_review_update.md) + portfolio_state.json + Current Quote + Market Status.

## 59. Structured AI-Output Parsing & Confidence Scoring
Objective: Quantify the "Strength" of AI conviction before allowing state changes.
Mandatory Schema:
{
  "analysis": "string (Critique)",
  "recommendation": "BUY | SELL | UPDATE | HOLD",
  "action_command": "string",
  "confidence_score": 0.0,
  "risk_assessment": "LOW | MEDIUM | HIGH"
}
Guardrail: If confidence_score < 0.70, the bot MUST ignore the action_command and default to a HOLD status message.

## 60. The Semi-Autonomous Gate (Buy/Sell)
Objective: Human oversight for high-impact capital allocation.
Logic: Proposals for /buy or /sell require a [ ✅ EXECUTE ] button click in Telegram.
Staleness Protection: Buttons expire after 300s (Point 39). If expired, the bot posts: "⚠️ Action Canceled: AI Proposal for {{ticker}} is now stale."

## 61. The "Protected" Autonomous Ratchet (Update)
Objective: Auto-lock profits while respecting market noise and avoiding spread-wicks.
Logic:
The bot may auto-execute /update ONLY IF:
new_sl > current_sl (Monotonicity Check).
Buffer Rule: new_sl must be at least 1.5% below the current_market_price.
Frequency Limit: Maximum of one auto-update per ticker every 4 hours.
If these conditions aren't met, the update is downgraded to a Semi-Autonomous Gate (requires button click).

## 62. Telegram Telemetry (Tiered Reporting)
Objective: Optimize the Signal-to-Noise Ratio (SNR) in the Telegram Channel.
Logic:
Tier 1 (High Priority): Trade Proposals (BUY/SELL) or Auto-Ratchet executions. (Notification: ON).
Tier 2 (Medium Priority): Confidence > 0.7 but recommendation is HOLD. (Notification: SILENT).
Tier 3 (Low Priority): Confidence < 0.7 or Market Closed. (LOG ONLY).
Formatting:
🤖 AI Strategy Report: {{ticker}}
Conviction: {{confidence_score}} | Risk: {{risk_assessment}}
Critique: {{analysis}}

## 63. Fiscal Budget Hard-Stop
Objective: Enforce the $300 limit at the execution level.
Logic: Before executing any /buy (manual or AI), the bot MUST calculate: Current_Equity + Proposed_Order_Value. If total > $300, the order is blocked with the message: "❌ Budget Violation: Proposed trade exceeds $300 limit."

## 64. Manual AI-Directed Analysis (/analyze)
Objective: Provide a way to force an immediate "Portfolio Review" without waiting for the next WATCHER_POLL_INTERVAL.
Logic:
Trigger: Telegram command /analyze or /analyze <ticker>.
Execution:
Step 1: Bypass the "Temporal Gate" of Point 58 (allow execution even if the market is closed or it's not pre-market).
Step 2: Fetch the latest quotes for all active assets.
Step 3: Invoke the Gemini 2.5 Flash logic defined in Point 58/59.
Response: The bot MUST reply with the full Tier 1 Intelligence Report (Point 62) regardless of the confidence score.
Guardrail: To prevent "API Spamming" and excessive costs, the /analyze command MUST have a 600-second (10-minute) cooldown per user. If triggered during cooldown, the bot replies: "⏳ Analysis cooling down. Next available in {{remaining_seconds}}s."
Contextual Scope: If a ticker is provided (e.g., /analyze XBI), the AI prompt should be modified to focus specifically on that ticker's recent price action and sector news.

## 65. AI Budget Awareness (State Injection)
Objective: Prevent AI from recommending trades that exceed the fiscal limit.
Implementation:
Update PortfolioState struct to include FiscalLimit and AvailableBudget.
Logic: AvailableBudget = FiscalLimit - CurrentTotalExposure.
Injection: These two fields MUST be included in the JSON payload sent to Gemini during /analyze or the automated review loop.
Requirement: If AvailableBudget is less than the price of a single share of a recommended ticker, the AI must be instructed to return HOLD.

## 66. Temporal Stagnation Exit (The "Dead Money" Guard)
Objective: Free up capital from assets that show zero momentum over a long period.
Logic:
Add OpenedAt (timestamp) to Position struct.
Configuration: Add MAX_STAGNATION_HOURS to .env (Default: 120 - 5 trading days).
Trigger: If now - OpenedAt > MAX_STAGNATION_HOURS AND Current_Gain_Loss < 1.0% (flat), trigger an alert: "⏳ STAGNATION ALERT: {{ticker}} has been flat for 5 days. Consider manual liquidation to free up budget."
Note: This is a notification only, not an auto-sell, to preserve user intent.
update README.md file with relevant information.
update .env file with sample MAX_STAGNATION_HOURS value.
update project_log.md file.

## 67. AI-Driven Portfolio Rotation
Objective: Allow the AI to propose "Swaps" when capital is fully deployed but a higher-conviction opportunity arises.
Implementation:
In the AI System Instruction, explicitly permit the SELL recommendation for the sole purpose of capital rotation.
Logic: If AvailableBudget < Required_Entry AND a Pillar asset shows high conviction, the AI should identify the "weakest link" in the current portfolio (lowest P/L or highest stagnation) and recommend a /sell <weak_ticker> followed by a /buy <strong_ticker>.
UI: The bot must present these as a linked "Rotation Proposal" if possible, or two sequential proposals.
update README.md file with relevant information.
update .env file with sample MAX_STAGNATION_HOURS value.
update project_log.md file.

## 68. Just-In-Time (JIT) Broker Reconciliation
Objective: Eliminate discrepancies between Alpaca reality and local state regarding budget and positions.
Logic: The /status, /analyze, and /buy commands must trigger an internal syncWithBroker() call BEFORE processing logic. This fetches fresh GetAccount() (Buying Power/Equity) and ListPositions() from Alpaca.

## 69. Dynamic Budget Calculation
Implementation: AvailableBudget MUST be calculated as min(Alpaca_Buying_Power, FiscalLimit - CurrentTotalExposure). This ensures the bot respects both the broker's physical cash limits and the user's strategic $300 cap.

## 70. Budget-Aware Payload Construction
Implementation: When preparing the AI payload (Spec 58/64), the bot must use an in-memory Snapshot object populated by the Spec 68 JIT sync. This ensures the AI sees the "True" available budget, accounting for any manual trades performed outside the bot.

## 71. Strategic Exit Instruction (AI)
Implementation: If AvailableBudget < (Latest_Price * 1), the AI is instructed to return HOLD unless it identifies a viable Rotation (Spec 67).

## 72. Watchlist Price Grounding (Env & State)
Objective: Provide the AI with real-time price context for "Priority Watchlist" assets.
Implementation:
Configuration: Add WATCHLIST_TICKERS to .env (Comma-separated list, e.g., VRT,PLTR,BTC).
State: Add a watchlist_prices map (ticker: price) to the PortfolioState struct.
Refresh Logic: During the Spec 68 JIT Sync, the bot MUST fetch the LatestTrade for all tickers in WATCHLIST_TICKERS and update the local state.

## 73. Atomic AI Recommendation (The "Single Action" Rule)
Objective: Prevent the AI from proposing multiple trades that collectively violate the budget.
Logic:
Constraint: The AI is strictly limited to ONE primary recommendation per review cycle (BUY, SELL, or UPDATE).
Exception: A "Rotation" (SELL A + BUY B) is treated as a single atomic recommendation.
Instruction: Update the AI System Instruction to explicitly forbid multi-buy lists (e.g., "BUY VRT and BUY PLTR") to prevent budget race conditions.

## 74. Price-Aware Payload (Context Injection)
Implementation:
When generating the JSON payload for Gemini, the Snapshot must include the watchlist_prices map.
The prompt must instruct the AI: "Use the provided watchlist_prices to calculate the total cost of your recommendation. Your total proposed cost MUST be < available_budget."

## 75. Batch Order Safety Gate
Objective: Hard-stop any AI command that contains multiple /buy strings if they are not part of a validated Rotation.
Logic: If the action_command from Spec 59 contains more than one /buy instruction, the bot must reject it and log: [AI_LOGIC_ERROR] Multiple buy orders suggested without rotation.

## 76. Broker-Synchronized Intent Mutation (/update)
Objective: Ensure manual risk parameter updates are validated against real-time market reality.
Implementation:
JIT Price Fetch: Before validating any /update <ticker> <sl> <tp> command, the bot MUST perform a fresh Alpaca REST call (GetLatestTrade) for the ticker.
Validation Snapshot: Use the price from the JIT fetch (the "Snapshot Price") to evaluate the Safety Gates defined in Spec 51.
SL Guardrail: Reject the update if new_sl >= Snapshot Price.
TP Guardrail: Reject the update if new_tp <= Snapshot Price.
State Integrity: Only update portfolio_state.json if both gates pass against the real-time Snapshot Price.
Feedback: If validation fails, the Telegram response must include the Snapshot Price used for the rejection (e.g., "❌ Rejected: New SL $175 is above current price $174.20").

## 77. Budget Reconciliation & The "Ghost Money" Fix
Objective: Resolve discrepancies between strategic limits (fiscal_limit) and broker reality (Equity).
Logic:
Redundancy Review: Do NOT delete fiscal_limit. It remains the strategic "Straitjacket" for the bot.
Dynamic Re-binding: The AvailableBudget calculation (Spec 69) is further refined.
The Formula:
Real_Cap = min(Alpaca_Equity, fiscal_limit)
AvailableBudget = Real_Cap - CurrentTotalExposure
AI Sync: The /status and AI Payload must clearly differentiate between "Strategic Limit" ($300) and "Broker Equity" ($296.68).
Safety Gate: If Alpaca_Equity falls below fiscal_limit due to losses, the bot MUST automatically shrink its "operating theater" to the lower value. This prevents the AI from recommending trades based on "Ghost Money" (the $3.32 gap in the logs).

## 78. Priority Watchlist Price Guardrail (Log Fix)
Objective: Prevent AI from entering "HOLD" states due to null watchlist data (as seen in logs).
Implementation:
Pre-Flight Check: The /analyze and polling loops MUST verify that watchlist_prices is populated before calling the AI.
Error Handling: If WATCHLIST_TICKERS is defined in .env but watchlist_prices is empty or null in the JSON, the bot must log a [CRITICAL_DATA_MISSING] error and attempt a forced price refresh before proceeding.

## 79. Decommission of Spec 75 (Multi-Buy Permission)
Objective: Allow the AI to suggest multiple entries in a single review cycle to maximize capital deployment efficiency.
Logic:
Decommission: Spec 75 (Batch Order Safety Gate) is hereby marked as OBSOLETE.
Permission: The AI is now permitted to return multiple /buy commands in its action_command field.
UI Requirement: The Telegram handler must parse the action_command string (e.g., delimited by ;) and present each order as an independent confirmation card or a single "Batch Execution" card.

## 80. Aggregate Budget Validation (Batch Safety)
Objective: Prevent partial execution failures when multiple trades are proposed.
Implementation:
Summation Logic: Before presenting any [✅ EXECUTE] buttons for a multi-order recommendation, the bot MUST calculate the total aggregate cost: Total_Batch_Cost = Σ(qty_i * price_i).
Hard-Stop: If Total_Batch_Cost > AvailableBudget, the bot must reject the entire recommendation with: "❌ Batch Rejection: Total cost ${{total}} exceeds available budget ${{budget}}."
Verification: This check must use fresh prices (Spec 72) to ensure math remains valid at the moment of proposal.

## 81. Sequential Execution Threading
Objective: Ensure multiple orders are processed without "Locked Shares" errors (Spec 54).
Implementation:
If a user confirms a "Batch," the bot must execute them sequentially, awaiting the "Filled" or "Accepted" status of Order N before initiating Order N+1.

## 82. Stop-Loss (SL) Monotonicity Guardrail
Objective: Prevent "SL Decay" where the exit floor is lowered during a price drop.
Logic:
Safety Gate: In the /update logic and any AI-directed SL update, the bot MUST validate that New_SL >= Current_SL.
Enforcement: If New_SL < Current_SL, the bot must reject the update with a [CRITICAL_RISK_VIOLATION] error.
Exception: The only permitted downward move is if a position is completely closed and re-opened.
AI Instruction: Update the AI prompt to explicitly state: "You are FORBIDDEN from lowering a Stop Loss once it is set. If the market moves against a position, either HOLD or recommend SELL."

## 83. Transition to Full Autonomy (The "Auto-Pilot" Shift)
Objective: Remove human intervention from the trade execution loop.
Logic:
Decommission: Spec 18, 60, and 38 (Confirm-to-Trade gates) are now toggled to AUTOMATIC for AI-driven actions.
Bypass: The CONFIRMATION_TTL_SEC (Spec 39) is ignored for autonomous actions.
Execution: Upon receiving a valid AI recommendation (Spec 59) with confidence_score >= 0.70, the bot MUST immediately initiate the action_command sequence without waiting for a Telegram callback.

## 84. Autonomous Execution Pipeline
Implementation:
The bot must parse the action_command and execute sequentially (Spec 81).
Audit Trail: Every autonomous action MUST be preceded by a Telegram notification: "🤖 AI EXECUTION START: {{ticker}} | {{action}}."
Result Reporting: Upon completion (or failure), the bot must send a follow-up: "✅ AI EXECUTION SUCCESS" or "❌ AI EXECUTION FAILED: {{reason}}."

## 85. Autonomous Slippage & Liquidity Guardrails
Objective: Prevent "Bad Fills" in an un-attended environment.
Implementation:
Slippage Gate: Before executing an autonomous Market Order, the bot must fetch the latest Bid/Ask spread.
Constraint: If (Ask - Bid) / Bid > 0.005 (0.5% spread), the bot must ABORT the autonomous execution and notify the user: "⚠️ High Spread detected. Autonomy paused for {{ticker}}."
Price Protection: The Deviation Gate (Spec 18.4) must be checked programmatically against the watchlist_prices (Spec 72) immediately before the order call.

## 86. The "Emergency Brake" (Global Killswitch)
Objective: Allow the user to stop the autonomous bot instantly via Telegram.
Command: /stop.
Logic:
Setting this flag in memory MUST prevent all future autonomous executions.
The bot must respond with: "🛑 AUTONOMY DISABLED. Revert to manual mode."
Command: /start re-enables autonomous mode.

## 87. Telegram Role Reversal (Notification First)
Objective: Redefine the UI to prioritize telemetry over interaction.
Implementation:
The bot remains passive in Telegram until an action is taken or a critical error occurs.
The /status and /portfolio commands remain active for manual oversight.
Manual trades (/buy, /sell, /update) still take absolute precedence over pending AI logic.

## 88. Broker-as-Truth (State Decommissioning)
Objective: Shift the source of truth for positions and risk parameters from local JSON to the Alpaca Broker.
Implementation:
Decommission: The "Virtual" SL/TP monitoring logic (Spec 20/21) is deprecated.
Truth Source: The bot must rely exclusively on alpaca.ListPositions() and alpaca.ListOrders() to determine current risk.
Sync: The portfolio_state.json is now relegated to a "History & Metadata" log rather than an active monitoring control.

## 89. Native Broker Risk Management (Bracket Orders)
Objective: Leverage Alpaca's native server-side SL/TP functionality.
Implementation:
Order Construction: Every /buy command (Manual or AI) MUST be executed as an Alpaca Bracket Order.
Parameters: stop_loss and take_profit prices must be passed directly to the alpaca.PlaceOrder request using the StopLoss and TakeProfit nested structs.
Legacy Position Handling: For any position "discovered" without a native bracket, the bot must attempt to place a LimitOrder (TP) and StopOrder (SL) manually to "Wrap" the position in broker-side protection.

## 90. Removal of Fiscal Guardrails (Account-Scale Trading)
Objective: Allow the bot to trade the full available balance of the Alpaca account.
Implementation:
Decommission: Spec 63 ($300 Hard-Stop) is removed.
Budget Logic: The AvailableBudget calculation (Spec 69/77) is simplified to AvailableBudget = Alpaca_Buying_Power.
Sizing: The AI is now instructed to use the full account_equity for its allocation math.

## 91. Autonomous Rotation Resilience
Objective: Ensure the bot can rotate capital between "locked" native orders.
Implementation:
Before executing a /buy that requires a preceding /sell (Rotation - Spec 67), the bot MUST explicitly check for and cancel any open orders (Bracket, Stop, Limit) associated with the ticker being sold.
Wait for order cancellation confirmation before proceeding to the liquidation and subsequent purchase.

## 92. Total Liquidation of Shadow Budget Logic
Objective: Purge all remaining "Fiscal Limit" and "Hard-Stop" logic to ensure the broker is the sole source of truth for capital.
Implementation:
Decommission: Remove the fiscal_limit field from PortfolioState struct and the portfolio_state.json schema.
Logic Refactor: All functions previously checking AvailableBudget (Spec 22, 26, 63, 65, 69) MUST be refactored to call alpaca.GetAccount() directly.
The "Truth" Variable: AvailableBudget is now a volatile, runtime-only value derived from account.NonMarginableBuyingPower.
AI Instruction: Update the AI system instruction to stop referencing a "$300 limit" and instead focus on the account_equity and buying_power provided in the dynamic JSON payload.

## 93. Multi-Broker SL/TP Abstraction
Objective: Standardize native risk management across different brokerages via the MarketProvider interface.
Implementation:
Interface Update: Add UpdatePositionRisk(ticker string, sl, tp decimal.Decimal) error to the MarketProvider interface.
Alpaca Implementation: This method must cancel existing Stop or Limit orders for the ticker and place a new Bracket Order or associated "Oversight" orders.
Fallthrough: If a broker doesn't support native brackets, the provider must implement a "Virtual" fallback or return a NotSupported error.

## 94. Command Suite Refactoring (Gen 2)
Objective: Streamline the Telegram interface for autonomous operation.
Implementation:
Consolidation:
/status: Now the primary dashboard. Integrates account equity, market clock, and active positions. Replaces the functionality of /portfolio.
/scan: The new trigger for AI analysis. Replaces /analyze. It must fetch fresh data, perform the review, and (if autonomy is on) execute trades.
Removals: Delete the following commands from the registry: /list, /price, /market, /search, /portfolio, /analyze, /refresh.
New Command: /config: Displays current operational settings (PollInterval, LogLevel, etc.). MANDATORY: Mask all secrets (API Keys, Tokens) using a maskSecret helper (e.g., APCA_...XXXX).

## 95. Help Registry Update (Gen 2)
Objective: Maintain accurate documentation for the streamlined interface.
Registry (Required):
/ping: Connectivity check.
/status: Detailed broker-native dashboard.
/buy <ticker> <qty> [sl] [tp]: Manual entry (native bracket).
/sell <ticker>: Universal exit (cancels orders + liquidates).
/update <ticker> <sl> <tp>: Mutate native risk parameters.
/scan: Trigger AI Portfolio Review & Autonomous Rotation.
/stop: Killswitch. Disables all autonomous execution.
/start: Enable autonomous execution.
/config: Inspect system parameters.

## 96. Autonomous Start/Stop Persistence
Objective: Ensure the "Emergency Brake" survives a bot restart.
Implementation:
Add AutonomyEnabled (bool) to portfolio_state.json.
The /stop command sets this to false and saves state.
The /start command sets this to true and saves state.
Logic Guard: The autonomous loop (Spec 83) MUST check this flag before calling the AI or executing trade commands.