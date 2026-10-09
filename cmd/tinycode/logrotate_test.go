package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRotatingFile_RotatesAndDropsOldest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tinycode.log")
	// Each 2-byte write after the first crosses the 3-byte cap and rotates.
	rf, err := openRotatingLog(path, 3, 2)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rf.Close() })

	for _, chunk := range []string{"aa", "bb", "cc", "dd"} {
		if _, err := rf.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}

	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != "dd" {
		t.Fatalf("current log = %q, want dd", current)
	}
	first, err := os.ReadFile(path + ".1")
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != "cc" {
		t.Fatalf("log.1 = %q, want cc", first)
	}
	second, err := os.ReadFile(path + ".2")
	if err != nil {
		t.Fatal(err)
	}
	if string(second) != "bb" {
		t.Fatalf("log.2 = %q, want bb", second)
	}
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Fatalf("log.3 should have been removed, stat err = %v", err)
	}
}
