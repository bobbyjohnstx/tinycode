package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestAnalyzeContext_EmptyMessages(t *testing.T) {
	b := AnalyzeContext(nil, 0, 0)
	if b.TotalTokens != 0 {
		t.Errorf("expected 0 total tokens, got %d", b.TotalTokens)
	}
	if b.Percent != 0 {
		t.Errorf("expected 0 percent, got %d", b.Percent)
	}
	if b.Suggestion != "" {
		t.Errorf("expected no suggestion, got %q", b.Suggestion)
	}
}

func TestAnalyzeContext_UserAssistantCategorization(t *testing.T) {
	msgs := []MessageView{
		{
			Info:  MessageInfo{Role: "user"},
			Parts: []PartView{{Type: "text", Text: "hello world"}}, // 11 chars -> 3 tokens
		},
		{
			Info:  MessageInfo{Role: "assistant"},
			Parts: []PartView{{Type: "text", Text: "hi there buddy"}}, // 14 chars -> 4 tokens
		},
	}
	b := AnalyzeContext(msgs, 1000, 0)
	if b.UserTokens != 3 {
		t.Errorf("expected 3 user tokens, got %d", b.UserTokens)
	}
	if b.AssistantTokens != 4 {
		t.Errorf("expected 4 assistant tokens, got %d", b.AssistantTokens)
	}
}

func TestAnalyzeContext_TokenEstimation(t *testing.T) {
	// 0.25 tok/char => 100 chars = 25 tokens
	text := make([]byte, 100)
	for i := range text {
		text[i] = 'a'
	}
	msgs := []MessageView{
		{
			Info:  MessageInfo{Role: "user"},
			Parts: []PartView{{Type: "text", Text: string(text)}},
		},
	}
	b := AnalyzeContext(msgs, 0, 0)
	if b.UserTokens != 25 {
		t.Errorf("expected 25 tokens for 100 chars, got %d", b.UserTokens)
	}
	if b.TotalTokens != 25 {
		t.Errorf("expected 25 total tokens, got %d", b.TotalTokens)
	}
}

func TestAnalyzeContext_PercentCalculation(t *testing.T) {
	// 400 chars = 100 tokens, context limit 200 => 50%
	text := make([]byte, 400)
	for i := range text {
		text[i] = 'x'
	}
	msgs := []MessageView{
		{
			Info:  MessageInfo{Role: "user"},
			Parts: []PartView{{Type: "text", Text: string(text)}},
		},
	}
	b := AnalyzeContext(msgs, 200, 0)
	if b.Percent != 50 {
		t.Errorf("expected 50%%, got %d%%", b.Percent)
	}
}

func TestAnalyzeContext_SuggestionOver70Percent(t *testing.T) {
	// 400 chars = 100 tokens, context limit 120 => 83%
	text := make([]byte, 400)
	for i := range text {
		text[i] = 'x'
	}
	msgs := []MessageView{
		{
			Info:  MessageInfo{Role: "user"},
			Parts: []PartView{{Type: "text", Text: string(text)}},
		},
	}
	b := AnalyzeContext(msgs, 120, 0)
	if b.Suggestion == "" {
		t.Error("expected suggestion when usage > 70%")
	}
}

func TestAnalyzeContext_NoSuggestionUnder70Percent(t *testing.T) {
	text := make([]byte, 40)
	for i := range text {
		text[i] = 'x'
	}
	msgs := []MessageView{
		{
			Info:  MessageInfo{Role: "user"},
			Parts: []PartView{{Type: "text", Text: string(text)}},
		},
	}
	b := AnalyzeContext(msgs, 1000, 0)
	if b.Suggestion != "" {
		t.Errorf("expected no suggestion at low usage, got %q", b.Suggestion)
	}
}

func TestAnalyzeContext_APITokensOverrideEstimate(t *testing.T) {
	msgs := []MessageView{
		{
			Info:  MessageInfo{Role: "user"},
			Parts: []PartView{{Type: "text", Text: "hello"}},
		},
	}
	b := AnalyzeContext(msgs, 1000, 500)
	if b.TotalTokens != 500 {
		t.Errorf("expected 500 total tokens from API, got %d", b.TotalTokens)
	}
}

func TestContextDialogOpenClose(t *testing.T) {
	d := NewContextDialog()

	if d.IsVisible() {
		t.Fatal("expected dialog hidden initially")
	}

	d.Show(ContextBreakdown{TotalTokens: 100, ContextLimit: 1000, Percent: 10})
	if !d.IsVisible() {
		t.Fatal("expected dialog visible after Show")
	}

	// ESC closes.
	d2, _ := d.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if d2.IsVisible() {
		t.Error("expected dialog to close on esc")
	}

	// q closes.
	d.Show(ContextBreakdown{})
	d3, _ := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if d3.IsVisible() {
		t.Error("expected dialog to close on q")
	}
}

func TestFormatTokenCount(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1,000"},
		{12450, "12,450"},
		{100000, "100,000"},
		{1234567, "1,234,567"},
	}
	for _, tc := range tests {
		got := formatTokenCount(tc.input)
		if got != tc.expected {
			t.Errorf("formatTokenCount(%d) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}
