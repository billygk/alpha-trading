# Alpha Watcher

![License](https://img.shields.io/badge/license-MIT-blue.svg) ![Go Version](https://img.shields.io/badge/go-1.22+-00ADD8.svg)|[![Release Build](https://github.com/billygk/alpha-trading/actions/workflows/release.yml/badge.svg)](https://github.com/billygk/alpha-trading/actions/workflows/release.yml)

**Alpha Watcher** is a high-availability, attended-automation trading supervisor written in Go. It acts as a bridge between your brokerage (Alpaca) and your mobile device (Telegram), providing real-time risk management, interactive trade execution, and automated state synchronization.

It is designed for traders who want the precision of algorithmic execution (trailing stops, instant calculations) with the safety of human confirmation (`Confirm-to-Trade`).

---

## 🌟 Key Features

### 🛡️ Automated Risk Management
- **Polling Loop**: Checks positions every hour (configurable) for Stop Loss (SL), Take Profit (TP), and Trailing Stop (TS) triggers.
- **Precedence Logic**: Prioritizes `TP > SL > TS` to maximize profit capture while guaranteeing protection.
- **Universal Temporal Gate**: All actionable alerts typically expire after 5 minutes (TTL) to prevent stale execution.
- **Alert Fatigue Prevention**: intelligently suppresses duplicate alerts for the same position within a 15-minute window.
- **HWM Monotonicity Guardrail**: Ensures the "High Water Mark" used for trailing stops never decreases due to systematic errors, guaranteeing the integrity of the trailing stop floor.
- **Temporal Stagnation Exit**: Monitors positions for "Dead Money" (held > 5 days with < 1% movement) and alerts you to liquidate them to free up capital (Spec 66).

### 💬 Interactive Telegram Control
- **Proposed Trades**: Use `/buy` to get a calculated trade proposal with risk/reward ratios before you commit.
- **One-Tap Execution**: Execute or Cancel trades directly from Telegram buttons.
- **Live Dashboard**: Get a full portfolio P/L and risk overview with `/status`.

### 🔄 Strict Exchange Synchronization
- **Mirror Sync**: The `/refresh` command forces the bot to align its local state 100% with the broker.
- **Auto-Discovery**: New positions opened manually on the broker are automatically imported and assigned default safety limits.
- **Cost-Basis Truth**: Uses the broker's `AvgEntryPrice` to ensure P/L calc matches your official dashboard.

### ⚙️ HFT-Grade Execution Reliability
- **Just-In-Time (JIT) Sync**: Automatically reconciles with the broker (Alpaca) immediately before critical actions (`/buy`, `/status`, `/analyze`) to ensure budget decisions are based on the absolute latest data (Spec 68).
- **Sequential Clearance**: Automatically cleans up "Zombie Orders" before placing new ones to prevent position locking.
- **Validation Loop**: Confirms trades are actually `Filled` on the exchange.

### 💰 Fiscal Discipline
- **Dynamic Budgeting**: Strict adherence to a logic of `Available = min(BuyingPower, FiscalLimit - Exposure)`. This prevents the bot from ever exceeding your global risk cap ($300 default) regardless of broker buying power (Spec 69).

---

## 🤖 AI Analysis & Guardrails (Beta)

**Alpha Watcher** integrates with Gemini 1.5 Pro to provide periodic portfolio reviews during market hours.

### Analysis Loop
- **Trigger**: Runs every hour during Market Open (and Pre-Market).
- **Logic**: Analyzes technical structure and P/L to recommend `BUY`, `SELL`, `UPDATE`, or `HOLD`.
- **Confidence Gate**: Recommendations with `< 0.70` confidence are ignored.
- **Portfolio Rotation**: Identifies opportunity costs. If budget is full, the AI searches for "weakest links" (stagnant or underperforming) and recommends rotating capital into higher-conviction opportunities (Spec 67).

### Automation Levels
1.  **Fully Autonomous Execution (Buy/Sell/Update)**:
    -   If Confidence >= 0.70 AND Autonomy Enabled: AI executes immediately.
    -   Includes strict Slippage Checks (Ask/Bid spread < 0.5%).
    -   Uses Native Bracket Orders (SL/TP) on the broker side.
2.  **Semi-Autonomous Fallback**: If Confidence < 0.70 or Autonomy Disabled, AI falls back to proposing trades via Telegram buttons.
3.  **Killswitch**: Use `/stop` to force manual mode instantly.

### Financial Guardrails
- **Dynamic Account Sizing**: The bot uses the full `Buying Power` available in the brokerage account (Spec 90). The previous $300 limit has been removed.
- **Aggregate Batch Budget**: AI can propose multiple buys, but the *sum* of their costs is validated against available Buying Power before execution (Spec 80).
- **Sequential Execution**: All batch orders are executed one-by-one with strict verification ("Filled") between steps to prevent race conditions (Spec 81).
- **SL Monotonicity**: The bot actively FORBIDS lowering a Stop Loss once set ("SL Decay") to prevent risk expansion (Spec 82).

---

## 🚀 Getting Started

### Prerequisites
1.  **Go 1.22+**: [Install Go](https://go.dev/doc/install).
2.  **Alpaca Account**: You need an API Key & Secret (Paper Trading recommended for testing).
3.  **Telegram Bot**: Create a bot via `@BotFather` and get your `CHAT_ID`.

### Installation

1.  **Clone the repository**
    ```bash
    git clone https://github.com/yourusername/alpha-trading.git
    cd alpha-trading
    ```

2.  **Setup Configuration**
    Create a `.env` file in the root directory:
    ```env
    # Alpaca Credentials
    APCA_API_KEY_ID=your_alpaca_key
    APCA_API_SECRET_KEY=your_alpaca_secret
    APCA_API_BASE_URL=https://paper-api.alpaca.markets
    
    # Telegram Credentials
    TELEGRAM_BOT_TOKEN=your_bot_token
    TELEGRAM_CHAT_ID=your_chat_id
    
    # AI Configuration
    GEMINI_API_KEY=your_gemini_key
    WATCHLIST_TICKERS=VRT,PLTR,BTC

    # Optional Overrides (Defaults shown)
    WATCHER_LOG_LEVEL=INFO
    WATCHER_POLL_INTERVAL=60
    AUTO_STATUS_ENABLED=true
    ```

3.  **Run the Bot**
    ```bash
    go run ./cmd/alpha_watcher
    ```

---

## 🛠️ Configuration Reference

| Variable | Default | Description |
|----------|---------|-------------|
| `WATCHER_LOG_LEVEL` | `INFO` | `DEBUG` shows full Telegram payloads. `INFO` is standard. |
| `WATCHER_POLL_INTERVAL` | `60` | Minutes between automatic price/risk checks. |
| `CONFIRMATION_TTL_SEC` | `300` | Seconds before an interactive "Confirm" button expires. |
| `DEFAULT_STOP_LOSS_PCT` | `5.0` | Default SL % applied to new or simplified orders. |
| `DEFAULT_TAKE_PROFIT_PCT` | `15.0` | Default TP % applied to new or simplified orders. |
| `DEFAULT_TRAILING_STOP_PCT` | `3.0` | Default Trailing Stop % applied to new or simplified orders. |
| `AUTO_STATUS_ENABLED` | `false` | If `true`, pushes the `/status` dashboard after every poll (during market hours). |
| `MAX_STAGNATION_HOURS` | `120` | Minimum hours a position must be held before checking for stagnation (Spec 66). |
| `GEMINI_MODEL` | `gemini-1.5-flash` | The Gemini model version to use for AI analysis (e.g. `gemini-2.5-pro`). |
| `WATCHLIST_TICKERS` | `""` | Comma-separated list of symbols (e.g., `VRT,PLTR`) for AI price-grounding (Spec 72). |

---

## 🤖 Command Reference

Interact with the bot using these Telegram commands:

### `/status`
Displays the **Live Dashboard**.
- Shows Market Status (Open/Closed).
- Lists all active positions with Day P/L, Total P/L, and distance to Stop Loss.
- Shows total Account Equity.

### `/buy <ticker> <qty> [sl] [tp]`
Proposes a new long position.
- **Example**: `/buy AAPL 10` (Uses default SL/TP)
- **Example**: `/buy TSLA 5 180 250` (Manual specific prices)
- **Response**: A card with calculated totals and risk metrics. Click **✅ EXECUTE** to place the Market Order.

### `/sell <ticker>`
**Universal Exit**. Liquidates position, cancels pending orders, and **purges** local state (Spec 57). Archives deleted position to `daily_performance.log`.

### `/refresh`
Force-syncs local state with Alpaca.
- **Note**: Accepts NO parameters. To change risk settings, use `/sell` then `/buy`.
- **Clean**: Removes local positions not found on broker.
- **Import**: Adds broker positions not found locally (assigns default SL/TP).
- **Update**: Re-syncs `Qty` and `EntryPrice`.

### `/update <ticker> <sl> <tp> [ts_pct]`
Manually update the risk parameters for an active position.
- **Safety Gates**: Validates that `New SL < Current Price` and `New TP > Current Price`.
- **Example**: `/update NVDA 120 160 5` (Set SL $120, TP $160, TS 5%)


### `/scan <sector>`
(Experimental) Checks sector health/sentiment.

### `/analyze [ticker]`
(Spec 64) Manually trigger an AI Portfolio Review.
- **Cool-down**: 10 minutes between calls.
- **Context**: Optional [ticker] focuses the AI's analysis on a specific asset.
- **Bypass**: Runs even if market is closed (Temporal Gate Override).

### `/portfolio`
Dump the raw `portfolio_state.json` file for debugging purposes.
- **Chunking**: Output is split into multiple messages if the file exceeds 3900 characters.

### `/stop`
**Emergency Brake**. Disables autonomous execution immediately. The bot reverts to Manual/Suggestion mode.

### `/start`
Re-enables autonomous execution.

---

## 🏗️ Architecture

The system operates on an **Event Loop** (Telegram Listener) and a **Polling Loop** (Watcher).

1.  **Watcher Loop**: Wakes up every `WATCHER_POLL_INTERVAL`.
    - Fetches market data (Alpaca Data API).
    - Checks `CurrentPrice` vs `StopLoss` / `TakeProfit` / `TrailingStop`.
    - If Triggered -> Sends Interruptable Alert to Telegram.
    - If Alert Confirmed by User -> Executes Trade.

2.  **Listener Loop**: Long-polls Telegram for updates.
    - Parses commands (`/buy`, `/status`).
    - Handles Button Callbacks (`EXECUTE`, `CANCEL`).
    - Enforces TTL (Temporal Gates) on all interactions.
