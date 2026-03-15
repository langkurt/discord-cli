package commands

import (
	"fmt"

	"github.com/spf13/cobra"
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
    "autoApprove": []
  }

The token from ~/.discocli/token is used automatically.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// MCP server implementation will be wired in Phase 4
		fmt.Println("MCP server not yet implemented — coming in Phase 4")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
}
