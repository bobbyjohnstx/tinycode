package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

// bufferedToolCall holds a tool call record waiting to be flushed to the DB.
type bufferedToolCall struct {
	ToolName   string
	ArgsHash   string
	Timestamp  int64
	DurationMs int64
	Success    int
}

// state holds the mutable telemetry plugin state.
type state struct {
	mu               sync.Mutex
	db               *sql.DB
	currentSessionID string
	buffer           []bufferedToolCall
}

// dbPath returns the SQLite database path from env or default.
func dbPath() string {
	if p := os.Getenv("TELEMETRY_DB"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "telemetry.db"
	}
	return filepath.Join(home, ".tinycode", "telemetry.db")
}

// initDB opens the SQLite DB and creates tables if needed.
func initDB(path string) (*sql.DB, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating db directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("setting journal mode: %w", err)
	}

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			started_at INTEGER NOT NULL,
			ended_at INTEGER,
			tool_count INTEGER DEFAULT 0
		)
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("creating sessions table: %w", err)
	}

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS tool_calls (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL,
			tool_name TEXT NOT NULL,
			args_hash TEXT NOT NULL,
			timestamp INTEGER NOT NULL,
			duration_ms INTEGER,
			success INTEGER NOT NULL DEFAULT 1,
			FOREIGN KEY (session_id) REFERENCES sessions(id)
		)
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("creating tool_calls table: %w", err)
	}

	return db, nil
}

// ensureDB lazily initializes the database connection.
func (s *state) ensureDB() (*sql.DB, error) {
	if s.db != nil {
		return s.db, nil
	}
	db, err := initDB(dbPath())
	if err != nil {
		return nil, err
	}
	s.db = db
	return db, nil
}

// hashArgs returns the first 16 hex chars of the SHA-256 hash of the input.
func hashArgs(input string) string {
	h := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", h)[:16]
}

// formatReport generates a telemetry summary report from the database.
func formatReport(db *sql.DB) (string, error) {
	var sessionCount, toolCallCount int

	if err := db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&sessionCount); err != nil {
		return "", fmt.Errorf("counting sessions: %w", err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM tool_calls").Scan(&toolCallCount); err != nil {
		return "", fmt.Errorf("counting tool calls: %w", err)
	}

	if sessionCount == 0 && toolCallCount == 0 {
		return "No telemetry data collected yet.", nil
	}

	avgPerSession := "0"
	if sessionCount > 0 {
		avgPerSession = fmt.Sprintf("%.1f", float64(toolCallCount)/float64(sessionCount))
	}

	lines := []string{
		"--- Telemetry Report ---",
		"",
		fmt.Sprintf("Total sessions: %d", sessionCount),
		fmt.Sprintf("Total tool calls: %d", toolCallCount),
		fmt.Sprintf("Average tool calls per session: %s", avgPerSession),
		"",
		"Top 10 tools by call count:",
	}

	rows, err := db.Query("SELECT tool_name, COUNT(*) as count FROM tool_calls GROUP BY tool_name ORDER BY count DESC LIMIT 10")
	if err != nil {
		return "", fmt.Errorf("querying top tools: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var count int
		if err := rows.Scan(&name, &count); err != nil {
			return "", fmt.Errorf("scanning top tool: %w", err)
		}
		lines = append(lines, fmt.Sprintf("  %s: %d", name, count))
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterating top tools: %w", err)
	}

	lines = append(lines, "", "Last 5 sessions:")
	sessRows, err := db.Query("SELECT id, started_at, ended_at, tool_count FROM sessions ORDER BY started_at DESC LIMIT 5")
	if err != nil {
		return "", fmt.Errorf("querying recent sessions: %w", err)
	}
	defer sessRows.Close()
	for sessRows.Next() {
		var id string
		var startedAt int64
		var endedAt sql.NullInt64
		var toolCount int
		if err := sessRows.Scan(&id, &startedAt, &endedAt, &toolCount); err != nil {
			return "", fmt.Errorf("scanning session: %w", err)
		}
		startStr := time.UnixMilli(startedAt).UTC().Format(time.RFC3339)
		endStr := "active"
		if endedAt.Valid {
			endStr = time.UnixMilli(endedAt.Int64).UTC().Format(time.RFC3339)
		}
		lines = append(lines, fmt.Sprintf("  %s | started: %s | ended: %s | tools: %d", id, startStr, endStr, toolCount))
	}
	if err := sessRows.Err(); err != nil {
		return "", fmt.Errorf("iterating sessions: %w", err)
	}

	return strings.Join(lines, "\n"), nil
}

// queryToolCalls returns a formatted list of tool call records.
func queryToolCalls(db *sql.DB, tool string, days int) (string, error) {
	var cutoff int64
	if days == 0 {
		cutoff = time.Now().UnixMilli() + 1
	} else {
		cutoff = time.Now().Add(-time.Duration(days) * 24 * time.Hour).UnixMilli()
	}

	query := "SELECT tool_name, args_hash, timestamp, duration_ms, success, session_id FROM tool_calls WHERE timestamp >= ?"
	args := []any{cutoff}

	if tool != "" {
		query += " AND tool_name = ?"
		args = append(args, tool)
	}
	query += " ORDER BY timestamp DESC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return "", fmt.Errorf("querying tool calls: %w", err)
	}
	defer rows.Close()

	type record struct {
		ToolName  string
		Timestamp int64
		Success   int
		SessionID string
	}
	var records []record
	for rows.Next() {
		var r record
		var argsHash string
		var durationMs sql.NullInt64
		if err := rows.Scan(&r.ToolName, &argsHash, &r.Timestamp, &durationMs, &r.Success, &r.SessionID); err != nil {
			return "", fmt.Errorf("scanning tool call: %w", err)
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterating tool calls: %w", err)
	}

	if len(records) == 0 {
		msg := "No tool calls found"
		if tool != "" {
			msg += fmt.Sprintf(" for %q", tool)
		}
		msg += fmt.Sprintf(" in the last %d day(s).", days)
		return msg, nil
	}

	lines := []string{
		fmt.Sprintf("Tool calls%s (last %d day(s)): %d result(s)",
			func() string {
				if tool != "" {
					return fmt.Sprintf(" for %q", tool)
				}
				return ""
			}(), days, len(records)),
		"",
	}

	for _, r := range records {
		ts := time.UnixMilli(r.Timestamp).UTC().Format(time.RFC3339)
		status := "ok"
		if r.Success == 0 {
			status = "fail"
		}
		lines = append(lines, fmt.Sprintf("  %s | %s | %s | session: %s", r.ToolName, ts, status, r.SessionID))
	}

	return strings.Join(lines, "\n"), nil
}

// newPlugin creates the telemetry plugin with tools and hooks.
func newPlugin() plugin.Plugin {
	s := &state{}
	return plugin.Plugin{
		ID:    "telemetry",
		Tools: buildTools(s),
		Hooks: buildHooks(s),
	}
}

// queryArgs is the input schema for telemetry_query.
type queryArgs struct {
	Tool string `json:"tool"`
	Days *int   `json:"days"`
}

func buildTools(s *state) []plugin.ToolDef {
	return []plugin.ToolDef{
		{
			Name:        "telemetry_report",
			Description: "Show a telemetry summary: total sessions, total tool calls, top 10 tools by call count, average calls per session, and last 5 sessions.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Execute: func(_ context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				s.mu.Lock()
				defer s.mu.Unlock()
				db, err := s.ensureDB()
				if err != nil {
					return "", fmt.Errorf("database error: %w", err)
				}
				return formatReport(db)
			},
		},
		{
			Name:        "telemetry_query",
			Description: "Query tool call telemetry records, optionally filtered by tool name and recency.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"tool": map[string]any{
						"type":        "string",
						"description": "Filter by tool name",
					},
					"days": map[string]any{
						"type":        "number",
						"description": "Number of days to look back (default 7)",
					},
				},
			},
			Execute: func(_ context.Context, raw json.RawMessage, _ plugin.ToolContext) (string, error) {
				var args queryArgs
				if len(raw) > 0 {
					if err := json.Unmarshal(raw, &args); err != nil {
						return "", fmt.Errorf("invalid arguments: %w", err)
					}
				}
				days := 7
				if args.Days != nil {
					days = *args.Days
				}

				s.mu.Lock()
				defer s.mu.Unlock()
				db, err := s.ensureDB()
				if err != nil {
					return "", fmt.Errorf("database error: %w", err)
				}
				return queryToolCalls(db, args.Tool, days)
			},
		},
	}
}

func buildHooks(s *state) plugin.HookHandlers {
	return plugin.HookHandlers{
		SessionStart: func(_ context.Context, event plugin.SessionStartEvent) error {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.currentSessionID = event.SessionID
			s.buffer = s.buffer[:0]
			db, err := s.ensureDB()
			if err != nil {
				return err
			}
			_, err = db.Exec("INSERT OR IGNORE INTO sessions (id, started_at) VALUES (?, ?)",
				event.SessionID, time.Now().UnixMilli())
			return err
		},
		ToolExecAfter: func(_ context.Context, input plugin.ToolExecAfterInput) (*plugin.ToolExecAfterOutput, error) {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.currentSessionID == "" {
				return nil, nil
			}
			success := 1
			if input.IsError {
				success = 0
			}
			s.buffer = append(s.buffer, bufferedToolCall{
				ToolName:   input.ToolName,
				ArgsHash:   hashArgs(input.Output),
				Timestamp:  time.Now().UnixMilli(),
				DurationMs: 0,
				Success:    success,
			})
			return nil, nil
		},
		SessionEnd: func(_ context.Context, event plugin.SessionEndEvent) error {
			s.mu.Lock()
			defer s.mu.Unlock()
			db, err := s.ensureDB()
			if err != nil {
				return err
			}

			if len(s.buffer) > 0 {
				tx, err := db.Begin()
				if err != nil {
					return fmt.Errorf("beginning transaction: %w", err)
				}
				stmt, err := tx.Prepare("INSERT INTO tool_calls (session_id, tool_name, args_hash, timestamp, duration_ms, success) VALUES (?, ?, ?, ?, ?, ?)")
				if err != nil {
					tx.Rollback()
					return fmt.Errorf("preparing insert: %w", err)
				}
				defer stmt.Close()
				for _, call := range s.buffer {
					if _, err := stmt.Exec(event.SessionID, call.ToolName, call.ArgsHash, call.Timestamp, call.DurationMs, call.Success); err != nil {
						tx.Rollback()
						return fmt.Errorf("inserting tool call: %w", err)
					}
				}
				if err := tx.Commit(); err != nil {
					return fmt.Errorf("committing transaction: %w", err)
				}
			}

			if _, err := db.Exec("UPDATE sessions SET ended_at = ?, tool_count = ? WHERE id = ?",
				time.Now().UnixMilli(), len(s.buffer), event.SessionID); err != nil {
				return fmt.Errorf("updating session: %w", err)
			}

			s.buffer = s.buffer[:0]
			s.currentSessionID = ""
			return nil
		},
		Dispose: func(_ context.Context) error {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.db != nil {
				err := s.db.Close()
				s.db = nil
				s.buffer = nil
				s.currentSessionID = ""
				return err
			}
			return nil
		},
	}
}

func main() {
	plugin.Run(newPlugin())
}
