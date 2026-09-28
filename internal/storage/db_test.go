package storage

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
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

func TestArchiveLegacyDB_NoFile(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tinycode.db")
	err := archiveLegacyDB(dbPath)
	if err != nil {
		t.Fatalf("expected no error for nonexistent file, got %v", err)
	}
}

func TestArchiveLegacyDB_FreshGoDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tinycode.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to create Go DB: %v", err)
	}
	db.Close()

	// archiveLegacyDB should leave a Go DB alone (has _migrations, no __drizzle_migrations).
	err = archiveLegacyDB(dbPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("Go DB should not have been archived")
	}
}

func TestArchiveLegacyDB_DetectsTSDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tinycode.db")

	// Create a fake TS database with __drizzle_migrations but no _migrations.
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	_, err = sqlDB.Exec(`
		CREATE TABLE __drizzle_migrations (id INTEGER PRIMARY KEY, hash TEXT, created_at INTEGER);
		CREATE TABLE session (id TEXT PRIMARY KEY, title TEXT);
	`)
	if err != nil {
		t.Fatalf("failed to set up fake TS schema: %v", err)
	}
	sqlDB.Close()

	err = archiveLegacyDB(dbPath)
	if err != nil {
		t.Fatalf("archiveLegacyDB failed: %v", err)
	}

	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Error("original DB file should have been renamed")
	}
	backupPath := dbPath + ".ts-backup"
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("backup file should exist")
	}
}

func TestArchiveLegacyDB_SkipsIfBackupExists(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tinycode.db")
	backupPath := dbPath + ".ts-backup"

	// Create fake TS database.
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	sqlDB.Exec("CREATE TABLE __drizzle_migrations (id INTEGER PRIMARY KEY)")
	sqlDB.Close()

	// Create pre-existing backup.
	os.WriteFile(backupPath, []byte("previous backup"), 0o600)

	err = archiveLegacyDB(dbPath)
	if err != nil {
		t.Fatalf("archiveLegacyDB failed: %v", err)
	}

	// Original should still be there because backup already existed.
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("original DB should not have been moved when backup already exists")
	}
}

func TestArchiveLegacyDB_MainFileArchived(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tinycode.db")

	// Create fake TS database with WAL mode active.
	sqlDB, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL")
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	sqlDB.Exec("CREATE TABLE __drizzle_migrations (id INTEGER PRIMARY KEY)")
	sqlDB.Exec("INSERT INTO __drizzle_migrations VALUES (1, 'abc', 12345)")
	sqlDB.Close()

	// The main DB file should exist and contain the data (WAL checkpoint on close).
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("DB file should exist")
	}

	err = archiveLegacyDB(dbPath)
	if err != nil {
		t.Fatalf("archiveLegacyDB failed: %v", err)
	}

	backupPath := dbPath + ".ts-backup"
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("backup file should exist")
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Error("original DB should have been renamed")
	}
}

func TestOpen_WithLegacyDB_CreatesFresh(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tinycode.db")

	// Create a fake TS database.
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	sqlDB.Exec("CREATE TABLE __drizzle_migrations (id INTEGER PRIMARY KEY)")
	sqlDB.Exec("CREATE TABLE session (id TEXT PRIMARY KEY, title TEXT)")
	sqlDB.Close()

	// Open via the normal path — should archive and create fresh.
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	// Verify it's a fresh Go database with the full schema.
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM _migrations").Scan(&count)
	if err != nil {
		t.Fatalf("_migrations table missing: %v", err)
	}
	if count == 0 {
		t.Error("expected migrations to be applied in fresh DB")
	}

	// Verify the session table has Go-specific columns.
	var colCount int
	err = db.QueryRow(`
		SELECT COUNT(*) FROM pragma_table_info('session') WHERE name IN ('agent', 'model', 'cost', 'tokens_input')
	`).Scan(&colCount)
	if err != nil {
		t.Fatalf("failed to check session columns: %v", err)
	}
	if colCount != 4 {
		t.Errorf("expected 4 Go-specific session columns, got %d", colCount)
	}
}
