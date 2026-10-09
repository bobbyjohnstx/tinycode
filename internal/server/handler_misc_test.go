package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/provider"
	"github.com/bobbyjohnstx/tinycode/internal/session"
)

func TestHandleCommandList_ReturnsJSON(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/command", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
}

func TestHandleSkillList_ReturnsJSON(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/skill", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
}

func TestHandleProjectList_ReturnsEmptyArrayWhenNoProjects(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/project", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	body := strings.TrimSpace(w.Body.String())
	if body != "[]" {
		t.Errorf("expected empty JSON array '[]', got %q", body)
	}
}

func TestHandleProjectCurrent_ReturnsProjectInfo(t *testing.T) {
	dir := t.TempDir()
	b := bus.New()
	t.Cleanup(func() { b.Close() })
	db := testDB(t)
	reg := provider.NewRegistry()
	srv := New(Config{Directory: dir}, Dependencies{Bus: b, DB: db, Registry: reg})

	req := httptest.NewRequest("GET", "/project/current", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]any
	json.NewDecoder(w.Body).Decode(&result)
	if result == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestHandlePathGet_ReturnsExpectedKeys(t *testing.T) {
	dir := t.TempDir()
	b := bus.New()
	t.Cleanup(func() { b.Close() })
	db := testDB(t)
	reg := provider.NewRegistry()
	srv := New(Config{Directory: dir}, Dependencies{Bus: b, DB: db, Registry: reg})

	req := httptest.NewRequest("GET", "/path", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]any
	json.NewDecoder(w.Body).Decode(&result)

	for _, key := range []string{"home", "state", "config", "worktree", "directory"} {
		if _, ok := result[key]; !ok {
			t.Errorf("missing expected key %q in path response", key)
		}
	}

	if result["directory"] != dir {
		t.Errorf("expected directory %q, got %v", dir, result["directory"])
	}
}

func TestHandleConfigProviders_ReturnsProviderSummaries(t *testing.T) {
	b := bus.New()
	t.Cleanup(func() { b.Close() })
	db := testDB(t)
	reg := provider.NewRegistry()
	reg.Register(&provider.Info{
		ID:     "ollama",
		Name:   "Ollama",
		Source: "auto",
		Models: map[string]*provider.Model{
			"llama3": {ID: "llama3", ProviderID: "ollama"},
			"phi3":   {ID: "phi3", ProviderID: "ollama"},
		},
	})
	srv := New(Config{}, Dependencies{Bus: b, DB: db, Registry: reg})

	req := httptest.NewRequest("GET", "/config/providers", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result []map[string]any
	json.NewDecoder(w.Body).Decode(&result)
	if len(result) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(result))
	}
	if result[0]["id"] != "ollama" {
		t.Errorf("expected id 'ollama', got %v", result[0]["id"])
	}
	if result[0]["name"] != "Ollama" {
		t.Errorf("expected name 'Ollama', got %v", result[0]["name"])
	}
	models, _ := result[0]["models"].(float64)
	if int(models) != 2 {
		t.Errorf("expected 2 models, got %v", result[0]["models"])
	}
}

func TestHandleConfigProviders_ReturnsEmptyArrayWhenNoProviders(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/config/providers", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := strings.TrimSpace(w.Body.String())
	if body != "[]" {
		t.Errorf("expected empty JSON array, got %q", body)
	}
}

func TestHandleMessageDelete_ReturnsNoContent(t *testing.T) {
	srv, b := testServer(t)

	// Create a session first, then add a message.
	createReq := httptest.NewRequest("POST", "/session?directory=/tmp", strings.NewReader(`{"title":"del-test"}`))
	createW := httptest.NewRecorder()
	srv.mux.ServeHTTP(createW, createReq)

	var created map[string]any
	json.NewDecoder(createW.Body).Decode(&created)
	sessionID := created["id"].(string)

	// Insert a message via the message store.
	ms := srv.messageStore()
	msg := &session.Message{
		ID:        "msg_del_1",
		SessionID: sessionID,
		Role:      session.RoleUser,
		Parts:     []session.Part{session.TextPart("delete me")},
		CreatedAt: time.Now(),
	}
	if err := ms.Append(msg); err != nil {
		t.Fatalf("append message: %v", err)
	}

	// Subscribe to message.removed event.
	sub := b.Subscribe("message.removed")
	defer sub.Unsubscribe()

	// DELETE the message.
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/session/%s/message/msg_del_1", sessionID), nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	// Verify bus event published.
	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		if props["messageID"] != "msg_del_1" {
			t.Errorf("expected messageID 'msg_del_1', got %v", props["messageID"])
		}
		if props["sessionID"] != sessionID {
			t.Errorf("expected sessionID %q, got %v", sessionID, props["sessionID"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message.removed event")
	}
}

func TestHandleVCSInfo_ReturnsResponseForNonGitDir(t *testing.T) {
	dir := t.TempDir()
	b := bus.New()
	t.Cleanup(func() { b.Close() })
	db := testDB(t)
	reg := provider.NewRegistry()
	srv := New(Config{Directory: dir}, Dependencies{Bus: b, DB: db, Registry: reg})

	req := httptest.NewRequest("GET", "/vcs", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]any
	json.NewDecoder(w.Body).Decode(&result)
	// Non-git directories return empty type with an error.
	if result["type"] == nil {
		t.Error("expected type field in response")
	}
}

func TestHandleVCSDiff_ReturnsResponseForNonGitDir(t *testing.T) {
	dir := t.TempDir()
	b := bus.New()
	t.Cleanup(func() { b.Close() })
	db := testDB(t)
	reg := provider.NewRegistry()
	srv := New(Config{Directory: dir}, Dependencies{Bus: b, DB: db, Registry: reg})

	req := httptest.NewRequest("GET", "/vcs/diff", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]any
	json.NewDecoder(w.Body).Decode(&result)
	// Non-git: returns error field.
	if _, ok := result["error"]; !ok {
		// If no error, at minimum must have "diff" key.
		if _, ok := result["diff"]; !ok {
			t.Error("expected either 'error' or 'diff' key in response")
		}
	}
}

func TestHandleProviderAuth_ReturnsAuthMethods(t *testing.T) {
	b := bus.New()
	t.Cleanup(func() { b.Close() })
	db := testDB(t)
	reg := provider.NewRegistry()
	reg.Register(&provider.Info{
		ID:     "openai",
		Name:   "OpenAI",
		Source: "config",
		Env:    []string{"OPENAI_API_KEY"},
		Models: map[string]*provider.Model{},
	})
	srv := New(Config{}, Dependencies{Bus: b, DB: db, Registry: reg})

	req := httptest.NewRequest("GET", "/provider/auth", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]any
	json.NewDecoder(w.Body).Decode(&result)
	if result["openai"] == nil {
		t.Error("expected 'openai' key in auth response")
	}
}

func TestHandleProviderAuth_LocalProviderReturnsEmptyArray(t *testing.T) {
	b := bus.New()
	t.Cleanup(func() { b.Close() })
	db := testDB(t)
	reg := provider.NewRegistry()
	reg.Register(&provider.Info{
		ID:     "ollama",
		Name:   "Ollama",
		Source: "custom",
		Models: map[string]*provider.Model{},
	})
	srv := New(Config{}, Dependencies{Bus: b, DB: db, Registry: reg})

	req := httptest.NewRequest("GET", "/provider/auth", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result map[string]any
	json.NewDecoder(w.Body).Decode(&result)
	arr, ok := result["ollama"].([]any)
	if !ok {
		t.Fatalf("expected array for ollama, got %T", result["ollama"])
	}
	if len(arr) != 0 {
		t.Errorf("expected empty array for local provider, got %v", arr)
	}
}

func TestHandleModelList_ReturnsModelsForProvider(t *testing.T) {
	b := bus.New()
	t.Cleanup(func() { b.Close() })
	db := testDB(t)
	reg := provider.NewRegistry()
	reg.Register(&provider.Info{
		ID:     "test-prov",
		Name:   "Test Provider",
		Source: "config",
		Models: map[string]*provider.Model{
			"model-a": {ID: "model-a", ProviderID: "test-prov"},
			"model-b": {ID: "model-b", ProviderID: "test-prov"},
		},
	})
	srv := New(Config{}, Dependencies{Bus: b, DB: db, Registry: reg})

	req := httptest.NewRequest("GET", "/provider/test-prov/model", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]any
	json.NewDecoder(w.Body).Decode(&result)
	if len(result) != 2 {
		t.Errorf("expected 2 models, got %d", len(result))
	}
}

func TestHandleModelList_Returns404ForUnknownProvider(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/provider/nonexistent/model", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleModelGet_ReturnsSpecificModel(t *testing.T) {
	b := bus.New()
	t.Cleanup(func() { b.Close() })
	db := testDB(t)
	reg := provider.NewRegistry()
	reg.Register(&provider.Info{
		ID:   "prov1",
		Name: "Provider 1",
		Models: map[string]*provider.Model{
			"my-model": {ID: "my-model", ProviderID: "prov1", Name: "My Model"},
		},
	})
	srv := New(Config{}, Dependencies{Bus: b, DB: db, Registry: reg})

	req := httptest.NewRequest("GET", "/provider/prov1/model/my-model", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]any
	json.NewDecoder(w.Body).Decode(&result)
	if result["id"] != "my-model" {
		t.Errorf("expected model id 'my-model', got %v", result["id"])
	}
}

func TestHandleModelGet_Returns404ForUnknownModel(t *testing.T) {
	b := bus.New()
	t.Cleanup(func() { b.Close() })
	db := testDB(t)
	reg := provider.NewRegistry()
	reg.Register(&provider.Info{
		ID:     "prov1",
		Name:   "Provider 1",
		Models: map[string]*provider.Model{},
	})
	srv := New(Config{}, Dependencies{Bus: b, DB: db, Registry: reg})

	req := httptest.NewRequest("GET", "/provider/prov1/model/nope", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleProviderBalance_Returns200ForNonOpenRouter(t *testing.T) {
	b := bus.New()
	t.Cleanup(func() { b.Close() })
	db := testDB(t)
	reg := provider.NewRegistry()
	reg.Register(&provider.Info{
		ID:     "ollama",
		Name:   "Ollama",
		Source: "custom",
		Models: map[string]*provider.Model{},
	})
	srv := New(Config{}, Dependencies{Bus: b, DB: db, Registry: reg})

	req := httptest.NewRequest("GET", "/provider/ollama/balance", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]any
	json.NewDecoder(w.Body).Decode(&result)
	if result["remaining"] != nil {
		t.Errorf("expected nil remaining for non-OpenRouter, got %v", result["remaining"])
	}
}

func TestHandleProviderBalance_Returns404ForUnknownProvider(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/provider/nonexistent/balance", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleFileSearch_ReturnsEmptyForEmptyQuery(t *testing.T) {
	dir := t.TempDir()
	b := bus.New()
	t.Cleanup(func() { b.Close() })
	db := testDB(t)
	reg := provider.NewRegistry()
	srv := New(Config{Directory: dir}, Dependencies{Bus: b, DB: db, Registry: reg})

	req := httptest.NewRequest("GET", "/find", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result map[string]any
	json.NewDecoder(w.Body).Decode(&result)
	results, _ := result["results"].([]any)
	if len(results) != 0 {
		t.Errorf("expected empty results for empty query, got %d", len(results))
	}
}

func TestHandleFileSearch_FindsMatchingContent(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.go"), []byte("package main\nfunc hello() {}\n"), 0644)

	b := bus.New()
	t.Cleanup(func() { b.Close() })
	db := testDB(t)
	reg := provider.NewRegistry()
	srv := New(Config{Directory: dir}, Dependencies{Bus: b, DB: db, Registry: reg})

	req := httptest.NewRequest("GET", "/find?q=hello", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]any
	json.NewDecoder(w.Body).Decode(&result)
	results, _ := result["results"].([]any)
	if len(results) == 0 {
		t.Error("expected at least one search result for 'hello'")
	}
}

func TestHandleFileFind_ReturnsEmptyForEmptyQuery(t *testing.T) {
	dir := t.TempDir()
	b := bus.New()
	t.Cleanup(func() { b.Close() })
	db := testDB(t)
	reg := provider.NewRegistry()
	srv := New(Config{Directory: dir}, Dependencies{Bus: b, DB: db, Registry: reg})

	req := httptest.NewRequest("GET", "/find/file", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := strings.TrimSpace(w.Body.String())
	if body != "[]" {
		t.Errorf("expected empty array, got %q", body)
	}
}

func TestHandleFileFind_FindsMatchingFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Readme\n"), 0644)

	b := bus.New()
	t.Cleanup(func() { b.Close() })
	db := testDB(t)
	reg := provider.NewRegistry()
	srv := New(Config{Directory: dir}, Dependencies{Bus: b, DB: db, Registry: reg})

	req := httptest.NewRequest("GET", "/find/file?q=main", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result []string
	json.NewDecoder(w.Body).Decode(&result)
	if len(result) == 0 {
		t.Error("expected at least one file matching 'main'")
	}
}

func TestHandleGlobalDispose_PublishesEvent(t *testing.T) {
	srv, b := testServer(t)

	sub := b.Subscribe("global.disposed")
	defer sub.Unsubscribe()

	req := httptest.NewRequest("POST", "/global/dispose", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	select {
	case evt := <-sub.C:
		props := evt.Properties.(map[string]any)
		if _, ok := props["timestamp"]; !ok {
			t.Error("expected timestamp in global.disposed event")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for global.disposed event")
	}
}

func TestResolveDefaultModelRef_ConfiguredDefault(t *testing.T) {
	reg := provider.NewRegistry()
	reg.Register(&provider.Info{
		ID:   "ollama",
		Name: "Ollama",
		Models: map[string]*provider.Model{
			"llama3": {ID: "llama3", ProviderID: "ollama"},
		},
	})

	ref := ResolveDefaultModelRef(reg, "ollama/llama3")
	if ref == nil {
		t.Fatal("expected non-nil ModelRef")
	}
	if ref.ProviderID != "ollama" {
		t.Errorf("expected providerID 'ollama', got %q", ref.ProviderID)
	}
	if ref.ModelID != "llama3" {
		t.Errorf("expected modelID 'llama3', got %q", ref.ModelID)
	}
}

func TestResolveDefaultModelRef_FallsBackWhenDefaultNotFound(t *testing.T) {
	reg := provider.NewRegistry()
	reg.Register(&provider.Info{
		ID:   "ollama",
		Name: "Ollama",
		Models: map[string]*provider.Model{
			"phi3": {ID: "phi3", ProviderID: "ollama"},
		},
	})

	// Request a model that doesn't exist — should fall back to available models.
	ref := ResolveDefaultModelRef(reg, "ollama/nonexistent")
	if ref == nil {
		t.Fatal("expected non-nil fallback ModelRef")
	}
	if ref.ProviderID != "ollama" {
		t.Errorf("expected fallback providerID 'ollama', got %q", ref.ProviderID)
	}
}

func TestResolveDefaultModelRef_ReturnsNilWhenNoModels(t *testing.T) {
	reg := provider.NewRegistry()
	ref := ResolveDefaultModelRef(reg, "")
	if ref != nil {
		t.Errorf("expected nil when no models available, got %+v", ref)
	}
}

func TestHandleVCSDiffRaw_ReturnsTextPlain(t *testing.T) {
	dir := t.TempDir()
	b := bus.New()
	t.Cleanup(func() { b.Close() })
	db := testDB(t)
	reg := provider.NewRegistry()
	srv := New(Config{Directory: dir}, Dependencies{Bus: b, DB: db, Registry: reg})

	req := httptest.NewRequest("GET", "/vcs/diff/raw", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if ct != "text/plain" {
		t.Errorf("expected Content-Type text/plain, got %q", ct)
	}
}
