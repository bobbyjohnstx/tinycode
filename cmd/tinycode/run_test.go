package main

import (
	"bufio"
	"strings"
	"testing"
)

func TestReadNextPrompt_TextMode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantText string
		wantOK   bool
	}{
		{
			name:     "returns trimmed line",
			input:    "  hello world  \n",
			wantText: "hello world",
			wantOK:   true,
		},
		{
			name:     "returns empty on EOF",
			input:    "",
			wantText: "",
			wantOK:   false,
		},
		{
			name:     "returns empty string for blank line",
			input:    "   \n",
			wantText: "",
			wantOK:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scanner := bufio.NewScanner(strings.NewReader(tt.input))
			text, ok := readNextPrompt(scanner, false)
			if text != tt.wantText {
				t.Errorf("text = %q, want %q", text, tt.wantText)
			}
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
		})
	}
}

func TestReadNextPrompt_JSONMode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantText string
		wantOK   bool
	}{
		{
			name:     "parses prompt type",
			input:    `{"type":"prompt","text":"fix the bug"}` + "\n",
			wantText: "fix the bug",
			wantOK:   true,
		},
		{
			name:     "exits on exit type",
			input:    `{"type":"exit"}` + "\n",
			wantText: "",
			wantOK:   false,
		},
		{
			name:     "skips permission_reply type",
			input:    `{"type":"permission_reply","id":"123","allow":true}` + "\n",
			wantText: "",
			wantOK:   true,
		},
		{
			name:     "skips malformed JSON",
			input:    "not json\n",
			wantText: "",
			wantOK:   true,
		},
		{
			name:     "skips unknown type",
			input:    `{"type":"unknown","data":"stuff"}` + "\n",
			wantText: "",
			wantOK:   true,
		},
		{
			name:     "returns false on EOF",
			input:    "",
			wantText: "",
			wantOK:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scanner := bufio.NewScanner(strings.NewReader(tt.input))
			text, ok := readNextPrompt(scanner, true)
			if text != tt.wantText {
				t.Errorf("text = %q, want %q", text, tt.wantText)
			}
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
		})
	}
}

func TestReadNextPrompt_TextMultiLine(t *testing.T) {
	input := "first prompt\nsecond prompt\n"
	scanner := bufio.NewScanner(strings.NewReader(input))

	text, ok := readNextPrompt(scanner, false)
	if text != "first prompt" || !ok {
		t.Errorf("first: text=%q ok=%v, want %q %v", text, ok, "first prompt", true)
	}

	text, ok = readNextPrompt(scanner, false)
	if text != "second prompt" || !ok {
		t.Errorf("second: text=%q ok=%v, want %q %v", text, ok, "second prompt", true)
	}

	text, ok = readNextPrompt(scanner, false)
	if text != "" || ok {
		t.Errorf("EOF: text=%q ok=%v, want %q %v", text, ok, "", false)
	}
}

func TestReadNextPrompt_JSONMultiLine(t *testing.T) {
	input := `{"type":"prompt","text":"turn 1"}` + "\n" +
		`{"type":"permission_reply","id":"x"}` + "\n" +
		`{"type":"prompt","text":"turn 2"}` + "\n" +
		`{"type":"exit"}` + "\n"
	scanner := bufio.NewScanner(strings.NewReader(input))

	text, ok := readNextPrompt(scanner, true)
	if text != "turn 1" || !ok {
		t.Errorf("turn 1: text=%q ok=%v", text, ok)
	}

	// permission_reply should be skipped
	text, ok = readNextPrompt(scanner, true)
	if text != "" || !ok {
		t.Errorf("permission_reply: text=%q ok=%v, want empty+true", text, ok)
	}

	text, ok = readNextPrompt(scanner, true)
	if text != "turn 2" || !ok {
		t.Errorf("turn 2: text=%q ok=%v", text, ok)
	}

	// exit should stop
	text, ok = readNextPrompt(scanner, true)
	if text != "" || ok {
		t.Errorf("exit: text=%q ok=%v, want empty+false", text, ok)
	}
}
