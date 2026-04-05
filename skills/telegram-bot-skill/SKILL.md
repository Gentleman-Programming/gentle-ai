---
name: telegram-bot-skill
description: >
  Generate a complete Telegram bot bridge that connects to OpenCode server via HTTP.
  Built in Go with session management, command routing, and Windows auto-start support.
  This skill provides explicit, unambiguous instructions for AI agents to scaffold the
  entire implementation from scratch.
license: Apache-2.0
metadata:
  author: jorgehara
  version: "2.0"
  reference: https://github.com/jorgehara/gentle-ia-telegram-bot-skill
---

# Telegram Bot Skill

## Purpose

This skill enables AI agents to generate a **production-ready Telegram bot** that acts as a bridge between Telegram's messaging platform and an OpenCode server running on `localhost:4096`.

**Key capabilities:**
- Message forwarding from Telegram to OpenCode
- Session management (one session per chat ID)
- Command routing (local commands vs. OpenCode prompts)
- Chat whitelist security
- Windows auto-start scripts with health checks
- MarkdownV2 response formatting

---

## When to Use This Skill

Load this skill when the user requests:
- "Create a Telegram bot that connects to OpenCode"
- "I want to chat with OpenCode from my phone via Telegram"
- "Set up a Telegram bridge for OpenCode"
- "Make a WhatsApp/Telegram bot for AI interactions" (adapt for Telegram)

**Trigger phrases:** telegram bot, telegram bridge, opencode telegram, mobile AI assistant

---

## Architecture Overview

```
┌──────────────────┐                    ┌──────────────────────────┐                  ┌───────────────────┐
│  Telegram User   │   Bot API          │  Go Bridge (this skill)  │   HTTP REST      │  OpenCode Server  │
│  (any device)    │ ◄────────────────► │  - Long polling          │ ◄──────────────► │  (localhost:4096) │
│                  │   HTTPS            │  - Session cache         │   POST /session  │                   │
└──────────────────┘                    │  - Command router        │   POST /message  └───────────────────┘
                                        └──────────────────────────┘
```

**Data flow:**
1. User sends message to Telegram bot
2. Bot validates chat ID against whitelist
3. Bot gets or creates OpenCode session for that chat
4. Message forwarded to `POST /session/{id}/message`
5. OpenCode processes prompt and returns response
6. Bot sends response back to Telegram with MarkdownV2 formatting

---

## Project Structure

When scaffolding, create this EXACT structure:

```
telegram-bot/
├── main.go                # Entry point, health check, signal handling
├── config.go              # Configuration loading and validation
├── telegram.go            # Telegram bot logic, command routing, message handlers
├── opencode.go            # OpenCode HTTP client with session management
├── go.mod                 # Go module definition
├── go.sum                 # Dependency checksums (auto-generated)
├── .env.example           # Environment variable template
├── .gitignore             # Git ignore rules
├── start-stack.bat        # Windows launcher with health check
├── add-startup.ps1        # Windows startup installer
└── README.md              # User documentation
```

---

## Prerequisites Check

Before scaffolding, verify these requirements with the user:

| Requirement | Check Command | Install Link |
|-------------|---------------|--------------|
| Go 1.21+ | `go version` | https://go.dev/dl |
| OpenCode CLI | `opencode --version` | `npm install -g opencode-ai` |
| Telegram Bot Token | User provides | https://t.me/BotFather |

If any requirement is missing, STOP and guide the user to install it first.

---

## Step-by-Step Scaffolding Instructions

### Step 1: Initialize the Go Module

```bash
mkdir telegram-bot
cd telegram-bot
go mod init github.com/<username>/telegram-bot
go get github.com/go-telegram-bot-api/telegram-bot-api/v5
go get github.com/joho/godotenv
```

**Critical:** Replace `<username>` with the actual GitHub username or use a generic path like `example.com/telegram-bot`.

---

### Step 2: Create `config.go`

This file handles ALL environment variable loading with safe defaults and validation.

**IMPORTANT PATTERNS:**
- Use `godotenv.Load()` silently (errors are OK if `.env` doesn't exist)
- ALWAYS validate required fields (fail fast if missing)
- Provide sensible defaults for optional fields
- Parse complex types (int64 slices, booleans) with helper functions

**Full implementation:**

```go
package main

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration
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

// LoadConfig loads configuration from environment variables
// Fails fast if required variables are missing
func LoadConfig() *Config {
	// Load .env file if present (silent fail is OK)
	_ = godotenv.Load()

	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("❌ TELEGRAM_BOT_TOKEN is required")
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

// IsAllowed checks if a chat ID is in the whitelist
// Empty whitelist = all allowed
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

// Helper functions for environment variable parsing

func getEnv(key, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultValue
}

func getInt(key string, defaultValue int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultValue
}

func getBool(key string, defaultValue bool) bool {
	if val := os.Getenv(key); val != "" {
		return val == "true" || val == "1" || val == "yes"
	}
	return defaultValue
}

func parseIDs(s string) []int64 {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	ids := make([]int64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if id, err := strconv.ParseInt(p, 10, 64); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}
```

**Key patterns to preserve:**
- `log.Fatal()` for missing required config
- Silent defaults for optional config
- Empty whitelist = allow all (development mode)

---

### Step 3: Create `opencode.go`

This file implements the OpenCode HTTP client with session caching.

**CRITICAL PATTERNS:**
- ALWAYS use `context.Context` (never `nil`)
- Cache sessions in-memory (map[int64]string)
- Use mutex for thread-safe session access
- Handle HTTP errors gracefully
- Extract text from response parts

**Full implementation:**

```go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
)

type OpencodeClient struct {
	BaseURL  string
	Username string
	Password string
	Client   *http.Client

	sessions sync.Map // map[int64]string (chatID -> sessionID)
}

func NewOpencodeClient(cfg *Config) *OpencodeClient {
	return &OpencodeClient{
		BaseURL:  cfg.OpencodeURL,
		Username: cfg.OpencodeUser,
		Password: cfg.OpencodePass,
		Client:   &http.Client{},
	}
}

// HealthCheck pings the OpenCode server
func (c *OpencodeClient) HealthCheck(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.BaseURL+"/global/health", nil)
	if err != nil {
		return err
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed: status %d", resp.StatusCode)
	}
	return nil
}

// GetOrCreateSession returns the session ID for a chat
// Creates a new session if none exists
func (c *OpencodeClient) GetOrCreateSession(ctx context.Context, chatID int64, projectDir string) (string, error) {
	// Check cache first
	if sessionID, ok := c.sessions.Load(chatID); ok {
		return sessionID.(string), nil
	}

	// Create new session
	payload := map[string]interface{}{
		"directory": projectDir,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/session", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Username != "" && c.Password != "" {
		req.SetBasicAuth(c.Username, c.Password)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("session creation failed: %d %s", resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	// Cache the session
	c.sessions.Store(chatID, result.SessionID)
	return result.SessionID, nil
}

// SendPrompt sends a message to an OpenCode session and returns the response
func (c *OpencodeClient) SendPrompt(ctx context.Context, sessionID, text string) (string, error) {
	payload := map[string]interface{}{
		"text": text,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/session/%s/message", c.BaseURL, sessionID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Username != "" && c.Password != "" {
		req.SetBasicAuth(c.Username, c.Password)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("prompt failed: %d %s", resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		Parts []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"parts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	// Extract all text parts
	var response string
	for _, part := range result.Parts {
		if part.Type == "text" {
			response += part.Text
		}
	}

	if response == "" {
		return "❌ No response from OpenCode", nil
	}

	return response, nil
}

// AbortSession aborts an in-flight session operation
func (c *OpencodeClient) AbortSession(ctx context.Context, sessionID string) error {
	url := fmt.Sprintf("%s/session/%s/abort", c.BaseURL, sessionID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return err
	}
	if c.Username != "" && c.Password != "" {
		req.SetBasicAuth(c.Username, c.Password)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("abort failed: status %d", resp.StatusCode)
	}
	return nil
}

// ClearSession removes a session from the cache
func (c *OpencodeClient) ClearSession(chatID int64) {
	c.sessions.Delete(chatID)
}
```

**Key patterns to preserve:**
- `sync.Map` for thread-safe session storage
- `context.Context` in every HTTP call
- Basic auth support (optional)
- Error messages include HTTP status codes

---

### Step 4: Create `telegram.go`

This file implements the Telegram bot with command routing and message handling.

**CRITICAL PATTERNS:**
- Use `context.WithTimeout` for ALL operations
- Escape MarkdownV2 characters before sending
- Lock processing per chat (prevent concurrent prompts)
- Route commands LOCALLY (don't forward to OpenCode)
- Send typing indicator while processing

**Full implementation:**

```go
package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type TelegramBot struct {
	API        *tg.BotAPI
	Config     *Config
	Client     *OpencodeClient
	processing map[int64]bool // track if a chat is currently processing
	mu         sync.Mutex
}

func NewTelegramBot(cfg *Config, client *OpencodeClient) (*TelegramBot, error) {
	bot, err := tg.NewBotAPI(cfg.BotToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot: %w", err)
	}

	bot.Debug = cfg.Debug
	log.Printf("✅ Authorized as @%s", bot.Self.UserName)

	return &TelegramBot{
		API:        bot,
		Config:     cfg,
		Client:     client,
		processing: make(map[int64]bool),
	}, nil
}

func (b *TelegramBot) StartPolling() {
	u := tg.NewUpdate(0)
	u.Timeout = 60

	updates := b.API.GetUpdatesChan(u)
	log.Println("🤖 Bot started. Waiting for messages...")

	for update := range updates {
		if update.Message == nil {
			continue
		}

		go b.handleMessage(update.Message)
	}
}

func (b *TelegramBot) handleMessage(msg *tg.Message) {
	chatID := msg.Chat.ID

	// Check whitelist
	if !b.Config.IsAllowed(chatID) {
		b.sendMessage(chatID, "🚫 Unauthorized. Use /id to get your Chat ID.", false)
		return
	}

	// Prevent concurrent processing for the same chat
	b.mu.Lock()
	if b.processing[chatID] {
		b.mu.Unlock()
		b.sendMessage(chatID, "⏳ Previous message still processing. Please wait.", false)
		return
	}
	b.processing[chatID] = true
	b.mu.Unlock()

	defer func() {
		b.mu.Lock()
		delete(b.processing, chatID)
		b.mu.Unlock()
	}()

	// Extract text
	text := msg.Text
	if text == "" {
		return
	}

	// Handle local commands (don't forward to OpenCode)
	if msg.IsCommand() {
		switch msg.Command() {
		case "start":
			b.handleStart(chatID)
			return
		case "id":
			b.handleGetID(msg)
			return
		case "reset":
			b.handleReset(chatID)
			return
		case "abort":
			b.handleAbort(chatID)
			return
		}
	}

	// Forward to OpenCode
	b.sendChatAction(chatID, tg.ChatTyping)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	sessionID, err := b.Client.GetOrCreateSession(ctx, chatID, b.Config.ProjectDir)
	if err != nil {
		b.sendMessage(chatID, fmt.Sprintf("❌ Error creating session: %v", err), false)
		return
	}

	response, err := b.Client.SendPrompt(ctx, sessionID, text)
	if err != nil {
		b.sendMessage(chatID, fmt.Sprintf("❌ Error: %v", err), false)
		return
	}

	b.sendMessage(chatID, response, b.Config.Markdown)
}

// Local command handlers

func (b *TelegramBot) handleStart(chatID int64) {
	msg := `👋 *Welcome to OpenCode Bridge*

Send me any message and I'll forward it to OpenCode for processing.

*Commands:*
• /id — Show your Chat ID
• /reset — Clear your session
• /abort — Stop current operation

Your messages are private and secure.`

	b.sendMessage(chatID, msg, true)
}

func (b *TelegramBot) handleGetID(msg *tg.Message) {
	response := fmt.Sprintf("🆔 *Your Chat ID:* `%d`\n\nAdd this to `ALLOWED_CHAT_IDS` in your `.env` file.", msg.Chat.ID)
	b.sendMessage(msg.Chat.ID, response, true)
}

func (b *TelegramBot) handleReset(chatID int64) {
	b.Client.ClearSession(chatID)
	b.sendMessage(chatID, "✅ Session reset. Next message will create a new session.", false)
}

func (b *TelegramBot) handleAbort(chatID int64) {
	sessionIDVal, ok := b.Client.sessions.Load(chatID)
	if !ok {
		b.sendMessage(chatID, "❌ No active session to abort.", false)
		return
	}

	sessionID := sessionIDVal.(string)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := b.Client.AbortSession(ctx, sessionID); err != nil {
		b.sendMessage(chatID, fmt.Sprintf("❌ Abort failed: %v", err), false)
		return
	}

	b.sendMessage(chatID, "✅ Operation aborted.", false)
}

// Helper methods

func (b *TelegramBot) sendMessage(chatID int64, text string, markdown bool) {
	msg := tg.NewMessage(chatID, text)
	if markdown {
		msg.ParseMode = tg.ModeMarkdownV2
		msg.Text = EscapeMarkdownV2(text)
	}
	if _, err := b.API.Send(msg); err != nil {
		log.Printf("❌ Failed to send message: %v", err)
	}
}

func (b *TelegramBot) sendChatAction(chatID int64, action string) {
	act := tg.NewChatAction(chatID, action)
	if _, err := b.API.Send(act); err != nil {
		log.Printf("❌ Failed to send chat action: %v", err)
	}
}

func (b *TelegramBot) Stop() {
	b.API.StopReceivingUpdates()
	log.Println("🛑 Bot stopped")
}

// EscapeMarkdownV2 escapes all reserved characters for Telegram MarkdownV2
func EscapeMarkdownV2(text string) string {
	replacer := strings.NewReplacer(
		"_", "\\_",
		"*", "\\*",
		"[", "\\[",
		"]", "\\]",
		"(", "\\(",
		")", "\\)",
		"~", "\\~",
		"`", "\\`",
		">", "\\>",
		"#", "\\#",
		"+", "\\+",
		"-", "\\-",
		"=", "\\=",
		"|", "\\|",
		"{", "\\{",
		"}", "\\}",
		".", "\\.",
		"!", "\\!",
	)
	return replacer.Replace(text)
}
```

**Key patterns to preserve:**
- Commands handled locally (start, id, reset, abort)
- Processing lock per chat (prevents concurrent requests)
- Context with 120s timeout for OpenCode calls
- MarkdownV2 escaping function (ALL special chars)

---

### Step 5: Create `main.go`

Entry point with graceful shutdown and health check.

**Full implementation:**

```go
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	log.Println("🚀 Starting Telegram Bridge...")

	// Load configuration
	cfg := LoadConfig()

	// Initialize OpenCode client
	client := NewOpencodeClient(cfg)

	// Health check (non-blocking)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.HealthCheck(ctx); err != nil {
		log.Printf("⚠️  OpenCode health check failed: %v", err)
		log.Println("⚠️  Bot will start anyway, but may fail to process messages")
	} else {
		log.Println("✅ OpenCode server is healthy")
	}

	// Initialize Telegram bot
	bot, err := NewTelegramBot(cfg, client)
	if err != nil {
		log.Fatalf("❌ Failed to initialize bot: %v", err)
	}

	// Start polling in background
	go bot.StartPolling()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("🛑 Shutting down gracefully...")
	bot.Stop()
	log.Println("✅ Shutdown complete")
}
```

**Key patterns:**
- Non-blocking health check (warn but don't fail)
- Graceful shutdown on SIGINT/SIGTERM
- Bot runs in background goroutine

---

### Step 6: Create `.env.example`

Template for environment variables.

```bash
# Required
TELEGRAM_BOT_TOKEN=your_bot_token_from_botfather

# Optional - Security
ALLOWED_CHAT_IDS=123456789,987654321

# Optional - OpenCode Server
OPENCODE_URL=http://localhost:4096
OPENCODE_USERNAME=opencode
OPENCODE_PASSWORD=

# Optional - Bridge Settings
OPENCODE_PROJECT_DIR=.
BRIDGE_PORT=8080
ENABLE_MARKDOWN=true
DEBUG=false
```

**Instructions to include:**
1. Copy this file to `.env`
2. Get bot token from [@BotFather](https://t.me/BotFather)
3. Run bot without `ALLOWED_CHAT_IDS` first
4. Send `/id` to bot to get your Chat ID
5. Add your Chat ID to `ALLOWED_CHAT_IDS`
6. Restart bot

---

### Step 7: Create `.gitignore`

```
# Binaries
*.exe
*.exe~
telegram-bot

# Environment
.env

# Go
go.sum

# OS
.DS_Store
Thumbs.db
```

---

### Step 8: Create `start-stack.bat` (Windows only)

**CRITICAL: This script includes a health check loop to prevent timeout errors.**

```batch
@echo off
REM ========================================
REM  Telegram Bridge - Auto Start Script
REM  Starts OpenCode + Bot with health check
REM ========================================

echo ========================================
echo  🚀 Starting Telegram Bridge Stack
echo ========================================

REM Change to script directory
cd /d "%~dp0"

REM Check if binary exists
if not exist "telegram-bot.exe" (
    echo ⚠️  Building telegram-bot.exe...
    go build -o telegram-bot.exe .
    if %ERRORLEVEL% NEQ 0 (
        echo ❌ Build failed
        pause
        exit /b 1
    )
)

echo ✅ Binary ready
echo.

REM Start OpenCode Server in separate window
echo 📡 Starting OpenCode Server...
start "OpenCode Server" cmd /k "cd /d %~dp0 && echo 🚀 OpenCode Server && opencode serve"

REM Wait for OpenCode to initialize
timeout /t 5 /nobreak >nul

REM Health check loop - wait until OpenCode responds
echo ⏳ Waiting for OpenCode to be ready...
:wait_opencode
ping -n 2 127.0.0.1 >nul
curl -s http://localhost:4096/health >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo ⏳ Still waiting for OpenCode...
    goto wait_opencode
)
echo ✅ OpenCode is ready

REM Start Telegram Bot in separate window
echo 🤖 Starting Telegram Bot...
start "Telegram Bot" cmd /k "cd /d %~dp0 && echo 🤖 Telegram Bot && telegram-bot.exe"

echo.
echo ========================================
echo ✅ Stack started successfully!
echo.
echo  Terminals:
echo   - OpenCode Server (port 4096)
echo   - Telegram Bot (polling)
echo.
echo  Send a message to your bot on Telegram
echo ========================================

REM Auto-close this window
timeout /t 5 /nobreak >nul
exit
```

**Key reliability features:**
- Health check loop until `/health` endpoint responds
- 5-second initial delay for OpenCode startup
- Working directory fix (`cd /d %~dp0`)
- Automatic build if binary missing
- Separate terminal windows for monitoring

---

### Step 9: Create `add-startup.ps1` (Windows only)

**Run ONCE to enable auto-start on Windows login.**

```powershell
# Telegram Bridge - Add to Windows Startup
# Run once: powershell -ExecutionPolicy Bypass -File add-startup.ps1

$target = "$PSScriptRoot\start-stack.bat"
$shortcutPath = "$env:APPDATA\Microsoft\Windows\Start Menu\Programs\Startup\telegram-bot.lnk"

$shell = New-Object -ComObject WScript.Shell
$shortcut = $shell.CreateShortcut($shortcutPath)
$shortcut.TargetPath = $target
$shortcut.WorkingDirectory = $PSScriptRoot
$shortcut.Description = "Telegram Bridge + OpenCode Auto-Start"
$shortcut.Save()

Write-Host "✅ Startup shortcut created at:"
Write-Host "   $shortcutPath"
Write-Host ""
Write-Host "✅ The bot will start automatically when Windows boots"
Write-Host ""
Write-Host "To disable: delete the shortcut from the Startup folder"
```

**Usage instructions:**
```powershell
# Run ONCE to enable auto-start
powershell -ExecutionPolicy Bypass -File add-startup.ps1

# To disable: delete the shortcut manually
```

---

### Step 10: Create `README.md`

User-facing documentation.

```markdown
# Telegram Bridge for OpenCode

A production-ready Telegram bot that connects to your local OpenCode server.

## Features

- ✅ Message forwarding from Telegram to OpenCode
- ✅ Session management (one session per chat)
- ✅ Chat whitelist security
- ✅ Windows auto-start support
- ✅ MarkdownV2 response formatting
- ✅ Graceful shutdown handling

## Quick Start

### 1. Prerequisites

- Go 1.21+ ([download](https://go.dev/dl))
- OpenCode CLI (`npm install -g opencode-ai`)
- Telegram Bot Token (get from [@BotFather](https://t.me/BotFather))

### 2. Installation

\`\`\`bash
# Clone or create project
git clone <your-repo>
cd telegram-bot

# Install dependencies
go mod download

# Copy environment template
cp .env.example .env
\`\`\`

### 3. Configuration

Edit `.env` and set your bot token:

\`\`\`bash
TELEGRAM_BOT_TOKEN=your_token_here
\`\`\`

### 4. Get Your Chat ID

\`\`\`bash
# Start bot without whitelist
go run .

# Send /id to your bot in Telegram
# Copy the Chat ID from the response

# Add to .env:
ALLOWED_CHAT_IDS=123456789
\`\`\`

### 5. Build and Run

\`\`\`bash
# Build
go build -o telegram-bot.exe .

# Run
./telegram-bot.exe
\`\`\`

## Windows Auto-Start

Use the included scripts for automatic startup:

\`\`\`powershell
# Run ONCE to enable auto-start
powershell -ExecutionPolicy Bypass -File add-startup.ps1
\`\`\`

Or double-click `start-stack.bat` to manually launch both OpenCode and the bot.

## Commands

| Command | Description |
|---------|-------------|
| `/start` | Show welcome message |
| `/id` | Get your Chat ID for whitelist |
| `/reset` | Clear your OpenCode session |
| `/abort` | Stop current operation |
| *(any text)* | Send to OpenCode as prompt |

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `TELEGRAM_BOT_TOKEN` | ✅ | — | Bot token from BotFather |
| `OPENCODE_URL` | No | `http://localhost:4096` | OpenCode server URL |
| `ALLOWED_CHAT_IDS` | No | (empty = all) | Comma-separated whitelist |
| `OPENCODE_PROJECT_DIR` | No | `.` | Working directory |
| `ENABLE_MARKDOWN` | No | `true` | MarkdownV2 formatting |
| `DEBUG` | No | `false` | Verbose logging |

## Security

- **Whitelist**: Only specified chat IDs can use the bot
- **Local only**: OpenCode server runs on localhost
- **No storage**: Sessions are in-memory only

## Troubleshooting

### "context deadline exceeded"

OpenCode server not responding. Check:
- `opencode serve` is running
- Port 4096 is not blocked
- No zombie processes on port 4096

### Bot not responding

Check:
- Bot token is correct
- Your chat ID is in `ALLOWED_CHAT_IDS`
- Bot process is running

## License

Apache-2.0
```

---

## Configuration Reference

When the AI generates the project, ensure these environment variables are documented:

| Variable | Type | Required | Default | Description |
|----------|------|----------|---------|-------------|
| `TELEGRAM_BOT_TOKEN` | string | ✅ | — | Bot token from [@BotFather](https://t.me/BotFather) |
| `ALLOWED_CHAT_IDS` | int64[] | No | `[]` (all allowed) | Comma-separated chat IDs. Get yours with `/id` command |
| `OPENCODE_URL` | string | No | `http://localhost:4096` | OpenCode server endpoint |
| `OPENCODE_USERNAME` | string | No | `opencode` | Basic auth username (if enabled) |
| `OPENCODE_PASSWORD` | string | No | `""` | Basic auth password (if enabled) |
| `OPENCODE_PROJECT_DIR` | string | No | `.` | Working directory for OpenCode sessions |
| `BRIDGE_PORT` | int | No | `8080` | HTTP port for the bridge server |
| `ENABLE_MARKDOWN` | bool | No | `true` | Enable MarkdownV2 formatting in responses |
| `DEBUG` | bool | No | `false` | Enable verbose logging |

---

## OpenCode API Endpoints Used

The bridge interacts with these OpenCode endpoints:

| Method | Endpoint | Purpose | Request | Response |
|--------|----------|---------|---------|----------|
| `GET` | `/global/health` | Health check on startup | None | `200 OK` |
| `POST` | `/session` | Create new session | `{"directory": "."}` | `{"sessionId": "..."}` |
| `POST` | `/session/{id}/message` | Send prompt | `{"text": "..."}` | `{"parts": [{"type": "text", "text": "..."}]}` |
| `POST` | `/session/{id}/abort` | Abort operation | None | `200 OK` |

---

## Critical Implementation Rules

When generating code from this skill, the AI MUST follow these rules:

### 1. Context Handling
- NEVER pass `nil` as context
- ALWAYS use `context.WithTimeout` for HTTP calls
- Default timeout: 120 seconds for prompts, 10 seconds for health checks

### 2. Thread Safety
- Use `sync.Map` for session storage (not regular map)
- Use mutex for processing state (one message per chat at a time)
- Never share HTTP client state without locks

### 3. Error Handling
- Log errors but don't crash the bot
- Send user-friendly error messages to Telegram
- Include HTTP status codes in error messages

### 4. MarkdownV2 Escaping
- ALWAYS escape before sending markdown responses
- Escape ALL special characters: `_*[]()~`>#+-=|{}.!`
- Use the exact `EscapeMarkdownV2` function provided

### 5. Command Routing
- Handle `/start`, `/id`, `/reset`, `/abort` LOCALLY (never forward to OpenCode)
- All other text goes to OpenCode via `SendPrompt`
- Commands with arguments should be forwarded to OpenCode (e.g., `/ask how do I...`)

### 6. Windows Scripts
- `start-stack.bat` MUST include health check loop
- Use `cd /d %~dp0` to set working directory
- Use `start "Title" cmd /k "commands"` for separate windows
- Health check loops until `curl http://localhost:4096/health` succeeds

### 7. Security
- Whitelist is OPTIONAL (empty = development mode, allow all)
- Validate chat ID before processing ANY message
- Never log sensitive data (tokens, passwords)

---

## Testing Checklist

After generating the project, verify:

- [ ] `go mod download` succeeds
- [ ] `go build` produces binary without errors
- [ ] Bot starts and shows "Authorized as @..." message
- [ ] `/start` command shows welcome message
- [ ] `/id` command returns correct chat ID
- [ ] Regular messages forward to OpenCode and return responses
- [ ] `/reset` clears session
- [ ] `/abort` stops in-flight operation
- [ ] Whitelist blocks unauthorized chats
- [ ] `start-stack.bat` opens two windows (Windows only)
- [ ] Health check loop waits for OpenCode to be ready
- [ ] `add-startup.ps1` creates shortcut in Startup folder

---

## Common Pitfalls to Avoid

### ❌ DON'T: Use `nil` context
```go
// WRONG
client.HealthCheck(nil)

// CORRECT
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
client.HealthCheck(ctx)
```

### ❌ DON'T: Send markdown without escaping
```go
// WRONG
msg.ParseMode = tg.ModeMarkdownV2
msg.Text = response

// CORRECT
msg.ParseMode = tg.ModeMarkdownV2
msg.Text = EscapeMarkdownV2(response)
```

### ❌ DON'T: Use regular map for sessions
```go
// WRONG (not thread-safe)
sessions := make(map[int64]string)

// CORRECT
var sessions sync.Map
```

### ❌ DON'T: Forward local commands to OpenCode
```go
// WRONG
if msg.Command() == "start" {
    client.SendPrompt(ctx, sessionID, "/start")
}

// CORRECT
if msg.Command() == "start" {
    b.handleStart(chatID)
    return
}
```

### ❌ DON'T: Start bot without health check loop in `start-stack.bat`
```batch
REM WRONG (may start bot before OpenCode is ready)
start "OpenCode" cmd /k "opencode serve"
timeout /t 3 /nobreak
start "Bot" cmd /k "telegram-bot.exe"

REM CORRECT (waits until OpenCode responds)
start "OpenCode" cmd /k "opencode serve"
timeout /t 5 /nobreak
:wait_opencode
curl -s http://localhost:4096/health >nul 2>&1
if %ERRORLEVEL% NEQ 0 goto wait_opencode
start "Bot" cmd /k "telegram-bot.exe"
```

---

## Example Usage Transcript

**User:** "Create a Telegram bot that connects to OpenCode"

**AI Response:**
1. Verify prerequisites (Go, OpenCode, bot token)
2. Scaffold complete project structure
3. Generate all 10 files with EXACT implementations
4. Provide setup instructions:
   - Copy `.env.example` to `.env`
   - Set `TELEGRAM_BOT_TOKEN`
   - Run bot and get Chat ID with `/id`
   - Add Chat ID to `ALLOWED_CHAT_IDS`
   - Restart bot
5. Test with `/start` command
6. (Windows) Optionally run `add-startup.ps1` for auto-start

---

## Version History

- **v2.0** — Complete rewrite as explicit AI-readable skill (2026-04-04)
  - Added full code implementations
  - Added health check loop in startup script
  - Added critical implementation rules
  - Added common pitfalls section
  - Removed scaffolding ambiguity

- **v1.0** — Initial template-based approach (deprecated)

---

## Reference Implementation

A complete, tested implementation following this spec is available at:

**https://github.com/jorgehara/gentle-ia-telegram-bot-skill**

Use this as a reference when uncertain about implementation details.
