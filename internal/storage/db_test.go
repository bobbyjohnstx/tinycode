package storage

import (
	"testing"
)

func TestOpenInMemory(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}
	defer db.Close()

	// Verify tables exist
	tables := []string{"project", "session", "message", "part", "todo", "permission", "_migrations"}
	for _, table := range tables {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err != nil {
			t.Errorf("table %s not found: %v", table, err)
		}
	}
}

func TestMigrationsApplied(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM _migrations").Scan(&count); err != nil {
		t.Fatalf("failed to query migrations: %v", err)
	}
	if count == 0 {
		t.Error("expected at least one migration to be recorded")
	}
}

func TestIdempotentMigrations(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Run migrations again — should be a no-op
	if err := db.migrate(); err != nil {
		t.Fatalf("idempotent migration failed: %v", err)
	}
}

func TestPragmas(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	var journalMode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("failed to query journal_mode: %v", err)
	}
	// In-memory databases may report "memory" instead of "wal"
	if journalMode != "wal" && journalMode != "memory" {
		t.Errorf("expected journal_mode wal or memory, got %s", journalMode)
	}

	var foreignKeys int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatalf("failed to query foreign_keys: %v", err)
	}
	if foreignKeys != 1 {
		t.Errorf("expected foreign_keys=1, got %d", foreignKeys)
	}
}

func TestProjectCRUD(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	now := int64(1700000000000)

	// Create
	_, err = db.Exec(
		"INSERT INTO project (id, worktree, name, sandboxes, time_created, time_updated) VALUES (?, ?, ?, ?, ?, ?)",
		"prj_001", "/home/user/project", "test-project", "[]", now, now,
	)
	if err != nil {
		t.Fatalf("insert project failed: %v", err)
	}

	// Read
	var id, worktree, name string
	err = db.QueryRow("SELECT id, worktree, name FROM project WHERE id = ?", "prj_001").Scan(&id, &worktree, &name)
	if err != nil {
		t.Fatalf("select project failed: %v", err)
	}
	if id != "prj_001" || worktree != "/home/user/project" || name != "test-project" {
		t.Errorf("unexpected project values: id=%s worktree=%s name=%s", id, worktree, name)
	}

	// Delete
	_, err = db.Exec("DELETE FROM project WHERE id = ?", "prj_001")
	if err != nil {
		t.Fatalf("delete project failed: %v", err)
	}
}

func TestSessionCRUD(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	now := int64(1700000000000)

	// Create project first (FK constraint)
	_, err = db.Exec(
		"INSERT INTO project (id, worktree, sandboxes, time_created, time_updated) VALUES (?, ?, ?, ?, ?)",
		"prj_001", "/tmp", "[]", now, now,
	)
	if err != nil {
		t.Fatalf("insert project failed: %v", err)
	}

	// Create session
	_, err = db.Exec(
		`INSERT INTO session (id, project_id, slug, directory, title, version, time_created, time_updated)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"ses_001", "prj_001", "test-session", "/tmp", "Test Session", "1", now, now,
	)
	if err != nil {
		t.Fatalf("insert session failed: %v", err)
	}

	// Read
	var title string
	err = db.QueryRow("SELECT title FROM session WHERE id = ?", "ses_001").Scan(&title)
	if err != nil {
		t.Fatalf("select session failed: %v", err)
	}
	if title != "Test Session" {
		t.Errorf("expected 'Test Session', got %s", title)
	}

	// Cascade delete — deleting project should delete session
	_, err = db.Exec("DELETE FROM project WHERE id = ?", "prj_001")
	if err != nil {
		t.Fatalf("delete project failed: %v", err)
	}

	var count int
	db.QueryRow("SELECT COUNT(*) FROM session WHERE id = ?", "ses_001").Scan(&count)
	if count != 0 {
		t.Error("expected session to be cascade-deleted with project")
	}
}

func TestMessagePartCascade(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	now := int64(1700000000000)

	db.Exec("INSERT INTO project (id, worktree, sandboxes, time_created, time_updated) VALUES (?, ?, ?, ?, ?)",
		"prj_001", "/tmp", "[]", now, now)
	db.Exec(`INSERT INTO session (id, project_id, slug, directory, title, version, time_created, time_updated)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"ses_001", "prj_001", "s", "/tmp", "T", "1", now, now)
	db.Exec("INSERT INTO message (id, session_id, time_created, time_updated, data) VALUES (?, ?, ?, ?, ?)",
		"msg_001", "ses_001", now, now, `{"role": "user"}`)
	db.Exec("INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES (?, ?, ?, ?, ?, ?)",
		"prt_001", "msg_001", "ses_001", now, now, `{"type": "text"}`)

	// Delete message — should cascade to parts
	db.Exec("DELETE FROM message WHERE id = ?", "msg_001")

	var count int
	db.QueryRow("SELECT COUNT(*) FROM part WHERE id = ?", "prt_001").Scan(&count)
	if count != 0 {
		t.Error("expected part to be cascade-deleted with message")
	}
}

func TestMigration002Tables(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	tables := []string{
		"session_message", "workspace", "account", "account_state",
		"event_sequence", "event", "data_migration",
	}
	for _, table := range tables {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err != nil {
			t.Errorf("table %s not found after migration 002: %v", table, err)
		}
	}

	// Verify 002 is recorded in _migrations.
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM _migrations WHERE name = ?", "002_missing_tables.sql").Scan(&count)
	if err != nil {
		t.Fatalf("failed to check migration record: %v", err)
	}
	if count != 1 {
		t.Errorf("migration 002_missing_tables.sql not recorded, count = %d", count)
	}
}

func TestDefaultPath(t *testing.T) {
	path := DefaultPath()
	if path == "" {
		t.Error("DefaultPath should return a non-empty path")
	}
}
