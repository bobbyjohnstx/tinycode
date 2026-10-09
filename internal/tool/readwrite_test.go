package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadRecordsFile(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello\n"), 0644)

	tc := &Context{Directory: dir, ReadFiles: NewSafeReadFiles()}
	result, err := executeRead(context.Background(), tc, mustJSON(t, readArgs{FilePath: path}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Output)
	}
	if !tc.ReadFiles.Has(path) {
		t.Error("expected ReadFiles to record the file after read")
	}
}

func TestEditRecordsFile(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("foo bar baz"), 0644)

	tc := &Context{Directory: dir, ReadFiles: NewSafeReadFiles()}
	result, err := executeEdit(context.Background(), tc, mustJSON(t, editArgs{
		FilePath:  path,
		OldString: "bar",
		NewString: "qux",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Output)
	}
	if !tc.ReadFiles.Has(path) {
		t.Error("expected ReadFiles to record the file after edit")
	}
}

func TestWriteNewFileNoWarning(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.txt")

	tc := &Context{Directory: dir, ReadFiles: NewSafeReadFiles()}
	result, err := executeWrite(context.Background(), tc, mustJSON(t, writeArgs{
		FilePath: path,
		Content:  "hello world",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Output)
	}
	if strings.Contains(result.Output, "WARNING") {
		t.Error("expected no warning for new file")
	}
}

func TestWriteExistingUnreadFileWarning(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.txt")
	os.WriteFile(path, []byte("original content"), 0644)

	tc := &Context{Directory: dir, ReadFiles: NewSafeReadFiles()}
	result, err := executeWrite(context.Background(), tc, mustJSON(t, writeArgs{
		FilePath: path,
		Content:  "overwritten",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "WARNING") {
		t.Error("expected warning for writing to existing unread file")
	}
	if !strings.Contains(result.Output, "have not read") {
		t.Error("expected warning message about not reading the file")
	}
}

func TestWrite_AppendsFormatterNote(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")

	tc := &Context{
		Directory: dir,
		ReadFiles: NewSafeReadFiles(),
		FormatFile: func(context.Context, string) (string, bool, error) {
			return "gofmt", true, nil
		},
	}
	result, err := executeWrite(context.Background(), tc, mustJSON(t, writeArgs{
		FilePath: path,
		Content:  "package main\n",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Formatted with gofmt") {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestWrite_KeepsFileWhenFormatterFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")

	tc := &Context{
		Directory: dir,
		ReadFiles: NewSafeReadFiles(),
		FormatFile: func(context.Context, string) (string, bool, error) {
			return "gofmt", false, os.ErrInvalid
		},
	}
	result, err := executeWrite(context.Background(), tc, mustJSON(t, writeArgs{
		FilePath: path,
		Content:  "package main\n",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("formatter failure should stay in the tool output: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Formatter failed:") {
		t.Fatalf("output = %q", result.Output)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "package main\n" {
		t.Fatalf("file = %q", got)
	}
}

func TestWriteExistingReadFileNoWarning(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	path := filepath.Join(dir, "existing.txt")
	os.WriteFile(path, []byte("original content"), 0644)

	tc := &Context{Directory: dir, ReadFiles: NewSafeReadFiles()}
	// Simulate having read the file.
	tc.ReadFiles.Mark(path)

	result, err := executeWrite(context.Background(), tc, mustJSON(t, writeArgs{
		FilePath: path,
		Content:  "overwritten",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Output)
	}
	if strings.Contains(result.Output, "WARNING") {
		t.Error("expected no warning for file that was already read")
	}
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	return data
}
