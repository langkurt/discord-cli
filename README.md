# 🎮 discocli — Discord CLI + MCP Server

Interact with Discord from your terminal. Built for developers and AI agents.

```bash
discocli sync --follow                  # real-time message capture
discocli search "deployment issue"      # offline full-text search
discocli send --to "#general" --text "Build passed ✅"
discocli serve                          # MCP server for AI agents
```

## Install

### Build from source

```bash
git clone https://github.com/langkurt/discord-cli
cd discord-cli
go build -o discocli ./cmd/discocli/
```

Move the binary somewhere on your `$PATH` (e.g. `~/bin/discocli`).

## Quick Start

```bash
# 1. Save your token
discocli auth

# 2. Sync message history
discocli sync --guild "My Server" --channel "general"

# 3. Search offline
discocli search "standup notes"

# 4. Send a message
discocli send --to "#general" --text "Hello from the terminal!"
```

## Commands

### Sync

```bash
discocli sync                                        # sync all accessible channels
discocli sync --guild "My Server"                    # one server
discocli sync --guild "My Server" --channel "art"    # one channel
discocli sync --since 30d                            # only messages from last 30 days
discocli sync --follow                               # real-time gateway sync (live tail)
```

Supports all channel types: text, announcement, voice, forum, media, and threads.
Incremental — resumes from where it left off on subsequent runs.

### Search

```bash
discocli search "query"
discocli search "query" --guild "My Server" --channel "general"
discocli search "query" --limit 20
```

Full-text search over locally synced messages using SQLite FTS5. All offline — no API calls.

### Send

```bash
discocli send --to "#general" --text "Hello from the terminal!"
discocli send --to "username#1234" --text "DM from the CLI"
```

### Guilds & Channels

```bash
discocli guilds                         # list all servers
discocli channels                       # list synced channels
discocli channels --guild "My Server"   # channels in a specific server
```

### Other

```bash
discocli auth      # save token to ~/.discocli/token
discocli whoami    # show authenticated user
discocli serve     # start MCP server (stdio)
```

---

## Media & Downloads

These commands build on the synced message database to pull media off Discord.

### download

```bash
discocli download                                          # all pending attachments
discocli download --channel "art" --type image             # images only (not GIFs)
discocli download --channel "art" --type gif
discocli download --channel "art" --type video
discocli download --channel "ai-art" --since 30d --min-reactions 3
discocli download --out ~/Desktop/media
```

Downloads direct Discord attachments (images, GIFs, videos) to disk.
Already-downloaded files are skipped on re-runs.

| Flag | Default | Description |
|------|---------|-------------|
| `--type` | `all` | `image`, `gif`, `video`, `all` |
| `--since` | — | `30d`, `6m`, `1y`, `2026-01-01` |
| `--min-reactions` | `0` | Only posts with ≥ N reactions |
| `--limit` | `0` | Max files (0 = unlimited) |
| `--out` | `~/.discocli/media` | Output directory |

### fetch-links

```bash
discocli fetch-links --guild "My Server" --channel "fan-art"
discocli fetch-links --channel "fan-art" --backfill   # scan old messages for URLs
discocli fetch-links --channel "fan-art" --limit 50
```

Downloads images from external links in messages (Twitter/X, Pixiv, ArtStation, Imgur, etc.).

Uses Discord's embed CDN proxy (`media.discordapp.net`) when available — same URL as the
in-app Download button. Falls back to `og:image` scraping for raw text URLs with no embed.

### top

```bash
discocli top --channel "fan-art"
discocli top --guild "My Server" --channel "fan-art" --limit 20
```

Shows the most-reacted messages in a channel.

## MCP Server (AI Agents)

Add to `~/.claude/.mcp.json`:

```json
{
  "mcpServers": {
    "discocli": {
      "command": "/Users/you/bin/discocli",
      "args": ["serve"]
    }
  }
}
```

### Available MCP Tools

| Tool | Description |
|------|-------------|
| `search_messages` | Full-text search across synced messages |
| `list_guilds` | List all Discord servers |
| `list_channels` | List channels in a server |
| `get_sync_status` | Show sync status and message counts |
| `send_message` | Send a message to a channel |
| `sync_channel` | Sync a channel's history |
| `download_attachments` | Download media from a channel |

## Local Storage

Everything is stored in `~/.discocli/`:

```
~/.discocli/
  token       # your Discord token (0600)
  data.db     # SQLite database (messages, attachments, links)
  media/      # downloaded files, organised by guild/channel
    Guild Name/
      channel-name/
        <msgID>_filename.jpg
      channel-name/links/
        <hash>_image.jpg
```

All searches are offline. No Discord API calls after sync.

## Token Setup

### User Token (Personal Use)

1. Open Discord in the browser
2. DevTools → Network → any API request → copy the `Authorization` header value
3. Run `discocli auth` and paste it

> ⚠️ User tokens are fine for personal/local use and AI agents acting on your behalf.
> They violate Discord's ToS if used for automation at scale.

### Bot Token

1. [Discord Developer Portal](https://discord.com/developers/applications) → New Application → Bot
2. Reset Token, enable **Message Content Intent** under Privileged Gateway Intents
3. Invite the bot with `Read Messages`, `Read Message History`, `Attach Files` permissions

## How It Works

- Messages sync into a local SQLite database with FTS5 full-text search
- Incremental sync tracks oldest/newest message IDs per channel — resumes automatically
- Rate limiting: 400–700ms jitter between batches, exponential backoff on 429s
- Attachment and embed-image URLs are captured during sync and downloaded on demand
- The MCP server speaks stdio — Claude Code spawns it on demand, no daemon needed

## License

MIT
