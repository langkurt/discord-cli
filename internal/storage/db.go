package storage

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
}

func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("migration failed: %w", err)
	}
	return db, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) migrate() error {
	_, err := db.conn.Exec(`
		CREATE TABLE IF NOT EXISTS guilds (
			id   TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			icon TEXT
		);

		CREATE TABLE IF NOT EXISTS channels (
			id       TEXT PRIMARY KEY,
			guild_id TEXT REFERENCES guilds(id),
			name     TEXT NOT NULL,
			type     INTEGER NOT NULL DEFAULT 0,
			topic    TEXT
		);

		CREATE TABLE IF NOT EXISTS messages (
			id           TEXT PRIMARY KEY,
			channel_id   TEXT NOT NULL REFERENCES channels(id),
			guild_id     TEXT,
			author_id    TEXT NOT NULL,
			author_name  TEXT NOT NULL,
			content      TEXT NOT NULL,
			timestamp    DATETIME NOT NULL,
			edited       INTEGER NOT NULL DEFAULT 0
		);

		CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
			content,
			author_name,
			content=messages,
			content_rowid=rowid,
			tokenize="unicode61 remove_diacritics 1"
		);

		CREATE TRIGGER IF NOT EXISTS messages_ai AFTER INSERT ON messages BEGIN
			INSERT INTO messages_fts(rowid, content, author_name)
			VALUES (new.rowid, new.content, new.author_name);
		END;

		CREATE TRIGGER IF NOT EXISTS messages_ad AFTER DELETE ON messages BEGIN
			INSERT INTO messages_fts(messages_fts, rowid, content, author_name)
			VALUES ('delete', old.rowid, old.content, old.author_name);
		END;

		CREATE TRIGGER IF NOT EXISTS messages_au AFTER UPDATE ON messages BEGIN
			INSERT INTO messages_fts(messages_fts, rowid, content, author_name)
			VALUES ('delete', old.rowid, old.content, old.author_name);
			INSERT INTO messages_fts(rowid, content, author_name)
			VALUES (new.rowid, new.content, new.author_name);
		END;

		CREATE TABLE IF NOT EXISTS sync_state (
			channel_id      TEXT PRIMARY KEY,
			last_message_id TEXT,
			synced_at       DATETIME
		);
	`)
	return err
}
