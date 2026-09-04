package main

import (
	"testing"
)

func TestPluginID(t *testing.T) {
	opts := options{ClusterID: "test-cluster"}
	p := newPlugin(opts)

	if p.ID != "cluster-ops" {
		t.Errorf("expected plugin ID 'cluster-ops', got %q", p.ID)
	}
}

func TestPluginHasThreeTools(t *testing.T) {
	opts := options{}
	p := newPlugin(opts)

	if len(p.Tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(p.Tools))
	}

	expected := []string{"oc-login", "oc-status", "cluster-info"}
	for i, name := range expected {
		if p.Tools[i].Name != name {
			t.Errorf("tool[%d]: expected %q, got %q", i, name, p.Tools[i].Name)
		}
	}
}

func TestParseOptions(t *testing.T) {
	raw := map[string]any{
		"clusterId":          "cluster-123",
		"apiUrl":             "https://api.cluster.example.com:6443",
		"consoleOfflineToken": "tok-abc-xyz",
	}

	opts := parseOptions(raw)

	if opts.ClusterID != "cluster-123" {
		t.Errorf("ClusterID: expected 'cluster-123', got %q", opts.ClusterID)
	}
	if opts.APIURL != "https://api.cluster.example.com:6443" {
		t.Errorf("APIURL: expected 'https://api.cluster.example.com:6443', got %q", opts.APIURL)
	}
	if opts.ConsoleOfflineToken != "tok-abc-xyz" {
		t.Errorf("ConsoleOfflineToken: expected 'tok-abc-xyz', got %q", opts.ConsoleOfflineToken)
	}
}

func TestParseOptionsEmpty(t *testing.T) {
	opts := parseOptions(map[string]any{})

	if opts.ClusterID != "" {
		t.Errorf("expected empty ClusterID, got %q", opts.ClusterID)
	}
	if opts.APIURL != "" {
		t.Errorf("expected empty APIURL, got %q", opts.APIURL)
	}
	if opts.ConsoleOfflineToken != "" {
		t.Errorf("expected empty ConsoleOfflineToken, got %q", opts.ConsoleOfflineToken)
	}
}

func TestShellEnvHook(t *testing.T) {
	opts := options{ClusterID: "my-cluster"}
	p := newPlugin(opts)

	if p.Hooks.ShellEnv == nil {
		t.Fatal("expected ShellEnv hook to be set")
	}

	// The hook is tested via the plugin SDK protocol in integration tests.
	// Here we verify it's registered.
	if p.Hooks.SessionStart == nil {
		t.Fatal("expected SessionStart hook to be set")
	}
}
