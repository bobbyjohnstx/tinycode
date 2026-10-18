package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseACPFlags(t *testing.T) {
	promptFile := filepath.Join(t.TempDir(), "extra.txt")
	if err := os.WriteFile(promptFile, []byte("from-file"), 0o644); err != nil {
		t.Fatal(err)
	}
	cwdDir := t.TempDir()

	tests := []struct {
		name     string
		args     []string
		wantModel string
		wantCWD  string
		wantAppend string
	}{
		{
			name:      "short model",
			args:      []string{"-m", "ollama/qwen"},
			wantModel: "ollama/qwen",
		},
		{
			name:      "long model overrides short if last",
			args:      []string{"-m", "a/b", "--model", "c/d"},
			wantModel: "c/d",
		},
		{
			name:     "cwd flag",
			args:     []string{"--cwd", cwdDir},
			wantCWD:  cwdDir,
		},
		{
			name:     "positional directory as cwd",
			args:     []string{cwdDir},
			wantCWD:  cwdDir,
		},
		{
			name:       "append system prompt file",
			args:       []string{"--append-system-prompt-file", promptFile},
			wantAppend: "from-file",
		},
		{
			name:       "append text then file",
			args:       []string{"--append-system-prompt", "prefix", "--append-system-prompt-file", promptFile},
			wantAppend: "prefix\n\nfrom-file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, cwd := parseACPFlags(tt.args)
			if tt.wantModel != "" && f.model != tt.wantModel {
				t.Errorf("model = %q, want %q", f.model, tt.wantModel)
			}
			if tt.wantCWD != "" && cwd != tt.wantCWD {
				t.Errorf("cwd = %q, want %q", cwd, tt.wantCWD)
			}
			if tt.wantAppend != "" && f.appendSystemPrompt != tt.wantAppend {
				t.Errorf("append = %q, want %q", f.appendSystemPrompt, tt.wantAppend)
			}
		})
	}
}
