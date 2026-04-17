package commands

import (
	"github.com/spf13/cobra"
	"github.com/langkurt/discord-cli/internal/mcp"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the MCP server (for AI agents)",
	Long: `Start discocli as an MCP server over stdio.

Add this to your mcp.json:

  "discocli": {
    "command": "discocli",
    "args": ["serve"],
    "env": {},
    "disabled": false,
    "autoApprove": ["search_messages", "list_guilds", "list_channels", "get_sync_status"]
  }

Available tools: search_messages, send_message, list_guilds, list_channels, sync_channel, get_sync_status

The token from ~/.discocli/token is used automatically.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return mcp.Serve(resolvedStore())
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
}
