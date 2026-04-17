package commands

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/langkurt/discord-cli/internal/config"
	"github.com/langkurt/discord-cli/internal/discord"
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
