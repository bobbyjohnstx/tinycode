package main

import (
	"strings"
	"testing"
)

func TestPluginID(t *testing.T) {
	p := newPlugin(options{})
	if p.ID != "quay" {
		t.Errorf("expected plugin ID 'quay', got %q", p.ID)
	}
}

func TestToolCountUnconfigured(t *testing.T) {
	p := newPlugin(options{})
	if len(p.Tools) != 5 {
		t.Fatalf("expected 5 unconfigured tools, got %d", len(p.Tools))
	}
}

func TestToolCountConfigured(t *testing.T) {
	p := newPlugin(options{RegistryURL: "https://quay.io"})
	if len(p.Tools) != 5 {
		t.Fatalf("expected 5 configured tools, got %d", len(p.Tools))
	}
}

func TestToolDefinitions(t *testing.T) {
	p := newPlugin(options{RegistryURL: "https://quay.io"})
	expectedNames := []string{
		"quay_search",
		"quay_tags",
		"quay_manifest",
		"quay_vulnerabilities",
		"quay_labels",
	}

	if len(p.Tools) != len(expectedNames) {
		t.Fatalf("expected %d tools, got %d", len(expectedNames), len(p.Tools))
	}

	for i, name := range expectedNames {
		tool := p.Tools[i]
		if tool.Name != name {
			t.Errorf("tool[%d]: expected name %q, got %q", i, name, tool.Name)
		}
		if tool.Description == "" {
			t.Errorf("tool %q: expected non-empty description", name)
		}
		if tool.Execute == nil {
			t.Errorf("tool %q: expected non-nil Execute", name)
		}
	}
}

func TestToolSchemas(t *testing.T) {
	p := newPlugin(options{RegistryURL: "https://quay.io"})
	for _, tool := range p.Tools {
		typ, ok := tool.Parameters["type"]
		if !ok || typ != "object" {
			t.Errorf("tool %q: expected type 'object', got %v", tool.Name, typ)
		}
		if _, ok := tool.Parameters["properties"]; !ok {
			t.Errorf("tool %q: missing 'properties' key", tool.Name)
		}
	}
}

func TestToolSchemasRequired(t *testing.T) {
	p := newPlugin(options{RegistryURL: "https://quay.io"})
	toolsWithRequired := map[string][]string{
		"quay_search":          {"query"},
		"quay_tags":            {"repository"},
		"quay_manifest":        {"repository", "digest"},
		"quay_vulnerabilities": {"repository", "digest"},
		"quay_labels":          {"repository", "digest"},
	}

	for _, tool := range p.Tools {
		expected, ok := toolsWithRequired[tool.Name]
		if !ok {
			continue
		}
		required, ok := tool.Parameters["required"].([]string)
		if !ok {
			t.Errorf("tool %q: expected required to be []string", tool.Name)
			continue
		}
		if len(required) != len(expected) {
			t.Errorf("tool %q: expected %d required fields, got %d", tool.Name, len(expected), len(required))
		}
	}
}

func TestParseOptions(t *testing.T) {
	t.Run("full options", func(t *testing.T) {
		opts := parseOptions(map[string]any{
			"registryUrl": "https://quay.io",
			"apiToken":    "tok-123",
		})
		if opts.RegistryURL != "https://quay.io" {
			t.Errorf("expected RegistryURL 'https://quay.io', got %q", opts.RegistryURL)
		}
		if opts.APIToken != "tok-123" {
			t.Errorf("expected APIToken 'tok-123', got %q", opts.APIToken)
		}
	})

	t.Run("empty map", func(t *testing.T) {
		opts := parseOptions(map[string]any{})
		if opts.RegistryURL != "" {
			t.Errorf("expected empty RegistryURL, got %q", opts.RegistryURL)
		}
		if opts.APIToken != "" {
			t.Errorf("expected empty APIToken, got %q", opts.APIToken)
		}
	})

	t.Run("wrong types", func(t *testing.T) {
		opts := parseOptions(map[string]any{
			"registryUrl": 42,
			"apiToken":    true,
		})
		if opts.RegistryURL != "" {
			t.Errorf("expected empty RegistryURL for int, got %q", opts.RegistryURL)
		}
		if opts.APIToken != "" {
			t.Errorf("expected empty APIToken for bool, got %q", opts.APIToken)
		}
	})
}

func TestParseRepository(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantNS    string
		wantName  string
		wantOK    bool
	}{
		{"valid", "redhat/ubi9", "redhat", "ubi9", true},
		{"valid with dots", "library/nginx.io", "library", "nginx.io", true},
		{"empty", "", "", "", false},
		{"no slash", "noslash", "", "", false},
		{"empty namespace", "/name", "", "", false},
		{"empty name", "ns/", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns, name, ok := parseRepository(tt.input)
			if ok != tt.wantOK {
				t.Errorf("parseRepository(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
			if ok {
				if ns != tt.wantNS {
					t.Errorf("parseRepository(%q) ns = %q, want %q", tt.input, ns, tt.wantNS)
				}
				if name != tt.wantName {
					t.Errorf("parseRepository(%q) name = %q, want %q", tt.input, name, tt.wantName)
				}
			}
		})
	}
}

func TestFormatVulnerabilities(t *testing.T) {
	t.Run("no vulnerabilities", func(t *testing.T) {
		result := formatVulnerabilities(&quaySecurityResult{
			Status: "scanned",
			Data: &struct {
				Layer *struct {
					Features []quayFeature `json:"Features"`
				} `json:"Layer"`
			}{
				Layer: &struct {
					Features []quayFeature `json:"Features"`
				}{
					Features: []quayFeature{
						{Name: "openssl", Version: "1.1.1", Vulnerabilities: nil},
					},
				},
			},
		})
		if !strings.Contains(result, "No vulnerabilities found") {
			t.Errorf("expected no-vuln message, got %q", result)
		}
		if !strings.Contains(result, "scanned") {
			t.Errorf("expected scan status, got %q", result)
		}
	})

	t.Run("nil data", func(t *testing.T) {
		result := formatVulnerabilities(&quaySecurityResult{Status: "queued"})
		if !strings.Contains(result, "No vulnerabilities found") {
			t.Errorf("expected no-vuln for nil data, got %q", result)
		}
	})

	t.Run("with vulnerabilities sorted by severity", func(t *testing.T) {
		result := formatVulnerabilities(&quaySecurityResult{
			Status: "scanned",
			Data: &struct {
				Layer *struct {
					Features []quayFeature `json:"Features"`
				} `json:"Layer"`
			}{
				Layer: &struct {
					Features []quayFeature `json:"Features"`
				}{
					Features: []quayFeature{
						{
							Name:    "libpng",
							Version: "1.6.37",
							Vulnerabilities: []quayVulnerability{
								{Name: "CVE-2022-1111", Severity: "Low", FixedBy: "1.6.38"},
							},
						},
						{
							Name:    "openssl",
							Version: "1.1.1",
							Vulnerabilities: []quayVulnerability{
								{Name: "CVE-2023-9999", Severity: "Critical", FixedBy: "1.1.2"},
								{Name: "CVE-2023-8888", Severity: "High", FixedBy: ""},
							},
						},
					},
				},
			},
		})
		if !strings.Contains(result, "Vulnerabilities found: 3") {
			t.Errorf("expected 3 vulns, got %q", result)
		}
		// Critical should appear before Low
		critIdx := strings.Index(result, "CVE-2023-9999")
		lowIdx := strings.Index(result, "CVE-2022-1111")
		if critIdx > lowIdx {
			t.Errorf("expected Critical before Low in output")
		}
		if !strings.Contains(result, "no fix available") {
			t.Errorf("expected 'no fix available' for empty FixedBy, got %q", result)
		}
	})

	t.Run("empty severity and CVE", func(t *testing.T) {
		result := formatVulnerabilities(&quaySecurityResult{
			Status: "scanned",
			Data: &struct {
				Layer *struct {
					Features []quayFeature `json:"Features"`
				} `json:"Layer"`
			}{
				Layer: &struct {
					Features []quayFeature `json:"Features"`
				}{
					Features: []quayFeature{
						{
							Name:    "",
							Version: "",
							Vulnerabilities: []quayVulnerability{
								{Name: "", Severity: "", FixedBy: ""},
							},
						},
					},
				},
			},
		})
		if !strings.Contains(result, "unknown") {
			t.Errorf("expected 'unknown' fallbacks for empty fields, got %q", result)
		}
	})
}

func TestSeverityOrderMap(t *testing.T) {
	expected := map[string]int{
		"Critical":   0,
		"High":       1,
		"Medium":     2,
		"Low":        3,
		"Negligible": 4,
		"Unknown":    5,
	}
	for sev, want := range expected {
		got, ok := severityOrder[sev]
		if !ok {
			t.Errorf("missing severity %q in severityOrder", sev)
			continue
		}
		if got != want {
			t.Errorf("severityOrder[%q] = %d, want %d", sev, got, want)
		}
	}
}
