package storage

import "fmt"

// UpsertGuild inserts or replaces a guild record.
func (db *DB) UpsertGuild(id, name, icon string) error {
	_, err := db.conn.Exec(`
		INSERT OR REPLACE INTO guilds (id, name, icon) VALUES (?, ?, ?)
	`, id, name, icon)
	if err != nil {
		return fmt.Errorf("upsert guild %s: %w", id, err)
	}
	return nil
}

// UpsertChannel inserts or replaces a channel record.
func (db *DB) UpsertChannel(id, guildID, name string, chType int, topic, parentID string) error {
	_, err := db.conn.Exec(`
		INSERT OR REPLACE INTO channels (id, guild_id, name, type, topic, parent_id) VALUES (?, ?, ?, ?, ?, ?)
	`, id, guildID, name, chType, topic, nullableStr(parentID))
	if err != nil {
		return fmt.Errorf("upsert channel %s: %w", id, err)
	}
	return nil
}

func nullableStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// GetChannelName returns the name of a channel by ID.
func (db *DB) GetChannelName(channelID string) (string, error) {
	var name string
	err := db.conn.QueryRow(`SELECT name FROM channels WHERE id = ?`, channelID).Scan(&name)
	if err != nil {
		return "", fmt.Errorf("channel %s not found: %w", channelID, err)
	}
	return name, nil
}
