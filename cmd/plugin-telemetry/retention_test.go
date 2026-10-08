package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestInitDBMode(t *testing.T) {
	dbFile := t.TempDir() + "/telemetry.db"
	db, err := initDB(dbFile)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	info, err := os.Stat(dbFile)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("db mode = %o", info.Mode().Perm())
	}
}

func TestQueryToolCalls_DaysAndLimit(t *testing.T) {
	db, err := initDB(t.TempDir() + "/telemetry.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now().UnixMilli()
	old := time.Now().Add(-30 * 24 * time.Hour).UnixMilli()
	if _, err := db.Exec(`INSERT INTO sessions (id, started_at) VALUES ('s', ?)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO tool_calls (session_id, tool_name, args_hash, timestamp, success) VALUES ('s', 'bash', 'h', ?, 1)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO tool_calls (session_id, tool_name, args_hash, timestamp, success) VALUES ('s', 'old', 'h', ?, 1)`, old); err != nil {
		t.Fatal(err)
	}

	got, err := queryToolCalls(db, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "last 7 day(s)") || strings.Contains(got, "old") {
		t.Fatalf("days<=0 should mean 7 days, got:\n%s", got)
	}

	for i := 0; i < telemetryQueryLimit+1; i++ {
		if _, err := db.Exec(`INSERT INTO tool_calls (session_id, tool_name, args_hash, timestamp, success) VALUES ('s', 'bash', 'h', ?, 1)`, now); err != nil {
			t.Fatal(err)
		}
	}
	got, err = queryToolCalls(db, "bash", 7)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Results capped at 500.") {
		t.Fatalf("expected cap notice, got:\n%s", got)
	}
}

func TestPruneTelemetry(t *testing.T) {
	db, err := initDB(t.TempDir() + "/telemetry.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	old := time.Now().Add(-100 * 24 * time.Hour).UnixMilli()
	if _, err := db.Exec(`INSERT INTO sessions (id, started_at) VALUES ('old', ?)`, old); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO tool_calls (session_id, tool_name, args_hash, timestamp, success) VALUES ('old', 'bash', 'h', ?, 1)`, old); err != nil {
		t.Fatal(err)
	}
	if err := pruneTelemetry(db); err != nil {
		t.Fatal(err)
	}
	var calls int
	if err := db.QueryRow(`SELECT COUNT(*) FROM tool_calls`).Scan(&calls); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("old calls remain: %d", calls)
	}
}
