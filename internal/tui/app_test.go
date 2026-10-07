package tui

import (
	"io"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
)

func readOutput(t *testing.T, tm *teatest.TestModel) []byte {
	t.Helper()
	tm.Quit()
	r := tm.FinalOutput(t, teatest.WithFinalTimeout(3*time.Second))
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestWelcomeScreen(t *testing.T) {
	app := NewApp("http://localhost:4096")
	tm := teatest.NewTestModel(t, app, teatest.WithInitialTermSize(100, 30))

	time.Sleep(100 * time.Millisecond)

	out := readOutput(t, tm)
	s := string(out)

	if !contains(out, "Loading configuration") {
		t.Error("expected boot check 'Loading configuration' in welcome screen")
	}
	if !contains(out, "Ask anything") {
		t.Error("expected placeholder text in prompt")
	}
	if !contains(out, "build") {
		t.Error("expected 'build' agent label in status bar")
	}
	if !contains(out, "tab agents") {
		t.Error("expected hints in status bar")
	}
	if contains(out, "rgb:") || contains(out, "]11;") {
		t.Errorf("escape sequence leaked into output:\n%s", s)
	}
}

func TestPromptWithSlashInput(t *testing.T) {
	app := NewApp("http://localhost:4096")
	tm := teatest.NewTestModel(t, app, teatest.WithInitialTermSize(100, 30))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})

	time.Sleep(100 * time.Millisecond)

	out := readOutput(t, tm)
	// Don't use golden comparison — the welcome tip is randomized.
	// Just verify /ex appears in the output and no escape garbage.
	if !contains(out, "/ex") {
		t.Errorf("expected /ex in output, got:\n%s", string(out))
	}
	if contains(out, "rgb:") || contains(out, "]11;") {
		t.Errorf("escape sequence leaked into output:\n%s", string(out))
	}
}

func contains(b []byte, s string) bool {
	return len(b) > 0 && len(s) > 0 && indexOf(b, []byte(s)) >= 0
}

func indexOf(b, sub []byte) int {
	for i := 0; i <= len(b)-len(sub); i++ {
		match := true
		for j := range sub {
			if b[i+j] != sub[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func TestLastAssistantText(t *testing.T) {
	tests := []struct {
		name     string
		messages []MessageView
		want     string
	}{
		{
			name:     "no messages",
			messages: nil,
			want:     "",
		},
		{
			name: "no assistant messages",
			messages: []MessageView{
				{Info: MessageInfo{Role: "user"}, Parts: []PartView{{Type: "text", Text: "hello"}}},
			},
			want: "",
		},
		{
			name: "single assistant message",
			messages: []MessageView{
				{Info: MessageInfo{Role: "user"}, Parts: []PartView{{Type: "text", Text: "hello"}}},
				{Info: MessageInfo{Role: "assistant"}, Parts: []PartView{{Type: "text", Text: "world"}}},
			},
			want: "world",
		},
		{
			name: "multiple assistant messages returns last",
			messages: []MessageView{
				{Info: MessageInfo{Role: "assistant"}, Parts: []PartView{{Type: "text", Text: "first"}}},
				{Info: MessageInfo{Role: "user"}, Parts: []PartView{{Type: "text", Text: "question"}}},
				{Info: MessageInfo{Role: "assistant"}, Parts: []PartView{{Type: "text", Text: "second"}}},
			},
			want: "second",
		},
		{
			name: "skips tool parts",
			messages: []MessageView{
				{Info: MessageInfo{Role: "assistant"}, Parts: []PartView{
					{Type: "text", Text: "response text"},
					{Type: "tool-call", ToolName: "bash", ToolArgs: "ls"},
					{Type: "text", Text: "more text"},
				}},
			},
			want: "response text\nmore text",
		},
		{
			name: "skips assistant with only tool parts",
			messages: []MessageView{
				{Info: MessageInfo{Role: "assistant"}, Parts: []PartView{{Type: "text", Text: "has text"}}},
				{Info: MessageInfo{Role: "assistant"}, Parts: []PartView{{Type: "tool-call", ToolName: "bash"}}},
			},
			want: "has text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lastAssistantText(tt.messages)
			if got != tt.want {
				t.Errorf("lastAssistantText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPromptInput(t *testing.T) {
	app := NewApp("http://localhost:4096")
	tm := teatest.NewTestModel(t, app, teatest.WithInitialTermSize(100, 30))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("H")})
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})

	time.Sleep(100 * time.Millisecond)

	out := readOutput(t, tm)
	if !contains(out, "Hello") {
		t.Errorf("expected 'Hello' in prompt, got:\n%s", string(out))
	}
	if contains(out, "rgb:") || contains(out, "]11;") {
		t.Errorf("escape sequence leaked into output:\n%s", string(out))
	}
}
