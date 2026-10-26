package session

import "testing"

func TestDetectToolIntent(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		// Should trigger — model describing tool actions
		{"I will write the findings to /tmp/review.md", true},
		{"Let me create a file with the results", true},
		{"I'll save the output to disk", true},
		{"I'll create the file now", true},
		{"Writing the file to /tmp/output.txt", true},
		{"I'll edit the configuration file", true},
		{"Let me write the documentation", true},
		{"I will update the file with these changes", true},
		{"I will save the findings to a markdown file", true},
		{"Writing the findings to the report", true},

		// Should NOT trigger — legitimate text responses
		{"The write tool is used for creating files", false},
		{"Here are the findings from the code review", false},
		{"The bug is in the file handler", false},
		{"You can save your work by committing", false},
		{"Creating robust tests requires planning", false},
		{"Writing good code takes practice", false},
		{"", false},
	}
	for _, tt := range tests {
		got := detectToolIntent(tt.text)
		if got != tt.want {
			t.Errorf("detectToolIntent(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}

func TestExtractAssistantText(t *testing.T) {
	msg := &Message{
		Parts: []Part{
			{Type: "text", Text: "Hello "},
			{Type: "reasoning", Text: "thinking..."},
			{Type: "text", Text: "world"},
		},
	}
	got := extractAssistantText(msg)
	if got != "Hello  world " {
		t.Errorf("extractAssistantText = %q, want %q", got, "Hello  world ")
	}

	if extractAssistantText(nil) != "" {
		t.Error("expected empty for nil message")
	}
}
