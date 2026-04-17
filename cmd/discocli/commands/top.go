package commands

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/langkurt/discord-cli/internal/config"
	"github.com/langkurt/discord-cli/internal/storage"
)

var (
	topGuild   string
	topChannel string
	topLimit   int
)

var topCmd = &cobra.Command{
	Use:   "top",
	Short: "Show most-reacted messages in a channel",
	Long: `List messages ranked by total reaction count.

Examples:
  discocli top --channel "🎨fan-art" --guild "BrownDust2 Official"
  discocli top --channel "🎨fan-art" --guild "BrownDust2 Official" --limit 20`,
	RunE: func(cmd *cobra.Command, args []string) error {
		store := resolvedStore()

		db, err := storage.Open(config.DBPath(store))
		if err != nil {
			return err
		}
		defer db.Close()

		results, err := db.TopReacted(storage.TopOptions{
			Guild:   topGuild,
			Channel: topChannel,
			Limit:   topLimit,
		})
		if err != nil {
			return fmt.Errorf("query failed: %w", err)
		}

		if len(results) == 0 {
			fmt.Println("No results. Run sync to fetch messages with reaction data.")
			return nil
		}

		fmt.Printf("%-6s  %-20s  %-18s  %s\n", "React", "Author", "Date", "Content")
		fmt.Println("──────────────────────────────────────────────────────────────────────")
		for _, r := range results {
			content := r.Content
			if len([]rune(content)) > 60 {
				content = string([]rune(content)[:57]) + "..."
			}
			if content == "" {
				content = "[attachment]"
			}
			fmt.Printf("%-6d  %-20s  %-18s  %s\n",
				r.ReactionCount,
				truncate(r.AuthorName, 20),
				r.Timestamp.Format("2006-01-02 15:04"),
				content,
			)
		}
		return nil
	},
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func init() {
	topCmd.Flags().StringVar(&topGuild, "guild", "", "Filter by guild name")
	topCmd.Flags().StringVar(&topChannel, "channel", "", "Filter by channel name")
	topCmd.Flags().IntVar(&topLimit, "limit", 10, "Number of results to show")
	rootCmd.AddCommand(topCmd)
}

