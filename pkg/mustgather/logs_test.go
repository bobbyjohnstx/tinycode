package mustgather

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestReadLogs(t *testing.T) {
	dir := t.TempDir()
	logContent := `I0115 10:30:45.123456       1 server.go:456] started
W0115 10:31:00.234567       1 server.go:789] apply request took too long
I0115 10:32:00.345678       1 server.go:456] compaction finished
`
	path := filepath.Join(dir, "current.log")
	os.WriteFile(path, []byte(logContent), 0o644)

	t.Run("read all lines", func(t *testing.T) {
		entries, err := ReadLogs(path, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 3 {
			t.Fatalf("got %d, want 3", len(entries))
		}
		if entries[0].Level != "INFO" {
			t.Errorf("entries[0].Level = %q", entries[0].Level)
		}
		if entries[1].Level != "WARN" {
			t.Errorf("entries[1].Level = %q", entries[1].Level)
		}
	})

	t.Run("limit lines", func(t *testing.T) {
		entries, err := ReadLogs(path, 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 2 {
			t.Fatalf("got %d, want 2", len(entries))
		}
	})
}

func TestSearchLogs(t *testing.T) {
	dir := t.TempDir()
	logContent := `line 1: normal
line 2: error something failed
line 3: normal
line 4: error connection refused
line 5: warning slow
`
	path := filepath.Join(dir, "test.log")
	os.WriteFile(path, []byte(logContent), 0o644)

	pat := regexp.MustCompile("error")
	entries, err := SearchLogs(path, pat, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d, want 2", len(entries))
	}
}

func TestSearchLogs_MaxResults(t *testing.T) {
	dir := t.TempDir()
	logContent := "error 1\nerror 2\nerror 3\nerror 4\n"
	path := filepath.Join(dir, "test.log")
	os.WriteFile(path, []byte(logContent), 0o644)

	pat := regexp.MustCompile("error")
	entries, err := SearchLogs(path, pat, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d, want 2", len(entries))
	}
}

func TestCountLogPatterns(t *testing.T) {
	dir := t.TempDir()
	logContent := `apply request took too long (100ms)
normal log line
slow fdatasync (45ms)
apply request took too long (200ms)
elected leader at term 5
`
	path := filepath.Join(dir, "etcd.log")
	os.WriteFile(path, []byte(logContent), 0o644)

	patterns := map[string]*regexp.Regexp{
		"slow_apply": regexp.MustCompile("apply request took too long"),
		"slow_fsync": regexp.MustCompile("slow fdatasync"),
		"leader":     regexp.MustCompile("elected leader"),
	}

	counts, err := CountLogPatterns(path, patterns)
	if err != nil {
		t.Fatal(err)
	}
	if counts["slow_apply"] != 2 {
		t.Errorf("slow_apply = %d, want 2", counts["slow_apply"])
	}
	if counts["slow_fsync"] != 1 {
		t.Errorf("slow_fsync = %d, want 1", counts["slow_fsync"])
	}
	if counts["leader"] != 1 {
		t.Errorf("leader = %d, want 1", counts["leader"])
	}
}

func TestFindLogFiles(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "pods", "etcd", "logs"), 0o755)
	os.WriteFile(filepath.Join(dir, "pods", "etcd", "logs", "current.log"), []byte("log"), 0o644)
	os.WriteFile(filepath.Join(dir, "pods", "etcd", "logs", "previous.log"), []byte("old"), 0o644)
	os.WriteFile(filepath.Join(dir, "pods", "etcd", "config.yaml"), []byte("yaml"), 0o644)

	logs, err := FindLogFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 2 {
		t.Errorf("got %d log files, want 2", len(logs))
	}
}
