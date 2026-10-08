package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPermissionAction_String_AllVariants(t *testing.T) {
	tests := []struct {
		action PermissionAction
		want   string
	}{
		{PermissionAllow, "once"},
		{PermissionAlways, "always"},
		{PermissionReject, "reject"},
		{PermissionAction(99), "reject"}, // unknown defaults to reject
	}
	for _, tt := range tests {
		got := tt.action.String()
		if got != tt.want {
			t.Errorf("PermissionAction(%d).String() = %q, want %q", int(tt.action), got, tt.want)
		}
	}
}

func TestParseArgs_ValidJSON(t *testing.T) {
	p := PermissionPrompt{
		request: PermissionRequest{
			Metadata: map[string]any{
				"args": `{"command":"ls -la","description":"list files"}`,
			},
		},
	}
	args := p.parseArgs()
	if args == nil {
		t.Fatal("expected non-nil args")
	}
	if cmd, ok := args["command"].(string); !ok || cmd != "ls -la" {
		t.Errorf("expected command 'ls -la', got %v", args["command"])
	}
	if desc, ok := args["description"].(string); !ok || desc != "list files" {
		t.Errorf("expected description 'list files', got %v", args["description"])
	}
}

func TestParseArgs_NilMetadata(t *testing.T) {
	p := PermissionPrompt{
		request: PermissionRequest{Metadata: nil},
	}
	if args := p.parseArgs(); args != nil {
		t.Errorf("expected nil args for nil metadata, got %v", args)
	}
}

func TestParseArgs_InvalidJSON(t *testing.T) {
	p := PermissionPrompt{
		request: PermissionRequest{
			Metadata: map[string]any{
				"args": `{invalid json`,
			},
		},
	}
	if args := p.parseArgs(); args != nil {
		t.Errorf("expected nil args for invalid JSON, got %v", args)
	}
}

func TestParseArgs_EmptyArgsString(t *testing.T) {
	p := PermissionPrompt{
		request: PermissionRequest{
			Metadata: map[string]any{
				"args": "",
			},
		},
	}
	if args := p.parseArgs(); args != nil {
		t.Errorf("expected nil args for empty string, got %v", args)
	}
}

func TestParseArgs_NonStringArgs(t *testing.T) {
	p := PermissionPrompt{
		request: PermissionRequest{
			Metadata: map[string]any{
				"args": 42,
			},
		},
	}
	if args := p.parseArgs(); args != nil {
		t.Errorf("expected nil args for non-string value, got %v", args)
	}
}

func TestArgPath_ExtractsFilePath(t *testing.T) {
	p := PermissionPrompt{}
	args := map[string]any{"file_path": "/some/path/file.go"}
	got := p.argPath(args, "file_path")
	// shortenCwd may transform the path, but it should be non-empty.
	if got == "" {
		t.Error("expected non-empty path from argPath")
	}
}

func TestArgPath_NilArgs(t *testing.T) {
	p := PermissionPrompt{}
	if got := p.argPath(nil, "file_path"); got != "" {
		t.Errorf("expected empty string for nil args, got %q", got)
	}
}

func TestArgPath_MissingKey(t *testing.T) {
	p := PermissionPrompt{}
	args := map[string]any{"other_key": "/some/path"}
	if got := p.argPath(args, "file_path"); got != "" {
		t.Errorf("expected empty string for missing key, got %q", got)
	}
}

func TestArgPath_EmptyValue(t *testing.T) {
	p := PermissionPrompt{}
	args := map[string]any{"file_path": ""}
	if got := p.argPath(args, "file_path"); got != "" {
		t.Errorf("expected empty string for empty value, got %q", got)
	}
}

func TestToolDescription_ShellCommand(t *testing.T) {
	p := PermissionPrompt{
		request: PermissionRequest{
			Permission: "shell",
			Metadata: map[string]any{
				"args": `{"command":"git status","description":"check git status"}`,
			},
		},
	}
	icon, title, body := p.toolDescription()
	if icon != "#" {
		t.Errorf("expected icon '#', got %q", icon)
	}
	if title != "check git status" {
		t.Errorf("expected title 'check git status', got %q", title)
	}
	if !strings.Contains(body, "$ git status") {
		t.Errorf("expected body to contain '$ git status', got %q", body)
	}
}

func TestToolDescription_ShellNoDescription(t *testing.T) {
	p := PermissionPrompt{
		request: PermissionRequest{
			Permission: "shell",
			Metadata: map[string]any{
				"args": `{"command":"ls -la"}`,
			},
		},
	}
	_, title, body := p.toolDescription()
	if title != "Shell command" {
		t.Errorf("expected title 'Shell command', got %q", title)
	}
	if !strings.Contains(body, "$ ls -la") {
		t.Errorf("expected body to contain '$ ls -la', got %q", body)
	}
}

func TestToolDescription_DestructiveShell(t *testing.T) {
	p := PermissionPrompt{
		request: PermissionRequest{
			Permission: "destructive-shell",
			Metadata: map[string]any{
				"args": `{"command":"rm -rf /tmp/test"}`,
			},
		},
	}
	icon, _, body := p.toolDescription()
	if icon != "#" {
		t.Errorf("expected icon '#', got %q", icon)
	}
	if !strings.Contains(body, "rm -rf") {
		t.Errorf("expected body to contain command, got %q", body)
	}
}

func TestToolDescription_EditFile(t *testing.T) {
	p := PermissionPrompt{
		request: PermissionRequest{
			Permission: "edit",
			Metadata: map[string]any{
				"args": `{"file_path":"/tmp/test.go"}`,
			},
		},
	}
	icon, title, _ := p.toolDescription()
	if icon != "→" {
		t.Errorf("expected icon '→', got %q", icon)
	}
	if !strings.Contains(title, "Edit") {
		t.Errorf("expected title to contain 'Edit', got %q", title)
	}
}

func TestToolDescription_EditNoPath(t *testing.T) {
	p := PermissionPrompt{
		request: PermissionRequest{
			Permission: "edit",
			Metadata:   map[string]any{},
		},
	}
	_, title, _ := p.toolDescription()
	if title != "Edit file" {
		t.Errorf("expected title 'Edit file', got %q", title)
	}
}

func TestToolDescription_ReadFile(t *testing.T) {
	p := PermissionPrompt{
		request: PermissionRequest{
			Permission: "read",
			Metadata: map[string]any{
				"args": `{"file_path":"/tmp/test.go"}`,
			},
		},
	}
	icon, title, _ := p.toolDescription()
	if icon != "→" {
		t.Errorf("expected icon '→', got %q", icon)
	}
	if !strings.Contains(title, "Read") {
		t.Errorf("expected title to contain 'Read', got %q", title)
	}
}

func TestToolDescription_ReadFilePathFallback(t *testing.T) {
	p := PermissionPrompt{
		request: PermissionRequest{
			Permission: "read",
			Metadata: map[string]any{
				"args": `{"filePath":"/tmp/test.go"}`,
			},
		},
	}
	_, title, _ := p.toolDescription()
	if !strings.Contains(title, "Read") || title == "Read file" {
		t.Errorf("expected title with path from filePath fallback, got %q", title)
	}
}

func TestToolDescription_ReadNoPath(t *testing.T) {
	p := PermissionPrompt{
		request: PermissionRequest{
			Permission: "read",
			Metadata:   map[string]any{},
		},
	}
	_, title, _ := p.toolDescription()
	if title != "Read file" {
		t.Errorf("expected title 'Read file', got %q", title)
	}
}

func TestToolDescription_Glob(t *testing.T) {
	p := PermissionPrompt{
		request: PermissionRequest{
			Permission: "glob",
			Metadata: map[string]any{
				"args": `{"pattern":"**/*.go"}`,
			},
		},
	}
	icon, title, _ := p.toolDescription()
	if icon != "✱" {
		t.Errorf("expected icon '✱', got %q", icon)
	}
	if title != `Glob "**/*.go"` {
		t.Errorf("expected title with pattern, got %q", title)
	}
}

func TestToolDescription_GlobNoPattern(t *testing.T) {
	p := PermissionPrompt{
		request: PermissionRequest{
			Permission: "glob",
			Metadata:   map[string]any{},
		},
	}
	_, title, _ := p.toolDescription()
	if title != "Glob" {
		t.Errorf("expected title 'Glob', got %q", title)
	}
}

func TestToolDescription_Grep(t *testing.T) {
	p := PermissionPrompt{
		request: PermissionRequest{
			Permission: "grep",
			Metadata: map[string]any{
				"args": `{"pattern":"TODO"}`,
			},
		},
	}
	icon, title, _ := p.toolDescription()
	if icon != "✱" {
		t.Errorf("expected icon '✱', got %q", icon)
	}
	if title != `Grep "TODO"` {
		t.Errorf("expected title with pattern, got %q", title)
	}
}

func TestToolDescription_DefaultWithToolName(t *testing.T) {
	p := PermissionPrompt{
		request: PermissionRequest{
			Permission: "custom",
			Metadata: map[string]any{
				"tool": "my-custom-tool",
				"args": `{"key":"value"}`,
			},
		},
	}
	icon, title, body := p.toolDescription()
	if icon != "⚙" {
		t.Errorf("expected icon '⚙', got %q", icon)
	}
	if title != "Call tool my-custom-tool" {
		t.Errorf("expected title 'Call tool my-custom-tool', got %q", title)
	}
	if body == "" {
		t.Error("expected non-empty body with JSON args")
	}
}

func TestToolDescription_DefaultNoToolNoArgs(t *testing.T) {
	p := PermissionPrompt{
		request: PermissionRequest{
			Permission: "unknown-perm",
			Metadata:   map[string]any{},
		},
	}
	icon, title, body := p.toolDescription()
	if icon != "⚙" {
		t.Errorf("expected icon '⚙', got %q", icon)
	}
	if title != "Permission: unknown-perm" {
		t.Errorf("expected fallback title, got %q", title)
	}
	if body != "" {
		t.Errorf("expected empty body when no args, got %q", body)
	}
}

func TestPermissionPrompt_ShowAndHide(t *testing.T) {
	p := NewPermissionPrompt()
	if p.IsVisible() {
		t.Error("expected prompt to start hidden")
	}
	p.Show(PermissionRequest{ID: "perm-1", Permission: "shell"})
	if !p.IsVisible() {
		t.Error("expected prompt to be visible after Show")
	}
	if p.selected != PermissionAllow {
		t.Errorf("expected selected to reset to PermissionAllow, got %d", p.selected)
	}
	p.Hide()
	if p.IsVisible() {
		t.Error("expected prompt to be hidden after Hide")
	}
}

func TestPermissionPrompt_Update_NavigateRight(t *testing.T) {
	p := NewPermissionPrompt()
	p.Show(PermissionRequest{ID: "perm-1", Permission: "shell"})

	// Start at Allow (0), move right to Always (1)
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if p.selected != PermissionAlways {
		t.Errorf("expected PermissionAlways after right, got %d", p.selected)
	}

	// Move right again to Reject (2)
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if p.selected != PermissionReject {
		t.Errorf("expected PermissionReject after second right, got %d", p.selected)
	}

	// Right at end should stay at Reject
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if p.selected != PermissionReject {
		t.Errorf("expected PermissionReject at boundary, got %d", p.selected)
	}
}

func TestPermissionPrompt_Update_NavigateLeft(t *testing.T) {
	p := NewPermissionPrompt()
	p.Show(PermissionRequest{ID: "perm-1", Permission: "shell"})

	// Left at start should stay at Allow
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	if p.selected != PermissionAllow {
		t.Errorf("expected PermissionAllow at left boundary, got %d", p.selected)
	}

	// Move right then left
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRight})
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if p.selected != PermissionAllow {
		t.Errorf("expected PermissionAllow after right+left, got %d", p.selected)
	}
}

func TestPermissionPrompt_Update_EnterDismisses(t *testing.T) {
	p := NewPermissionPrompt()
	req := PermissionRequest{ID: "perm-1", SessionID: "ses_1", Permission: "shell"}
	p.Show(req)

	// Select "Always" then press Enter
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRight})
	p, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if p.visible {
		t.Error("expected prompt to be hidden after Enter")
	}
	if cmd == nil {
		t.Fatal("expected cmd from Enter")
	}
	msg := cmd()
	dismissed, ok := msg.(PermissionDismissedMsg)
	if !ok {
		t.Fatalf("expected PermissionDismissedMsg, got %T", msg)
	}
	if dismissed.Action != PermissionAlways {
		t.Errorf("expected action PermissionAlways, got %d", dismissed.Action)
	}
	if dismissed.Request.ID != "perm-1" {
		t.Errorf("expected request ID 'perm-1', got %q", dismissed.Request.ID)
	}
}

func TestPermissionPrompt_Update_EscDoesNotDismiss(t *testing.T) {
	p := NewPermissionPrompt()
	p.Show(PermissionRequest{ID: "perm-1", Permission: "shell"})

	p, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !p.visible {
		t.Error("expected prompt to remain visible after Esc")
	}
	if cmd != nil {
		t.Error("expected no cmd from Esc")
	}
}

func TestPermissionPrompt_Update_IgnoresWhenNotVisible(t *testing.T) {
	p := NewPermissionPrompt()
	// Not visible
	p, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Error("expected no cmd when prompt not visible")
	}
}

func TestPermissionPrompt_Update_IgnoresNonKeyMsg(t *testing.T) {
	p := NewPermissionPrompt()
	p.Show(PermissionRequest{ID: "perm-1", Permission: "shell"})

	// Send a non-key message
	p, cmd := p.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if cmd != nil {
		t.Error("expected no cmd for non-key message")
	}
}

func TestPermissionPrompt_View_EmptyWhenHidden(t *testing.T) {
	p := NewPermissionPrompt()
	if v := p.View(); v != "" {
		t.Errorf("expected empty view when hidden, got %q", v)
	}
}

func TestPermissionPrompt_View_ContainsPermissionRequired(t *testing.T) {
	p := NewPermissionPrompt()
	p.SetSize(80, 24)
	p.Show(PermissionRequest{
		Permission: "shell",
		Metadata: map[string]any{
			"args": `{"command":"ls","description":"list files"}`,
		},
	})
	view := p.View()
	if !strings.Contains(view, "Permission required") {
		t.Error("expected 'Permission required' in view")
	}
	if !strings.Contains(view, "Allow once") {
		t.Error("expected 'Allow once' button in view")
	}
	if !strings.Contains(view, "Allow always") {
		t.Error("expected 'Allow always' button in view")
	}
	if !strings.Contains(view, "Reject") {
		t.Error("expected 'Reject' button in view")
	}
}
