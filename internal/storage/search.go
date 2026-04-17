package storage

import (
	"fmt"
	"strings"
	"time"
)

// TopOptions filters for TopReacted queries.
type TopOptions struct {
	Guild   string
	Channel string
	Limit   int
}

// TopReactedResult is a message ranked by reaction count.
type TopReactedResult struct {
	MessageID     string
	AuthorName    string
	Content       string
	Timestamp     time.Time
	ReactionCount int
	ChannelName   string
	GuildName     string
}

// TopReacted returns messages ranked by reaction_count descending.
func (db *DB) TopReacted(opts TopOptions) ([]TopReactedResult, error) {
	if opts.Limit == 0 {
		opts.Limit = 10
	}

	var conditions []string
	var args []any

	conditions = append(conditions, "m.reaction_count > 0")

	if opts.Channel != "" {
		conditions = append(conditions, "c.name = ?")
		args = append(args, opts.Channel)
	}
	if opts.Guild != "" {
		conditions = append(conditions, "g.name = ?")
		args = append(args, opts.Guild)
	}

	where := "WHERE " + strings.Join(conditions, " AND ")
	args = append(args, opts.Limit)

	q := fmt.Sprintf(`
		SELECT
			m.id, m.author_name, m.content, m.timestamp,
			m.reaction_count,
			COALESCE(c.name, m.channel_id) AS channel_name,
			COALESCE(g.name, 'DM') AS guild_name
		FROM messages m
		JOIN channels c ON m.channel_id = c.id
		LEFT JOIN guilds g ON c.guild_id = g.id
		%s
		ORDER BY m.reaction_count DESC
		LIMIT ?
	`, where)

	rows, err := db.conn.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("top reacted query: %w", err)
	}
	defer rows.Close()

	var results []TopReactedResult
	for rows.Next() {
		var r TopReactedResult
		var ts string
		if err := rows.Scan(
			&r.MessageID, &r.AuthorName, &r.Content, &ts,
			&r.ReactionCount, &r.ChannelName, &r.GuildName,
		); err != nil {
			return nil, err
		}
		r.Timestamp, _ = time.Parse(time.RFC3339, ts)
		results = append(results, r)
	}
	return results, rows.Err()
}

type SearchOptions struct {
	Channel string
	Guild   string
	Limit   int
}

type SearchResult struct {
	ID          string
	ChannelID   string
	ChannelName string
	GuildName   string
	AuthorName  string
	Content     string
	Timestamp   time.Time
	Rank        float64
}

func (db *DB) SearchMessages(query string, opts SearchOptions) ([]SearchResult, error) {
	if opts.Limit == 0 {
		opts.Limit = 20
	}

	var conditions []string
	var args []interface{}

	args = append(args, query)

	if opts.Channel != "" {
		conditions = append(conditions, "c.name = ?")
		args = append(args, opts.Channel)
	}
	if opts.Guild != "" {
		conditions = append(conditions, "g.name = ?")
		args = append(args, opts.Guild)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "AND " + strings.Join(conditions, " AND ")
	}

	args = append(args, opts.Limit)

	q := fmt.Sprintf(`
		SELECT
			m.id, m.channel_id, c.name AS channel_name,
			COALESCE(g.name, 'DM') AS guild_name,
			m.author_name, m.content, m.timestamp,
			rank
		FROM messages_fts
		JOIN messages m ON messages_fts.rowid = m.rowid
		JOIN channels c ON m.channel_id = c.id
		LEFT JOIN guilds g ON m.guild_id = g.id
		WHERE messages_fts MATCH ?
		%s
		ORDER BY rank
		LIMIT ?
	`, whereClause)

	rows, err := db.conn.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var ts string
		if err := rows.Scan(
			&r.ID, &r.ChannelID, &r.ChannelName, &r.GuildName,
			&r.AuthorName, &r.Content, &ts, &r.Rank,
		); err != nil {
			return nil, err
		}
		r.Timestamp, _ = time.Parse(time.RFC3339, ts)
		results = append(results, r)
	}
	return results, rows.Err()
}
