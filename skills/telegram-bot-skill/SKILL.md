---
name: telegram-bot-skill
description: >
  Scaffold and configure a Telegram bot bridge that connects to an OpenCode backend server.
  Built in Go. Trigger: When a user wants to interact with OpenCode via Telegram, create a
  Telegram bot that forwards messages to OpenCode and returns AI responses.
license: Apache-2.0
metadata:
  author: jorgehara
  version: "1.0"
  reference: https://github.com/jorgehara/gentle-ia-telegram-bot-skill
---

# Telegram Bot Skill

## When to Use

Load this skill when the user wants to:
- Interact with OpenCode from Telegram (mobile-first workflow)
- Create a Telegram bot that bridges to OpenCode's HTTP API
- Set up a private AI assistant accessible from any device via Telegram

---

## Prerequisites

Before scaffolding, verify the user has:

| Requirement | How to Check | Install |
|-------------|-------------|---------|
| Go 1.21+ | `go version` | [go.dev/dl](https://go.dev/dl) |
| OpenCode | `opencode --version` | `npm install -g opencode-ai` |
| Telegram Bot Token | Have it ready | Create via [@BotFather](https://t.me/BotFather) |

---

## Architecture

```
┌──────────────────┐   Telegram Bot API   ┌──────────────────────────┐   REST HTTP   ┌───────────────────┐
│  Telegram User   │ ◄──────────────────► │  Go Bridge               │ ◄───────────► │  OpenCode Server  │
│  (any device)    │    Long Polling      │  (this skill)            │  Port 4096    │  (opencode serve) │
└──────────────────┘                      └──────────────────────────┘               └───────────────────┘
```

### Data Flow

1. User sends a message to the Telegram bot
2. Bridge validates the sender against the whitelist (`ALLOWED_CHAT_IDS`)
3. Bridge creates or retrieves an OpenCode session for that chat ID
4. Message is forwarded to `POST /session/{id}/message` on the OpenCode server
5. OpenCode processes the prompt and returns the AI response
6. Bridge sends the response back to Telegram (with MarkdownV2 escaping)

---

## Project Structure

```
telegram-bot/
├── main.go          # Entry point — init, health check, graceful shutdown
├── config.go        # Env var loading, whitelist helper, Config struct
├── telegram.go      # Bot polling, message handlers, command routing
├── opencode.go      # OpenCode HTTP client, session management
├── go.mod           # Module definition
├── .env.example     # Environment variable template
├── .gitignore       # Excludes .env and compiled binaries
├── start-stack.bat  # Windows: start OpenCode + bot in two terminals
├── add-startup.ps1  # Windows: add bridge to system startup
└── INSTALL.md       # Full installation and configuration guide
```

---

## Scaffolding Steps

### Step 1 — Initialize the module

```bash
mkdir telegram-bot && cd telegram-bot
go mod init github.com/<username>/telegram-bot
go get github.com/go-telegram-bot-api/telegram-bot-api/v5
go get github.com/joho/godotenv
```

### Step 2 — Create `config.go`

Responsible for loading all configuration from environment variables with safe defaults.

```go
package main

import (
    "log"
    "os"
    "strconv"
    "strings"
    "time"

    "github.com/joho/godotenv"
)

type Config struct {
    BotToken       string
    AllowedChatIDs []int64
    OpencodeURL    string
    OpencodeUser   string
    OpencodePass   string
    ProjectDir     string
    HTTPPort       int
    Markdown       bool
    Debug          bool
}

func LoadConfig() *Config {
    _ = godotenv.Load() // silent if .env missing

    token := os.Getenv("TELEGRAM_BOT_TOKEN")
    if token == "" {
        log.Fatal("TELEGRAM_BOT_TOKEN is required")
    }

    return &Config{
        BotToken:       token,
        AllowedChatIDs: parseIDs(os.Getenv("ALLOWED_CHAT_IDS")),
        OpencodeURL:    getEnv("OPENCODE_URL", "http://localhost:4096"),
        OpencodeUser:   getEnv("OPENCODE_USERNAME", "opencode"),
        OpencodePass:   os.Getenv("OPENCODE_PASSWORD"),
        ProjectDir:     getEnv("OPENCODE_PROJECT_DIR", "."),
        HTTPPort:       getInt("BRIDGE_PORT", 8080),
        Markdown:       getBool("ENABLE_MARKDOWN", true),
        Debug:          getBool("DEBUG", false),
    }
}

func (c *Config) IsAllowed(chatID int64) bool {
    if len(c.AllowedChatIDs) == 0 {
        return true
    }
    for _, id := range c.AllowedChatIDs {
        if id == chatID {
            return true
        }
    }
    return false
}

// helpers: getEnv, getInt, getBool, parseIDs (standard env parsing)
```

### Step 3 — Create `opencode.go`

HTTP client for the OpenCode REST API. Key methods:

```go
// HealthCheck — GET /global/health
func (c *OpencodeClient) HealthCheck(ctx context.Context) error

// GetOrCreateSession — manages one session per chat ID
// POST /session on first call, cached thereafter
func (c *OpencodeClient) GetOrCreateSession(ctx context.Context, chatID int64, dir string) (string, error)

// SendPrompt — POST /session/{id}/message
// Sends text, waits for response, extracts text parts
func (c *OpencodeClient) SendPrompt(ctx context.Context, sessionID, text string) (string, error)

// AbortSession — POST /session/{id}/abort
func (c *OpencodeClient) AbortSession(ctx context.Context, sessionID string) error

// ClearSession — removes session from local cache
func (c *OpencodeClient) ClearSession(chatID int64)
```

### Step 4 — Create `telegram.go`

Bot handler with command routing:

```go
// Commands handled locally (never forwarded to OpenCode)
// /start  → sendWelcomeMessage
// /id     → handleGetID (shows Chat ID for whitelist config)
// /reset  → handleReset (clears session)
// /abort  → handleAbort (calls session abort)

// All other text → forwarded to OpenCode via SendPrompt
```

**Critical pattern** — always use a real `context.Context`, never `nil`:

```go
ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
defer cancel()
sessionID, err := client.GetOrCreateSession(ctx, chatID, cfg.ProjectDir)
```

**MarkdownV2 escaping** — escape all reserved characters before sending:

```go
func EscapeMarkdownV2(text string) string {
    replacer := strings.NewReplacer(
        "_", "\\_", "*", "\\*", "[", "\\[", "]", "\\]",
        "(", "\\(", ")", "\\)", "~", "\\~", "`", "\\`",
        ">", "\\>", "#", "\\#", "+", "\\+", "-", "\\-",
        "=", "\\=", "|", "\\|", "{", "\\{", "}", "\\}",
        ".", "\\.", "!", "\\!",
    )
    return replacer.Replace(text)
}
```

### Step 5 — Create `main.go`

```go
func main() {
    cfg := LoadConfig()
    client := NewOpencodeClient(cfg)

    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    if err := client.HealthCheck(ctx); err != nil {
        log.Printf("Warning: OpenCode not reachable: %v", err)
    }

    bot, err := NewTelegramBot(cfg, client)
    if err != nil {
        log.Fatalf("Bot init failed: %v", err)
    }

    go bot.StartPolling()

    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit

    bot.Stop()
}
```

### Step 6 — Build and Run

```bash
# Build
go build -o telegram-bot .

# Run (with .env file present)
./telegram-bot

# Or pass env inline
TELEGRAM_BOT_TOKEN=your_token OPENCODE_URL=http://localhost:4096 ./telegram-bot
```

---

## Configuration Reference

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `TELEGRAM_BOT_TOKEN` | ✅ | — | Bot token from @BotFather |
| `OPENCODE_URL` | No | `http://localhost:4096` | OpenCode server URL |
| `OPENCODE_USERNAME` | No | `opencode` | Basic auth username |
| `OPENCODE_PASSWORD` | No | (empty) | Basic auth password |
| `ALLOWED_CHAT_IDS` | No | (empty = all) | Comma-separated whitelist |
| `OPENCODE_PROJECT_DIR` | No | `.` | Working directory for sessions |
| `BRIDGE_PORT` | No | `8080` | HTTP port for the bridge |
| `ENABLE_MARKDOWN` | No | `true` | MarkdownV2 formatting |
| `DEBUG` | No | `false` | Verbose logging |

### Getting Your Chat ID

1. Start the bot **without** `ALLOWED_CHAT_IDS`
2. Send `/id` to your bot in Telegram
3. Copy the Chat ID from the response
4. Set `ALLOWED_CHAT_IDS=<your-id>` in `.env`
5. Restart the bot

---

## Bot Commands

| Command | Handled by | Description |
|---------|------------|-------------|
| `/start` | Bridge | Welcome message |
| `/id` | Bridge | Shows your Chat ID |
| `/reset` | Bridge | Resets OpenCode session |
| `/abort` | Bridge | Aborts current operation |
| *(any text)* | OpenCode | Forwarded as AI prompt |

---

## Security Patterns

### Chat Whitelist

```go
func (b *TelegramBot) handleMessage(msg *tg.Message) {
    if !b.Config.IsAllowed(msg.Chat.ID) {
        b.sendMessage(msg.Chat.ID, "🚫 Unauthorized.")
        return
    }
    // ...
}
```

### Concurrency Lock (one prompt per chat)

```go
b.mu.Lock()
if b.processing[chatID] {
    b.mu.Unlock()
    b.sendMessage(chatID, "⏳ Previous message still processing...")
    return
}
b.processing[chatID] = true
b.mu.Unlock()
defer func() {
    b.mu.Lock()
    delete(b.processing, chatID)
    b.mu.Unlock()
}()
```

---

## Auto-Start (Windows)

Two helper scripts are included for Windows users:

**`start-stack.bat`** — opens two terminals:
1. `opencode serve` (OpenCode backend)
2. `./telegram-bot.exe` (the bridge)

**`add-startup.ps1`** — creates a Windows startup shortcut so the stack launches automatically on login.

```powershell
powershell -ExecutionPolicy Bypass -File add-startup.ps1
```

---

## Known Limitations

| Limitation | Notes |
|------------|-------|
| Sessions are in-memory | Lost on restart — `/reset` creates a new one |
| Text only | No file/image forwarding (yet) |
| Single message per chat | Sequential, not parallel per chat |
| Long polling only | Webhook mode not implemented |

---

## Reference Implementation

A complete, tested implementation is available at:

**[github.com/jorgehara/gentle-ia-telegram-bot-skill](https://github.com/jorgehara/gentle-ia-telegram-bot-skill)**

Tested on Windows (Go 1.26), compatible with Linux and macOS.

---

## Related OpenCode API Endpoints

| Method | Endpoint | Used For |
|--------|----------|---------|
| `GET` | `/global/health` | Health check on startup |
| `POST` | `/session` | Create a new session |
| `POST` | `/session/{id}/message` | Send prompt, get response |
| `POST` | `/session/{id}/abort` | Abort in-flight prompt |
