package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/briandowns/spinner"
	"github.com/spf13/cobra"
	"github.com/virat-mankali/discord-cli/internal/config"
	"github.com/virat-mankali/discord-cli/internal/discord"
	"github.com/virat-mankali/discord-cli/internal/storage"
)

var (
	flGuild    string
	flChannel  string
	flOut      string
	flLimit    int
	flBackfill bool
)

var fetchLinksCmd = &cobra.Command{
	Use:   "fetch-links",
	Short: "Download images from linked content (Twitter, Pixiv, etc.) in synced messages",
	Long: `Downloads images from external links found in Discord messages.

Two download strategies (chosen automatically per link):
  • Discord CDN (fast, reliable): used when Discord generated an embed for the link.
    The image is served from Discord's own proxy — same as the in-app Download button.
  • og:image scrape (fallback): for text URLs with no embed, fetches the page and
    extracts the og:image meta tag.  Supports: fxtwitter.com, phixiv.net, artstation.com, etc.

Links are captured automatically during sync.  For channels synced before embed
support was added, clear sync_state and re-sync to populate Discord CDN URLs.
Use --backfill to extract text URLs from already-synced message content.

Examples:
  discocli fetch-links --guild "BrownDust2 Official" --channel "🎨fan-art"
  discocli fetch-links --guild "BrownDust2 Official" --channel "🎨fan-art" --backfill
  discocli fetch-links --channel "🎨fan-art" --limit 50`,

	RunE: func(cmd *cobra.Command, args []string) error {
		store := resolvedStore()

		db, err := storage.Open(config.DBPath(store))
		if err != nil {
			return err
		}
		defer db.Close()

		// Resolve channel name → ID
		var channelID, channelName, guildName string
		if flChannel != "" || flGuild != "" {
			channels, err := db.ListSyncedChannels()
			if err != nil {
				return fmt.Errorf("listing channels: %w", err)
			}
			for _, ch := range channels {
				if (flChannel == "" || ch.ChannelName == flChannel) &&
					(flGuild == "" || ch.GuildName == flGuild) {
					channelID = ch.ChannelID
					channelName = ch.ChannelName
					guildName = ch.GuildName
					break
				}
			}
			if flChannel != "" && channelID == "" {
				return fmt.Errorf("channel %q not found in local database (run sync first)", flChannel)
			}
		}

		// Backfill: scan existing messages and extract link URLs
		if flBackfill {
			if channelID == "" {
				return fmt.Errorf("--backfill requires --channel")
			}
			s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
			s.Suffix = " Scanning messages for links..."
			s.Start()
			count, err := discord.ExtractAndStoreLinks(db, channelID)
			s.Stop()
			if err != nil {
				return fmt.Errorf("backfill failed: %w", err)
			}
			fmt.Printf("✅ Extracted %d links from existing messages\n", count)
		}

		// Show stats
		pending, downloaded, failed, err := db.LinkStats(channelID)
		if err != nil {
			return fmt.Errorf("stats error: %w", err)
		}
		fmt.Printf("Links: %d pending, %d downloaded, %d failed\n", pending, downloaded, failed)
		if pending == 0 {
			if !flBackfill {
				fmt.Println("Nothing to download. Run with --backfill to extract links from existing messages.")
			}
			return nil
		}

		// Resolve output dir
		outDir := flOut
		if len(outDir) >= 2 && outDir[:2] == "~/" {
			home, _ := os.UserHomeDir()
			outDir = filepath.Join(home, outDir[2:])
		}

		opts := discord.FetchLinksOptions{
			ChannelID:   channelID,
			ChannelName: channelName,
			GuildName:   guildName,
			OutDir:      outDir,
			Limit:       flLimit,
		}

		s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		s.Start()

		result, err := discord.FetchAndDownloadLinks(db, opts, func(i, total int, url string) {
			short := url
			if len(short) > 50 {
				short = short[:47] + "..."
			}
			s.Suffix = fmt.Sprintf(" [%d/%d] %s", i, total, short)
		})
		s.Stop()

		if err != nil {
			return err
		}

		fmt.Printf("✅ Downloaded: %d  Failed: %d\n", result.Downloaded, result.Failed)
		if result.Downloaded > 0 {
			fmt.Printf("   Saved to: %s\n", outDir)
		}
		if result.Failed > 0 {
			fmt.Printf("   Failed links are marked — re-run to skip them next time.\n")
		}
		return nil
	},
}

func init() {
	fetchLinksCmd.Flags().StringVar(&flGuild, "guild", "", "Filter by guild name")
	fetchLinksCmd.Flags().StringVar(&flChannel, "channel", "", "Filter by channel name")
	fetchLinksCmd.Flags().StringVar(&flOut, "out", "~/.discocli/media", "Output directory")
	fetchLinksCmd.Flags().IntVar(&flLimit, "limit", 0, "Max links to process (0 = unlimited)")
	fetchLinksCmd.Flags().BoolVar(&flBackfill, "backfill", false, "Extract links from already-synced messages first")
	rootCmd.AddCommand(fetchLinksCmd)
}
