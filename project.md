# Discord CLI + MCP Server — Complete Build Spec

> **Philosophy:** Build the core logic once. Expose it as a CLI for humans and an MCP server for AI agents.  
> **Stack:** Go, SQLite (FTS5), Discord REST + Gateway API, MCP protocol over stdio  
> **Inspired by:** steipete/wacli, virat-mankali/telegram-cli

---

## Table of Contents

1. [Project Overview](#1-project-overview)
2. [Prerequisites & Setup](#2-prerequisites--setup)
3. [Project Structure](#3-project-structure)
4. [Discord API Fundamentals](#4-discord-api-fundamentals)
5. [Phase 1 — Core Library](#5-phase-1--core-library)
6. [Phase 2 — CLI (Cobra)](#6-phase-2--cli-cobra)
7. [Phase 3 — SQLite Storage & FTS5 Search](#7-phase-3--sqlite-storage--fts5-search)
8. [Phase 4 — MCP Server](#8-phase-4--mcp-server)
9. [Phase 5 — Releases (GoReleaser + Homebrew)](#9-phase-5--releases-goreleaser--homebrew)
10. [Phase 6 — GitHub Actions CI/CD](#10-phase-6--github-actions-cicd)
11. [Best Practices & Conventions](#11-best-practices--conventions)
12. [README Template](#12-readme-template)
13. [MCP Config Example](#13-mcp-config-example)

---

## 1. Project Overview

### What You're Building

`discocli` — a single Go binary that:

- Authenticates with Discord using a **user token** (acts as a real Discord user, not a bot) OR a **bot token**
- Syncs message history from channels/DMs into a local SQLite database
- Provides fast **offline full-text search** across synced messages
- Sends messages and files to any channel, DM, or thread
- Exposes all of the above as an **MCP server** so AI agents (Claude, Cursor, etc.) can call it natively

### Why This Gets GitHub Stars

- Every AI developer uses Discord
- No clean Go CLI + MCP tool exists for Discord
- Peter's `discrawl` only mirrors — no send, no MCP, no good UX
- AI agents need Discord access in 2026; you're building the plumbing

### Core Commands (Target UX)

```bash
discocli auth                                          # authenticate
discocli sync                                          # sync all accessible channels
discocli sync --guild "My Server" --channel general    # sync specific channel
discocli sync --follow                                 # real-time sync (tail -f style)
discocli search "deployment failed"                    # offline full-text search
discocli search "bug" --channel general --limit 50
discocli send --to "#general" --text "Build passed ✅"
discocli send --to "@username" --text "Hey!"           # DM
discocli send --to "#general" --media ./screenshot.png --text "Look at this"
discocli guilds                                        # list all servers you're in
discocli channels --guild "My Server"                  # list channels in a server
discocli serve                                         # start MCP server (stdio)
```

---

## 2. Prerequisites & Setup

### Tools Required

```bash
# Go 1.22+
go version

# SQLite with CGO-free driver (no CGO needed)
# we'll use modernc.org/sqlite — pure Go, no CGO

# GoReleaser for cross-platform builds
brew install goreleaser

# golangci-lint for linting
brew install golangci-lint
```

### Go Module Init

```bash
mkdir discocli && cd discocli
git init
go mod init github.com/virat-mankali/discord-cli
```

### Key Dependencies

```bash
# HTTP client (Discord REST API)
go get github.com/bwmarrin/discordgo         # Discord Gateway + REST

# CLI framework
go get github.com/spf13/cobra

# SQLite (pure Go, no CGO — critical for cross-platform builds)
go get modernc.org/sqlite

# Structured logging
go get go.uber.org/zap

# Pretty terminal output
go get github.com/charmbracelet/lipgloss
go get github.com/charmbracelet/bubbletea   # optional: TUI mode later

# Config management
go get github.com/spf13/viper

# MCP protocol (official Go SDK)
go get github.com/mark3labs/mcp-go

# Progress bars / spinners
go get github.com/briandowns/spinner
```

> **Why `modernc.org/sqlite` over `mattn/go-sqlite3`?**  
> `mattn/go-sqlite3` requires CGO, which breaks cross-platform builds with GoReleaser.  
> `modernc.org/sqlite` is a pure Go port — CGO-free, works on all GOOS/GOARCH combos.

---

## 3. Project Structure

```
discocli/
├── cmd/
│   └── discocli/
│       ├── main.go          # Entry point — just calls root.Execute()
│       ├── root.go          # Root cobra command, --store flag, global flags
│       ├── auth.go          # `discocli auth`
│       ├── sync.go          # `discocli sync`
│       ├── search.go        # `discocli search`
│       ├── send.go          # `discocli send`
│       ├── guilds.go        # `discocli guilds`
│       ├── channels.go      # `discocli channels`
│       └── serve.go         # `discocli serve` (starts MCP server)
│
├── internal/
│   ├── config/
│   │   └── paths.go         # Store directory helpers (~/.discocli)
│   ├── discord/
│   │   ├── client.go        # discordgo wrapper — REST calls
│   │   ├── gateway.go       # WebSocket gateway for real-time sync
│   │   ├── auth.go          # Token storage + validation
│   │   └── types.go         # Internal types (Message, Channel, Guild)
│   ├── storage/
│   │   ├── db.go            # SQLite connection + migrations
│   │   ├── messages.go      # Message CRUD
│   │   ├── channels.go      # Channel/guild metadata CRUD
│   │   └── search.go        # FTS5 search queries
│   └── mcp/
│       ├── server.go        # MCP server setup
│       └── tools.go         # Tool definitions + handlers
│
├── .github/
│   └── workflows/
│       ├── ci.yml           # lint + test on every push
│       └── release.yml      # goreleaser on git tag push
│
├── .goreleaser.yaml
├── .gitignore
├── go.mod
├── go.sum
├── LICENSE
├── project.md               # internal notes / agentic context
└── README.md
```

---

## 4. Discord API Fundamentals

### Two Auth Modes

| Mode | Token Type | Use Case |
|------|-----------|----------|
| **User Token** | `user_token` from Discord app | Acts as a real user — reads DMs, all channels you're in |
| **Bot Token** | `Bot xxxxx` prefix | Safer, more stable, limited to servers bot is in |

> **Important:** User tokens violate Discord ToS if used for automation at scale. For personal/local use and AI agents acting on behalf of the owner, it's acceptable. Be clear in your README. Bot tokens are always safer.

### Getting a Bot Token (Recommended)

1. Go to https://discord.com/developers/applications
2. Create a new application → Bot tab → Reset Token → copy token
3. Enable **Message Content Intent** under Privileged Gateway Intents
4. Invite bot to your server with OAuth2 URL generator:
   - Scopes: `bot`
   - Permissions: `Read Messages`, `Send Messages`, `Read Message History`, `Attach Files`

### Getting a User Token (Power Users)

```
1. Open Discord in browser
2. DevTools → Network tab → filter "api"
3. Any request → Headers → Authorization: YOUR_USER_TOKEN
```

Store token at `~/.discocli/token` with `0600` permissions — never in env vars printed to logs.

### Key Discord API Endpoints

```
GET  /users/@me                          # validate token, get own user
GET  /users/@me/guilds                   # list all guilds (servers)
GET  /guilds/{guild.id}/channels         # list channels in a guild
GET  /channels/{channel.id}/messages     # fetch message history
     ?limit=100&before={message.id}      # pagination
POST /channels/{channel.id}/messages     # send a message
POST /channels/{channel.id}/messages     # send with file (multipart)
GET  /users/@me/channels                 # list DMs
POST /users/@me/channels                 # open a DM with a user
```

### Rate Limits

Discord has per-route rate limits. Key ones:

- `GET /channels/{id}/messages`: 5 requests / 5 seconds per channel
- `POST /channels/{id}/messages`: 5 requests / 5 seconds per channel
- Global limit: 50 requests / second

`discordgo` handles most of this automatically. For bulk history fetch, always add a small sleep between paginated requests.

```go
// Safe pagination pattern
for {
    msgs, err := s.ChannelMessages(channelID, 100, beforeID, "", "")
    if err != nil { break }
    if len(msgs) == 0 { break }
    // process msgs...
    beforeID = msgs[len(msgs)-1].ID
    time.Sleep(500 * time.Millisecond) // respect rate limits
}
```

---

## 5. Phase 1 — Core Library

### `internal/config/paths.go`

```go
package config

import (
    "os"
    "path/filepath"
)

// StoreDir returns the default store path (~/.discocli)
// Can be overridden with --store flag.
func StoreDir(override string) string {
    if override != "" {
        return override
    }
    home, _ := os.UserHomeDir()
    return filepath.Join(home, ".discocli")
}

func TokenPath(store string) string {
    return filepath.Join(store, "token")
}

func DBPath(store string) string {
    return filepath.Join(store, "data.db")
}

func EnsureStore(store string) error {
    return os.MkdirAll(store, 0700)
}
```

### `internal/discord/auth.go`

```go
package discord

import (
    "fmt"
    "os"
    "strings"
)

const (
    TokenTypeBot  = "bot"
    TokenTypeUser = "user"
)

type StoredToken struct {
    Token     string
    TokenType string // "bot" or "user"
}

// SaveToken writes token to disk with 0600 permissions
func SaveToken(path, token, tokenType string) error {
    content := fmt.Sprintf("%s\n%s\n", tokenType, token)
    return os.WriteFile(path, []byte(content), 0600)
}

// LoadToken reads token from disk
func LoadToken(path string) (*StoredToken, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("not authenticated — run `discocli auth` first")
    }
    lines := strings.SplitN(strings.TrimSpace(string(data)), "\n", 2)
    if len(lines) != 2 {
        return nil, fmt.Errorf("corrupted token file — run `discocli auth` again")
    }
    return &StoredToken{TokenType: lines[0], Token: lines[1]}, nil
}

// FormatToken returns the token in the format discordgo expects
func (t *StoredToken) FormatToken() string {
    if t.TokenType == TokenTypeBot {
        return "Bot " + t.Token
    }
    return t.Token // user tokens don't have a prefix
}
```

### `internal/discord/client.go`

```go
package discord

import (
    "fmt"
    "github.com/bwmarrin/discordgo"
)

// NewSession creates an authenticated discordgo session
func NewSession(token *StoredToken) (*discordgo.Session, error) {
    s, err := discordgo.New(token.FormatToken())
    if err != nil {
        return nil, fmt.Errorf("failed to create Discord session: %w", err)
    }
    // Validate token
    _, err = s.User("@me")
    if err != nil {
        return nil, fmt.Errorf("invalid token — run `discocli auth` again: %w", err)
    }
    return s, nil
}

// FetchMessages paginates through ALL messages in a channel
// and calls onBatch for each batch of up to 100 messages.
func FetchMessages(s *discordgo.Session, channelID string, onBatch func([]*discordgo.Message) error) error {
    var beforeID string
    for {
        msgs, err := s.ChannelMessages(channelID, 100, beforeID, "", "")
        if err != nil {
            return fmt.Errorf("failed to fetch messages: %w", err)
        }
        if len(msgs) == 0 {
            break
        }
        if err := onBatch(msgs); err != nil {
            return err
        }
        beforeID = msgs[len(msgs)-1].ID
        time.Sleep(500 * time.Millisecond)
    }
    return nil
}
```

### `internal/discord/types.go`

```go
package discord

import "time"

// These are internal types — not discordgo types directly.
// We normalize everything before storing.

type Guild struct {
    ID   string
    Name string
    Icon string
}

type Channel struct {
    ID      string
    GuildID string
    Name    string
    Type    int // 0=text, 1=DM, 2=voice, 4=category, 5=announcement, 11=thread
    Topic   string
}

type Message struct {
    ID        string
    ChannelID string
    GuildID   string
    AuthorID  string
    AuthorName string
    Content   string
    Timestamp time.Time
    Attachments []Attachment
    Edited    bool
}

type Attachment struct {
    ID       string
    Filename string
    URL      string
    Size     int
}
```

---

## 6. Phase 2 — CLI (Cobra)

### `cmd/discocli/main.go`

```go
package main

import (
    "github.com/YOUR_USERNAME/discocli/cmd/discocli/commands"
)

func main() {
    commands.Execute()
}
```

### `cmd/discocli/root.go`

```go
package commands

import (
    "fmt"
    "os"
    "github.com/spf13/cobra"
    "github.com/YOUR_USERNAME/discocli/internal/config"
)

var storeDir string

var rootCmd = &cobra.Command{
    Use:   "discocli",
    Short: "Discord CLI — sync, search, send from your terminal",
    Long: `discocli is a Discord client for your terminal and AI agents.
Sync message history, search offline, send messages — all from the CLI.
Run 'discocli serve' to start the MCP server for AI agent access.`,
}

func Execute() {
    if err := rootCmd.Execute(); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}

func init() {
    rootCmd.PersistentFlags().StringVar(
        &storeDir, "store", "",
        "Storage directory (default: ~/.discocli)",
    )
}

// resolvedStore returns the effective store directory
func resolvedStore() string {
    return config.StoreDir(storeDir)
}
```

### `cmd/discocli/auth.go`

```go
package commands

import (
    "bufio"
    "fmt"
    "os"
    "strings"

    "github.com/spf13/cobra"
    "github.com/YOUR_USERNAME/discocli/internal/config"
    "github.com/YOUR_USERNAME/discocli/internal/discord"
)

var authCmd = &cobra.Command{
    Use:   "auth",
    Short: "Authenticate with Discord",
    Long: `Authenticate with Discord using a bot token or user token.

For a bot token:
  1. Go to https://discord.com/developers/applications
  2. Create an application → Bot → Reset Token

For a user token (advanced, personal use only):
  Open Discord in browser → DevTools → Network → any request → Authorization header`,
    RunE: func(cmd *cobra.Command, args []string) error {
        store := resolvedStore()
        if err := config.EnsureStore(store); err != nil {
            return err
        }

        reader := bufio.NewReader(os.Stdin)

        fmt.Println("Token type:")
        fmt.Println("  [1] Bot token (recommended)")
        fmt.Println("  [2] User token (personal use only)")
        fmt.Print("Choice: ")
        choice, _ := reader.ReadString('\n')
        choice = strings.TrimSpace(choice)

        var tokenType string
        switch choice {
        case "1":
            tokenType = discord.TokenTypeBot
        case "2":
            tokenType = discord.TokenTypeUser
        default:
            return fmt.Errorf("invalid choice")
        }

        fmt.Print("Paste your token: ")
        token, _ := reader.ReadString('\n')
        token = strings.TrimSpace(token)

        if token == "" {
            return fmt.Errorf("token cannot be empty")
        }

        // Validate before saving
        fmt.Println("Validating token...")
        stored := &discord.StoredToken{Token: token, TokenType: tokenType}
        session, err := discord.NewSession(stored)
        if err != nil {
            return fmt.Errorf("authentication failed: %w", err)
        }

        me, _ := session.User("@me")
        if err := discord.SaveToken(config.TokenPath(store), token, tokenType); err != nil {
            return fmt.Errorf("failed to save token: %w", err)
        }

        fmt.Printf("✅ Authenticated as %s#%s\n", me.Username, me.Discriminator)
        fmt.Printf("   Session saved to %s\n", config.TokenPath(store))
        return nil
    },
}

func init() {
    rootCmd.AddCommand(authCmd)
}
```

### `cmd/discocli/sync.go`

```go
package commands

import (
    "fmt"
    "github.com/briandowns/spinner"
    "github.com/spf13/cobra"
    "time"
    // internal imports
)

var (
    syncGuild   string
    syncChannel string
    syncFollow  bool
)

var syncCmd = &cobra.Command{
    Use:   "sync",
    Short: "Sync Discord messages to local database",
    Long: `Sync message history from Discord into the local SQLite database.
Without flags, syncs all accessible channels.
Use --follow to keep running and capture new messages in real-time.`,
    RunE: func(cmd *cobra.Command, args []string) error {
        store := resolvedStore()

        token, err := discord.LoadToken(config.TokenPath(store))
        if err != nil {
            return err
        }

        session, err := discord.NewSession(token)
        if err != nil {
            return err
        }
        defer session.Close()

        db, err := storage.Open(config.DBPath(store))
        if err != nil {
            return err
        }
        defer db.Close()

        // If --follow, start gateway for real-time events
        if syncFollow {
            return runGatewaySync(session, db, syncGuild, syncChannel)
        }

        // Otherwise, do a one-shot historical sync
        s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
        s.Suffix = " Syncing messages..."
        s.Start()

        count, err := performHistoricalSync(session, db, syncGuild, syncChannel)
        s.Stop()

        if err != nil {
            return err
        }
        fmt.Printf("✅ Synced %d messages\n", count)
        return nil
    },
}

func init() {
    syncCmd.Flags().StringVar(&syncGuild, "guild", "", "Guild (server) name or ID to sync")
    syncCmd.Flags().StringVar(&syncChannel, "channel", "", "Channel name or ID to sync")
    syncCmd.Flags().BoolVar(&syncFollow, "follow", false, "Keep running and sync new messages in real-time")
    rootCmd.AddCommand(syncCmd)
}
```

### `cmd/discocli/search.go`

```go
package commands

import (
    "fmt"
    "github.com/spf13/cobra"
)

var (
    searchChannel string
    searchGuild   string
    searchLimit   int
)

var searchCmd = &cobra.Command{
    Use:   "search <query>",
    Short: "Search synced messages offline",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        store := resolvedStore()
        query := args[0]

        db, err := storage.Open(config.DBPath(store))
        if err != nil {
            return err
        }
        defer db.Close()

        results, err := db.SearchMessages(query, storage.SearchOptions{
            Channel: searchChannel,
            Guild:   searchGuild,
            Limit:   searchLimit,
        })
        if err != nil {
            return err
        }

        if len(results) == 0 {
            fmt.Println("No results found. Run `discocli sync` to fetch messages first.")
            return nil
        }

        for _, msg := range results {
            fmt.Printf("[%s] #%s — %s: %s\n",
                msg.Timestamp.Format("2006-01-02 15:04"),
                msg.ChannelName,
                msg.AuthorName,
                msg.Content,
            )
        }
        fmt.Printf("\n%d result(s) for %q\n", len(results), query)
        return nil
    },
}

func init() {
    searchCmd.Flags().StringVar(&searchChannel, "channel", "", "Filter by channel name")
    searchCmd.Flags().StringVar(&searchGuild, "guild", "", "Filter by guild name")
    searchCmd.Flags().IntVar(&searchLimit, "limit", 20, "Max results to return")
    rootCmd.AddCommand(searchCmd)
}
```

### `cmd/discocli/send.go`

```go
package commands

import (
    "fmt"
    "os"
    "path/filepath"
    "github.com/spf13/cobra"
)

var (
    sendTo    string
    sendText  string
    sendMedia string
)

var sendCmd = &cobra.Command{
    Use:   "send",
    Short: "Send a message or file to a channel or user",
    Long: `Send a text message or media file to any Discord channel, DM, or thread.

Examples:
  discocli send --to "#general" --text "Hello!"
  discocli send --to "@username" --text "Hey, DM!"
  discocli send --to "#general" --media ./screenshot.png --text "Check this"`,
    RunE: func(cmd *cobra.Command, args []string) error {
        if sendTo == "" {
            return fmt.Errorf("--to is required")
        }
        if sendText == "" && sendMedia == "" {
            return fmt.Errorf("either --text or --media is required")
        }

        store := resolvedStore()
        token, err := discord.LoadToken(config.TokenPath(store))
        if err != nil {
            return err
        }
        session, err := discord.NewSession(token)
        if err != nil {
            return err
        }
        defer session.Close()

        db, err := storage.Open(config.DBPath(store))
        if err != nil {
            return err
        }
        defer db.Close()

        // Resolve --to to a channel ID
        channelID, err := resolveRecipient(session, db, sendTo)
        if err != nil {
            return fmt.Errorf("could not resolve recipient %q: %w", sendTo, err)
        }

        if sendMedia != "" {
            f, err := os.Open(sendMedia)
            if err != nil {
                return fmt.Errorf("could not open file: %w", err)
            }
            defer f.Close()

            _, err = session.ChannelFileSendWithMessage(channelID, sendText, filepath.Base(sendMedia), f)
            if err != nil {
                return fmt.Errorf("failed to send file: %w", err)
            }
            fmt.Printf("✅ Sent file %s to %s\n", sendMedia, sendTo)
        } else {
            _, err = session.ChannelMessageSend(channelID, sendText)
            if err != nil {
                return fmt.Errorf("failed to send message: %w", err)
            }
            fmt.Printf("✅ Sent to %s\n", sendTo)
        }
        return nil
    },
}

func init() {
    sendCmd.Flags().StringVar(&sendTo, "to", "", "Recipient: #channel-name, @username, or channel ID (required)")
    sendCmd.Flags().StringVar(&sendText, "text", "", "Text message to send")
    sendCmd.Flags().StringVar(&sendMedia, "media", "", "Path to file to send")
    sendCmd.MarkFlagRequired("to")
    rootCmd.AddCommand(sendCmd)
}
```

---

## 7. Phase 3 — SQLite Storage & FTS5 Search

### `internal/storage/db.go`

```go
package storage

import (
    "database/sql"
    "fmt"
    _ "modernc.org/sqlite"
)

type DB struct {
    conn *sql.DB
}

func Open(path string) (*DB, error) {
    conn, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_foreign_keys=on")
    if err != nil {
        return nil, fmt.Errorf("failed to open database: %w", err)
    }
    db := &DB{conn: conn}
    if err := db.migrate(); err != nil {
        return nil, fmt.Errorf("migration failed: %w", err)
    }
    return db, nil
}

func (db *DB) Close() error {
    return db.conn.Close()
}

func (db *DB) migrate() error {
    _, err := db.conn.Exec(`
        -- Guilds (servers)
        CREATE TABLE IF NOT EXISTS guilds (
            id   TEXT PRIMARY KEY,
            name TEXT NOT NULL,
            icon TEXT
        );

        -- Channels
        CREATE TABLE IF NOT EXISTS channels (
            id       TEXT PRIMARY KEY,
            guild_id TEXT REFERENCES guilds(id),
            name     TEXT NOT NULL,
            type     INTEGER NOT NULL DEFAULT 0,
            topic    TEXT
        );

        -- Messages
        CREATE TABLE IF NOT EXISTS messages (
            id           TEXT PRIMARY KEY,
            channel_id   TEXT NOT NULL REFERENCES channels(id),
            guild_id     TEXT,
            author_id    TEXT NOT NULL,
            author_name  TEXT NOT NULL,
            content      TEXT NOT NULL,
            timestamp    DATETIME NOT NULL,
            edited       INTEGER NOT NULL DEFAULT 0
        );

        -- FTS5 virtual table for full-text search
        -- content= makes it a content table pointing at messages
        CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
            content,
            author_name,
            content=messages,
            content_rowid=rowid,
            tokenize="unicode61 remove_diacritics 1"
        );

        -- Triggers to keep FTS in sync with messages table
        CREATE TRIGGER IF NOT EXISTS messages_ai AFTER INSERT ON messages BEGIN
            INSERT INTO messages_fts(rowid, content, author_name)
            VALUES (new.rowid, new.content, new.author_name);
        END;

        CREATE TRIGGER IF NOT EXISTS messages_ad AFTER DELETE ON messages BEGIN
            INSERT INTO messages_fts(messages_fts, rowid, content, author_name)
            VALUES ('delete', old.rowid, old.content, old.author_name);
        END;

        CREATE TRIGGER IF NOT EXISTS messages_au AFTER UPDATE ON messages BEGIN
            INSERT INTO messages_fts(messages_fts, rowid, content, author_name)
            VALUES ('delete', old.rowid, old.content, old.author_name);
            INSERT INTO messages_fts(rowid, content, author_name)
            VALUES (new.rowid, new.content, new.author_name);
        END;

        -- Sync state (track how far we've synced per channel)
        CREATE TABLE IF NOT EXISTS sync_state (
            channel_id     TEXT PRIMARY KEY,
            last_message_id TEXT,
            synced_at      DATETIME
        );
    `)
    return err
}
```

### `internal/storage/search.go`

```go
package storage

import (
    "fmt"
    "strings"
    "time"
)

type SearchOptions struct {
    Channel string
    Guild   string
    Limit   int
}

type SearchResult struct {
    ID          string
    ChannelID   string
    ChannelName string
    GuildName   string
    AuthorName  string
    Content     string
    Timestamp   time.Time
    Rank        float64
}

func (db *DB) SearchMessages(query string, opts SearchOptions) ([]SearchResult, error) {
    if opts.Limit == 0 {
        opts.Limit = 20
    }

    // Build the WHERE clause for optional filters
    var conditions []string
    var args []interface{}

    args = append(args, query)

    if opts.Channel != "" {
        conditions = append(conditions, "c.name = ?")
        args = append(args, opts.Channel)
    }
    if opts.Guild != "" {
        conditions = append(conditions, "g.name = ?")
        args = append(args, opts.Guild)
    }

    whereClause := ""
    if len(conditions) > 0 {
        whereClause = "AND " + strings.Join(conditions, " AND ")
    }

    args = append(args, opts.Limit)

    q := fmt.Sprintf(`
        SELECT
            m.id, m.channel_id, c.name AS channel_name,
            COALESCE(g.name, 'DM') AS guild_name,
            m.author_name, m.content, m.timestamp,
            rank
        FROM messages_fts
        JOIN messages m ON messages_fts.rowid = m.rowid
        JOIN channels c ON m.channel_id = c.id
        LEFT JOIN guilds g ON m.guild_id = g.id
        WHERE messages_fts MATCH ?
        %s
        ORDER BY rank
        LIMIT ?
    `, whereClause)

    rows, err := db.conn.Query(q, args...)
    if err != nil {
        return nil, fmt.Errorf("search failed: %w", err)
    }
    defer rows.Close()

    var results []SearchResult
    for rows.Next() {
        var r SearchResult
        var ts string
        if err := rows.Scan(
            &r.ID, &r.ChannelID, &r.ChannelName, &r.GuildName,
            &r.AuthorName, &r.Content, &ts, &r.Rank,
        ); err != nil {
            return nil, err
        }
        r.Timestamp, _ = time.Parse(time.RFC3339, ts)
        results = append(results, r)
    }
    return results, rows.Err()
}
```

---

## 8. Phase 4 — MCP Server

### What is MCP?

MCP (Model Context Protocol) is the standard for giving AI agents access to external tools. When you run `discocli serve`, it starts an MCP server over **stdio**. The AI agent (Claude, Cursor, etc.) communicates with it via JSON-RPC over stdin/stdout. The agent can then call your tools like functions.

### `internal/mcp/server.go`

```go
package mcp

import (
    "context"
    "github.com/mark3labs/mcp-go/mcp"
    "github.com/mark3labs/mcp-go/server"
    "github.com/YOUR_USERNAME/discocli/internal/config"
    "github.com/YOUR_USERNAME/discocli/internal/discord"
    "github.com/YOUR_USERNAME/discocli/internal/storage"
)

func Serve(storeDir string) error {
    s := server.NewMCPServer(
        "discocli",
        "1.0.0",
        server.WithToolCapabilities(true),
    )

    // Register all tools
    RegisterTools(s, storeDir)

    // Start serving over stdio (standard for MCP)
    return server.ServeStdio(s)
}
```

### `internal/mcp/tools.go`

```go
package mcp

import (
    "context"
    "fmt"
    "github.com/mark3labs/mcp-go/mcp"
    "github.com/mark3labs/mcp-go/server"
)

func RegisterTools(s *server.MCPServer, storeDir string) {

    // ── search_messages ──────────────────────────────────────────────────────
    s.AddTool(mcp.NewTool("search_messages",
        mcp.WithDescription("Search Discord messages stored locally. Run sync first."),
        mcp.WithString("query",
            mcp.Required(),
            mcp.Description("Full-text search query"),
        ),
        mcp.WithString("channel",
            mcp.Description("Filter by channel name (optional)"),
        ),
        mcp.WithString("guild",
            mcp.Description("Filter by server/guild name (optional)"),
        ),
        mcp.WithNumber("limit",
            mcp.Description("Max results (default 20)"),
        ),
    ), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        query := req.Params.Arguments["query"].(string)
        channel, _ := req.Params.Arguments["channel"].(string)
        guild, _ := req.Params.Arguments["guild"].(string)
        limit := 20
        if l, ok := req.Params.Arguments["limit"].(float64); ok {
            limit = int(l)
        }

        db, err := storage.Open(config.DBPath(storeDir))
        if err != nil {
            return mcp.NewToolResultError(err.Error()), nil
        }
        defer db.Close()

        results, err := db.SearchMessages(query, storage.SearchOptions{
            Channel: channel, Guild: guild, Limit: limit,
        })
        if err != nil {
            return mcp.NewToolResultError(err.Error()), nil
        }

        var out string
        for _, r := range results {
            out += fmt.Sprintf("[%s] #%s (%s) — %s: %s\n",
                r.Timestamp.Format("2006-01-02 15:04"),
                r.ChannelName, r.GuildName, r.AuthorName, r.Content)
        }
        if out == "" {
            out = "No results found."
        }
        return mcp.NewToolResultText(out), nil
    })

    // ── send_message ─────────────────────────────────────────────────────────
    s.AddTool(mcp.NewTool("send_message",
        mcp.WithDescription("Send a text message to a Discord channel or DM."),
        mcp.WithString("channel_id",
            mcp.Required(),
            mcp.Description("Discord channel ID to send to"),
        ),
        mcp.WithString("text",
            mcp.Required(),
            mcp.Description("Message text to send"),
        ),
    ), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        channelID := req.Params.Arguments["channel_id"].(string)
        text := req.Params.Arguments["text"].(string)

        token, err := discord.LoadToken(config.TokenPath(storeDir))
        if err != nil {
            return mcp.NewToolResultError(err.Error()), nil
        }
        session, err := discord.NewSession(token)
        if err != nil {
            return mcp.NewToolResultError(err.Error()), nil
        }
        defer session.Close()

        msg, err := session.ChannelMessageSend(channelID, text)
        if err != nil {
            return mcp.NewToolResultError(fmt.Sprintf("failed to send: %v", err)), nil
        }
        return mcp.NewToolResultText(fmt.Sprintf("Message sent (ID: %s)", msg.ID)), nil
    })

    // ── list_guilds ───────────────────────────────────────────────────────────
    s.AddTool(mcp.NewTool("list_guilds",
        mcp.WithDescription("List all Discord servers (guilds) the account is in."),
    ), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        token, err := discord.LoadToken(config.TokenPath(storeDir))
        if err != nil {
            return mcp.NewToolResultError(err.Error()), nil
        }
        session, err := discord.NewSession(token)
        if err != nil {
            return mcp.NewToolResultError(err.Error()), nil
        }
        defer session.Close()

        guilds, err := session.UserGuilds(100, "", "", false)
        if err != nil {
            return mcp.NewToolResultError(err.Error()), nil
        }
        var out string
        for _, g := range guilds {
            out += fmt.Sprintf("ID: %s  Name: %s\n", g.ID, g.Name)
        }
        return mcp.NewToolResultText(out), nil
    })

    // ── list_channels ─────────────────────────────────────────────────────────
    s.AddTool(mcp.NewTool("list_channels",
        mcp.WithDescription("List channels in a Discord server."),
        mcp.WithString("guild_id",
            mcp.Required(),
            mcp.Description("Guild (server) ID"),
        ),
    ), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        guildID := req.Params.Arguments["guild_id"].(string)

        token, err := discord.LoadToken(config.TokenPath(storeDir))
        if err != nil {
            return mcp.NewToolResultError(err.Error()), nil
        }
        session, err := discord.NewSession(token)
        if err != nil {
            return mcp.NewToolResultError(err.Error()), nil
        }
        defer session.Close()

        channels, err := session.GuildChannels(guildID)
        if err != nil {
            return mcp.NewToolResultError(err.Error()), nil
        }
        var out string
        for _, c := range channels {
            if c.Type == 0 { // text channels only
                out += fmt.Sprintf("ID: %s  #%s\n", c.ID, c.Name)
            }
        }
        return mcp.NewToolResultText(out), nil
    })

    // ── sync_channel ──────────────────────────────────────────────────────────
    s.AddTool(mcp.NewTool("sync_channel",
        mcp.WithDescription("Sync message history from a specific channel into the local database."),
        mcp.WithString("channel_id",
            mcp.Required(),
            mcp.Description("Channel ID to sync"),
        ),
    ), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        channelID := req.Params.Arguments["channel_id"].(string)

        // ... sync logic here, return count
        return mcp.NewToolResultText(fmt.Sprintf("Synced channel %s", channelID)), nil
    })
}
```

### `cmd/discocli/serve.go`

```go
package commands

import (
    "github.com/spf13/cobra"
    "github.com/YOUR_USERNAME/discocli/internal/mcp"
)

var serveCmd = &cobra.Command{
    Use:   "serve",
    Short: "Start the MCP server (for AI agents)",
    Long: `Start discocli as an MCP server over stdio.

Add this to your mcp.json:

  "discocli": {
    "command": "discocli",
    "args": ["serve"],
    "env": {},
    "disabled": false,
    "autoApprove": []
  }

The token from ~/.discocli/token is used automatically.`,
    RunE: func(cmd *cobra.Command, args []string) error {
        return mcp.Serve(resolvedStore())
    },
}

func init() {
    rootCmd.AddCommand(serveCmd)
}
```

---

## 9. Phase 5 — Releases (GoReleaser + Homebrew)

### `.goreleaser.yaml`

```yaml
version: 2

project_name: discocli

before:
  hooks:
    - go mod tidy
    - go generate ./...

builds:
  - id: discocli
    main: ./cmd/discocli/
    binary: discocli
    env:
      - CGO_ENABLED=0          # pure Go — no CGO needed with modernc.org/sqlite
    goos:
      - linux
      - darwin
      - windows
    goarch:
      - amd64
      - arm64
    ldflags:
      - -s -w                  # strip debug info — smaller binary
      - -X main.version={{.Version}}
      - -X main.commit={{.Commit}}
      - -X main.date={{.Date}}

archives:
  - id: default
    format: tar.gz
    name_template: "{{ .ProjectName }}_{{ .Os }}_{{ .Arch }}"
    format_overrides:
      - goos: windows
        format: zip
    files:
      - README.md
      - LICENSE

checksum:
  name_template: "checksums.txt"

changelog:
  sort: asc
  filters:
    exclude:
      - "^docs:"
      - "^test:"
      - "^chore:"
      - Merge pull request
      - Merge branch

brews:
  - name: discocli
    repository:
      owner: YOUR_USERNAME
      name: homebrew-tap              # you need a repo called homebrew-tap
      token: "{{ .Env.HOMEBREW_TAP_GITHUB_TOKEN }}"
    homepage: "https://github.com/YOUR_USERNAME/discocli"
    description: "Discord CLI — sync, search, send from your terminal. MCP-ready for AI agents."
    license: "MIT"
    test: |
      system "#{bin}/discocli --version"
    install: |
      bin.install "discocli"
```

### Homebrew Tap Setup

You need a separate GitHub repo named `homebrew-tap`:

```
github.com/YOUR_USERNAME/homebrew-tap/
└── Formula/
    └── discocli.rb   ← auto-generated by GoReleaser
```

Create the repo, then GoReleaser will push the formula on every release automatically.

### Creating a Release

```bash
# Tag a version
git tag v1.0.0
git push origin v1.0.0

# GoReleaser picks it up via GitHub Actions (see next phase)
# Or run locally:
goreleaser release --clean
```

### Version Info in Binary

Add this to `cmd/discocli/root.go`:

```go
var (
    version = "dev"
    commit  = "none"
    date    = "unknown"
)

func init() {
    rootCmd.Version = fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date)
}
```

---

## 10. Phase 6 — GitHub Actions CI/CD

### `.github/workflows/ci.yml`

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  lint-and-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"
          cache: true

      - name: Lint
        uses: golangci/golangci-lint-action@v4
        with:
          version: latest

      - name: Test
        run: go test ./... -v -race -coverprofile=coverage.out

      - name: Build
        run: go build ./cmd/discocli/
```

### `.github/workflows/release.yml`

```yaml
name: Release

on:
  push:
    tags:
      - "v*"

permissions:
  contents: write

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0   # GoReleaser needs full git history for changelog

      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"
          cache: true

      - name: Run GoReleaser
        uses: goreleaser/goreleaser-action@v5
        with:
          version: latest
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          HOMEBREW_TAP_GITHUB_TOKEN: ${{ secrets.HOMEBREW_TAP_GITHUB_TOKEN }}
```

> **Setup:** In your GitHub repo Settings → Secrets, add `HOMEBREW_TAP_GITHUB_TOKEN` — a Personal Access Token with `repo` scope, so GoReleaser can push to your `homebrew-tap` repo.

---

## 11. Best Practices & Conventions

### Error Handling

```go
// Always wrap errors with context
if err != nil {
    return fmt.Errorf("sync failed for channel %s: %w", channelID, err)
}
```

### Graceful Shutdown (for --follow / serve)

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()

// Pass ctx to your goroutines
// When Ctrl+C is pressed, ctx.Done() fires
select {
case <-ctx.Done():
    fmt.Println("\nShutting down...")
}
```

### Token Security

- Store at `~/.discocli/token` with `0600` perms (only owner can read)
- Never log the token — not even in debug mode
- Never commit `.discocli/` to git — add to `.gitignore`

### `.gitignore`

```
.discocli/
dist/
*.db
token
*.log
```

### Logging

Use `go.uber.org/zap` for structured logging. Suppress all logs unless `--verbose` flag is set. Clean CLI output is king.

```go
var verbose bool

func initLogger() *zap.Logger {
    if verbose {
        l, _ := zap.NewDevelopment()
        return l
    }
    return zap.NewNop() // silence in normal mode
}
```

### Database Best Practices

- Always open with `?_journal_mode=WAL` — allows concurrent reads while writing
- Use `?_foreign_keys=on` — enforces referential integrity
- Run migrations idempotently with `CREATE TABLE IF NOT EXISTS`
- Use `INSERT OR IGNORE` or `INSERT OR REPLACE` for synced data (upsert pattern)

```go
// Upsert pattern for sync — never fail on duplicates
_, err = db.Exec(`
    INSERT OR REPLACE INTO messages (id, channel_id, author_id, author_name, content, timestamp)
    VALUES (?, ?, ?, ?, ?, ?)
`, msg.ID, msg.ChannelID, msg.AuthorID, msg.AuthorName, msg.Content, msg.Timestamp)
```

### Naming the Repo

Options to consider:
- `discocli` — clean, direct
- `discli` — shorter
- `discord-cli` — more searchable on GitHub
- `discsync` — emphasizes the sync angle

Whatever you pick, make sure the binary name matches.

---

## 12. README Template

````markdown
# 🎮 discocli — Discord CLI + MCP Server

Interact with Discord from your terminal. Built for developers and AI agents.

```bash
discocli sync --follow          # real-time message capture
discocli search "deployment"    # offline full-text search
discocli send --to "#general" --text "Build passed ✅"
discocli serve                  # start MCP server for AI agents
```

## Install

### Homebrew (macOS & Linux)
```bash
brew install YOUR_USERNAME/tap/discocli
```

### Build from source
```bash
git clone https://github.com/YOUR_USERNAME/discocli
cd discocli
go build -o discocli ./cmd/discocli/
```

## Quick Start

```bash
# 1. Authenticate (bot token or user token)
discocli auth

# 2. Sync messages
discocli sync

# 3. Search offline
discocli search "standup notes"

# 4. Send a message
discocli send --to "#general" --text "Hello from the terminal!"
```

## MCP Server (AI Agents)

Add to your `mcp.json`:

```json
"discocli": {
  "command": "discocli",
  "args": ["serve"],
  "env": {},
  "disabled": false,
  "autoApprove": []
}
```

Available MCP tools: `search_messages`, `send_message`, `list_guilds`, `list_channels`, `sync_channel`

## Inspired by

- [steipete/wacli](https://github.com/steipete/wacli) — WhatsApp CLI
- [virat-mankali/telegram-cli](https://github.com/virat-mankali/telegram-cli) — Telegram CLI
````

---

## 13. MCP Config Example

Once built, users add this to their Claude/Cursor `mcp.json`:

```jsonc
{
  "mcpServers": {
    "discocli": {
      "command": "discocli",
      "args": ["serve"],
      "env": {},
      "disabled": false,
      "autoApprove": [
        "search_messages",
        "list_guilds",
        "list_channels"
      ]
    }
  }
}
```

> `autoApprove` for read-only tools is fine. Leave `send_message` and `sync_channel` out of autoApprove so the agent asks before sending.

---

## Build Order (Recommended)

Follow this order when coding with your agent:

1. `go mod init` + install dependencies
2. `internal/config/paths.go` — paths first, everything depends on this
3. `internal/storage/db.go` — schema + migrations
4. `internal/discord/auth.go` + `client.go` + `types.go`
5. `cmd/discocli/root.go` + `auth.go` — get auth working first
6. `internal/storage/messages.go` + `channels.go`
7. `cmd/discocli/sync.go` — get sync working, verify data in DB
8. `internal/storage/search.go`
9. `cmd/discocli/search.go` — verify search works
10. `cmd/discocli/send.go`
11. `cmd/discocli/guilds.go` + `channels.go`
12. `internal/mcp/server.go` + `tools.go`
13. `cmd/discocli/serve.go` — wire MCP
14. `.goreleaser.yaml` + GitHub Actions
15. README + project.md

---

*Built with the same philosophy as steipete: ship beats perfect. Solve your own problem, then share it with the world.*
