package commands

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/virat-mankali/discord-cli/internal/config"
	"github.com/virat-mankali/discord-cli/internal/discord"
)

var channelsGuild string

var channelsCmd = &cobra.Command{
	Use:   "channels",
	Short: "List channels in a Discord server",
	RunE: func(cmd *cobra.Command, args []string) error {
		if channelsGuild == "" {
			return fmt.Errorf("--guild is required")
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

		// Resolve guild name to ID if needed
		guildID := channelsGuild
		if !isSnowflake(guildID) {
			g, err := discord.ResolveGuildByName(session, channelsGuild)
			if err != nil {
				return err
			}
			guildID = g.ID
		}

		channels, err := session.GuildChannels(guildID)
		if err != nil {
			return fmt.Errorf("failed to list channels: %w", err)
		}

		count := 0
		for _, c := range channels {
			if !discord.IsSyncableChannel(c.Type) && !discord.IsThreadContainer(c.Type) {
				continue
			}
			label := discord.ChannelTypeLabel(c.Type)
			fmt.Printf("  %s  #%-30s  [%s]\n", c.ID, c.Name, label)
			count++
		}
		fmt.Printf("\n%d syncable channel(s)\n", count)
		return nil
	},
}

// isSnowflake checks if a string looks like a Discord snowflake ID (all digits).
func isSnowflake(s string) bool {
	if len(s) < 17 || len(s) > 20 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func init() {
	channelsCmd.Flags().StringVar(&channelsGuild, "guild", "", "Guild name or ID (required)")
	channelsCmd.MarkFlagRequired("guild")
	rootCmd.AddCommand(channelsCmd)
}
