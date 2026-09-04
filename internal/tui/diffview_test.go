package tui

import (
	"strings"
	"testing"
)

func TestDiffView_SetDiffEmpty(t *testing.T) {
	d := NewDiffView()
	d.SetDiff("file.go", "")

	if d.LineCount() != 0 {
		t.Errorf("expected 0 lines for empty diff, got %d", d.LineCount())
	}
	if d.View() != "" {
		t.Errorf("expected empty view, got %q", d.View())
	}
}

func TestDiffView_SetDiffParseLines(t *testing.T) {
	diff := `--- a/file.go
+++ b/file.go
@@ -1,3 +1,3 @@
 context line
-removed line
+added line`

	d := NewDiffView()
	d.SetDiff("file.go", diff)

	if d.LineCount() != 6 {
		t.Fatalf("expected 6 lines, got %d", d.LineCount())
	}
}

func TestDiffView_SetDiffResetsScroll(t *testing.T) {
	d := NewDiffView()
	d.SetSize(80, 2)
	d.SetDiff("file.go", "+a\n+b\n+c\n+d\n+e")
	d.ScrollDown()
	d.ScrollDown()

	// Setting new diff should reset scroll.
	d.SetDiff("other.go", "+x\n+y")
	view := d.View()
	if !strings.Contains(view, "x") {
		t.Errorf("expected scroll reset to show first line, got %q", view)
	}
}

func TestDiffView_ScrollBounds(t *testing.T) {
	d := NewDiffView()
	d.SetSize(80, 3)
	d.SetDiff("file.go", "+line1\n+line2\n+line3\n+line4\n+line5")

	// Scroll up at top should be no-op.
	d.ScrollUp()
	view := d.View()
	if !strings.Contains(view, "line1") {
		t.Error("expected line1 visible at top")
	}

	// Scroll down until bottom.
	for i := 0; i < 10; i++ {
		d.ScrollDown()
	}
	view = d.View()
	if !strings.Contains(view, "line5") {
		t.Error("expected line5 visible at bottom")
	}

	// Over-scrolling should not go past content.
	d.ScrollDown()
	view2 := d.View()
	if view != view2 {
		t.Error("expected no change after over-scrolling")
	}
}

func TestDiffView_SetSizeMinimum(t *testing.T) {
	d := NewDiffView()
	d.SetSize(0, 0)

	// Should clamp to 1x1, not panic.
	d.SetDiff("f.go", "+a")
	view := d.View()
	if view == "" {
		t.Error("expected non-empty view even with min size")
	}
}

func TestDiffView_SideBySideThreshold(t *testing.T) {
	diff := "-old\n+new"
	d := NewDiffView()
	d.SetDiff("f.go", diff)

	// Narrow: unified mode (lines start with +/-)
	d.SetSize(80, 10)
	narrow := d.View()

	// Wide: side-by-side (contains " | " separator)
	d.SetSize(200, 10)
	wide := d.View()

	if !strings.Contains(wide, " | ") {
		t.Errorf("expected side-by-side separator in wide view, got %q", wide)
	}

	// Narrow should not have the separator pattern between columns.
	lines := strings.Split(narrow, "\n")
	for _, line := range lines {
		if strings.Contains(line, " | ") {
			t.Errorf("did not expect side-by-side separator in narrow view, got line %q", line)
		}
	}
}

func TestParseDiffLines_HeaderTypes(t *testing.T) {
	diff := "diff --git a/f b/f\n--- a/f\n+++ b/f\n@@ -1 +1 @@"
	lines := parseDiffLines(diff)

	for _, line := range lines {
		if line.Type != DiffHeader {
			t.Errorf("expected header type for %q, got %d", line.Content, line.Type)
		}
	}
}

func TestTruncateLine(t *testing.T) {
	if truncateLine("short", 80) != "short" {
		t.Error("short line should not be truncated")
	}
	long := strings.Repeat("x", 100)
	result := truncateLine(long, 20)
	if len(result) != 20 {
		t.Errorf("expected length 20, got %d", len(result))
	}
	if !strings.HasSuffix(result, "...") {
		t.Error("expected ellipsis suffix")
	}
}
