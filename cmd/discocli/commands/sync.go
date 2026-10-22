package commands

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/briandowns/spinner"
	"github.com/spf13/cobra"
	"github.com/langkurt/discord-cli/internal/config"
	"github.com/langkurt/discord-cli/internal/discord"
	"github.com/langkurt/discord-cli/internal/storage"
)

var (
	syncGuild   string
	syncChannel string
	syncFollow  bool
	syncSince   string
	syncAll     bool
	syncQuiet   bool
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync Discord messages to local database",
	Long: `Sync message history from Discord into the local SQLite database.
Without flags, syncs the default guild saved in <store>/guild, or all
accessible channels if no default is set (or --all is passed).
Use --follow to keep running and capture new messages in real-time; it
catches up on anything missed each time it (re)connects. Run it in the
background with ` + "`discocli daemon install`" + `.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		store := resolvedStore()
		if err := config.EnsureStore(store); err != nil {
			return err
		}

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

		if syncGuild == "" && !syncAll {
			syncGuild = config.DefaultGuild(store)
		}

		if syncFollow {
			return runGatewaySync(session, db, syncGuild)
		}

		var stopBefore string
		if syncSince != "" {
			t, err := discord.ParseSince(syncSince)
			if err != nil {
				return err
			}
			stopBefore = discord.TimeToSnowflake(t)
		}

		s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		s.Suffix = " Syncing messages..."
		s.Start()

		count, err := performHistoricalSync(session, db, syncGuild, syncChannel, stopBefore)
		s.Stop()

		if err != nil {
			return err
		}

		stats, _ := db.Stats()
		fmt.Printf("✅ Synced %d new messages\n", count)
		fmt.Printf("   Database: %d guilds, %d channels, %d total messages\n",
			stats.Guilds, stats.Channels, stats.Messages)
		return nil
	},
}

func performHistoricalSync(session *discordgo.Session, db *storage.DB, guild, channel, stopBefore string) (int, error) {
	// Resolve which channels to sync
	channelIDs, err := resolveTargetChannels(session, db, guild, channel)
	if err != nil {
		return 0, err
	}

	totalCount := 0
	for _, chID := range channelIDs {
		chName, _ := db.GetChannelName(chID)
		if chName == "" {
			chName = chID
		}
		start := time.Now()
		count, err := syncChannelHistory(session, db, chID, stopBefore)
		elapsed := time.Since(start).Round(time.Second)
		if err != nil {
			// Unreadable channels fail the same way every run; in the
			// background log they would only bury real problems.
			if !(syncQuiet && discord.IsForbidden(err)) {
				fmt.Fprintf(os.Stderr, "warning: failed to sync channel %s: %v\n", chName, err)
			}
			continue
		}
		if count > 0 && !syncQuiet {
			fmt.Printf("  #%-40s %5d msgs  %s\n", chName, count, elapsed)
		}
		totalCount += count
	}
	return totalCount, nil
}

func resolveTargetChannels(session *discordgo.Session, db *storage.DB, guild, channel string) ([]string, error) {
	// If specific channel ID given
	if channel != "" && isSnowflake(channel) {
		return []string{channel}, nil
	}

	// Get guilds to scan
	var guildIDs []string
	if guild != "" {
		if isSnowflake(guild) {
			guildIDs = []string{guild}
		} else {
			g, err := discord.ResolveGuildByName(session, guild)
			if err != nil {
				return nil, err
			}
			guildIDs = []string{g.ID}
		}
	} else {
		guilds, err := session.UserGuilds(100, "", "", false)
		if err != nil {
			return nil, fmt.Errorf("failed to list guilds: %w", err)
		}
		for _, g := range guilds {
			guildIDs = append(guildIDs, g.ID)
		}
	}

	var channelIDs []string
	for _, gID := range guildIDs {
		// Store guild metadata
		guild, err := session.Guild(gID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to fetch guild %s: %v\n", gID, err)
			continue
		}
		_ = db.UpsertGuild(guild.ID, guild.Name, guild.Icon)

		channels, err := session.GuildChannels(gID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to list channels for guild %s: %v\n", gID, err)
			continue
		}
		for _, ch := range channels {
			// Filter by channel name if specified
			if channel != "" && ch.Name != channel {
				continue
			}

			switch {
			case discord.IsSyncableChannel(ch.Type):
				_ = db.UpsertChannel(ch.ID, gID, ch.Name, int(ch.Type), ch.Topic, "")
				channelIDs = append(channelIDs, ch.ID)

			case discord.IsThreadContainer(ch.Type):
				_ = db.UpsertChannel(ch.ID, gID, ch.Name, int(ch.Type), ch.Topic, "")
				threads, err := discord.FetchThreads(session, ch.ID, gID)
				if err != nil {
					if !(syncQuiet && discord.IsForbidden(err)) {
						fmt.Fprintf(os.Stderr, "warning: failed to fetch threads for %s (#%s): %v\n", ch.ID, ch.Name, err)
					}
					continue
				}
				for _, t := range threads {
					_ = db.UpsertChannel(t.ID, gID, t.Name, int(t.Type), "", ch.ID)
					channelIDs = append(channelIDs, t.ID)
				}
			}
		}
	}
	return channelIDs, nil
}

func syncChannelHistory(session *discordgo.Session, db *storage.DB, channelID, stopBefore string) (int, error) {
	// Load existing sync state for incremental sync
	state, err := db.GetSyncState(channelID)
	if err != nil {
		return 0, err
	}

	count := 0
	var newestID, oldestID string

	// Phase A: fetch NEW messages since last sync (afterID pagination)
	if state != nil && state.LastMessageID != "" {
		afterID := state.LastMessageID
		for {
			msgs, err := session.ChannelMessages(channelID, 100, "", afterID, "")
			if err != nil || len(msgs) == 0 {
				break
			}
			for _, m := range msgs {
				if err := upsertMsg(db, m); err != nil {
					return count, err
				}
				if newestID == "" || m.ID > newestID {
					newestID = m.ID
				}
				count++
			}
			afterID = msgs[0].ID // Discord returns newest-first; walk forward
			time.Sleep(500 * time.Millisecond)
		}
	}

	// Phase B: fetch HISTORICAL messages (beforeID pagination, resume from oldest)
	beforeID := ""
	if state != nil && state.OldestMessageID != "" {
		beforeID = state.OldestMessageID // resume from where we stopped
	}

	err = discord.FetchMessages(session, channelID, func(msgs []*discordgo.Message) error {
		for _, m := range msgs {
			if err := upsertMsg(db, m); err != nil {
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
		return count, err
	}

	// Persist sync state
	if newestID != "" || oldestID != "" {
		n := newestID
		o := oldestID
		if state != nil {
			if state.LastMessageID > n {
				n = state.LastMessageID
			}
			if o == "" || (state.OldestMessageID != "" && state.OldestMessageID < o) {
				o = state.OldestMessageID
			}
		}
		_ = db.UpdateSyncState(channelID, n, o)
	}
	return count, nil
}

func upsertMsg(db *storage.DB, m *discordgo.Message) error {
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
	for _, a := range m.Attachments {
		_ = db.UpsertAttachment(a.ID, m.ID, m.ChannelID, a.URL, a.Filename, a.ContentType, int64(a.Size))
	}
	// Capture embed images: use Discord's own CDN proxy URL (no scraping needed).
	// m.Embeds is populated when Discord generates a preview for a linked URL.
	for _, emb := range m.Embeds {
		if emb.Image == nil {
			continue
		}
		srcURL := emb.Image.URL
		proxyURL := emb.Image.ProxyURL
		if srcURL == "" && proxyURL == "" {
			continue
		}
		if srcURL == "" {
			srcURL = proxyURL
		}
		id := discord.LinkID(m.ID, srcURL)
		_ = db.UpsertLink(id, m.ID, m.ChannelID, srcURL, proxyURL)
	}
	// Also capture text URLs from content for channels where embeds weren't generated.
	for _, u := range discord.ExtractURLs(m.Content) {
		id := discord.LinkID(m.ID, u)
		_ = db.UpsertLink(id, m.ID, m.ChannelID, u, "")
	}
	return nil
}

// runGatewaySync stores new messages as the gateway delivers them. On every
// fresh connection (startup, or a reconnect Discord could not resume) it runs
// an incremental REST sync in the background, so nothing sent while the
// machine was asleep or offline is missed. Resumed connections need no
// catch-up: Discord replays the missed events itself.
func runGatewaySync(session *discordgo.Session, db *storage.DB, guild string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	guildID := guild
	if guild != "" && !isSnowflake(guild) {
		g, err := discord.ResolveGuildByName(session, guild)
		if err != nil {
			return err
		}
		guildID = g.ID
	}

	logf := func(format string, args ...any) {
		fmt.Printf("%s "+format+"\n", append([]any{time.Now().Format(time.RFC3339)}, args...)...)
	}

	var catchingUp sync.Mutex
	session.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		go func() {
			if !catchingUp.TryLock() {
				return
			}
			defer catchingUp.Unlock()
			logf("connected; catching up")
			start := time.Now()
			n, err := performHistoricalSync(s, db, guildID, "", "")
			if err != nil {
				logf("catch-up failed: %v", err)
				return
			}
			logf("caught up: %d new messages in %s", n, time.Since(start).Round(time.Second))
		}()
	})

	store := func(s *discordgo.Session, m *discordgo.Message) {
		if m.Author == nil || (guildID != "" && m.GuildID != guildID) {
			return
		}
		name := ensureChannel(s, db, m.ChannelID)
		if err := upsertMsg(db, m); err != nil {
			logf("store message %s: %v", m.ID, err)
			return
		}
		if !syncQuiet {
			fmt.Printf("[%s] #%s — %s: %s\n",
				m.Timestamp.Local().Format("15:04:05"), name, m.Author.Username, m.Content)
		}
	}
	session.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) { store(s, m.Message) })
	session.AddHandler(func(s *discordgo.Session, m *discordgo.MessageUpdate) { store(s, m.Message) })

	markDeleted := func(guild, id string) {
		if guildID != "" && guild != guildID {
			return
		}
		chID, author, content, err := db.MarkMessageDeleted(id)
		if err != nil {
			logf("%v", err)
			return
		}
		if !syncQuiet {
			name, _ := db.GetChannelName(chID)
			fmt.Printf("[deleted] #%s — %s: %s\n", name, author, content)
		}
	}
	session.AddHandler(func(s *discordgo.Session, m *discordgo.MessageDelete) { markDeleted(m.GuildID, m.ID) })
	session.AddHandler(func(s *discordgo.Session, m *discordgo.MessageDeleteBulk) {
		for _, id := range m.Messages {
			markDeleted(m.GuildID, id)
		}
	})

	// User accounts get empty message content unless the intent is asked for.
	session.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages | discordgo.IntentsMessageContent
	if err := session.Open(); err != nil {
		return fmt.Errorf("failed to open gateway: %w", err)
	}
	defer session.Close()

	if guildID != "" {
		logf("live sync active for guild %s — press Ctrl+C to stop", guildID)
	} else {
		logf("live sync active for all guilds — press Ctrl+C to stop")
	}
	<-ctx.Done()
	logf("stopping")
	return nil
}

// ensureChannel returns the channel's name, recording the channel first if
// this is the first message seen in it (e.g. a thread created after the last
// sync) so it shows up in search and channel listings.
func ensureChannel(s *discordgo.Session, db *storage.DB, channelID string) string {
	if name, _ := db.GetChannelName(channelID); name != "" {
		return name
	}
	ch, err := s.State.Channel(channelID)
	if err != nil {
		if ch, err = s.Channel(channelID); err != nil {
			return channelID
		}
	}
	_ = db.UpsertChannel(ch.ID, ch.GuildID, ch.Name, int(ch.Type), ch.Topic, ch.ParentID)
	return ch.Name
}

func init() {
	syncCmd.Flags().StringVar(&syncGuild, "guild", "", "Guild (server) name or ID to sync")
	syncCmd.Flags().StringVar(&syncChannel, "channel", "", "Channel name or ID to sync")
	syncCmd.Flags().BoolVar(&syncAll, "all", false, "Sync all guilds, ignoring the default guild")
	syncCmd.Flags().BoolVar(&syncFollow, "follow", false, "Keep running and sync new messages in real-time")
	syncCmd.Flags().BoolVar(&syncQuiet, "quiet", false, "Do not print messages or per-channel progress (for background use)")
	syncCmd.Flags().StringVar(&syncSince, "since", "", "Only sync messages on/after this date (e.g. 30d, 6m, 1y, 2026-01-01)")
	rootCmd.AddCommand(syncCmd)
}
