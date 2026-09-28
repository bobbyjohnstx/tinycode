package storage

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

var (
	ErrNotFound = errors.New("not found")
)

type DB struct {
	*sql.DB
	path string
}

func Open(dbPath string) (*DB, error) {
	if dbPath == ":memory:" {
		return openDB(dbPath)
	}

	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating data directory: %w", err)
	}

	if err := archiveLegacyDB(dbPath); err != nil {
		slog.Warn("legacy DB detection failed, continuing", "error", err)
	}

	return openDB(dbPath)
}

// archiveLegacyDB detects a TypeScript tinycode database (uses __drizzle_migrations)
// and renames it so Go can start with a fresh schema. The TS and Go versions share
// the same DB path but use incompatible migration tracking and may have schema
// differences that cause silent query failures.
func archiveLegacyDB(dbPath string) error {
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil
	}

	backupPath := dbPath + ".ts-backup"
	if _, err := os.Stat(backupPath); err == nil {
		slog.Info("legacy DB backup already exists, skipping", "backup", backupPath)
		return nil
	}

	isLegacy, err := isLegacyTSDB(dbPath)
	if err != nil {
		return err
	}
	if !isLegacy {
		return nil
	}

	slog.Warn("detected TypeScript tinycode database, archiving",
		"path", dbPath, "backup", backupPath)

	if err := os.Rename(dbPath, backupPath); err != nil {
		return fmt.Errorf("archiving legacy database: %w", err)
	}

	for _, suffix := range []string{"-wal", "-shm"} {
		src := dbPath + suffix
		if _, err := os.Stat(src); err == nil {
			os.Rename(src, backupPath+suffix)
		}
	}

	slog.Info("legacy database archived", "backup", backupPath)
	return nil
}

// isLegacyTSDB opens the database read-only and checks whether it was created
// by TypeScript tinycode (has __drizzle_migrations table but no _migrations).
// Opening the DB may checkpoint any WAL file, which is acceptable — the data
// is preserved in the main DB file.
func isLegacyTSDB(dbPath string) (bool, error) {
	probe, err := sql.Open("sqlite", dbPath+"?mode=ro")
	if err != nil {
		return false, fmt.Errorf("probing database: %w", err)
	}
	defer probe.Close()
	probe.SetMaxOpenConns(1)

	var drizzleCount, goMigCount int
	if err := probe.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='__drizzle_migrations'",
	).Scan(&drizzleCount); err != nil {
		return false, fmt.Errorf("checking for drizzle table: %w", err)
	}
	if err := probe.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='_migrations'",
	).Scan(&goMigCount); err != nil {
		return false, fmt.Errorf("checking for go migrations table: %w", err)
	}

	return drizzleCount > 0 && goMigCount == 0, nil
}

func openDB(dsn string) (*DB, error) {
	if dsn != ":memory:" {
		dsn = dsn + "?_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=5000&_cache_size=-64000&_foreign_keys=ON"
	}

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	sqlDB.SetMaxOpenConns(1)

	if dsn == ":memory:" {
		for _, pragma := range []string{
			"PRAGMA journal_mode = WAL",
			"PRAGMA synchronous = NORMAL",
			"PRAGMA busy_timeout = 5000",
			"PRAGMA cache_size = -64000",
			"PRAGMA foreign_keys = ON",
		} {
			if _, err := sqlDB.Exec(pragma); err != nil {
				sqlDB.Close()
				return nil, fmt.Errorf("setting pragma %q: %w", pragma, err)
			}
		}
	}

	db := &DB{
		DB:   sqlDB,
		path: dsn,
	}

	if err := db.migrate(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	slog.Info("database opened", "path", dsn)
	return db, nil
}

func (db *DB) migrate() error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS _migrations (
		name TEXT PRIMARY KEY,
		applied_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
	)`); err != nil {
		return fmt.Errorf("creating migrations table: %w", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("reading migrations directory: %w", err)
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM _migrations WHERE name = ?", name).Scan(&count); err != nil {
			return fmt.Errorf("checking migration %s: %w", name, err)
		}
		if count > 0 {
			continue
		}

		content, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("reading migration %s: %w", name, err)
		}

		slog.Info("applying migration", "name", name)
		if _, err := db.Exec(string(content)); err != nil {
			return fmt.Errorf("applying migration %s: %w", name, err)
		}

		if _, err := db.Exec("INSERT INTO _migrations (name) VALUES (?)", name); err != nil {
			return fmt.Errorf("recording migration %s: %w", name, err)
		}
	}

	return nil
}

func (db *DB) Close() error {
	slog.Info("closing database", "path", db.path)
	return db.DB.Close()
}

func DefaultPath() string {
	if v := os.Getenv("TINYCODE_DB"); v != "" {
		if v == ":memory:" || filepath.IsAbs(v) {
			return v
		}
		return filepath.Join(dataDir(), v)
	}
	return filepath.Join(dataDir(), "tinycode.db")
}

func dataDir() string {
	if v := os.Getenv("TINYCODE_DATA_DIR"); v != "" {
		return v
	}
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return filepath.Join(v, "tinycode")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "tinycode")
}
