package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStore_SetGetDelete_PersistAndMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := s.Set("openai", map[string]string{"apiKey": "sk-test"}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("mode = %o, want 0600", mode)
	}

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("re-Open: %v", err)
	}
	if got := s2.APIKey("openai"); got != "sk-test" {
		t.Fatalf("APIKey = %q, want sk-test", got)
	}

	if err := s2.Delete("openai"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	s3, err := Open(path)
	if err != nil {
		t.Fatalf("re-Open after delete: %v", err)
	}
	if _, ok := s3.Get("openai"); ok {
		t.Fatal("expected openai credentials removed")
	}
}

func TestResolveAPIKey_Precedence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	_ = s.Set("p", map[string]string{"apiKey": "from-store"})

	t.Setenv("P_API_KEY", "")
	got := ResolveAPIKey([]string{"P_API_KEY"}, map[string]any{"apiKey": "from-config"}, s, "p")
	if got != "from-config" {
		t.Fatalf("want from-config, got %q", got)
	}

	t.Setenv("P_API_KEY", "from-env")
	got = ResolveAPIKey([]string{"P_API_KEY"}, map[string]any{"apiKey": "from-config"}, s, "p")
	if got != "from-env" {
		t.Fatalf("want from-env, got %q", got)
	}

	t.Setenv("P_API_KEY", "")
	got = ResolveAPIKey([]string{"P_API_KEY"}, nil, s, "p")
	if got != "from-store" {
		t.Fatalf("want from-store, got %q", got)
	}
}

func TestOpen_MissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "auth.json")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open missing: %v", err)
	}
	if len(s.All()) != 0 {
		t.Fatal("expected empty store")
	}
}
