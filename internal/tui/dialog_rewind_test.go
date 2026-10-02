package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestExtractTurns_NoMessages(t *testing.T) {
	turns := ExtractTurns(nil)
	if len(turns) != 0 {
		t.Fatalf("expected 0 turns, got %d", len(turns))
	}
}

func TestExtractTurns_SingleUserTurn(t *testing.T) {
	messages := []MessageView{
		{Info: MessageInfo{ID: "m1", Role: "user"}, Parts: []PartView{{Type: "text", Text: "Hello"}}},
		{Info: MessageInfo{ID: "m2", Role: "assistant"}, Parts: []PartView{{Type: "text", Text: "Hi"}}},
	}
	turns := ExtractTurns(messages)
	// Single turn excluded (can't rewind to current)
	if len(turns) != 0 {
		t.Fatalf("expected 0 turns (single turn excluded), got %d", len(turns))
	}
}

func TestExtractTurns_MultipleUserTurns(t *testing.T) {
	messages := []MessageView{
		{Info: MessageInfo{ID: "m1", Role: "user"}, Parts: []PartView{{Type: "text", Text: "First question"}}},
		{Info: MessageInfo{ID: "m2", Role: "assistant"}, Parts: []PartView{{Type: "text", Text: "First answer"}}},
		{Info: MessageInfo{ID: "m3", Role: "user"}, Parts: []PartView{{Type: "text", Text: "Second question"}}},
		{Info: MessageInfo{ID: "m4", Role: "assistant"}, Parts: []PartView{{Type: "text", Text: "Second answer"}}},
		{Info: MessageInfo{ID: "m5", Role: "user"}, Parts: []PartView{{Type: "text", Text: "Third question"}}},
		{Info: MessageInfo{ID: "m6", Role: "assistant"}, Parts: []PartView{{Type: "text", Text: "Third answer"}}},
	}
	turns := ExtractTurns(messages)
	// 3 user turns, exclude the latest = 2 turns
	if len(turns) != 2 {
		t.Fatalf("expected 2 turns, got %d", len(turns))
	}
	// Most recent first
	if turns[0].Index != 2 {
		t.Errorf("turns[0].Index = %d, want 2", turns[0].Index)
	}
	if turns[0].MessageID != "m3" {
		t.Errorf("turns[0].MessageID = %q, want %q", turns[0].MessageID, "m3")
	}
	if turns[0].Preview != "Second question" {
		t.Errorf("turns[0].Preview = %q, want %q", turns[0].Preview, "Second question")
	}
	if turns[1].Index != 1 {
		t.Errorf("turns[1].Index = %d, want 1", turns[1].Index)
	}
	if turns[1].MessageID != "m1" {
		t.Errorf("turns[1].MessageID = %q, want %q", turns[1].MessageID, "m1")
	}
}

func TestExtractTurns_SkipsSystemAndToolMessages(t *testing.T) {
	messages := []MessageView{
		{Info: MessageInfo{ID: "s1", Role: "system"}, Parts: []PartView{{Type: "text", Text: "System msg"}}},
		{Info: MessageInfo{ID: "m1", Role: "user"}, Parts: []PartView{{Type: "text", Text: "Q1"}}},
		{Info: MessageInfo{ID: "m2", Role: "assistant"}, Parts: []PartView{{Type: "text", Text: "A1"}}},
		{Info: MessageInfo{ID: "t1", Role: "tool"}, Parts: []PartView{{Type: "tool-result", Text: "result"}}},
		{Info: MessageInfo{ID: "m3", Role: "user"}, Parts: []PartView{{Type: "text", Text: "Q2"}}},
		{Info: MessageInfo{ID: "m4", Role: "assistant"}, Parts: []PartView{{Type: "text", Text: "A2"}}},
	}
	turns := ExtractTurns(messages)
	if len(turns) != 1 {
		t.Fatalf("expected 1 turn (latest excluded), got %d", len(turns))
	}
	if turns[0].MessageID != "m1" {
		t.Errorf("turns[0].MessageID = %q, want %q", turns[0].MessageID, "m1")
	}
}

func TestExtractTurns_TruncatesLongPreview(t *testing.T) {
	longText := "This is a very long message that should be truncated to fit within the preview limit of eighty characters total"
	messages := []MessageView{
		{Info: MessageInfo{ID: "m1", Role: "user"}, Parts: []PartView{{Type: "text", Text: longText}}},
		{Info: MessageInfo{ID: "m2", Role: "assistant"}, Parts: []PartView{{Type: "text", Text: "A1"}}},
		{Info: MessageInfo{ID: "m3", Role: "user"}, Parts: []PartView{{Type: "text", Text: "Short"}}},
		{Info: MessageInfo{ID: "m4", Role: "assistant"}, Parts: []PartView{{Type: "text", Text: "A2"}}},
	}
	turns := ExtractTurns(messages)
	if len(turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(turns))
	}
	if len(turns[0].Preview) > 80 {
		t.Errorf("preview too long: %d chars", len(turns[0].Preview))
	}
}

func TestRewindDialog_Navigation(t *testing.T) {
	dlg := NewRewindDialog()
	turns := []RewindTurn{
		{Index: 3, MessageID: "m5", Preview: "Third"},
		{Index: 2, MessageID: "m3", Preview: "Second"},
		{Index: 1, MessageID: "m1", Preview: "First"},
	}
	dlg.Show(turns)

	if !dlg.IsVisible() {
		t.Fatal("dialog should be visible after Show")
	}
	if dlg.selected != 0 {
		t.Fatalf("selected should start at 0, got %d", dlg.selected)
	}

	// Navigate down
	dlg, _ = dlg.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if dlg.selected != 1 {
		t.Fatalf("selected should be 1 after j, got %d", dlg.selected)
	}

	// Navigate up
	dlg, _ = dlg.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if dlg.selected != 0 {
		t.Fatalf("selected should be 0 after k, got %d", dlg.selected)
	}

	// Can't go above 0
	dlg, _ = dlg.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if dlg.selected != 0 {
		t.Fatalf("selected should remain 0, got %d", dlg.selected)
	}
}

func TestRewindDialog_Enter(t *testing.T) {
	dlg := NewRewindDialog()
	turns := []RewindTurn{
		{Index: 2, MessageID: "m3", Preview: "Second"},
		{Index: 1, MessageID: "m1", Preview: "First"},
	}
	dlg.Show(turns)

	var resultCmd tea.Cmd
	dlg, resultCmd = dlg.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if dlg.IsVisible() {
		t.Fatal("dialog should be hidden after enter")
	}
	if resultCmd == nil {
		t.Fatal("expected a command from enter")
	}

	msg := resultCmd()
	selected, ok := msg.(RewindSelectedMsg)
	if !ok {
		t.Fatalf("expected RewindSelectedMsg, got %T", msg)
	}
	if selected.Turn.MessageID != "m3" {
		t.Errorf("selected turn messageID = %q, want %q", selected.Turn.MessageID, "m3")
	}
}

func TestRewindDialog_Escape(t *testing.T) {
	dlg := NewRewindDialog()
	dlg.Show([]RewindTurn{{Index: 1, MessageID: "m1", Preview: "First"}})

	dlg, cmd := dlg.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if dlg.IsVisible() {
		t.Fatal("dialog should be hidden after esc")
	}
	if cmd != nil {
		t.Fatal("escape should not emit a command")
	}
}

func TestRewindDialog_InvisibleIgnoresKeys(t *testing.T) {
	dlg := NewRewindDialog()
	// Not visible — should not react to keys.
	dlg, cmd := dlg.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("invisible dialog should not emit a command")
	}
}
