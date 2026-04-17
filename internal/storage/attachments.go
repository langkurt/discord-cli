package storage

import (
	"database/sql"
	"fmt"
	"time"
)

// Attachment represents a file attached to a Discord message.
type Attachment struct {
	ID          string
	MessageID   string
	ChannelID   string
	URL         string
	Filename    string
	ContentType string
	Size        int64
	LocalPath   string // empty until downloaded
}

// UpsertAttachment stores attachment metadata (idempotent).
func (db *DB) UpsertAttachment(id, messageID, channelID, url, filename, contentType string, size int64) error {
	_, err := db.conn.Exec(`
		INSERT OR IGNORE INTO attachments (id, message_id, channel_id, url, filename, content_type, size)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, id, messageID, channelID, url, filename, contentType, size)
	if err != nil {
		return fmt.Errorf("upsert attachment %s: %w", id, err)
	}
	return nil
}

// ListPendingAttachments returns attachments not yet downloaded.
// mediaType: "image" (non-gif), "gif", "video", "all"
// channelID / guildID: pass "" to skip filter.
// limit: 0 = unlimited.
// since: zero value = no filter; otherwise only attachments from messages on/after this time.
func (db *DB) ListPendingAttachments(channelID, guildID, mediaType string, limit int, since time.Time) ([]Attachment, error) {
	query := `
		SELECT a.id, a.message_id, a.channel_id, a.url, a.filename,
		       COALESCE(a.content_type,''), a.size
		FROM attachments a
	`
	var args []any
	var where []string

	where = append(where, "a.local_path IS NULL")

	if channelID != "" {
		where = append(where, "a.channel_id = ?")
		args = append(args, channelID)
	}

	needsMessageJoin := guildID != "" || !since.IsZero()
	if needsMessageJoin {
		query += " JOIN messages m ON a.message_id = m.id"
	}

	if guildID != "" {
		where = append(where, "m.guild_id = ?")
		args = append(args, guildID)
	}

	if !since.IsZero() {
		where = append(where, "m.timestamp >= ?")
		args = append(args, since.UTC().Format(time.RFC3339))
	}

	switch mediaType {
	case "gif":
		where = append(where, "a.content_type = 'image/gif'")
	case "image":
		where = append(where, "(a.content_type LIKE 'image/%' AND a.content_type != 'image/gif')")
	case "video":
		where = append(where, "a.content_type LIKE 'video/%'")
	// "all" or "" — no filter
	}

	if len(where) > 0 {
		query += " WHERE " + joinAnd(where)
	}
	query += " ORDER BY a.id"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list pending attachments: %w", err)
	}
	defer rows.Close()

	var result []Attachment
	for rows.Next() {
		var a Attachment
		if err := rows.Scan(&a.ID, &a.MessageID, &a.ChannelID, &a.URL, &a.Filename, &a.ContentType, &a.Size); err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

// MarkDownloaded records the local file path after a successful download.
func (db *DB) MarkDownloaded(id, localPath string) error {
	_, err := db.conn.Exec(`UPDATE attachments SET local_path = ? WHERE id = ?`, localPath, id)
	return err
}

// AttachmentStats returns pending and downloaded counts for a channel (or all if channelID is "").
func (db *DB) AttachmentStats(channelID string) (pending, downloaded int, err error) {
	var row *sql.Row
	if channelID == "" {
		row = db.conn.QueryRow(`
			SELECT
				SUM(CASE WHEN local_path IS NULL THEN 1 ELSE 0 END),
				SUM(CASE WHEN local_path IS NOT NULL THEN 1 ELSE 0 END)
			FROM attachments
		`)
	} else {
		row = db.conn.QueryRow(`
			SELECT
				SUM(CASE WHEN local_path IS NULL THEN 1 ELSE 0 END),
				SUM(CASE WHEN local_path IS NOT NULL THEN 1 ELSE 0 END)
			FROM attachments WHERE channel_id = ?
		`, channelID)
	}
	err = row.Scan(&pending, &downloaded)
	return
}

// joinAnd joins conditions with AND — avoids importing strings in this file.
func joinAnd(parts []string) string {
	out := parts[0]
	for _, p := range parts[1:] {
		out += " AND " + p
	}
	return out
}
