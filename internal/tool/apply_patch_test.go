package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestApplyPatch_SingleFileSingleHunk(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "hello.txt")
	os.WriteFile(file, []byte("line1\nline2\nline3\n"), 0644)

	patch := "--- a/hello.txt\n+++ b/hello.txt\n@@ -1,3 +1,3 @@\n line1\n-line2\n+line2_modified\n line3\n"

	tc := &Context{Directory: dir}
	args, _ := json.Marshal(applyPatchArgs{Patch: patch})
	result, err := executeApplyPatch(context.Background(), tc, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Output)
	}

	got, _ := os.ReadFile(file)
	want := "line1\nline2_modified\nline3\n"
	if string(got) != want {
		t.Errorf("got %q, want %q", string(got), want)
	}
}

func TestApplyPatch_SingleFileMultipleHunks(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "multi.txt")
	os.WriteFile(file, []byte("a\nb\nc\nd\ne\nf\ng\n"), 0644)

	patch := `--- a/multi.txt
+++ b/multi.txt
@@ -1,3 +1,3 @@
 a
-b
+B
 c
@@ -5,3 +5,3 @@
 e
-f
+F
 g
`

	tc := &Context{Directory: dir}
	args, _ := json.Marshal(applyPatchArgs{Patch: patch})
	result, err := executeApplyPatch(context.Background(), tc, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Output)
	}

	got, _ := os.ReadFile(file)
	want := "a\nB\nc\nd\ne\nF\ng\n"
	if string(got) != want {
		t.Errorf("got %q, want %q", string(got), want)
	}
}

func TestApplyPatch_MultiFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello\n"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("world\n"), 0644)

	patch := `--- a/a.txt
+++ b/a.txt
@@ -1 +1 @@
-hello
+HELLO
--- a/b.txt
+++ b/b.txt
@@ -1 +1 @@
-world
+WORLD
`

	tc := &Context{Directory: dir}
	args, _ := json.Marshal(applyPatchArgs{Patch: patch})
	result, err := executeApplyPatch(context.Background(), tc, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Output)
	}

	gotA, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if string(gotA) != "HELLO\n" {
		t.Errorf("a.txt: got %q, want %q", string(gotA), "HELLO\n")
	}
	gotB, _ := os.ReadFile(filepath.Join(dir, "b.txt"))
	if string(gotB) != "WORLD\n" {
		t.Errorf("b.txt: got %q, want %q", string(gotB), "WORLD\n")
	}
}

func TestApplyPatch_ContextMismatch_NoFilesModified(t *testing.T) {
	dir := t.TempDir()
	original := "line1\nline2\nline3\n"
	file := filepath.Join(dir, "fail.txt")
	os.WriteFile(file, []byte(original), 0644)

	patch := "--- a/fail.txt\n+++ b/fail.txt\n@@ -1,3 +1,3 @@\n line1\n-WRONG_CONTEXT\n+replaced\n line3\n"

	tc := &Context{Directory: dir}
	args, _ := json.Marshal(applyPatchArgs{Patch: patch})
	result, err := executeApplyPatch(context.Background(), tc, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected tool error for context mismatch")
	}

	got, _ := os.ReadFile(file)
	if string(got) != original {
		t.Errorf("file was modified despite error: got %q, want %q", string(got), original)
	}
}

func TestApplyPatch_CreateNewFile(t *testing.T) {
	dir := t.TempDir()

	patch := `--- /dev/null
+++ b/newfile.txt
@@ -0,0 +1,3 @@
+first
+second
+third
`

	tc := &Context{Directory: dir}
	args, _ := json.Marshal(applyPatchArgs{Patch: patch})
	result, err := executeApplyPatch(context.Background(), tc, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Output)
	}

	got, readErr := os.ReadFile(filepath.Join(dir, "newfile.txt"))
	if readErr != nil {
		t.Fatalf("new file not created: %v", readErr)
	}
	want := "first\nsecond\nthird\n"
	if string(got) != want {
		t.Errorf("got %q, want %q", string(got), want)
	}
}

func TestApplyPatch_DeleteFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "remove.txt")
	os.WriteFile(file, []byte("gone\n"), 0644)

	patch := `--- a/remove.txt
+++ /dev/null
@@ -1 +0,0 @@
-gone
`

	tc := &Context{Directory: dir}
	args, _ := json.Marshal(applyPatchArgs{Patch: patch})
	result, err := executeApplyPatch(context.Background(), tc, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Output)
	}

	if _, statErr := os.Stat(file); !os.IsNotExist(statErr) {
		t.Error("file should have been deleted")
	}
}

func TestApplyPatch_EmptyPatch(t *testing.T) {
	tc := &Context{Directory: t.TempDir()}
	args, _ := json.Marshal(applyPatchArgs{Patch: ""})
	result, err := executeApplyPatch(context.Background(), tc, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for empty patch")
	}
}

func TestApplyPatch_CreateInSubdirectory(t *testing.T) {
	dir := t.TempDir()

	patch := `--- /dev/null
+++ b/sub/dir/file.txt
@@ -0,0 +1 @@
+content
`

	tc := &Context{Directory: dir}
	args, _ := json.Marshal(applyPatchArgs{Patch: patch})
	result, err := executeApplyPatch(context.Background(), tc, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Output)
	}

	got, readErr := os.ReadFile(filepath.Join(dir, "sub", "dir", "file.txt"))
	if readErr != nil {
		t.Fatalf("file not created: %v", readErr)
	}
	if string(got) != "content\n" {
		t.Errorf("got %q, want %q", string(got), "content\n")
	}
}

func TestParsePatch_InvalidHeader(t *testing.T) {
	_, err := parsePatch("--- a/file\n+++ b/file\n@@ invalid @@\n")
	if err == nil {
		t.Error("expected error for invalid hunk header")
	}
}

func TestApplyHunks_AddLines(t *testing.T) {
	original := "a\nb\nc\n"
	h := hunk{
		oldStart: 2,
		oldCount: 1,
		newStart: 2,
		newCount: 3,
		lines: []diffLine{
			{op: ' ', text: "b"},
			{op: '+', text: "b1"},
			{op: '+', text: "b2"},
		},
	}
	got, err := applyHunks(original, []hunk{h})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "a\nb\nb1\nb2\nc\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApplyHunks_RemoveLines(t *testing.T) {
	original := "a\nb\nc\nd\n"
	h := hunk{
		oldStart: 2,
		oldCount: 2,
		newStart: 2,
		newCount: 0,
		lines: []diffLine{
			{op: '-', text: "b"},
			{op: '-', text: "c"},
		},
	}
	got, err := applyHunks(original, []hunk{h})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "a\nd\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStripDiffPrefix(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"a/src/main.go", "src/main.go"},
		{"b/src/main.go", "src/main.go"},
		{"/dev/null", "/dev/null"},
		{"plain.txt", "plain.txt"},
	}
	for _, tt := range tests {
		got := stripDiffPrefix(tt.in)
		if got != tt.want {
			t.Errorf("stripDiffPrefix(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
