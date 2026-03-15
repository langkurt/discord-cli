package commands

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/virat-mankali/discord-cli/internal/config"
	"github.com/virat-mankali/discord-cli/internal/storage"
)

var (
	searchChannel string
	searchGuild   string
	searchLimit   int
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search synced messages offline",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store := resolvedStore()
		query := args[0]

		db, err := storage.Open(config.DBPath(store))
		if err != nil {
			return err
		}
		defer db.Close()

		results, err := db.SearchMessages(query, storage.SearchOptions{
			Channel: searchChannel,
			Guild:   searchGuild,
			Limit:   searchLimit,
		})
		if err != nil {
			return err
		}

		if len(results) == 0 {
			fmt.Println("No results found. Run `discocli sync` to fetch messages first.")
			return nil
		}

		for _, msg := range results {
			fmt.Printf("[%s] #%s (%s) — %s: %s\n",
				msg.Timestamp.Format("2006-01-02 15:04"),
				msg.ChannelName,
				msg.GuildName,
				msg.AuthorName,
				msg.Content,
			)
		}
		fmt.Printf("\n%d result(s) for %q\n", len(results), query)
		return nil
	},
}

func init() {
	searchCmd.Flags().StringVar(&searchChannel, "channel", "", "Filter by channel name")
	searchCmd.Flags().StringVar(&searchGuild, "guild", "", "Filter by guild name")
	searchCmd.Flags().IntVar(&searchLimit, "limit", 20, "Max results to return")
	rootCmd.AddCommand(searchCmd)
}
