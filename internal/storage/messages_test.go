package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.UpsertGuild("g1", "Guild", ""); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertChannel("c1", "g1", "general", 0, "", ""); err != nil {
		t.Fatal(err)
	}
	return db
}

func upsert(t *testing.T, db *DB, id, content string, reactions int) {
	t.Helper()
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := db.UpsertMessage(id, "c1", "g1", "a1", "alice", content, ts, false, reactions); err != nil {
		t.Fatal(err)
	}
}

func revisions(t *testing.T, db *DB, id string) []string {
	t.Helper()
	rows, err := db.conn.Query(`SELECT content FROM message_revisions WHERE message_id = ? ORDER BY id`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			t.Fatal(err)
		}
		out = append(out, c)
	}
	return out
}

func TestUpsertKeepsEditHistory(t *testing.T) {
	db := openTestDB(t)
	upsert(t, db, "m1", "first draft", 0)
	upsert(t, db, "m1", "first draft", 3) // re-sync, reactions changed: not an edit
	upsert(t, db, "m1", "second draft", 3)
	upsert(t, db, "m1", "final", 3)

	got := revisions(t, db, "m1")
	if len(got) != 2 || got[0] != "first draft" || got[1] != "second draft" {
		t.Fatalf("revisions = %q, want [first draft, second draft]", got)
	}
}

func TestUpsertFillingEmptyContentIsNotAnEdit(t *testing.T) {
	db := openTestDB(t)
	upsert(t, db, "m1", "", 0)
	upsert(t, db, "m1", "hello", 0)
	if got := revisions(t, db, "m1"); len(got) != 0 {
		t.Fatalf("revisions = %q, want none", got)
	}
}

func TestUpsertKeepsSearchIndexCurrent(t *testing.T) {
	db := openTestDB(t)
	upsert(t, db, "m1", "alpha", 0)
	upsert(t, db, "m1", "bravo", 0)

	if res, err := db.SearchMessages("alpha", SearchOptions{}); err != nil || len(res) != 0 {
		t.Fatalf("search old text: %d results, err %v; want 0", len(res), err)
	}
	if res, err := db.SearchMessages("bravo", SearchOptions{}); err != nil || len(res) != 1 {
		t.Fatalf("search new text: %d results, err %v; want 1", len(res), err)
	}
	var indexed, stored int
	_ = db.conn.QueryRow(`SELECT (SELECT COUNT(*) FROM messages_fts_docsize), (SELECT COUNT(*) FROM messages)`).Scan(&indexed, &stored)
	if indexed != stored {
		t.Fatalf("search index has %d rows for %d messages", indexed, stored)
	}
}

func TestMarkMessageDeletedKeepsMessage(t *testing.T) {
	db := openTestDB(t)
	upsert(t, db, "m1", "oops", 0)

	ch, author, content, err := db.MarkMessageDeleted("m1")
	if err != nil {
		t.Fatal(err)
	}
	if ch != "c1" || author != "alice" || content != "oops" {
		t.Fatalf("got (%q, %q, %q)", ch, author, content)
	}
	// A later re-sync must not clear the deletion.
	upsert(t, db, "m1", "oops", 0)
	var deletedAt string
	if err := db.conn.QueryRow(`SELECT COALESCE(deleted_at, '') FROM messages WHERE id = 'm1'`).Scan(&deletedAt); err != nil {
		t.Fatal(err)
	}
	if deletedAt == "" {
		t.Fatal("deleted_at cleared or never set")
	}
	if res, _ := db.SearchMessages("oops", SearchOptions{}); len(res) != 1 {
		t.Fatalf("deleted message should stay searchable, got %d results", len(res))
	}
}
