package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsBinaryData(t *testing.T) {
	tests := []struct {
		name   string
		data   []byte
		wantBi bool
	}{
		{"empty", []byte{}, false},
		{"plain text", []byte("hello world\nthis is text"), false},
		{"go source", []byte("package main\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}"), false},
		{"null bytes", []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, true},
		{"mixed binary", makeBinaryData(100, 0.5), true},
		{"mostly text with few control chars", makeBinaryData(100, 0.05), false},
		{"utf8 text", []byte("日本語テスト hello world"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isBinaryData(tt.data)
			if got != tt.wantBi {
				t.Errorf("isBinaryData() = %v, want %v", got, tt.wantBi)
			}
		})
	}
}

func TestExecuteRead_TruncatesLargeFile(t *testing.T) {
	dir := t.TempDir()
	largePath := filepath.Join(dir, "large.txt")

	// Create a file larger than MaxOutputSize (10MB).
	f, err := os.Create(largePath)
	if err != nil {
		t.Fatal(err)
	}
	chunk := strings.Repeat("A", 1024*1024) + "\n" // ~1MB line
	for i := 0; i < 11; i++ {
		f.WriteString(chunk)
	}
	f.Close()

	args := readArgs{FilePath: largePath}
	raw, _ := json.Marshal(args)
	tc := &Context{Directory: dir, ReadFiles: NewSafeReadFiles()}

	result, err := executeRead(context.Background(), tc, raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Output, "[file truncated at 10MB") {
		t.Error("expected truncation notice in output")
	}
}

func TestExecuteRead_ImageDuplicateCheck(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "test.png")
	// Write a minimal valid PNG-ish file (content doesn't matter for this test).
	if err := os.WriteFile(imgPath, []byte("fake-png-data"), 0o644); err != nil {
		t.Fatal(err)
	}

	tc := &Context{
		Directory:  dir,
		ReadFiles:  NewSafeReadFiles(),
		ReadImages: NewSafeReadFiles(),
	}
	raw, _ := json.Marshal(readArgs{FilePath: imgPath})

	// First read: should return base64 data.
	result1, err := executeRead(context.Background(), tc, raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result1.IsError {
		t.Fatalf("unexpected tool error: %s", result1.Output)
	}
	if !strings.HasPrefix(result1.Output, "data:image/png;base64,") {
		t.Fatalf("expected data URI, got %q", result1.Output)
	}

	// Second read of the same image: should return short note.
	result2, err := executeRead(context.Background(), tc, raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result2.IsError {
		t.Fatal("duplicate image read should not be an error")
	}
	if result2.Output != "Image already attached" {
		t.Errorf("expected 'Image already attached', got %q", result2.Output)
	}
}

func TestExecuteRead_NonImageUnchanged(t *testing.T) {
	dir := t.TempDir()
	txtPath := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(txtPath, []byte("hello\nworld\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tc := &Context{
		Directory:  dir,
		ReadFiles:  NewSafeReadFiles(),
		ReadImages: NewSafeReadFiles(),
	}
	raw, _ := json.Marshal(readArgs{FilePath: txtPath})

	// First read.
	result1, err := executeRead(context.Background(), tc, raw)
	if err != nil || result1.IsError {
		t.Fatalf("unexpected error on first read: %v / %s", err, result1.Output)
	}
	if !strings.Contains(result1.Output, "hello") {
		t.Fatalf("expected file content, got %q", result1.Output)
	}

	// Second read of same text file: should still return content (no duplicate check for non-images).
	result2, err := executeRead(context.Background(), tc, raw)
	if err != nil || result2.IsError {
		t.Fatalf("unexpected error on second read: %v / %s", err, result2.Output)
	}
	if !strings.Contains(result2.Output, "hello") {
		t.Fatalf("expected file content on re-read, got %q", result2.Output)
	}
}

func TestExecuteRead_ImageDuplicateCheck_Symlink(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "original.png")
	if err := os.WriteFile(imgPath, []byte("fake-png-data"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(dir, "link.png")
	if err := os.Symlink(imgPath, linkPath); err != nil {
		t.Skip("symlinks not supported")
	}

	tc := &Context{
		Directory:  dir,
		ReadFiles:  NewSafeReadFiles(),
		ReadImages: NewSafeReadFiles(),
	}

	raw1, _ := json.Marshal(readArgs{FilePath: imgPath})
	result1, err := executeRead(context.Background(), tc, raw1)
	if err != nil || result1.IsError {
		t.Fatalf("unexpected error on first read: %v / %s", err, result1.Output)
	}
	if !strings.HasPrefix(result1.Output, "data:image/png;base64,") {
		t.Fatalf("expected data URI, got %q", result1.Output)
	}

	raw2, _ := json.Marshal(readArgs{FilePath: linkPath})
	result2, err := executeRead(context.Background(), tc, raw2)
	if err != nil {
		t.Fatalf("unexpected error on symlink read: %v", err)
	}
	// resolvePath resolves symlinks, so both paths canonicalize to the same file.
	// The duplicate check correctly catches this as already attached.
	if result2.Output != "Image already attached" {
		t.Errorf("expected duplicate detection for symlink to same image, got %q", result2.Output)
	}
}

func TestExecuteRead_ImageDuplicateCheck_DifferentExtensions(t *testing.T) {
	dir := t.TempDir()
	jpgPath := filepath.Join(dir, "photo.jpg")
	pngPath := filepath.Join(dir, "icon.png")
	if err := os.WriteFile(jpgPath, []byte("fake-jpg-data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pngPath, []byte("fake-png-data"), 0o644); err != nil {
		t.Fatal(err)
	}

	tc := &Context{
		Directory:  dir,
		ReadFiles:  NewSafeReadFiles(),
		ReadImages: NewSafeReadFiles(),
	}

	raw1, _ := json.Marshal(readArgs{FilePath: jpgPath})
	result1, err := executeRead(context.Background(), tc, raw1)
	if err != nil || result1.IsError {
		t.Fatalf("unexpected error reading jpg: %v / %s", err, result1.Output)
	}
	if !strings.HasPrefix(result1.Output, "data:image/jpeg;base64,") {
		t.Fatalf("expected jpeg data URI, got %q", result1.Output)
	}

	raw2, _ := json.Marshal(readArgs{FilePath: pngPath})
	result2, err := executeRead(context.Background(), tc, raw2)
	if err != nil || result2.IsError {
		t.Fatalf("unexpected error reading png: %v / %s", err, result2.Output)
	}
	if !strings.HasPrefix(result2.Output, "data:image/png;base64,") {
		t.Fatalf("expected png data URI, got %q", result2.Output)
	}

	// Re-read jpg should be duplicate
	result3, err := executeRead(context.Background(), tc, raw1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result3.Output != "Image already attached" {
		t.Errorf("expected duplicate for jpg, got %q", result3.Output)
	}

	// Re-read png should also be duplicate
	result4, err := executeRead(context.Background(), tc, raw2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result4.Output != "Image already attached" {
		t.Errorf("expected duplicate for png, got %q", result4.Output)
	}
}

func TestExecuteRead_ImageNilReadImages(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "test.png")
	if err := os.WriteFile(imgPath, []byte("fake-png-data"), 0o644); err != nil {
		t.Fatal(err)
	}

	tc := &Context{
		Directory:  dir,
		ReadFiles:  NewSafeReadFiles(),
		ReadImages: nil,
	}
	raw, _ := json.Marshal(readArgs{FilePath: imgPath})

	result, err := executeRead(context.Background(), tc, raw)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %s", err, result.Output)
	}
	if !strings.HasPrefix(result.Output, "data:image/png;base64,") {
		t.Fatalf("expected data URI, got %q", result.Output)
	}

	// Second read with nil ReadImages should still return data (no tracking)
	result2, err := executeRead(context.Background(), tc, raw)
	if err != nil || result2.IsError {
		t.Fatalf("unexpected error: %v / %s", err, result2.Output)
	}
	if !strings.HasPrefix(result2.Output, "data:image/png;base64,") {
		t.Fatalf("expected data URI on second read with nil tracker, got %q", result2.Output)
	}
}

func makeBinaryData(size int, binaryRatio float64) []byte {
	data := make([]byte, size)
	binaryCount := int(float64(size) * binaryRatio)
	for i := 0; i < size; i++ {
		if i < binaryCount {
			data[i] = 0x01 // non-text control char
		} else {
			data[i] = 'a'
		}
	}
	return data
}
