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
	tc := &Context{Directory: dir, ReadFiles: make(map[string]bool)}

	result, err := executeRead(context.Background(), tc, raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Output, "[file truncated at 10MB") {
		t.Error("expected truncation notice in output")
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
