package tool

import (
	"testing"
)

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"abc", "abc", 0},
		{"kitten", "sitting", 3},
		{"sunday", "saturday", 3},
		{"abc", "axc", 1},
	}
	for _, tt := range tests {
		got := LevenshteinDistance(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("LevenshteinDistance(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestSimilarity(t *testing.T) {
	tests := []struct {
		a, b    string
		wantMin float64
		wantMax float64
	}{
		{"", "", 1.0, 1.0},
		{"abc", "abc", 1.0, 1.0},
		{"abc", "axc", 0.6, 0.7},
		{"hello", "world", 0.0, 0.3},
	}
	for _, tt := range tests {
		got := Similarity(tt.a, tt.b)
		if got < tt.wantMin || got > tt.wantMax {
			t.Errorf("Similarity(%q, %q) = %f, want [%f, %f]", tt.a, tt.b, got, tt.wantMin, tt.wantMax)
		}
	}
}

func TestSimpleReplacer(t *testing.T) {
	r := &SimpleReplacer{}
	tests := []struct {
		name       string
		content    string
		old        string
		new        string
		replaceAll bool
		wantOK     bool
		want       string
	}{
		{"exact match", "hello world", "world", "go", false, true, "hello go"},
		{"no match", "hello world", "xyz", "go", false, false, ""},
		{"multiple no replaceAll", "aaa", "a", "b", false, false, ""},
		{"multiple replaceAll", "aaa", "a", "b", true, true, "bbb"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := r.Replace(tt.content, tt.old, tt.new, tt.replaceAll)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLineTrimmedReplacer(t *testing.T) {
	r := &LineTrimmedReplacer{}

	tests := []struct {
		name    string
		content string
		old     string
		new     string
		wantOK  bool
		want    string
	}{
		{
			"whitespace mismatch",
			"  func foo() {\n    return 1\n  }",
			"func foo() {\n  return 1\n}",
			"func bar() {\n  return 2\n}",
			true,
			"func bar() {\n  return 2\n}",
		},
		{
			"tabs vs spaces",
			"\tfunc foo() {\n\t\treturn 1\n\t}",
			"func foo() {\n    return 1\n}",
			"func bar() {\n    return 2\n}",
			true,
			"func bar() {\n    return 2\n}",
		},
		{
			"no match",
			"func foo() {}",
			"func bar() {}",
			"func baz() {}",
			false,
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := r.Replace(tt.content, tt.old, tt.new, false)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBlockAnchorReplacer(t *testing.T) {
	r := &BlockAnchorReplacer{Threshold: 0.0}

	tests := []struct {
		name    string
		content string
		old     string
		new     string
		wantOK  bool
	}{
		{
			"anchored match",
			"func foo() {\n\tx := 1\n\treturn x\n}",
			"func foo() {\n\tx := 1\n\treturn x\n}",
			"func foo() {\n\treturn 2\n}",
			true,
		},
		{
			"single line needle",
			"func foo() {}",
			"func foo() {}",
			"func bar() {}",
			false, // needs >= 2 lines
		},
		{
			"anchor mismatch",
			"func foo() {\n\treturn 1\n}",
			"func bar() {\n\treturn 1\n}",
			"func baz() {}",
			false,
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

func TestWhitespaceNormalizedReplacer(t *testing.T) {
	r := &WhitespaceNormalizedReplacer{}

	tests := []struct {
		name    string
		content string
		old     string
		new     string
		wantOK  bool
		want    string
	}{
		{
			"extra spaces",
			"x  =  1",
			"x = 1",
			"x = 2",
			true,
			"x = 2",
		},
		{
			"multiline extra ws",
			"  func  foo()  {\n    return  1\n  }",
			"func foo() {\n  return 1\n}",
			"func bar() {\n  return 2\n}",
			true,
			"func bar() {\n  return 2\n}",
		},
		{
			"no match",
			"hello world",
			"goodbye world",
			"hi",
			false,
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := r.Replace(tt.content, tt.old, tt.new, false)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIndentationFlexibleReplacer(t *testing.T) {
	r := &IndentationFlexibleReplacer{}

	tests := []struct {
		name    string
		content string
		old     string
		new     string
		wantOK  bool
		want    string
	}{
		{
			"different indent level",
			"\t\tfunc foo() {\n\t\t\treturn 1\n\t\t}",
			"func foo() {\n\treturn 1\n}",
			"func bar() {\n\treturn 2\n}",
			true,
			"\t\tfunc bar() {\n\t\t\treturn 2\n\t\t}",
		},
		{
			"no match",
			"func foo() {}",
			"func bar() {}",
			"func baz() {}",
			false,
			"",
		},
		{
			"same indent",
			"func foo() {\n\treturn 1\n}",
			"func foo() {\n\treturn 1\n}",
			"func bar() {\n\treturn 2\n}",
			true,
			"func bar() {\n\treturn 2\n}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := r.Replace(tt.content, tt.old, tt.new, false)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTrimmedBoundaryReplacer(t *testing.T) {
	r := &TrimmedBoundaryReplacer{}

	tests := []struct {
		name    string
		content string
		old     string
		new     string
		wantOK  bool
		want    string
	}{
		{
			"leading blank lines",
			"hello world",
			"\n\nhello world",
			"hi world",
			true,
			"hi world",
		},
		{
			"trailing blank lines",
			"hello world",
			"hello world\n\n",
			"hi world",
			true,
			"hi world",
		},
		{
			"no blank boundary",
			"hello world",
			"hello world",
			"hi world",
			false, // trimmedOld == oldString, skipped
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := r.Replace(tt.content, tt.old, tt.new, false)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMultiOccurrenceReplacer(t *testing.T) {
	r := &MultiOccurrenceReplacer{}

	tests := []struct {
		name    string
		content string
		old     string
		new     string
		wantOK  bool
	}{
		{
			"picks best match",
			"func foo() {\n\treturn 1\n}\n\nfunc foo() {\n\treturn 2\n}",
			"func foo() {\n\treturn 1\n}",
			"func bar() {\n\treturn 1\n}",
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

	// Verify replaceAll is rejected.
	t.Run("replaceAll rejected", func(t *testing.T) {
		_, ok := r.Replace("func foo() {}", "func foo() {}", "func bar() {}", true)
		if ok {
			t.Fatal("expected replaceAll to be rejected")
		}
	})
}

func TestCascadeReplace(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		old      string
		new      string
		wantOK   bool
		wantText string
	}{
		{
			"exact match uses simple",
			"hello world",
			"hello",
			"hi",
			true,
			"hi world",
		},
		{
			"whitespace mismatch uses line-trimmed",
			"  func foo() {\n    return 1\n  }",
			"func foo() {\n  return 1\n}",
			"func bar() {}",
			true,
			"func bar() {}",
		},
		{
			"total failure",
			"hello world",
			"completely different text that does not exist anywhere",
			"replacement",
			false,
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, ok := cascadeReplace(tt.content, tt.old, tt.new, false)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got != tt.wantText {
				t.Errorf("got %q, want %q", got, tt.wantText)
			}
		})
	}
}

func TestEscapeNormalizedReplacer(t *testing.T) {
	r := &EscapeNormalizedReplacer{}

	tests := []struct {
		name    string
		content string
		old     string
		new     string
		wantOK  bool
		want    string
	}{
		{
			"CRLF vs LF",
			"hello\r\nworld",
			"hello\nworld",
			"hi earth",
			true,
			"hi earth",
		},
		{
			"tab vs spaces",
			"hello\tworld",
			"hello    world",
			"hi earth",
			true,
			"hi earth",
		},
		{
			"no match",
			"hello world",
			"goodbye world",
			"hi",
			false,
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := r.Replace(tt.content, tt.old, tt.new, false)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestContextAwareReplacer(t *testing.T) {
	r := &ContextAwareReplacer{}

	tests := []struct {
		name    string
		content string
		old     string
		new     string
		wantOK  bool
	}{
		{
			"context match",
			"import (\n\t\"fmt\"\n\t\"os\"\n)",
			"import (\n\t\"fmt\"\n\t\"os\"\n)",
			"import (\n\t\"fmt\"\n)",
			true,
		},
		{
			"too few lines",
			"hello world",
			"hello world",
			"hi",
			false,
		},
		{
			"no match",
			"a\nb\nc\nd",
			"x\ny\nz\nw",
			"replaced",
			false,
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
