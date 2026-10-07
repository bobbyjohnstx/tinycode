package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/config"
	"github.com/bobbyjohnstx/tinycode/internal/permission"
	"github.com/bobbyjohnstx/tinycode/internal/project"
	"github.com/bobbyjohnstx/tinycode/internal/provider"
	"github.com/bobbyjohnstx/tinycode/internal/storage"
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

	_, _, toolCtx := initTooling(b, t.TempDir(), nil)

	if toolCtx.JobManager == nil {
		t.Error("expected JobManager to be set")
	}
}

func TestInitTooling_SetsBus(t *testing.T) {
	b := bus.New()
	defer b.Close()

	_, _, toolCtx := initTooling(b, t.TempDir(), nil)

	if toolCtx.Bus == nil {
		t.Error("expected Bus to be set")
	}
	if toolCtx.Bus != b {
		t.Error("expected Bus to be the same instance passed to initTooling")
	}
}

func TestInitTooling_WiresDBAndRuleStore(t *testing.T) {
	b := bus.New()
	defer b.Close()

	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	dir := t.TempDir()
	_, permSvc, toolCtx := initTooling(b, dir, db.DB)
	if toolCtx.DB == nil {
		t.Fatal("expected tool Context.DB to be set")
	}
	if toolCtx.DB != db.DB {
		t.Fatal("expected tool Context.DB to be the opened database")
	}

	// ensureProject + SetStore should leave a project row for the working dir.
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM project`).Scan(&count); err != nil {
		t.Fatalf("count projects: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 project row after initTooling, got %d", count)
	}

	// Persist via RuleStore and reload into a fresh service.
	store := permission.NewSQLiteRuleStore(db.DB)
	proj := project.FromDirectory(dir)
	rules := permission.Ruleset{{Permission: "bash", Pattern: "*", Action: permission.ActionAllow}}
	if err := store.SaveRules(proj.ID, rules); err != nil {
		t.Fatalf("SaveRules: %v", err)
	}
	_ = permSvc // wired; load path covered by SetStore on a new service
	svc2 := permission.NewService(b)
	svc2.SetStore(store, proj.ID)
	if err := svc2.Ask(t.Context(), permission.AskInput{
		SessionID: "s", Permission: "bash", Patterns: []string{"true"},
	}); err != nil {
		t.Fatalf("Ask after SetStore reload: %v", err)
	}
}

func TestPermissionAskToolArgs_PrefersCommand(t *testing.T) {
	got := permissionAskToolArgs(permission.Request{
		Patterns: []string{"fallback"},
		Metadata: map[string]any{
			"command": "rm -rf /",
			"args":    `{"command":"ignored"}`,
		},
	})
	if got != "rm -rf /" {
		t.Fatalf("expected command metadata, got %q", got)
	}
}

func TestPermissionAskToolName_FromMetadata(t *testing.T) {
	got := permissionAskToolName(permission.Request{
		Permission: "edit",
		Metadata:   map[string]any{"tool": "write"},
	})
	if got != "write" {
		t.Fatalf("expected tool metadata, got %q", got)
	}
	got = permissionAskToolName(permission.Request{Permission: "destructive-shell"})
	if got != "destructive-shell" {
		t.Fatalf("expected permission fallback, got %q", got)
	}
}

func TestWirePermissionAskHook_DeniesDangerousCommand(t *testing.T) {
	b := bus.New()
	defer b.Close()
	permSvc := permission.NewService(b)

	// Simulate the deny path wirePermissionAskHook uses when a plugin rejects.
	permSvc.SetAskInterceptor(func(req permission.Request) error {
		if permissionAskToolArgs(req) == "rm -rf /" {
			return fmt.Errorf("%w: blocked by safety-net: command matches dangerous pattern", permission.ErrRejected)
		}
		return nil
	})

	err := permSvc.Ask(t.Context(), permission.AskInput{
		SessionID:  "ses_test",
		Permission: "destructive-shell",
		Patterns:   []string{"rm -rf /"},
		Metadata:   map[string]any{"command": "rm -rf /"},
	})
	if err == nil {
		t.Fatal("expected deny for rm -rf /")
	}
	if !errors.Is(err, permission.ErrRejected) {
		t.Fatalf("expected ErrRejected, got %v", err)
	}
	if !strings.Contains(err.Error(), "safety-net") {
		t.Fatalf("expected safety-net reason, got %v", err)
	}
}

func TestRegisterConfigProviders_CopiesHeaders(t *testing.T) {
	reg := provider.NewRegistry()
	cfg := &config.Info{
		Provider: map[string]config.ProviderConfig{
			"custom-llm": {
				Headers: map[string]string{"X-From-Config": "a"},
				Options: map[string]any{
					"baseURL": "http://127.0.0.1:8080/v1",
					"apiKey":  "k",
					"headers": map[string]any{"X-From-Options": "b"},
				},
				Models: map[string]config.ProviderModelConfig{
					"m1": {},
				},
			},
		},
	}

	registerConfigProviders(reg, cfg)

	m, err := reg.GetModel("custom-llm", "m1")
	if err != nil {
		t.Fatalf("GetModel: %v", err)
	}
	if m.Headers["X-From-Config"] != "a" {
		t.Errorf("X-From-Config = %q", m.Headers["X-From-Config"])
	}
	if m.Headers["X-From-Options"] != "b" {
		t.Errorf("X-From-Options = %q", m.Headers["X-From-Options"])
	}
}

func TestBrowserBaseURL_RewritesWildcard(t *testing.T) {
	u, err := url.Parse("http://0.0.0.0:4096")
	if err != nil {
		t.Fatal(err)
	}
	got := browserBaseURL(u)
	if got != "http://127.0.0.1:4096" {
		t.Fatalf("got %q, want http://127.0.0.1:4096", got)
	}
	u2, _ := url.Parse("http://127.0.0.1:4096")
	if browserBaseURL(u2) != "http://127.0.0.1:4096" {
		t.Fatalf("unexpected rewrite of loopback: %q", browserBaseURL(u2))
	}
}
