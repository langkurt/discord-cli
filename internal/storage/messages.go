package storage

import (
	"fmt"
	"time"
)

// UpsertMessage inserts or replaces a message (idempotent for sync).
func (db *DB) UpsertMessage(id, channelID, guildID, authorID, authorName, content string, timestamp time.Time, edited bool) error {
	editedInt := 0
	if edited {
		editedInt = 1
	}
	_, err := db.conn.Exec(`
		INSERT OR REPLACE INTO messages (id, channel_id, guild_id, author_id, author_name, content, timestamp, edited)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, id, channelID, guildID, authorID, authorName, content, timestamp.Format(time.RFC3339), editedInt)
	if err != nil {
		return fmt.Errorf("upsert message %s: %w", id, err)
	}
	return nil
}

// GetLastMessageID returns the last synced message ID for a channel.
func (db *DB) GetLastMessageID(channelID string) (string, error) {
	var lastID string
	err := db.conn.QueryRow(`SELECT last_message_id FROM sync_state WHERE channel_id = ?`, channelID).Scan(&lastID)
	if err != nil {
		return "", nil // no sync state yet
	}
	return lastID, nil
}

// UpdateSyncState records the latest synced message for a channel.
func (db *DB) UpdateSyncState(channelID, lastMessageID string) error {
	_, err := db.conn.Exec(`
		INSERT OR REPLACE INTO sync_state (channel_id, last_message_id, synced_at)
		VALUES (?, ?, ?)
	`, channelID, lastMessageID, time.Now().Format(time.RFC3339))
	return err
}
