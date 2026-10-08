package acp

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/session"
	"github.com/bobbyjohnstx/tinycode/internal/storage"
)

func testStoreAdapter(t *testing.T) *StoreAdapter {
	t.Helper()
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	store := session.NewStore(db.DB)
	return NewStoreAdapter(store)
}

func TestNewStoreAdapter_ReturnsNonNil(t *testing.T) {
	adapter := testStoreAdapter(t)
	if adapter == nil {
		t.Fatal("expected non-nil adapter")
	}
	if adapter.store == nil {
		t.Fatal("expected non-nil store")
	}
	if adapter.msgs == nil {
		t.Fatal("expected non-nil message store")
	}
}

func TestStoreAdapter_Create_ReturnsSessionWithFields(t *testing.T) {
	adapter := testStoreAdapter(t)
	ctx := context.Background()

	info, err := adapter.Create(ctx, session.CreateInput{
		ProjectID: "proj_1",
		Directory: "/tmp/test",
		Title:     "My Session",
		Agent:     "code",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if info.ID == "" {
		t.Error("expected non-empty session ID")
	}
	if info.ProjectID != "proj_1" {
		t.Errorf("expected project ID 'proj_1', got %q", info.ProjectID)
	}
	if info.Directory != "/tmp/test" {
		t.Errorf("expected directory '/tmp/test', got %q", info.Directory)
	}
	if info.Title != "My Session" {
		t.Errorf("expected title 'My Session', got %q", info.Title)
	}
	if info.Agent != "code" {
		t.Errorf("expected agent 'code', got %q", info.Agent)
	}
}

func TestStoreAdapter_Get_ReturnsCreatedSession(t *testing.T) {
	adapter := testStoreAdapter(t)
	ctx := context.Background()

	created, err := adapter.Create(ctx, session.CreateInput{
		ProjectID: "proj_1",
		Directory: "/tmp/test",
		Title:     "Test Session",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := adapter.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("expected ID %q, got %q", created.ID, got.ID)
	}
	if got.Title != "Test Session" {
		t.Errorf("expected title 'Test Session', got %q", got.Title)
	}
}

func TestStoreAdapter_Get_NonexistentReturnsError(t *testing.T) {
	adapter := testStoreAdapter(t)
	ctx := context.Background()

	_, err := adapter.Get(ctx, "ses_nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestStoreAdapter_List_FiltersByProject(t *testing.T) {
	adapter := testStoreAdapter(t)
	ctx := context.Background()

	adapter.Create(ctx, session.CreateInput{
		ProjectID: "proj_a", Directory: "/tmp/a", Title: "A1",
	})
	adapter.Create(ctx, session.CreateInput{
		ProjectID: "proj_a", Directory: "/tmp/a", Title: "A2",
	})
	adapter.Create(ctx, session.CreateInput{
		ProjectID: "proj_b", Directory: "/tmp/b", Title: "B1",
	})

	list, err := adapter.List(ctx, "proj_a")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 sessions for proj_a, got %d", len(list))
	}
	for _, info := range list {
		if info.ProjectID != "proj_a" {
			t.Errorf("expected project ID 'proj_a', got %q", info.ProjectID)
		}
	}
}

func TestStoreAdapter_List_EmptyProjectReturnsEmptySlice(t *testing.T) {
	adapter := testStoreAdapter(t)
	ctx := context.Background()

	list, err := adapter.List(ctx, "proj_nonexistent")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(list))
	}
}

func TestStoreAdapter_Delete_RemovesSession(t *testing.T) {
	adapter := testStoreAdapter(t)
	ctx := context.Background()

	created, _ := adapter.Create(ctx, session.CreateInput{
		ProjectID: "proj_1", Directory: "/tmp", Title: "Doomed",
	})

	if err := adapter.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := adapter.Get(ctx, created.ID)
	if err == nil {
		t.Fatal("expected error after deleting session")
	}
}

func TestStoreAdapter_UpdateModel_PersistsChange(t *testing.T) {
	adapter := testStoreAdapter(t)
	ctx := context.Background()

	created, _ := adapter.Create(ctx, session.CreateInput{
		ProjectID: "proj_1", Directory: "/tmp", Title: "Model Test",
	})

	model := &session.ModelRef{ProviderID: "ollama", ModelID: "qwen3:8b"}
	if err := adapter.UpdateModel(ctx, created.ID, model); err != nil {
		t.Fatalf("UpdateModel: %v", err)
	}

	got, _ := adapter.Get(ctx, created.ID)
	if got.Model == nil {
		t.Fatal("expected non-nil model after update")
	}
	if got.Model.ProviderID != "ollama" {
		t.Errorf("expected provider 'ollama', got %q", got.Model.ProviderID)
	}
	if got.Model.ModelID != "qwen3:8b" {
		t.Errorf("expected model 'qwen3:8b', got %q", got.Model.ModelID)
	}
}

func TestStoreAdapter_UpdateAgent_PersistsChange(t *testing.T) {
	adapter := testStoreAdapter(t)
	ctx := context.Background()

	created, _ := adapter.Create(ctx, session.CreateInput{
		ProjectID: "proj_1", Directory: "/tmp", Title: "Agent Test", Agent: "code",
	})

	if err := adapter.UpdateAgent(ctx, created.ID, "plan"); err != nil {
		t.Fatalf("UpdateAgent: %v", err)
	}

	got, _ := adapter.Get(ctx, created.ID)
	if got.Agent != "plan" {
		t.Errorf("expected agent 'plan', got %q", got.Agent)
	}
}

func TestStoreAdapter_ListMessages_ReturnsAppendedMessages(t *testing.T) {
	adapter := testStoreAdapter(t)
	ctx := context.Background()

	created, _ := adapter.Create(ctx, session.CreateInput{
		ProjectID: "proj_1", Directory: "/tmp", Title: "Msg Test",
	})

	msg := &session.Message{
		ID:        "msg_001",
		SessionID: created.ID,
		Role:      session.RoleUser,
		Parts:     []session.Part{{Type: session.PartText, Text: "hello"}},
		CreatedAt: time.Now(),
	}
	if err := adapter.msgs.Append(msg); err != nil {
		t.Fatalf("Append message: %v", err)
	}

	msgs, err := adapter.ListMessages(ctx, created.ID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != session.RoleUser {
		t.Errorf("expected role 'user', got %q", msgs[0].Role)
	}
	if len(msgs[0].Parts) != 1 || msgs[0].Parts[0].Text != "hello" {
		t.Errorf("expected text 'hello', got %+v", msgs[0].Parts)
	}
}

func TestStoreAdapter_ListMessages_EmptySessionReturnsNil(t *testing.T) {
	adapter := testStoreAdapter(t)
	ctx := context.Background()

	created, _ := adapter.Create(ctx, session.CreateInput{
		ProjectID: "proj_1", Directory: "/tmp", Title: "Empty",
	})

	msgs, err := adapter.ListMessages(ctx, created.ID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages, got %d", len(msgs))
	}
}

func TestStoreAdapter_Fork_CopiesMessagesWithNewIDs(t *testing.T) {
	adapter := testStoreAdapter(t)
	ctx := context.Background()

	parent, _ := adapter.Create(ctx, session.CreateInput{
		ProjectID: "proj_1",
		Directory: "/tmp",
		Title:     "Parent",
		Agent:     "code",
		Model:     &session.ModelRef{ProviderID: "ollama", ModelID: "qwen3:8b"},
	})

	for i, text := range []string{"hello", "world"} {
		msg := &session.Message{
			ID:        fmt.Sprintf("msg_%03d", i),
			SessionID: parent.ID,
			Role:      session.RoleUser,
			Parts:     []session.Part{{Type: session.PartText, Text: text}},
			CreatedAt: time.Now(),
		}
		adapter.msgs.Append(msg)
	}

	forked, err := adapter.Fork(ctx, parent.ID, "")
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	if forked.ID == parent.ID {
		t.Error("forked session should have a different ID")
	}
	if forked.Title != "Parent (fork)" {
		t.Errorf("expected default fork title 'Parent (fork)', got %q", forked.Title)
	}
	if forked.ParentID != parent.ID {
		t.Errorf("expected parent ID %q, got %q", parent.ID, forked.ParentID)
	}
	if forked.Agent != "code" {
		t.Errorf("expected agent 'code', got %q", forked.Agent)
	}
	if forked.Model == nil || forked.Model.ModelID != "qwen3:8b" {
		t.Errorf("expected model to be copied, got %+v", forked.Model)
	}

	forkedMsgs, err := adapter.ListMessages(ctx, forked.ID)
	if err != nil {
		t.Fatalf("ListMessages on fork: %v", err)
	}
	if len(forkedMsgs) != 2 {
		t.Fatalf("expected 2 forked messages, got %d", len(forkedMsgs))
	}

	parentMsgs, _ := adapter.ListMessages(ctx, parent.ID)
	for i, fm := range forkedMsgs {
		if fm.ID == parentMsgs[i].ID {
			t.Errorf("message %d: forked message should have a new ID, got same %q", i, fm.ID)
		}
		if fm.Parts[0].Text != parentMsgs[i].Parts[0].Text {
			t.Errorf("message %d: expected text %q, got %q", i, parentMsgs[i].Parts[0].Text, fm.Parts[0].Text)
		}
	}
}

func TestStoreAdapter_Fork_CustomTitle(t *testing.T) {
	adapter := testStoreAdapter(t)
	ctx := context.Background()

	parent, _ := adapter.Create(ctx, session.CreateInput{
		ProjectID: "proj_1", Directory: "/tmp", Title: "Parent",
	})

	forked, err := adapter.Fork(ctx, parent.ID, "Custom Fork Title")
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	if forked.Title != "Custom Fork Title" {
		t.Errorf("expected title 'Custom Fork Title', got %q", forked.Title)
	}
}

func TestStoreAdapter_Fork_NonexistentParentReturnsError(t *testing.T) {
	adapter := testStoreAdapter(t)
	ctx := context.Background()

	_, err := adapter.Fork(ctx, "ses_nonexistent", "")
	if err == nil {
		t.Fatal("expected error when forking nonexistent session")
	}
}

func TestStoreAdapter_Fork_EmptyParentCopiesNoMessages(t *testing.T) {
	adapter := testStoreAdapter(t)
	ctx := context.Background()

	parent, _ := adapter.Create(ctx, session.CreateInput{
		ProjectID: "proj_1", Directory: "/tmp", Title: "Empty Parent",
	})

	forked, err := adapter.Fork(ctx, parent.ID, "")
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}

	msgs, err := adapter.ListMessages(ctx, forked.ID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages for fork of empty parent, got %d", len(msgs))
	}
}
