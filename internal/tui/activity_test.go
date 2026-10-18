package tui

import "testing"

func TestFirstActivityLine_SingleLine(t *testing.T) {
	if got := firstActivityLine("hello world"); got != "hello world" {
		t.Errorf("got %q, want %q", got, "hello world")
	}
}

func TestFirstActivityLine_MultiLine(t *testing.T) {
	if got := firstActivityLine("first\nsecond\nthird"); got != "first" {
		t.Errorf("got %q, want %q", got, "first")
	}
}

func TestFirstActivityLine_Empty(t *testing.T) {
	if got := firstActivityLine(""); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestFirstActivityLine_TrailingNewline(t *testing.T) {
	if got := firstActivityLine("only\n"); got != "only" {
		t.Errorf("got %q, want %q", got, "only")
	}
}

func TestTruncateActivity_Short(t *testing.T) {
	if got := truncateActivity("hi", 10); got != "hi" {
		t.Errorf("got %q, want %q", got, "hi")
	}
}

func TestTruncateActivity_Exact(t *testing.T) {
	if got := truncateActivity("abcde", 5); got != "abcde" {
		t.Errorf("got %q, want %q", got, "abcde")
	}
}

func TestTruncateActivity_Truncated(t *testing.T) {
	got := truncateActivity("abcdefghij", 5)
	if got != "abcd…" {
		t.Errorf("got %q, want %q", got, "abcd…")
	}
}

func TestTruncateActivity_MinLength(t *testing.T) {
	got := truncateActivity("abcdefgh", 2)
	if got != "abc…" {
		t.Errorf("got %q, want %q (clamped to 4)", got, "abc…")
	}
}

func TestTruncateActivity_Unicode(t *testing.T) {
	got := truncateActivity("日本語テスト", 5)
	if got != "日本語テ…" {
		t.Errorf("got %q, want %q", got, "日本語テ…")
	}
}

func TestFormatActivity_Reasoning(t *testing.T) {
	got := formatActivity(PartView{Type: "reasoning"})
	if got != "thinking" {
		t.Errorf("got %q, want thinking", got)
	}
}

func TestFormatActivity_Thinking(t *testing.T) {
	got := formatActivity(PartView{Type: "thinking"})
	if got != "thinking" {
		t.Errorf("got %q, want thinking", got)
	}
}

func TestFormatActivity_ToolCall(t *testing.T) {
	got := formatActivity(PartView{Type: "tool-call", ToolName: "bash", ToolArgs: `{"command":"echo hi"}`})
	if got != "bash echo hi" {
		t.Errorf("got %q", got)
	}
}

func TestFormatActivity_UnknownType(t *testing.T) {
	got := formatActivity(PartView{Type: "text"})
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestFormatToolActivity_Bash(t *testing.T) {
	got := formatToolActivity("bash", `{"command":"echo hello"}`)
	if got != "bash echo hello" {
		t.Errorf("got %q", got)
	}
}

func TestFormatToolActivity_BashNoCommand(t *testing.T) {
	got := formatToolActivity("bash", `{}`)
	if got != "bash" {
		t.Errorf("got %q, want bash", got)
	}
}

func TestFormatToolActivity_BashMultiline(t *testing.T) {
	got := formatToolActivity("bash", `{"command":"echo first\necho second"}`)
	if got != "bash echo first" {
		t.Errorf("got %q", got)
	}
}

func TestFormatToolActivity_Read(t *testing.T) {
	got := formatToolActivity("read", `{"file_path":"/tmp/project/main.go"}`)
	if got != "read main.go" {
		t.Errorf("got %q", got)
	}
}

func TestFormatToolActivity_Write(t *testing.T) {
	got := formatToolActivity("write", `{"file_path":"/tmp/a.txt"}`)
	if got != "write a.txt" {
		t.Errorf("got %q", got)
	}
}

func TestFormatToolActivity_Edit(t *testing.T) {
	got := formatToolActivity("edit", `{}`)
	if got != "edit" {
		t.Errorf("got %q", got)
	}
}

func TestFormatToolActivity_WebFetch(t *testing.T) {
	got := formatToolActivity("web_fetch", "")
	if got != "webfetch" {
		t.Errorf("got %q", got)
	}
}

func TestFormatToolActivity_WebFetchAlt(t *testing.T) {
	got := formatToolActivity("webfetch", "")
	if got != "webfetch" {
		t.Errorf("got %q", got)
	}
}

func TestFormatToolActivity_EmptyName(t *testing.T) {
	got := formatToolActivity("", "")
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestFormatToolActivity_Unknown(t *testing.T) {
	got := formatToolActivity("custom_tool", "")
	if got != "custom_tool" {
		t.Errorf("got %q", got)
	}
}
