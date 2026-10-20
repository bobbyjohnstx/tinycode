package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchRegistry_Filters(t *testing.T) {
	all := SearchRegistry("")
	if len(all) == 0 {
		t.Fatal("expected registry entries")
	}
	hits := SearchRegistry("wiki")
	if len(hits) == 0 {
		t.Fatal("expected wiki hit")
	}
	found := false
	for _, h := range hits {
		if h.Name == "wiki" {
			found = true
		}
	}
	if !found {
		t.Fatalf("wiki not in hits: %+v", hits)
	}
	none := SearchRegistry("zzzz-no-such-plugin")
	if len(none) != 0 {
		t.Fatalf("expected empty, got %d", len(none))
	}
}

func TestInitScaffold_WritesArgsNotArguments(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "plugin-hello")
	if err := InitScaffold(dir, "hello"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	if !strings.Contains(src, `"args"`) && !strings.Contains(src, "args") {
		t.Fatal("expected args mention in scaffold")
	}
	if strings.Contains(src, `json:"arguments"`) {
		t.Fatal("scaffold must not use json:\"arguments\"")
	}
	readme, err := os.ReadFile(filepath.Join(dir, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(readme), `json:"args"`) {
		t.Fatal("README must document json:\"args\"")
	}
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		t.Fatal(err)
	}
}

func TestInitScaffold_InvalidName(t *testing.T) {
	if err := InitScaffold(t.TempDir(), "Bad_Name"); err == nil {
		t.Fatal("expected error")
	}
}
