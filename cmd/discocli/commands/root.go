package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/langkurt/discord-cli/internal/config"
)

var (
	storeDir string
	version  = "dev"
	commit   = "none"
	date     = "unknown"
)

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
	rootCmd.Version = fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date)
}

// resolvedStore returns the effective store directory.
func resolvedStore() string {
	return config.StoreDir(storeDir)
}
