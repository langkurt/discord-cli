package storage

import "fmt"

// Link represents an external URL extracted from a message.
// ProxyURL, when non-empty, is Discord's own CDN proxy for the image
// (media.discordapp.net) — captured from message embeds during sync.
// Downloading from ProxyURL requires no scraping.
type Link struct {
	ID         string
	MessageID  string
	ChannelID  string
	URL        string // original link (text URL or embed source URL)
	ProxyURL   string // Discord CDN proxy URL; empty for text-only URLs
	LocalPath  string
	Failed     bool
	FailReason string
}

// UpsertLink stores a link (idempotent — keyed on id = hash of url+message).
// proxyURL may be empty for links extracted from message text (no embed).
// When a proxy_url is provided on an existing row it will be updated.
func (db *DB) UpsertLink(id, messageID, channelID, url, proxyURL string) error {
	_, err := db.conn.Exec(`
		INSERT INTO links (id, message_id, channel_id, url, proxy_url)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			proxy_url = CASE WHEN excluded.proxy_url != '' THEN excluded.proxy_url ELSE proxy_url END
	`, id, messageID, channelID, url, proxyURL)
	if err != nil {
		return fmt.Errorf("upsert link %s: %w", id, err)
	}
	return nil
}

// MarkLinkDownloaded records the local path after a successful fetch.
func (db *DB) MarkLinkDownloaded(id, localPath string) error {
	_, err := db.conn.Exec(`
		UPDATE links SET local_path = ?, failed = 0, fail_reason = NULL WHERE id = ?
	`, localPath, id)
	return err
}

// MarkLinkFailed records a failure reason so we don't retry endlessly.
func (db *DB) MarkLinkFailed(id, reason string) error {
	_, err := db.conn.Exec(`
		UPDATE links SET failed = 1, fail_reason = ? WHERE id = ?
	`, reason, id)
	return err
}

// ListPendingLinks returns links not yet downloaded or permanently failed.
// channelID: "" = all channels. guildID: "" = no filter. limit: 0 = unlimited.
func (db *DB) ListPendingLinks(channelID, guildID string, limit int) ([]Link, error) {
	query := `
		SELECT l.id, l.message_id, l.channel_id, l.url, COALESCE(l.proxy_url, '')
		FROM links l
	`
	var args []any
	var where []string

	where = append(where, "l.local_path IS NULL")
	where = append(where, "l.failed = 0")

	if channelID != "" {
		where = append(where, "l.channel_id = ?")
		args = append(args, channelID)
	}

	if guildID != "" {
		query += " JOIN messages m ON l.message_id = m.id"
		where = append(where, "m.guild_id = ?")
		args = append(args, guildID)
	}

	query += " WHERE " + joinAnd(where)
	query += " ORDER BY l.id"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list pending links: %w", err)
	}
	defer rows.Close()

	var result []Link
	for rows.Next() {
		var l Link
		if err := rows.Scan(&l.ID, &l.MessageID, &l.ChannelID, &l.URL, &l.ProxyURL); err != nil {
			return nil, err
		}
		result = append(result, l)
	}
	return result, rows.Err()
}

// LinkStats returns pending/downloaded/failed counts for a channel (or all if "").
func (db *DB) LinkStats(channelID string) (pending, downloaded, failed int, err error) {
	var q string
	var args []any
	if channelID == "" {
		q = `SELECT
			COALESCE(SUM(CASE WHEN local_path IS NULL AND failed = 0 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN local_path IS NOT NULL THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN failed = 1 THEN 1 ELSE 0 END), 0)
		FROM links`
	} else {
		q = `SELECT
			COALESCE(SUM(CASE WHEN local_path IS NULL AND failed = 0 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN local_path IS NOT NULL THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN failed = 1 THEN 1 ELSE 0 END), 0)
		FROM links WHERE channel_id = ?`
		args = append(args, channelID)
	}
	err = db.conn.QueryRow(q, args...).Scan(&pending, &downloaded, &failed)
	return
}
