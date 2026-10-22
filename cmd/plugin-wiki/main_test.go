package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

func callTool(t *testing.T, tool plugin.ToolDef, args any, dir string) string {
	t.Helper()
	data, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	result, err := tool.Execute(context.Background(), data, plugin.ToolContext{
		SessionID: "test",
		Directory: dir,
	})
	if err != nil {
		t.Fatalf("tool %s failed: %v", tool.Name, err)
	}
	return result
}

func TestWriteAndRead(t *testing.T) {
	dir := t.TempDir()
	w := writeTool()
	r := readTool()

	result := callTool(t, w, map[string]string{
		"title":    "Auth Design",
		"body":     "JWT tokens with refresh rotation.",
		"category": "architecture",
		"tags":     "auth, jwt",
	}, dir)

	if !strings.Contains(result, "Wiki page written") {
		t.Fatalf("expected success message, got %q", result)
	}

	path := filepath.Join(dir, wikiDir, "auth-design.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected wiki file at %s", path)
	}

	content := callTool(t, r, map[string]string{"title": "Auth Design"}, dir)
	if !strings.Contains(content, "JWT tokens with refresh rotation") {
		t.Fatalf("expected body in read result, got %q", content)
	}
	if !strings.Contains(content, "title: Auth Design") {
		t.Fatalf("expected frontmatter title, got %q", content)
	}
	if !strings.Contains(content, "category: architecture") {
		t.Fatalf("expected category, got %q", content)
	}
	if !strings.Contains(content, "tags: auth, jwt") {
		t.Fatalf("expected tags, got %q", content)
	}
}

func TestWritePreservesCreatedDate(t *testing.T) {
	dir := t.TempDir()
	w := writeTool()

	callTool(t, w, map[string]string{
		"title": "Evolving Page",
		"body":  "Version 1",
	}, dir)

	// Inject a distinctive created date so we can verify it survives rewrites.
	path := filepath.Join(dir, wikiDir, "evolving-page.md")
	data1, _ := os.ReadFile(path)
	fm1, body1 := parseFrontmatter(string(data1))
	fm1["created"] = "2020-01-01T00:00:00Z"
	var sb strings.Builder
	sb.WriteString("---\n")
	for k, v := range fm1 {
		sb.WriteString(k + ": " + v + "\n")
	}
	sb.WriteString("---\n")
	sb.WriteString(body1)
	os.WriteFile(path, []byte(sb.String()), 0o644)

	callTool(t, w, map[string]string{
		"title": "Evolving Page",
		"body":  "Version 2",
	}, dir)

	data2, _ := os.ReadFile(path)
	fm2, _ := parseFrontmatter(string(data2))
	if fm2["created"] != "2020-01-01T00:00:00Z" {
		t.Errorf("created should be preserved, got %q", fm2["created"])
	}
	if !strings.Contains(string(data2), "Version 2") {
		t.Error("body should be updated")
	}
}

func TestWriteDefaultCategory(t *testing.T) {
	dir := t.TempDir()
	callTool(t, writeTool(), map[string]string{
		"title": "No Category",
		"body":  "Some content",
	}, dir)

	data, _ := os.ReadFile(filepath.Join(dir, wikiDir, "no-category.md"))
	if !strings.Contains(string(data), "category: reference") {
		t.Error("expected default category 'reference'")
	}
}

func TestReadBySlug(t *testing.T) {
	dir := t.TempDir()
	callTool(t, writeTool(), map[string]string{
		"title": "My Page Title",
		"body":  "Content here",
	}, dir)

	r := readTool()
	content := callTool(t, r, map[string]string{"title": "my-page-title"}, dir)
	if !strings.Contains(content, "Content here") {
		t.Fatalf("slug-based read failed: %q", content)
	}
}

func TestReadByFilename(t *testing.T) {
	dir := t.TempDir()
	callTool(t, writeTool(), map[string]string{
		"title": "File Read",
		"body":  "Via filename",
	}, dir)

	r := readTool()
	content := callTool(t, r, map[string]string{"title": "file-read.md"}, dir)
	if !strings.Contains(content, "Via filename") {
		t.Fatalf("filename-based read failed: %q", content)
	}
}

func TestReadNotFound(t *testing.T) {
	dir := t.TempDir()
	r := readTool()
	args, _ := json.Marshal(map[string]string{"title": "nonexistent"})
	_, err := r.Execute(context.Background(), args, plugin.ToolContext{Directory: dir})
	if err == nil {
		t.Fatal("expected error for nonexistent page")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got %v", err)
	}
}

func TestSearch(t *testing.T) {
	dir := t.TempDir()
	w := writeTool()
	callTool(t, w, map[string]string{
		"title": "Go Patterns",
		"body":  "Use table-driven tests",
		"tags":  "golang, testing",
	}, dir)
	callTool(t, w, map[string]string{
		"title": "Python Patterns",
		"body":  "Use pytest fixtures",
		"tags":  "python",
	}, dir)

	s := searchTool()

	result := callTool(t, s, map[string]string{"query": "table-driven"}, dir)
	if !strings.Contains(result, "Go Patterns") {
		t.Errorf("expected Go Patterns in search results, got %q", result)
	}
	if strings.Contains(result, "Python Patterns") {
		t.Error("Python Patterns should not match 'table-driven'")
	}

	result = callTool(t, s, map[string]string{"query": "golang"}, dir)
	if !strings.Contains(result, "Go Patterns") {
		t.Error("tag search should match")
	}

	result = callTool(t, s, map[string]string{"query": "nonexistent"}, dir)
	if !strings.Contains(result, "No wiki pages match") {
		t.Errorf("expected no-match message, got %q", result)
	}
}

func TestSearchEmptyWiki(t *testing.T) {
	dir := t.TempDir()
	result := callTool(t, searchTool(), map[string]string{"query": "anything"}, dir)
	if !strings.Contains(result, "No wiki pages found") {
		t.Errorf("expected empty wiki message, got %q", result)
	}
}

func TestList(t *testing.T) {
	dir := t.TempDir()
	w := writeTool()
	callTool(t, w, map[string]string{
		"title":    "First",
		"body":     "First page",
		"category": "architecture",
	}, dir)
	callTool(t, w, map[string]string{
		"title":    "Second",
		"body":     "Second page",
		"category": "decision",
		"tags":     "important",
	}, dir)

	result := callTool(t, listTool(), map[string]string{}, dir)
	if !strings.Contains(result, "2 wiki page(s)") {
		t.Fatalf("expected 2 pages, got %q", result)
	}
	if !strings.Contains(result, "First") || !strings.Contains(result, "Second") {
		t.Errorf("expected both pages listed, got %q", result)
	}
	if !strings.Contains(result, "[architecture]") {
		t.Error("expected category in list")
	}
	if !strings.Contains(result, "(important)") {
		t.Error("expected tags in list")
	}
}

func TestListEmptyWiki(t *testing.T) {
	dir := t.TempDir()
	result := callTool(t, listTool(), map[string]string{}, dir)
	if !strings.Contains(result, "No wiki pages") {
		t.Errorf("expected empty message, got %q", result)
	}
}

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Auth Design", "auth-design"},
		{"Go: Table-Driven Tests", "go-table-driven-tests"},
		{"  spaces  ", "spaces"},
		{"UPPER CASE", "upper-case"},
		{"special!@#chars", "special-chars"},
		{"", "untitled"},
		{"---", "untitled"},
	}
	for _, tt := range tests {
		got := slugify(tt.input)
		if got != tt.want {
			t.Errorf("slugify(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseFrontmatter(t *testing.T) {
	content := "---\ntitle: Test Page\ncategory: decision\ntags: a, b\n---\n\nBody here."
	fm, body := parseFrontmatter(content)
	if fm == nil {
		t.Fatal("expected frontmatter")
	}
	if fm["title"] != "Test Page" {
		t.Errorf("title = %q", fm["title"])
	}
	if fm["category"] != "decision" {
		t.Errorf("category = %q", fm["category"])
	}
	if !strings.Contains(body, "Body here") {
		t.Errorf("body = %q", body)
	}
}

func TestParseFrontmatter_NoFrontmatter(t *testing.T) {
	fm, body := parseFrontmatter("Just plain text")
	if fm != nil {
		t.Error("expected nil frontmatter")
	}
	if body != "Just plain text" {
		t.Errorf("body = %q", body)
	}
}

func TestWriteEmptyTitle(t *testing.T) {
	w := writeTool()
	args, _ := json.Marshal(map[string]string{"title": "", "body": "content"})
	_, err := w.Execute(context.Background(), args, plugin.ToolContext{Directory: t.TempDir()})
	if err == nil {
		t.Fatal("expected error for empty title")
	}
}

func TestWriteEmptyBody(t *testing.T) {
	w := writeTool()
	args, _ := json.Marshal(map[string]string{"title": "Title", "body": ""})
	_, err := w.Execute(context.Background(), args, plugin.ToolContext{Directory: t.TempDir()})
	if err == nil {
		t.Fatal("expected error for empty body")
	}
}

func TestSearchCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	callTool(t, writeTool(), map[string]string{
		"title": "Kubernetes Tips",
		"body":  "Use kubectl get pods",
	}, dir)

	result := callTool(t, searchTool(), map[string]string{"query": "KUBECTL"}, dir)
	if !strings.Contains(result, "Kubernetes Tips") {
		t.Errorf("case-insensitive search failed: %q", result)
	}
}
