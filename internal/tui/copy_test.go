package tui

import "testing"

func TestNthAssistantText(t *testing.T) {
	messages := []MessageView{
		{Info: MessageInfo{Role: "assistant"}, Parts: []PartView{{Type: "text", Text: "first"}}},
		{Info: MessageInfo{Role: "user"}, Parts: []PartView{{Type: "text", Text: "question"}}},
		{Info: MessageInfo{Role: "assistant"}, Parts: []PartView{{Type: "text", Text: "second"}}},
		{Info: MessageInfo{Role: "user"}, Parts: []PartView{{Type: "text", Text: "another"}}},
		{Info: MessageInfo{Role: "assistant"}, Parts: []PartView{{Type: "text", Text: "third"}}},
	}

	tests := []struct {
		name string
		n    int
		want string
	}{
		{name: "n=1 returns last", n: 1, want: "third"},
		{name: "n=2 returns second-to-last", n: 2, want: "second"},
		{name: "n=3 returns third-to-last", n: 3, want: "first"},
		{name: "n=4 out of range", n: 4, want: ""},
		{name: "n=0 defaults to 1", n: 0, want: "third"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nthAssistantText(messages, tt.n)
			if got != tt.want {
				t.Errorf("nthAssistantText(_, %d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}

func TestNthAssistantTextSkipsToolOnly(t *testing.T) {
	messages := []MessageView{
		{Info: MessageInfo{Role: "assistant"}, Parts: []PartView{{Type: "text", Text: "has text"}}},
		{Info: MessageInfo{Role: "assistant"}, Parts: []PartView{{Type: "tool-call", ToolName: "bash"}}},
	}

	got := nthAssistantText(messages, 1)
	if got != "has text" {
		t.Errorf("nthAssistantText() = %q, want %q", got, "has text")
	}
}

func TestNthAssistantTextEmpty(t *testing.T) {
	got := nthAssistantText(nil, 1)
	if got != "" {
		t.Errorf("nthAssistantText(nil, 1) = %q, want empty", got)
	}
}

func TestLastAssistantTextPreserved(t *testing.T) {
	messages := []MessageView{
		{Info: MessageInfo{Role: "assistant"}, Parts: []PartView{{Type: "text", Text: "first"}}},
		{Info: MessageInfo{Role: "user"}, Parts: []PartView{{Type: "text", Text: "q"}}},
		{Info: MessageInfo{Role: "assistant"}, Parts: []PartView{{Type: "text", Text: "last"}}},
	}

	got := lastAssistantText(messages)
	if got != "last" {
		t.Errorf("lastAssistantText() = %q, want %q", got, "last")
	}
}

func TestExtractCodeBlocks(t *testing.T) {
	text := "Here is some code:\n```go\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n```\n\nAnd more:\n```python\nprint(\"world\")\n```\n"

	blocks := extractCodeBlocks(text)
	if len(blocks) != 2 {
		t.Fatalf("extractCodeBlocks() returned %d blocks, want 2", len(blocks))
	}

	if blocks[0].Language != "go" {
		t.Errorf("block[0].Language = %q, want %q", blocks[0].Language, "go")
	}
	if blocks[0].Code != "func main() {\n\tfmt.Println(\"hello\")\n}" {
		t.Errorf("block[0].Code = %q", blocks[0].Code)
	}
	if blocks[0].Preview != "func main() {" {
		t.Errorf("block[0].Preview = %q, want %q", blocks[0].Preview, "func main() {")
	}

	if blocks[1].Language != "python" {
		t.Errorf("block[1].Language = %q, want %q", blocks[1].Language, "python")
	}
	if blocks[1].Code != "print(\"world\")" {
		t.Errorf("block[1].Code = %q", blocks[1].Code)
	}
}

func TestExtractCodeBlocksNoLanguage(t *testing.T) {
	text := "```\nno language\n```\n"

	blocks := extractCodeBlocks(text)
	if len(blocks) != 1 {
		t.Fatalf("extractCodeBlocks() returned %d blocks, want 1", len(blocks))
	}
	if blocks[0].Language != "" {
		t.Errorf("block.Language = %q, want empty", blocks[0].Language)
	}
	if blocks[0].Code != "no language" {
		t.Errorf("block.Code = %q, want %q", blocks[0].Code, "no language")
	}
}

func TestExtractCodeBlocksNone(t *testing.T) {
	blocks := extractCodeBlocks("plain text with no code blocks")
	if len(blocks) != 0 {
		t.Errorf("extractCodeBlocks() returned %d blocks, want 0", len(blocks))
	}
}

func TestFirstNonEmptyLine(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "simple", input: "hello", want: "hello"},
		{name: "leading blank", input: "\n  \nhello\nworld", want: "hello"},
		{name: "empty", input: "", want: ""},
		{name: "long line", input: "a" + string(make([]byte, 100)), want: "a" + string(make([]byte, 56)) + "..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := firstNonEmptyLine(tt.input)
			if got != tt.want {
				t.Errorf("firstNonEmptyLine() = %q, want %q", got, tt.want)
			}
		})
	}
}
