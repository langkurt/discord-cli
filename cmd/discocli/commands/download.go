package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/briandowns/spinner"
	"github.com/spf13/cobra"
	"github.com/virat-mankali/discord-cli/internal/config"
	"github.com/virat-mankali/discord-cli/internal/discord"
	"github.com/virat-mankali/discord-cli/internal/storage"
	"time"
)

var (
	dlGuild   string
	dlChannel string
	dlType    string
	dlOut     string
	dlLimit   int
)

var downloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download attachments (images, GIFs, videos) from synced messages",
	Long: `Download media attachments from locally synced Discord messages.

Runs against the local database — no Discord API calls needed.
Run 'discocli sync' first to populate attachment metadata.

Examples:
  discocli download                              # download everything
  discocli download --type gif                   # only GIFs
  discocli download --type image                 # images (not GIFs)
  discocli download --guild "My Server" --channel "memes" --type gif --limit 50
  discocli download --out ~/Desktop/discord-media`,

	RunE: func(cmd *cobra.Command, args []string) error {
		store := resolvedStore()

		db, err := storage.Open(config.DBPath(store))
		if err != nil {
			return err
		}
		defer db.Close()

		// Validate --type
		validTypes := map[string]bool{"image": true, "gif": true, "video": true, "all": true}
		if !validTypes[dlType] {
			return fmt.Errorf("invalid --type %q: must be one of image, gif, video, all", dlType)
		}

		// Resolve guild/channel names to IDs for filtering
		var guildID, channelID, guildName, channelName string

		if dlGuild != "" {
			guilds, err := db.ListSyncedChannels()
			if err != nil {
				return fmt.Errorf("listing guilds: %w", err)
			}
			// Collect unique guild names → match
			seen := map[string]string{} // name → first channel's guild reference
			for _, ch := range guilds {
				if ch.GuildName == dlGuild {
					guildName = ch.GuildName
					// GuildID not stored in SyncedChannel — use channel filter instead
					_ = seen
					break
				}
			}
			if guildName == "" {
				return fmt.Errorf("guild %q not found in local database (run sync first)", dlGuild)
			}
		}

		if dlChannel != "" {
			channels, err := db.ListSyncedChannels()
			if err != nil {
				return fmt.Errorf("listing channels: %w", err)
			}
			for _, ch := range channels {
				if ch.ChannelName == dlChannel && (dlGuild == "" || ch.GuildName == dlGuild) {
					channelID = ch.ChannelID
					channelName = ch.ChannelName
					if guildName == "" {
						guildName = ch.GuildName
					}
					break
				}
			}
			if channelID == "" {
				return fmt.Errorf("channel %q not found in local database (run sync first)", dlChannel)
			}
		}

		// Show pre-download stats
		pending, downloaded, err := db.AttachmentStats(channelID)
		if err != nil {
			return fmt.Errorf("stats error: %w", err)
		}
		fmt.Printf("Attachments in scope: %d pending, %d already downloaded\n", pending, downloaded)
		if pending == 0 {
			fmt.Println("Nothing to download.")
			return nil
		}

		// Resolve output dir (expand ~)
		outDir := dlOut
		if len(outDir) >= 2 && outDir[:2] == "~/" {
			home, _ := os.UserHomeDir()
			outDir = filepath.Join(home, outDir[2:])
		}

		opts := discord.DownloadOptions{
			ChannelID:   channelID,
			GuildID:     guildID,
			MediaType:   dlType,
			OutDir:      outDir,
			Limit:       dlLimit,
			GuildName:   guildName,
			ChannelName: channelName,
		}

		s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		s.Suffix = " Downloading..."
		s.Start()

		currentFile := ""
		result, err := discord.DownloadAttachments(db, opts, func(i, total int, filename string) {
			currentFile = filename
			s.Suffix = fmt.Sprintf(" [%d/%d] %s", i, total, currentFile)
		})
		s.Stop()

		if err != nil {
			return err
		}

		fmt.Printf("✅ Downloaded: %d  Skipped: %d  Failed: %d\n", result.Downloaded, result.Skipped, result.Failed)
		if result.Downloaded > 0 {
			fmt.Printf("   Saved to: %s\n", outDir)
		}
		return nil
	},
}

func init() {
	downloadCmd.Flags().StringVar(&dlGuild, "guild", "", "Filter by server name")
	downloadCmd.Flags().StringVar(&dlChannel, "channel", "", "Filter by channel name")
	downloadCmd.Flags().StringVar(&dlType, "type", "all", "Media type: image, gif, video, all")
	downloadCmd.Flags().StringVar(&dlOut, "out", "~/.discocli/media", "Output directory")
	downloadCmd.Flags().IntVar(&dlLimit, "limit", 0, "Max files to download (0 = unlimited)")
	rootCmd.AddCommand(downloadCmd)
}
