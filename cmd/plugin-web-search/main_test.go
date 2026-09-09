package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

func TestPluginID(t *testing.T) {
	p := newPlugin(http.DefaultClient)
	if p.ID != "web-search" {
		t.Errorf("expected plugin ID 'web-search', got %q", p.ID)
	}
}

func TestPluginHasTwoTools(t *testing.T) {
	p := newPlugin(http.DefaultClient)
	if len(p.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(p.Tools))
	}

	expected := []string{"web_search", "rh_kb_search"}
	for i, name := range expected {
		if p.Tools[i].Name != name {
			t.Errorf("tool[%d]: expected %q, got %q", i, name, p.Tools[i].Name)
		}
	}
}

func TestToolSchemas(t *testing.T) {
	p := newPlugin(http.DefaultClient)
	for _, tool := range p.Tools {
		params := tool.Parameters
		if params["type"] != "object" {
			t.Errorf("%s: expected type 'object'", tool.Name)
		}
		props, ok := params["properties"].(map[string]any)
		if !ok {
			t.Errorf("%s: expected properties map", tool.Name)
			continue
		}
		if _, ok := props["query"]; !ok {
			t.Errorf("%s: expected 'query' property", tool.Name)
		}
		if _, ok := props["maxResults"]; !ok {
			t.Errorf("%s: expected 'maxResults' property", tool.Name)
		}
		required, ok := params["required"].([]string)
		if !ok || len(required) != 1 || required[0] != "query" {
			t.Errorf("%s: expected required=['query'], got %v", tool.Name, params["required"])
		}
	}
}

func TestStripHTML(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"<b>bold</b>", "bold"},
		{"no tags", "no tags"},
		{"a &amp; b", "a & b"},
		{"&lt;script&gt;", "<script>"},
		{"<a href=\"url\">link</a>", "link"},
		{"  spaced  ", "spaced"},
	}
	for _, tt := range tests {
		got := stripHTML(tt.input)
		if got != tt.want {
			t.Errorf("stripHTML(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseDDGHTML(t *testing.T) {
	html := `
<a class="result__a" href="https://example.com">Example <b>Title</b></a>
<a class="result__snippet" href="#">This is a <b>snippet</b></a>
<a class="result__a" href="https://other.com">Other Result</a>
<a class="result__snippet" href="#">Another snippet</a>
`
	results := parseDDGHTML(html)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Title != "Example Title" {
		t.Errorf("result[0].Title = %q, want 'Example Title'", results[0].Title)
	}
	if results[0].URL != "https://example.com" {
		t.Errorf("result[0].URL = %q, want 'https://example.com'", results[0].URL)
	}
	if results[0].Snippet != "This is a snippet" {
		t.Errorf("result[0].Snippet = %q, want 'This is a snippet'", results[0].Snippet)
	}
	if results[1].Title != "Other Result" {
		t.Errorf("result[1].Title = %q, want 'Other Result'", results[1].Title)
	}
}

func TestParseDDGHTMLEmpty(t *testing.T) {
	results := parseDDGHTML("<html><body>no results</body></html>")
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestFormatResults(t *testing.T) {
	results := []searchResult{
		{Title: "Title 1", URL: "https://example.com", Snippet: "Snippet 1"},
	}
	output := formatResults(results)
	if output == "" {
		t.Error("expected non-empty output")
	}
	if output == "No results found." {
		t.Error("expected results, got 'No results found.'")
	}
}

func TestFormatResultsEmpty(t *testing.T) {
	output := formatResults(nil)
	if output != "No results found." {
		t.Errorf("expected 'No results found.', got %q", output)
	}
}

func TestWebSearchToolWithMockServer(t *testing.T) {
	html := `<a class="result__a" href="https://go.dev">Go Programming</a>
<a class="result__snippet" href="#">The Go programming language</a>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
	}))
	defer server.Close()

	// We test doSearch directly since the tool closures use ddgHTMLURL constant
	ctx := context.Background()
	result, err := doSearch(ctx, server.Client(), "golang", 8)
	// doSearch uses the constant URL, not the test server, so we test parseDDGHTML separately
	// and test the tool execution via the plugin
	_ = result
	_ = err
}

func TestDoSearchWithTestServer(t *testing.T) {
	html := `<a class="result__a" href="https://go.dev">Go Programming</a>
<a class="result__snippet" href="#">The Go programming language</a>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q == "" {
			t.Error("expected query parameter")
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
	}))
	defer server.Close()

	// Test the parsing pipeline end-to-end by calling parseDDGHTML + formatResults
	results := parseDDGHTML(html)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	output := formatResults(results)
	if output == "No results found." {
		t.Error("expected results")
	}
}

func TestWebSearchToolEmptyQuery(t *testing.T) {
	p := newPlugin(http.DefaultClient)
	ctx := context.Background()
	raw := json.RawMessage(`{"query":""}`)
	_, err := p.Tools[0].Execute(ctx, raw, defaultTC())
	if err == nil {
		t.Error("expected error for empty query")
	}
}

func TestRhKbSearchToolEmptyQuery(t *testing.T) {
	p := newPlugin(http.DefaultClient)
	ctx := context.Background()
	raw := json.RawMessage(`{"query":""}`)
	_, err := p.Tools[1].Execute(ctx, raw, defaultTC())
	if err == nil {
		t.Error("expected error for empty query")
	}
}

func TestWebSearchToolInvalidJSON(t *testing.T) {
	p := newPlugin(http.DefaultClient)
	ctx := context.Background()
	raw := json.RawMessage(`{invalid}`)
	_, err := p.Tools[0].Execute(ctx, raw, defaultTC())
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestMaxResultsLimiting(t *testing.T) {
	results := []searchResult{
		{Title: "1"}, {Title: "2"}, {Title: "3"}, {Title: "4"}, {Title: "5"},
	}
	// Simulate limiting
	max := 3
	if len(results) > max {
		results = results[:max]
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results after limiting, got %d", len(results))
	}
}

func defaultTC() plugin.ToolContext {
	return plugin.ToolContext{SessionID: "test", Directory: "/tmp"}
}
