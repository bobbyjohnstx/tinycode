package session

import (
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestAppendBatch_Atomic(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	ms := NewMessageStore(store)

	// Seed a project and session so FK constraints pass.
	now := time.Now().UnixMilli()
	if _, err := db.Exec(
		`INSERT INTO project (id, worktree, time_created, time_updated) VALUES (?, ?, ?, ?)`,
		"proj-1", "/tmp", now, now,
	); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO session (id, project_id, slug, directory, title, version, time_created, time_updated) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"ses-1", "proj-1", "s", "/tmp", "test", "1", now, now,
	); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	msgs := []Message{
		{ID: "msg-1", Role: RoleUser, Parts: []Part{TextPart("hello")}, CreatedAt: time.Now()},
		{ID: "msg-1", Role: RoleUser, Parts: []Part{TextPart("dup id")}, CreatedAt: time.Now()}, // duplicate PK
		{ID: "msg-3", Role: RoleAssistant, Parts: []Part{TextPart("hi")}, CreatedAt: time.Now()},
	}

	err := ms.AppendBatch("ses-1", msgs)
	if err == nil {
		t.Fatal("expected error from duplicate PK, got nil")
	}

	// Verify none persisted.
	count, err := ms.Count("ses-1")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 messages after failed batch, got %d", count)
	}
}

func TestAppendBatch_Success(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	ms := NewMessageStore(store)

	now := time.Now().UnixMilli()
	if _, err := db.Exec(
		`INSERT INTO project (id, worktree, time_created, time_updated) VALUES (?, ?, ?, ?)`,
		"proj-1", "/tmp", now, now,
	); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO session (id, project_id, slug, directory, title, version, time_created, time_updated) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"ses-1", "proj-1", "s", "/tmp", "test", "1", now, now,
	); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	msgs := []Message{
		{ID: "msg-1", Role: RoleUser, Parts: []Part{TextPart("hello")}, CreatedAt: time.Now()},
		{ID: "msg-2", Role: RoleAssistant, Parts: []Part{TextPart("hi")}, CreatedAt: time.Now()},
		{ID: "msg-3", Role: RoleUser, Parts: []Part{TextPart("bye")}, CreatedAt: time.Now()},
	}

	if err := ms.AppendBatch("ses-1", msgs); err != nil {
		t.Fatalf("AppendBatch: %v", err)
	}

	count, err := ms.Count("ses-1")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 messages, got %d", count)
	}

	listed, err := ms.List("ses-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(listed) != 3 {
		t.Fatalf("expected 3 listed messages, got %d", len(listed))
	}
	for i, m := range listed {
		if m.ID != msgs[i].ID {
			t.Errorf("message %d: got ID %s, want %s", i, m.ID, msgs[i].ID)
		}
	}
}
