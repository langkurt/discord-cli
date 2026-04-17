package commands

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/langkurt/discord-cli/internal/config"
	"github.com/langkurt/discord-cli/internal/discord"
)

var guildsCmd = &cobra.Command{
	Use:   "guilds",
	Short: "List all Discord servers you're in",
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

		guilds, err := session.UserGuilds(100, "", "", false)
		if err != nil {
			return fmt.Errorf("failed to list guilds: %w", err)
		}

		if len(guilds) == 0 {
			fmt.Println("No guilds found.")
			return nil
		}

		for _, g := range guilds {
			fmt.Printf("  %s  %s\n", g.ID, g.Name)
		}
		fmt.Printf("\n%d guild(s)\n", len(guilds))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(guildsCmd)
}
