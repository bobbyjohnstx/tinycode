package plugin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// fakePlugin implements BuiltinPlugin for testing.
type fakePlugin struct {
	id    string
	tools []BuiltinTool
	hooks BuiltinHooks
}

func (f *fakePlugin) ID() string          { return f.id }
func (f *fakePlugin) Tools() []BuiltinTool { return f.tools }
func (f *fakePlugin) Hooks() BuiltinHooks  { return f.hooks }

func TestRegister_AddsPlugin(t *testing.T) {
	m := NewBuiltinManager()
	p := &fakePlugin{
		id: "test-plugin",
		tools: []BuiltinTool{
			{
				Name:        "test-tool",
				Description: "a test tool",
				Parameters:  map[string]any{"type": "object"},
				Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
					return "executed", nil
				},
			},
		},
	}

	m.Register(p)

	if len(m.plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(m.plugins))
	}
	if m.plugins[0].ID() != "test-plugin" {
		t.Errorf("expected plugin id 'test-plugin', got %q", m.plugins[0].ID())
	}
}

func TestCallTool_DispatchesCorrectly(t *testing.T) {
	m := NewBuiltinManager()
	m.Register(&fakePlugin{
		id: "p1",
		tools: []BuiltinTool{
			{
				Name: "greet",
				Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
					var input struct {
						Name string `json:"name"`
					}
					json.Unmarshal(args, &input)
					return "hello " + input.Name, nil
				},
			},
		},
	})

	result, err := m.CallTool(context.Background(), "greet", json.RawMessage(`{"name":"world"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello world" {
		t.Errorf("expected 'hello world', got %q", result)
	}
}

func TestCallTool_UnknownToolErrors(t *testing.T) {
	m := NewBuiltinManager()

	_, err := m.CallTool(context.Background(), "nonexistent", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
	if !strings.Contains(err.Error(), "unknown built-in tool") {
		t.Errorf("expected 'unknown built-in tool' error, got %q", err.Error())
	}
}

func TestCallTool_MultiplePlugins(t *testing.T) {
	m := NewBuiltinManager()
	m.Register(&fakePlugin{
		id: "p1",
		tools: []BuiltinTool{
			{Name: "tool-a", Execute: func(ctx context.Context, args json.RawMessage) (string, error) { return "from-a", nil }},
		},
	})
	m.Register(&fakePlugin{
		id: "p2",
		tools: []BuiltinTool{
			{Name: "tool-b", Execute: func(ctx context.Context, args json.RawMessage) (string, error) { return "from-b", nil }},
		},
	})

	a, err := m.CallTool(context.Background(), "tool-a", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a != "from-a" {
		t.Errorf("expected 'from-a', got %q", a)
	}

	b, err := m.CallTool(context.Background(), "tool-b", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b != "from-b" {
		t.Errorf("expected 'from-b', got %q", b)
	}
}

func TestDispatchHook_CallsMatchingHandlers(t *testing.T) {
	var called []string

	m := NewBuiltinManager()
	m.Register(&fakePlugin{
		id: "p1",
		hooks: BuiltinHooks{
			SessionStart: func(ctx context.Context, sessionID string) error {
				called = append(called, "p1:start:"+sessionID)
				return nil
			},
			SessionEnd: func(ctx context.Context, sessionID string) error {
				called = append(called, "p1:end:"+sessionID)
				return nil
			},
		},
	})
	m.Register(&fakePlugin{
		id: "p2",
		hooks: BuiltinHooks{
			SessionStart: func(ctx context.Context, sessionID string) error {
				called = append(called, "p2:start:"+sessionID)
				return nil
			},
		},
	})

	if err := m.DispatchHook("session.start", "sess-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(called) != 2 {
		t.Fatalf("expected 2 calls, got %d: %v", len(called), called)
	}
	if called[0] != "p1:start:sess-1" || called[1] != "p2:start:sess-1" {
		t.Errorf("unexpected calls: %v", called)
	}

	called = nil
	if err := m.DispatchHook("session.end", "sess-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(called) != 1 || called[0] != "p1:end:sess-1" {
		t.Errorf("expected [p1:end:sess-1], got %v", called)
	}
}

func TestDispatchHook_NoOpForUnregistered(t *testing.T) {
	m := NewBuiltinManager()
	m.Register(&fakePlugin{
		id:    "p1",
		hooks: BuiltinHooks{}, // no hooks registered
	})

	if err := m.DispatchHook("session.start", "sess-1"); err != nil {
		t.Fatalf("expected no error for unhandled hook, got: %v", err)
	}
	if err := m.DispatchHook("session.end", "sess-1"); err != nil {
		t.Fatalf("expected no error for unhandled hook, got: %v", err)
	}
	if err := m.DispatchHook("dispose", nil); err != nil {
		t.Fatalf("expected no error for unhandled hook, got: %v", err)
	}
	if err := m.DispatchHook("unknown.hook", nil); err != nil {
		t.Fatalf("expected no error for unknown hook name, got: %v", err)
	}
}

func TestDispatchHook_Dispose(t *testing.T) {
	disposed := false
	m := NewBuiltinManager()
	m.Register(&fakePlugin{
		id: "p1",
		hooks: BuiltinHooks{
			Dispose: func(ctx context.Context) error {
				disposed = true
				return nil
			},
		},
	})

	if err := m.DispatchHook("dispose", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !disposed {
		t.Error("expected dispose hook to be called")
	}
}
