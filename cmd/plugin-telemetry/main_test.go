package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

func TestPluginID(t *testing.T) {
	p := newPlugin()
	if p.ID != "telemetry" {
		t.Errorf("expected plugin ID 'telemetry', got %q", p.ID)
	}
}

func TestPluginHasTwoTools(t *testing.T) {
	p := newPlugin()
	if len(p.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(p.Tools))
	}
	expected := []string{"telemetry_report", "telemetry_query"}
	for i, name := range expected {
		if p.Tools[i].Name != name {
			t.Errorf("tool[%d]: expected %q, got %q", i, name, p.Tools[i].Name)
		}
	}
}

func TestPluginHasHooks(t *testing.T) {
	p := newPlugin()
	if p.Hooks.SessionStart == nil {
		t.Error("expected SessionStart hook to be set")
	}
	if p.Hooks.ToolExecAfter == nil {
		t.Error("expected ToolExecAfter hook to be set")
	}
	if p.Hooks.SessionEnd == nil {
		t.Error("expected SessionEnd hook to be set")
	}
	if p.Hooks.Dispose == nil {
		t.Error("expected Dispose hook to be set")
	}
}

func TestHashArgs(t *testing.T) {
	h := hashArgs("test input")
	if len(h) != 16 {
		t.Errorf("expected 16-char hash, got %d chars: %q", len(h), h)
	}
	// Deterministic.
	h2 := hashArgs("test input")
	if h != h2 {
		t.Errorf("expected deterministic hash, got %q and %q", h, h2)
	}
	// Different inputs produce different hashes.
	h3 := hashArgs("different input")
	if h == h3 {
		t.Errorf("expected different hashes for different inputs")
	}
}

func TestInitDB(t *testing.T) {
	dbFile := filepath.Join(t.TempDir(), "test.db")
	db, err := initDB(dbFile)
	if err != nil {
		t.Fatalf("initDB failed: %v", err)
	}
	defer db.Close()

	// Verify tables exist.
	var name string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='sessions'").Scan(&name)
	if err != nil {
		t.Errorf("sessions table not found: %v", err)
	}
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='tool_calls'").Scan(&name)
	if err != nil {
		t.Errorf("tool_calls table not found: %v", err)
	}
}

func TestInitDBCreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")
	dbFile := filepath.Join(dir, "test.db")
	db, err := initDB(dbFile)
	if err != nil {
		t.Fatalf("initDB failed: %v", err)
	}
	db.Close()

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("expected directory to be created")
	}
}

func TestFormatReportEmpty(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	result, err := formatReport(db)
	if err != nil {
		t.Fatalf("formatReport failed: %v", err)
	}
	if result != "No telemetry data collected yet." {
		t.Errorf("expected empty report, got: %q", result)
	}
}

func TestFormatReportWithData(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	now := time.Now().UnixMilli()
	db.Exec("INSERT INTO sessions (id, started_at, ended_at, tool_count) VALUES (?, ?, ?, ?)",
		"sess-1", now-60000, now, 3)
	db.Exec("INSERT INTO tool_calls (session_id, tool_name, args_hash, timestamp, success) VALUES (?, ?, ?, ?, ?)",
		"sess-1", "shell", "abc123", now-30000, 1)
	db.Exec("INSERT INTO tool_calls (session_id, tool_name, args_hash, timestamp, success) VALUES (?, ?, ?, ?, ?)",
		"sess-1", "shell", "def456", now-20000, 1)
	db.Exec("INSERT INTO tool_calls (session_id, tool_name, args_hash, timestamp, success) VALUES (?, ?, ?, ?, ?)",
		"sess-1", "read", "ghi789", now-10000, 1)

	result, err := formatReport(db)
	if err != nil {
		t.Fatalf("formatReport failed: %v", err)
	}
	if !strings.Contains(result, "Total sessions: 1") {
		t.Errorf("expected total sessions 1 in report: %s", result)
	}
	if !strings.Contains(result, "Total tool calls: 3") {
		t.Errorf("expected total tool calls 3 in report: %s", result)
	}
	if !strings.Contains(result, "Average tool calls per session: 3.0") {
		t.Errorf("expected average 3.0 in report: %s", result)
	}
	if !strings.Contains(result, "shell: 2") {
		t.Errorf("expected shell: 2 in top tools: %s", result)
	}
	if !strings.Contains(result, "sess-1") {
		t.Errorf("expected sess-1 in recent sessions: %s", result)
	}
}

func TestFormatReportActiveSession(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	now := time.Now().UnixMilli()
	db.Exec("INSERT INTO sessions (id, started_at, tool_count) VALUES (?, ?, ?)",
		"sess-active", now, 0)

	result, err := formatReport(db)
	if err != nil {
		t.Fatalf("formatReport failed: %v", err)
	}
	if !strings.Contains(result, "active") {
		t.Errorf("expected 'active' for session with no ended_at: %s", result)
	}
}

func TestQueryToolCallsNoResults(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	result, err := queryToolCalls(db, "", 7)
	if err != nil {
		t.Fatalf("queryToolCalls failed: %v", err)
	}
	if !strings.Contains(result, "No tool calls found") {
		t.Errorf("expected no results message, got: %q", result)
	}
}

func TestQueryToolCallsWithFilter(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	now := time.Now().UnixMilli()
	db.Exec("INSERT INTO sessions (id, started_at) VALUES (?, ?)", "sess-1", now)
	db.Exec("INSERT INTO tool_calls (session_id, tool_name, args_hash, timestamp, success) VALUES (?, ?, ?, ?, ?)",
		"sess-1", "shell", "h1", now, 1)
	db.Exec("INSERT INTO tool_calls (session_id, tool_name, args_hash, timestamp, success) VALUES (?, ?, ?, ?, ?)",
		"sess-1", "read", "h2", now, 1)

	result, err := queryToolCalls(db, "shell", 7)
	if err != nil {
		t.Fatalf("queryToolCalls failed: %v", err)
	}
	if !strings.Contains(result, "1 result(s)") {
		t.Errorf("expected 1 result for shell filter, got: %s", result)
	}
	if !strings.Contains(result, "shell") {
		t.Errorf("expected shell in output: %s", result)
	}
}

func TestQueryToolCallsWithDaysFilter(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	now := time.Now().UnixMilli()
	oldTS := now - 10*24*60*60*1000 // 10 days ago
	db.Exec("INSERT INTO sessions (id, started_at) VALUES (?, ?)", "sess-1", oldTS)
	db.Exec("INSERT INTO tool_calls (session_id, tool_name, args_hash, timestamp, success) VALUES (?, ?, ?, ?, ?)",
		"sess-1", "shell", "h1", oldTS, 1)
	db.Exec("INSERT INTO tool_calls (session_id, tool_name, args_hash, timestamp, success) VALUES (?, ?, ?, ?, ?)",
		"sess-1", "shell", "h2", now, 1)

	result, err := queryToolCalls(db, "", 3)
	if err != nil {
		t.Fatalf("queryToolCalls failed: %v", err)
	}
	if !strings.Contains(result, "1 result(s)") {
		t.Errorf("expected 1 result within 3 days, got: %s", result)
	}
}

func TestQueryToolCallsFailedStatus(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	now := time.Now().UnixMilli()
	db.Exec("INSERT INTO sessions (id, started_at) VALUES (?, ?)", "sess-1", now)
	db.Exec("INSERT INTO tool_calls (session_id, tool_name, args_hash, timestamp, success) VALUES (?, ?, ?, ?, ?)",
		"sess-1", "shell", "h1", now, 0)

	result, err := queryToolCalls(db, "", 7)
	if err != nil {
		t.Fatalf("queryToolCalls failed: %v", err)
	}
	if !strings.Contains(result, "fail") {
		t.Errorf("expected 'fail' status in output: %s", result)
	}
}

func TestSessionLifecycle(t *testing.T) {
	dbFile := filepath.Join(t.TempDir(), "lifecycle.db")
	os.Setenv("TELEMETRY_DB", dbFile)
	defer os.Unsetenv("TELEMETRY_DB")

	p := newPlugin()
	ctx := context.Background()

	// Start session.
	err := p.Hooks.SessionStart(ctx, plugin.SessionStartEvent{SessionID: "sess-lifecycle"})
	if err != nil {
		t.Fatalf("SessionStart failed: %v", err)
	}

	// Record some tool calls.
	_, err = p.Hooks.ToolExecAfter(ctx, plugin.ToolExecAfterInput{
		SessionID: "sess-lifecycle",
		ToolName:  "shell",
		Output:    "hello world",
	})
	if err != nil {
		t.Fatalf("ToolExecAfter failed: %v", err)
	}
	_, err = p.Hooks.ToolExecAfter(ctx, plugin.ToolExecAfterInput{
		SessionID: "sess-lifecycle",
		ToolName:  "read",
		Output:    "file contents",
		IsError:   true,
	})
	if err != nil {
		t.Fatalf("ToolExecAfter failed: %v", err)
	}

	// End session.
	err = p.Hooks.SessionEnd(ctx, plugin.SessionEndEvent{SessionID: "sess-lifecycle"})
	if err != nil {
		t.Fatalf("SessionEnd failed: %v", err)
	}

	// Verify data via report tool.
	tc := plugin.ToolContext{}
	result, err := p.Tools[0].Execute(ctx, nil, tc) // telemetry_report
	if err != nil {
		t.Fatalf("telemetry_report failed: %v", err)
	}
	if !strings.Contains(result, "Total sessions: 1") {
		t.Errorf("expected 1 session in report: %s", result)
	}
	if !strings.Contains(result, "Total tool calls: 2") {
		t.Errorf("expected 2 tool calls in report: %s", result)
	}

	// Query tool should show data.
	queryJSON := json.RawMessage(`{"tool":"shell","days":1}`)
	qResult, err := p.Tools[1].Execute(ctx, queryJSON, tc) // telemetry_query
	if err != nil {
		t.Fatalf("telemetry_query failed: %v", err)
	}
	if !strings.Contains(qResult, "1 result(s)") {
		t.Errorf("expected 1 result for shell query: %s", qResult)
	}

	// Dispose.
	err = p.Hooks.Dispose(ctx)
	if err != nil {
		t.Fatalf("Dispose failed: %v", err)
	}
}

func TestToolExecAfterNoSession(t *testing.T) {
	p := newPlugin()
	ctx := context.Background()

	// ToolExecAfter without SessionStart should be a no-op.
	out, err := p.Hooks.ToolExecAfter(ctx, plugin.ToolExecAfterInput{
		ToolName: "shell",
		Output:   "output",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != nil {
		t.Errorf("expected nil output, got: %v", out)
	}
}

func TestToolExecAfterTracksErrors(t *testing.T) {
	dbFile := filepath.Join(t.TempDir(), "errors.db")
	os.Setenv("TELEMETRY_DB", dbFile)
	defer os.Unsetenv("TELEMETRY_DB")

	p := newPlugin()
	ctx := context.Background()

	p.Hooks.SessionStart(ctx, plugin.SessionStartEvent{SessionID: "err-sess"})
	p.Hooks.ToolExecAfter(ctx, plugin.ToolExecAfterInput{
		ToolName: "shell",
		Output:   "error output",
		IsError:  true,
	})
	p.Hooks.SessionEnd(ctx, plugin.SessionEndEvent{SessionID: "err-sess"})

	// Verify the failed call is recorded.
	queryJSON := json.RawMessage(`{"days":1}`)
	result, err := p.Tools[1].Execute(ctx, queryJSON, plugin.ToolContext{})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if !strings.Contains(result, "fail") {
		t.Errorf("expected 'fail' status in query result: %s", result)
	}

	p.Hooks.Dispose(ctx)
}

func TestDisposeIdempotent(t *testing.T) {
	p := newPlugin()
	ctx := context.Background()

	// Dispose without any DB initialized should not error.
	if err := p.Hooks.Dispose(ctx); err != nil {
		t.Errorf("first Dispose failed: %v", err)
	}
	// Double dispose should be safe.
	if err := p.Hooks.Dispose(ctx); err != nil {
		t.Errorf("second Dispose failed: %v", err)
	}
}

func TestQueryToolCallsDefaultDays(t *testing.T) {
	dbFile := filepath.Join(t.TempDir(), "defaults.db")
	os.Setenv("TELEMETRY_DB", dbFile)
	defer os.Unsetenv("TELEMETRY_DB")

	p := newPlugin()
	ctx := context.Background()

	p.Hooks.SessionStart(ctx, plugin.SessionStartEvent{SessionID: "s1"})
	p.Hooks.ToolExecAfter(ctx, plugin.ToolExecAfterInput{ToolName: "shell", Output: "ok"})
	p.Hooks.SessionEnd(ctx, plugin.SessionEndEvent{SessionID: "s1"})

	// Query with no args should default to 7 days.
	result, err := p.Tools[1].Execute(ctx, json.RawMessage(`{}`), plugin.ToolContext{})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if !strings.Contains(result, "last 7 day(s)") {
		t.Errorf("expected default 7 days in output: %s", result)
	}

	p.Hooks.Dispose(ctx)
}

func TestToolSchema(t *testing.T) {
	p := newPlugin()

	// telemetry_report has no parameters.
	reportParams := p.Tools[0].Parameters
	if reportParams["type"] != "object" {
		t.Errorf("expected type 'object', got %v", reportParams["type"])
	}

	// telemetry_query has tool and days params.
	queryParams := p.Tools[1].Parameters
	props, ok := queryParams["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties to be map[string]any")
	}
	if _, ok := props["tool"]; !ok {
		t.Error("missing 'tool' property")
	}
	if _, ok := props["days"]; !ok {
		t.Error("missing 'days' property")
	}
}

// setupTestDB creates an in-memory SQLite DB with the telemetry schema.
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbFile := filepath.Join(t.TempDir(), "test.db")
	db, err := initDB(dbFile)
	if err != nil {
		t.Fatalf("setupTestDB failed: %v", err)
	}
	return db
}
