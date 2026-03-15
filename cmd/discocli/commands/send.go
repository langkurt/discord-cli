package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/spf13/cobra"
	"github.com/virat-mankali/discord-cli/internal/config"
	"github.com/virat-mankali/discord-cli/internal/discord"
	"github.com/virat-mankali/discord-cli/internal/storage"
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

// resolveRecipient turns "#channel", "@username", or a raw ID into a channel ID.
func resolveRecipient(session *discordgo.Session, db *storage.DB, to string) (string, error) {
	// Raw snowflake ID
	if isSnowflake(to) {
		return to, nil
	}

	// #channel-name — search across guilds
	if strings.HasPrefix(to, "#") {
		name := strings.TrimPrefix(to, "#")
		guilds, err := session.UserGuilds(100, "", "", false)
		if err != nil {
			return "", err
		}
		for _, g := range guilds {
			ch, err := discord.ResolveChannelByName(session, g.ID, name)
			if err == nil {
				return ch.ID, nil
			}
		}
		return "", fmt.Errorf("channel #%s not found in any guild", name)
	}

	// @username — open a DM
	if strings.HasPrefix(to, "@") {
		username := strings.TrimPrefix(to, "@")
		// Search guilds for the user
		guilds, err := session.UserGuilds(100, "", "", false)
		if err != nil {
			return "", err
		}
		for _, g := range guilds {
			members, err := session.GuildMembersSearch(g.ID, username, 5)
			if err != nil {
				continue
			}
			for _, m := range members {
				if m.User.Username == username {
					dm, err := session.UserChannelCreate(m.User.ID)
					if err != nil {
						return "", fmt.Errorf("failed to open DM with %s: %w", username, err)
					}
					return dm.ID, nil
				}
			}
		}
		return "", fmt.Errorf("user @%s not found", username)
	}

	return "", fmt.Errorf("invalid recipient format — use #channel, @username, or a channel ID")
}

func init() {
	sendCmd.Flags().StringVar(&sendTo, "to", "", "Recipient: #channel-name, @username, or channel ID (required)")
	sendCmd.Flags().StringVar(&sendText, "text", "", "Text message to send")
	sendCmd.Flags().StringVar(&sendMedia, "media", "", "Path to file to send")
	sendCmd.MarkFlagRequired("to")
	rootCmd.AddCommand(sendCmd)
}
