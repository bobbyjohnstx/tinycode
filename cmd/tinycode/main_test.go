package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTruncateStr(t *testing.T) {
	tests := []struct {
		name string
		s    string
		max  int
		want string
	}{
		{name: "short", s: "hi", max: 10, want: "hi"},
		{name: "exact", s: "abcd", max: 4, want: "abcd"},
		{name: "ellipsis", s: "abcdefgh", max: 5, want: "ab..."},
		{name: "max below 4", s: "abcdef", max: 3, want: "abc"},
		{name: "empty", s: "", max: 4, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncateStr(tt.s, tt.max); got != tt.want {
				t.Errorf("truncateStr(%q, %d) = %q, want %q", tt.s, tt.max, got, tt.want)
			}
		})
	}
}

func TestCopyFile_PreservesExecutable(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.sh")
	dst := filepath.Join(dir, "dst.sh")
	if err := os.WriteFile(src, []byte("#!/bin/sh\necho ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(src, dst); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "#!/bin/sh\necho ok\n" {
		t.Fatalf("content = %q", got)
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("expected executable bits on dst, mode=%v", info.Mode())
	}
}

func TestCopyFile_MissingSource(t *testing.T) {
	err := copyFile(filepath.Join(t.TempDir(), "missing"), filepath.Join(t.TempDir(), "dst"))
	if err == nil {
		t.Fatal("expected error for missing source")
	}
}
