package mcp

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/bwmarrin/discordgo"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/virat-mankali/discord-cli/internal/config"
	"github.com/virat-mankali/discord-cli/internal/discord"
	"github.com/virat-mankali/discord-cli/internal/storage"
)

// RegisterTools adds all discocli MCP tools to the server.
func RegisterTools(s *server.MCPServer, storeDir string) {
	registerSearchMessages(s, storeDir)
	registerSendMessage(s, storeDir)
	registerListGuilds(s, storeDir)
	registerListChannels(s, storeDir)
	registerSyncChannel(s, storeDir)
	registerGetSyncStatus(s, storeDir)
	registerDownloadAttachments(s, storeDir)
}

// ── search_messages ──────────────────────────────────────────────────────────

func registerSearchMessages(s *server.MCPServer, storeDir string) {
	tool := mcplib.NewTool("search_messages",
		mcplib.WithDescription("Search Discord messages stored locally using full-text search. Messages must be synced first with sync_channel."),
		mcplib.WithString("query",
			mcplib.Required(),
			mcplib.Description("Full-text search query"),
		),
		mcplib.WithString("channel",
			mcplib.Description("Filter by channel name (optional)"),
		),
		mcplib.WithString("guild",
			mcplib.Description("Filter by server/guild name (optional)"),
		),
		mcplib.WithNumber("limit",
			mcplib.Description("Max results to return (default 20, max 100)"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		query, err := req.RequireString("query")
		if err != nil {
			return mcplib.NewToolResultError(err.Error()), nil
		}

		channel := req.GetString("channel", "")
		guild := req.GetString("guild", "")
		limit := int(req.GetFloat("limit", 20))
		if limit > 100 {
			limit = 100
		}

		db, err := storage.Open(config.DBPath(storeDir))
		if err != nil {
			return mcplib.NewToolResultError(fmt.Sprintf("database error: %v", err)), nil
		}
		defer db.Close()

		results, err := db.SearchMessages(query, storage.SearchOptions{
			Channel: channel,
			Guild:   guild,
			Limit:   limit,
		})
		if err != nil {
			return mcplib.NewToolResultError(fmt.Sprintf("search error: %v", err)), nil
		}

		if len(results) == 0 {
			return mcplib.NewToolResultText("No results found. Run sync_channel first to fetch messages."), nil
		}

		var sb strings.Builder
		for _, r := range results {
			fmt.Fprintf(&sb, "[%s] #%s (%s) — %s: %s\n",
				r.Timestamp.Format("2006-01-02 15:04"),
				r.ChannelName, r.GuildName, r.AuthorName, r.Content)
		}
		fmt.Fprintf(&sb, "\n%d result(s) for %q", len(results), query)
		return mcplib.NewToolResultText(sb.String()), nil
	})
}

// ── send_message ─────────────────────────────────────────────────────────────

func registerSendMessage(s *server.MCPServer, storeDir string) {
	tool := mcplib.NewTool("send_message",
		mcplib.WithDescription("Send a text message to a Discord channel or DM. Requires the channel ID — use list_channels to find it."),
		mcplib.WithString("channel_id",
			mcplib.Required(),
			mcplib.Description("Discord channel ID to send to"),
		),
		mcplib.WithString("text",
			mcplib.Required(),
			mcplib.Description("Message text to send"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		channelID, err := req.RequireString("channel_id")
		if err != nil {
			return mcplib.NewToolResultError(err.Error()), nil
		}
		text, err := req.RequireString("text")
		if err != nil {
			return mcplib.NewToolResultError(err.Error()), nil
		}

		session, cleanup, err := newSession(storeDir)
		if err != nil {
			return mcplib.NewToolResultError(err.Error()), nil
		}
		defer cleanup()

		msg, err := session.ChannelMessageSend(channelID, text)
		if err != nil {
			return mcplib.NewToolResultError(fmt.Sprintf("failed to send: %v", err)), nil
		}
		return mcplib.NewToolResultText(fmt.Sprintf("Message sent (ID: %s)", msg.ID)), nil
	})
}

// ── list_guilds ──────────────────────────────────────────────────────────────

func registerListGuilds(s *server.MCPServer, storeDir string) {
	tool := mcplib.NewTool("list_guilds",
		mcplib.WithDescription("List all Discord servers (guilds) the account has access to."),
	)

	s.AddTool(tool, func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		session, cleanup, err := newSession(storeDir)
		if err != nil {
			return mcplib.NewToolResultError(err.Error()), nil
		}
		defer cleanup()

		guilds, err := session.UserGuilds(100, "", "", false)
		if err != nil {
			return mcplib.NewToolResultError(fmt.Sprintf("failed to list guilds: %v", err)), nil
		}

		if len(guilds) == 0 {
			return mcplib.NewToolResultText("No guilds found."), nil
		}

		var sb strings.Builder
		for _, g := range guilds {
			fmt.Fprintf(&sb, "ID: %s  Name: %s\n", g.ID, g.Name)
		}
		fmt.Fprintf(&sb, "\n%d guild(s)", len(guilds))
		return mcplib.NewToolResultText(sb.String()), nil
	})
}

// ── list_channels ────────────────────────────────────────────────────────────

func registerListChannels(s *server.MCPServer, storeDir string) {
	tool := mcplib.NewTool("list_channels",
		mcplib.WithDescription("List text channels in a Discord server. Use list_guilds first to get the guild ID."),
		mcplib.WithString("guild_id",
			mcplib.Required(),
			mcplib.Description("Guild (server) ID"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		guildID, err := req.RequireString("guild_id")
		if err != nil {
			return mcplib.NewToolResultError(err.Error()), nil
		}

		session, cleanup, err := newSession(storeDir)
		if err != nil {
			return mcplib.NewToolResultError(err.Error()), nil
		}
		defer cleanup()

		channels, err := session.GuildChannels(guildID)
		if err != nil {
			return mcplib.NewToolResultError(fmt.Sprintf("failed to list channels: %v", err)), nil
		}

		var sb strings.Builder
		count := 0
		for _, c := range channels {
			if c.Type == discordgo.ChannelTypeGuildText {
				fmt.Fprintf(&sb, "ID: %s  #%s\n", c.ID, c.Name)
				count++
			}
		}
		fmt.Fprintf(&sb, "\n%d text channel(s)", count)
		return mcplib.NewToolResultText(sb.String()), nil
	})
}

// ── sync_channel ─────────────────────────────────────────────────────────────

func registerSyncChannel(s *server.MCPServer, storeDir string) {
	tool := mcplib.NewTool("sync_channel",
		mcplib.WithDescription("Sync message history from a specific Discord channel into the local database for searching. Use list_channels to find the channel ID."),
		mcplib.WithString("channel_id",
			mcplib.Required(),
			mcplib.Description("Channel ID to sync"),
		),
		mcplib.WithString("guild_id",
			mcplib.Description("Guild ID the channel belongs to (helps store metadata)"),
		),
		mcplib.WithString("since",
			mcplib.Description("Only sync messages on/after this date (e.g. 30d, 6m, 1y, 2026-01-01)"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		channelID, err := req.RequireString("channel_id")
		if err != nil {
			return mcplib.NewToolResultError(err.Error()), nil
		}
		guildID := req.GetString("guild_id", "")
		sinceStr := req.GetString("since", "")

		var stopBefore string
		if sinceStr != "" {
			t, err := discord.ParseSince(sinceStr)
			if err != nil {
				return mcplib.NewToolResultError(fmt.Sprintf("invalid since: %v", err)), nil
			}
			stopBefore = discord.TimeToSnowflake(t)
		}

		session, cleanup, err := newSession(storeDir)
		if err != nil {
			return mcplib.NewToolResultError(err.Error()), nil
		}
		defer cleanup()

		db, err := storage.Open(config.DBPath(storeDir))
		if err != nil {
			return mcplib.NewToolResultError(fmt.Sprintf("database error: %v", err)), nil
		}
		defer db.Close()

		// Store channel metadata if we have guild info
		if guildID != "" {
			ch, chErr := session.Channel(channelID)
			if chErr == nil {
				_ = db.UpsertChannel(ch.ID, guildID, ch.Name, int(ch.Type), ch.Topic, ch.ParentID)
			}
			guild, gErr := session.Guild(guildID)
			if gErr == nil {
				_ = db.UpsertGuild(guild.ID, guild.Name, guild.Icon)
			}
		}

		// Perform sync
		count := 0
		var newestID, oldestID string

		state, _ := db.GetSyncState(channelID)
		beforeID := ""
		if state != nil && state.OldestMessageID != "" {
			beforeID = state.OldestMessageID
		}

		err = discord.FetchMessages(session, channelID, func(msgs []*discordgo.Message) error {
			for _, m := range msgs {
				reactions := 0
				for _, r := range m.Reactions {
					reactions += r.Count
				}
				if err := db.UpsertMessage(
					m.ID, m.ChannelID, m.GuildID,
					m.Author.ID, m.Author.Username,
					m.Content, m.Timestamp,
					m.EditedTimestamp != nil && !m.EditedTimestamp.IsZero(),
					reactions,
				); err != nil {
					return err
				}
				if newestID == "" || m.ID > newestID {
					newestID = m.ID
				}
				if oldestID == "" || m.ID < oldestID {
					oldestID = m.ID
				}
				count++
			}
			return nil
		}, beforeID, stopBefore)
		if err != nil {
			return mcplib.NewToolResultError(fmt.Sprintf("sync failed: %v", err)), nil
		}

		if newestID != "" {
			_ = db.UpdateSyncState(channelID, newestID, oldestID)
		}

		return mcplib.NewToolResultText(fmt.Sprintf("Synced %d messages from channel %s", count, channelID)), nil
	})
}

// ── get_sync_status ──────────────────────────────────────────────────────────

func registerGetSyncStatus(s *server.MCPServer, storeDir string) {
	tool := mcplib.NewTool("get_sync_status",
		mcplib.WithDescription("Show which channels have been synced and how many messages are stored locally."),
	)

	s.AddTool(tool, func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		db, err := storage.Open(config.DBPath(storeDir))
		if err != nil {
			return mcplib.NewToolResultError(fmt.Sprintf("database error: %v", err)), nil
		}
		defer db.Close()

		stats, err := db.Stats()
		if err != nil {
			return mcplib.NewToolResultError(fmt.Sprintf("stats error: %v", err)), nil
		}

		channels, err := db.ListSyncedChannels()
		if err != nil {
			return mcplib.NewToolResultError(fmt.Sprintf("error listing channels: %v", err)), nil
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Database: %d guilds, %d channels, %d messages\n\n", stats.Guilds, stats.Channels, stats.Messages)

		if len(channels) == 0 {
			sb.WriteString("No channels synced yet. Use sync_channel to start.")
		} else {
			sb.WriteString("Synced channels:\n")
			for _, ch := range channels {
				fmt.Fprintf(&sb, "  #%s (%s) — %d messages, last synced %s\n",
					ch.ChannelName, ch.GuildName, ch.MessageCount,
					ch.SyncedAt.Format("2006-01-02 15:04"))
			}
		}
		return mcplib.NewToolResultText(sb.String()), nil
	})
}

// ── download_attachments ─────────────────────────────────────────────────────

func registerDownloadAttachments(s *server.MCPServer, storeDir string) {
	tool := mcplib.NewTool("download_attachments",
		mcplib.WithDescription("Download media attachments (images, GIFs, videos) from synced Discord messages to local disk."),
		mcplib.WithString("channel",
			mcplib.Description("Filter by channel name (optional)"),
		),
		mcplib.WithString("guild",
			mcplib.Description("Filter by server/guild name (optional)"),
		),
		mcplib.WithString("media_type",
			mcplib.Description("Media type to download: image, gif, video, all (default: all)"),
		),
		mcplib.WithNumber("limit",
			mcplib.Description("Max number of files to download (0 = unlimited)"),
		),
		mcplib.WithString("out_dir",
			mcplib.Description("Output directory (default: ~/.discocli/media)"),
		),
		mcplib.WithString("since",
			mcplib.Description("Only download attachments from messages on/after this date (e.g. 30d, 6m, 1y, 2026-01-01)"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		channel := req.GetString("channel", "")
		guild := req.GetString("guild", "")
		mediaType := req.GetString("media_type", "all")
		limit := int(req.GetFloat("limit", 0))
		outDir := req.GetString("out_dir", "~/.discocli/media")
		sinceStr := req.GetString("since", "")

		// Expand ~
		if len(outDir) >= 2 && outDir[:2] == "~/" {
			home, _ := os.UserHomeDir()
			outDir = home + outDir[1:]
		}

		db, err := storage.Open(config.DBPath(storeDir))
		if err != nil {
			return mcplib.NewToolResultError(fmt.Sprintf("database error: %v", err)), nil
		}
		defer db.Close()

		// Resolve channel name → ID
		var channelID, channelName, guildName string
		if channel != "" || guild != "" {
			channels, err := db.ListSyncedChannels()
			if err != nil {
				return mcplib.NewToolResultError(fmt.Sprintf("error listing channels: %v", err)), nil
			}
			for _, ch := range channels {
				if (channel == "" || ch.ChannelName == channel) && (guild == "" || ch.GuildName == guild) {
					channelID = ch.ChannelID
					channelName = ch.ChannelName
					guildName = ch.GuildName
					break
				}
			}
		}

		opts := discord.DownloadOptions{
			ChannelID:   channelID,
			MediaType:   mediaType,
			OutDir:      outDir,
			Limit:       limit,
			GuildName:   guildName,
			ChannelName: channelName,
		}

		if sinceStr != "" {
			t, err := discord.ParseSince(sinceStr)
			if err != nil {
				return mcplib.NewToolResultError(fmt.Sprintf("invalid since: %v", err)), nil
			}
			opts.Since = t
		}

		result, err := discord.DownloadAttachments(db, opts, nil)
		if err != nil {
			return mcplib.NewToolResultError(fmt.Sprintf("download error: %v", err)), nil
		}

		return mcplib.NewToolResultText(fmt.Sprintf(
			"Downloaded: %d  Failed: %d  Saved to: %s",
			result.Downloaded, result.Failed, outDir,
		)), nil
	})
}

// ── helpers ──────────────────────────────────────────────────────────────────

// newSession creates a Discord session from the stored token.
// Returns the session and a cleanup function.
func newSession(storeDir string) (*discordgo.Session, func(), error) {
	token, err := discord.LoadToken(config.TokenPath(storeDir))
	if err != nil {
		return nil, nil, err
	}
	session, err := discord.NewSession(token)
	if err != nil {
		return nil, nil, err
	}
	return session, func() { session.Close() }, nil
}
