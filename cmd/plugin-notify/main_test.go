package main

import (
	"encoding/json"
	"testing"
)

func TestPluginID(t *testing.T) {
	p := newPlugin()
	if p.ID != "notify" {
		t.Errorf("expected plugin ID 'notify', got %q", p.ID)
	}
}

func TestToolDefinition(t *testing.T) {
	p := newPlugin()
	if len(p.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(p.Tools))
	}

	tool := p.Tools[0]
	if tool.Name != "notify" {
		t.Errorf("expected tool name 'notify', got %q", tool.Name)
	}
	if tool.Description == "" {
		t.Error("expected non-empty tool description")
	}
	if tool.Execute == nil {
		t.Error("expected non-nil Execute function")
	}
}

func TestToolSchema(t *testing.T) {
	p := newPlugin()
	params := p.Tools[0].Parameters

	typ, ok := params["type"]
	if !ok || typ != "object" {
		t.Errorf("expected type 'object', got %v", typ)
	}

	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties to be map[string]any")
	}

	for _, field := range []string{"title", "message"} {
		prop, ok := props[field].(map[string]any)
		if !ok {
			t.Errorf("expected property %q to be map[string]any", field)
			continue
		}
		if prop["type"] != "string" {
			t.Errorf("expected %q type 'string', got %v", field, prop["type"])
		}
	}

	required, ok := params["required"].([]string)
	if !ok {
		t.Fatal("expected required to be []string")
	}
	if len(required) != 2 {
		t.Fatalf("expected 2 required fields, got %d", len(required))
	}
	reqSet := map[string]bool{required[0]: true, required[1]: true}
	if !reqSet["title"] || !reqSet["message"] {
		t.Errorf("expected required [title, message], got %v", required)
	}
}

func TestArgsUnmarshal(t *testing.T) {
	raw := json.RawMessage(`{"title":"Build Complete","message":"All tests passed"}`)
	var args notifyArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if args.Title != "Build Complete" {
		t.Errorf("expected title 'Build Complete', got %q", args.Title)
	}
	if args.Message != "All tests passed" {
		t.Errorf("expected message 'All tests passed', got %q", args.Message)
	}
}

func TestEscapeAppleScript(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`hello`, `"hello"`},
		{`say "hi"`, `"say \"hi\""`},
		{`back\slash`, `"back\\slash"`},
		{`both "and" \ here`, `"both \"and\" \\ here"`},
	}
	for _, tt := range tests {
		got := escapeAppleScript(tt.input)
		if got != tt.want {
			t.Errorf("escapeAppleScript(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
