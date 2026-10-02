package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/config"
)

func TestGenerateToken_Length(t *testing.T) {
	token := generateToken()
	if len(token) != 64 {
		t.Errorf("expected 64-char hex token, got %d chars: %s", len(token), token)
	}
}

func TestGenerateToken_Unique(t *testing.T) {
	a := generateToken()
	b := generateToken()
	if a == b {
		t.Errorf("expected unique tokens, got identical: %s", a)
	}
}

func TestServerConfig_Defaults(t *testing.T) {
	cfg := serverConfig(&config.Info{}, false)
	if cfg.Port != 4096 {
		t.Errorf("expected default port 4096, got %d", cfg.Port)
	}
	if cfg.Hostname != "127.0.0.1" {
		t.Errorf("expected default host 127.0.0.1, got %s", cfg.Hostname)
	}
}

func TestLoadOrCreateWebToken_CreatesNew(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TINYCODE_DATA_DIR", dir)

	token := loadOrCreateWebToken()
	if len(token) != 64 {
		t.Errorf("expected 64-char token, got %d chars", len(token))
	}

	// Verify file was created
	data, err := os.ReadFile(filepath.Join(dir, "web_token"))
	if err != nil {
		t.Fatalf("expected token file to be created: %v", err)
	}
	if got := string(data); got != token+"\n" {
		t.Errorf("expected file contents %q, got %q", token+"\n", got)
	}
}

func TestLoadOrCreateWebToken_ReadsExisting(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TINYCODE_DATA_DIR", dir)

	// Write a known 64-char token
	knownToken := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := os.WriteFile(filepath.Join(dir, "web_token"), []byte(knownToken+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	token := loadOrCreateWebToken()
	if token != knownToken {
		t.Errorf("expected existing token %s, got %s", knownToken, token)
	}
}

func TestIsLocalhostAddr(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"127.0.0.1", true},
		{"::1", true},
		{"localhost", true},
		{"0.0.0.0", false},
		{"192.168.1.1", false},
		{"10.0.0.1", false},
		{"example.com", false},
		{"::", false},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			if got := isLocalhostAddr(tt.host); got != tt.want {
				t.Errorf("isLocalhostAddr(%q) = %v, want %v", tt.host, got, tt.want)
			}
		})
	}
}

func TestValidateNoAuthSafety_LocalhostAllowed(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "::1", "localhost"} {
		t.Run(host, func(t *testing.T) {
			level, err := validateNoAuthSafety(host, false)
			if err != nil {
				t.Errorf("expected no error for localhost %q, got %v", host, err)
			}
			if level != "" {
				t.Errorf("expected empty level for localhost, got %q", level)
			}
		})
	}
}

func TestValidateNoAuthSafety_NonLocalhostBlocked(t *testing.T) {
	for _, host := range []string{"0.0.0.0", "192.168.1.1", "10.0.0.1"} {
		t.Run(host, func(t *testing.T) {
			_, err := validateNoAuthSafety(host, false)
			if err == nil {
				t.Errorf("expected error for non-localhost %q, got nil", host)
			}
		})
	}
}

func TestValidateNoAuthSafety_ForceOverride(t *testing.T) {
	level, err := validateNoAuthSafety("0.0.0.0", true)
	if err != nil {
		t.Errorf("expected no error with force override, got %v", err)
	}
	if level != "warn" {
		t.Errorf("expected warn level with force override, got %q", level)
	}
}

func TestInitTooling_SetsJobManager(t *testing.T) {
	b := bus.New()
	defer b.Close()

	_, _, toolCtx := initTooling(b, t.TempDir())

	if toolCtx.JobManager == nil {
		t.Error("expected JobManager to be set")
	}
}

func TestInitTooling_SetsBus(t *testing.T) {
	b := bus.New()
	defer b.Close()

	_, _, toolCtx := initTooling(b, t.TempDir())

	if toolCtx.Bus == nil {
		t.Error("expected Bus to be set")
	}
	if toolCtx.Bus != b {
		t.Error("expected Bus to be the same instance passed to initTooling")
	}
}
