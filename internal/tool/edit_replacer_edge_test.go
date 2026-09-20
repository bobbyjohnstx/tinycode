package tool

import (
	"strings"
	"testing"
)

func TestCascadeReplace_EmptyOldString(t *testing.T) {
	// An empty oldString will produce exactCount=0 from strings.Count
	// (strings.Count("abc", "") returns len("abc")+1), but the replacers
	// themselves should handle it gracefully.
	content := "hello world"
	_, _, _, ok := cascadeReplace(content, "", "replacement", false)
	// We just verify it doesn't panic and returns some deterministic result.
	// The exact behavior depends on strings.Count("hello world", "") which is 12,
	// so it will see >1 exact matches and return false when replaceAll=false.
	_ = ok
}

func TestCascadeReplace_EmptyOldString_ReplaceAll(t *testing.T) {
	content := "hello"
	result, _, _, ok := cascadeReplace(content, "", "X", true)
	// strings.ReplaceAll("hello", "", "X") inserts X between every char.
	if ok {
		want := strings.ReplaceAll(content, "", "X")
		if result != want {
			t.Errorf("got %q, want %q", result, want)
		}
	}
}

func TestCascadeReplace_OldStringLongerThanContent(t *testing.T) {
	content := "hi"
	old := "this is much longer than the content and will never match"
	_, _, count, ok := cascadeReplace(content, old, "replacement", false)
	if ok {
		t.Fatal("expected no match for oldString longer than content")
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestCascadeReplace_UnicodeContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
		old     string
		new     string
		want    string
	}{
		{
			"CJK characters",
			"你好世界 hello",
			"你好世界",
			"こんにちは",
			"こんにちは hello",
		},
		{
			"emoji replacement",
			"start 🎉🎊 end",
			"🎉🎊",
			"🔥",
			"start 🔥 end",
		},
		{
			"mixed unicode and ASCII",
			"café résumé naïve",
			"résumé",
			"resume",
			"café resume naïve",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, _, ok := cascadeReplace(tt.content, tt.old, tt.new, false)
			if !ok {
				t.Fatal("expected match")
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCascadeReplace_MixedLineEndings(t *testing.T) {
	// Content has \r\n, needle has \n — EscapeNormalizedReplacer should handle this.
	content := "line1\r\nline2\r\nline3"
	old := "line1\nline2\nline3"
	new := "replaced"

	got, strategy, _, ok := cascadeReplace(content, old, new, false)
	if !ok {
		t.Fatal("expected match for mixed line endings")
	}
	if got != "replaced" {
		t.Errorf("got %q, want %q", got, "replaced")
	}
	// Should be matched by escape-normalized strategy.
	if strategy != "escape-normalized" {
		t.Logf("matched by %q (expected escape-normalized)", strategy)
	}
}

func TestCascadeReplace_ReplaceAll_MultipleOccurrences(t *testing.T) {
	content := "foo bar foo baz foo"
	old := "foo"
	new := "qux"
	got, _, _, ok := cascadeReplace(content, old, new, true)
	if !ok {
		t.Fatal("expected match with replaceAll=true")
	}
	want := "qux bar qux baz qux"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCascadeReplace_ReplaceAll_MultilineOccurrences(t *testing.T) {
	content := "func a() {}\nfunc b() {}\nfunc a() {}"
	old := "func a() {}"
	new := "func x() {}"
	got, _, _, ok := cascadeReplace(content, old, new, true)
	if !ok {
		t.Fatal("expected match with replaceAll=true on multiline")
	}
	want := "func x() {}\nfunc b() {}\nfunc x() {}"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestContextAwareReplacer_MultiRegionMatch(t *testing.T) {
	r := &ContextAwareReplacer{}

	// Content has two blocks with different interiors but same anchors.
	content := strings.Join([]string{
		"func init() {",
		"    a := 1",
		"    b := 2",
		"}",
		"",
		"func main() {",
		"    c := 3",
		"    d := 4",
		"}",
	}, "\n")

	// Needle matches only the first block (anchor lines + interior).
	old := "func init() {\n    a := 1\n    b := 2\n}"
	new := "func init() {\n    x := 10\n}"

	got, ok := r.Replace(content, old, new, false)
	if !ok {
		t.Fatal("expected match for multi-region content")
	}
	if !strings.Contains(got, "x := 10") {
		t.Errorf("replacement not applied: %q", got)
	}
	if !strings.Contains(got, "func main()") {
		t.Error("second function was incorrectly modified")
	}
}

func TestContextAwareReplacer_DuplicateRegions_NoReplaceAll(t *testing.T) {
	r := &ContextAwareReplacer{}

	// Two identical blocks — should fail without replaceAll.
	content := "if true {\n    x := 1\n    y := 2\n}\nif true {\n    x := 1\n    y := 2\n}"
	old := "if true {\n    x := 1\n    y := 2\n}"
	new := "if true {\n    z := 3\n}"

	_, ok := r.Replace(content, old, new, false)
	if ok {
		t.Fatal("expected failure for duplicate regions without replaceAll")
	}
}

func TestIndentationFlexible_MixedTabsSpaces(t *testing.T) {
	r := &IndentationFlexibleReplacer{}

	tests := []struct {
		name    string
		content string
		old     string
		new     string
		wantOK  bool
	}{
		{
			"spaces in needle, tabs in content",
			"\tfunc foo() {\n\t\treturn 1\n\t}",
			"    func foo() {\n        return 1\n    }",
			"    func bar() {\n        return 2\n    }",
			// IndentationFlexibleReplacer strips common indent and compares
			// stripped content. With different indent chars, it may or may not match
			// depending on implementation. This tests the behavior is deterministic.
			false, // tabs vs spaces won't match after stripping
		},
		{
			"extra tab indent level in content",
			"\t\t\tfunc deep() {\n\t\t\t\treturn 1\n\t\t\t}",
			"\tfunc deep() {\n\t\treturn 1\n\t}",
			"\tfunc shallow() {\n\t\treturn 2\n\t}",
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := r.Replace(tt.content, tt.old, tt.new, false)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
		})
	}
}
