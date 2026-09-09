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

	return openDB(dbPath)
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
