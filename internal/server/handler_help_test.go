package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHelp(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/help", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var body HelpResponse
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if len(body.Keybindings) == 0 {
		t.Error("expected keybindings to be non-empty")
	}
	if len(body.Commands) == 0 {
		t.Error("expected commands to be non-empty")
	}
	if len(body.Features) == 0 {
		t.Error("expected features to be non-empty")
	}

	// Verify keybinding structure has required fields.
	for _, kb := range body.Keybindings {
		if kb.Key == "" {
			t.Error("keybinding missing key")
		}
		if kb.Description == "" {
			t.Error("keybinding missing description")
		}
		if kb.Category == "" {
			t.Error("keybinding missing category")
		}
	}

	// Verify commands include builtins.
	found := false
	for _, cmd := range body.Commands {
		if cmd.Name == "init" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected builtin 'init' command in help response")
	}

	// Verify feature structure.
	for _, feat := range body.Features {
		if feat.Name == "" {
			t.Error("feature missing name")
		}
		if feat.Description == "" {
			t.Error("feature missing description")
		}
	}
}
