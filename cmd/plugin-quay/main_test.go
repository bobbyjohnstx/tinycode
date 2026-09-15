package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode-go/internal/redhat"
)

func TestPluginID(t *testing.T) {
	p := newPlugin(options{})
	if p.ID != "quay" {
		t.Errorf("expected plugin ID 'quay', got %q", p.ID)
	}
}

func TestToolCountUnconfigured(t *testing.T) {
	p := newPlugin(options{})
	if len(p.Tools) != 6 {
		t.Fatalf("expected 6 unconfigured tools, got %d", len(p.Tools))
	}
}

func TestToolCountConfigured(t *testing.T) {
	p := newPlugin(options{RegistryURL: "https://quay.io"})
	if len(p.Tools) != 6 {
		t.Fatalf("expected 6 configured tools, got %d", len(p.Tools))
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
		"quay_health",
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

// --- httptest mock tests ---

func newMockQuayClient(handler http.Handler) (*quayClient, *httptest.Server) {
	srv := httptest.NewServer(handler)
	return &quayClient{
		api: redhat.NewAPIClient(redhat.APIClientConfig{BaseURL: srv.URL}),
	}, srv
}

func TestSearchRepositories(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/v1/find/repositories", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if q := r.URL.Query().Get("query"); q != "ubi" {
				t.Errorf("expected query=ubi, got %q", q)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"results":[{"namespace":"redhat","name":"ubi9","description":"Universal Base Image 9","star_count":42}]}`)
		})
		client, srv := newMockQuayClient(mux)
		defer srv.Close()

		repos, err := client.searchRepositories(context.Background(), "ubi")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(repos) != 1 {
			t.Fatalf("expected 1 repo, got %d", len(repos))
		}
		if repos[0].Namespace != "redhat" {
			t.Errorf("expected namespace 'redhat', got %q", repos[0].Namespace)
		}
		if repos[0].Name != "ubi9" {
			t.Errorf("expected name 'ubi9', got %q", repos[0].Name)
		}
		if repos[0].Description != "Universal Base Image 9" {
			t.Errorf("expected description, got %q", repos[0].Description)
		}
		if repos[0].StarCount != 42 {
			t.Errorf("expected 42 stars, got %d", repos[0].StarCount)
		}
	})

	t.Run("server error", func(t *testing.T) {
		client, srv := newMockQuayClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "internal error", http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := client.searchRepositories(context.Background(), "ubi")
		if err == nil {
			t.Fatal("expected error for 500 response")
		}
		if !strings.Contains(err.Error(), "500") {
			t.Errorf("expected error to contain '500', got %q", err.Error())
		}
	})

	t.Run("empty results", func(t *testing.T) {
		client, srv := newMockQuayClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"results":[]}`)
		}))
		defer srv.Close()

		repos, err := client.searchRepositories(context.Background(), "nonexistent")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(repos) != 0 {
			t.Errorf("expected 0 repos, got %d", len(repos))
		}
	})
}

func TestListTags(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/v1/repository/redhat/ubi9/tag/", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"tags":[{"name":"latest","manifest_digest":"sha256:abc123def456","size":52428800,"last_modified":"Mon, 01 Jan 2024 00:00:00 -0000"}]}`)
		})
		client, srv := newMockQuayClient(mux)
		defer srv.Close()

		tags, err := client.listTags(context.Background(), "redhat", "ubi9")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(tags) != 1 {
			t.Fatalf("expected 1 tag, got %d", len(tags))
		}
		if tags[0].Name != "latest" {
			t.Errorf("expected tag name 'latest', got %q", tags[0].Name)
		}
		if tags[0].ManifestDigest != "sha256:abc123def456" {
			t.Errorf("expected digest, got %q", tags[0].ManifestDigest)
		}
		if tags[0].Size != 52428800 {
			t.Errorf("expected size 52428800, got %d", tags[0].Size)
		}
	})

	t.Run("not found", func(t *testing.T) {
		client, srv := newMockQuayClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		}))
		defer srv.Close()

		_, err := client.listTags(context.Background(), "redhat", "nonexistent")
		if err == nil {
			t.Fatal("expected error for 404 response")
		}
		if !strings.Contains(err.Error(), "404") {
			t.Errorf("expected error to contain '404', got %q", err.Error())
		}
	})
}

func TestGetManifest(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/v1/repository/redhat/ubi9/manifest/sha256:abc123", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"digest":"sha256:abc123","is_manifest_list":false,"manifest_data":"{\"layers\":[]}","config_media_type":"application/vnd.oci.image.config.v1+json","layers_compressed_size":10485760}`)
		})
		client, srv := newMockQuayClient(mux)
		defer srv.Close()

		manifest, err := client.getManifest(context.Background(), "redhat", "ubi9", "sha256:abc123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if manifest.Digest != "sha256:abc123" {
			t.Errorf("expected digest 'sha256:abc123', got %q", manifest.Digest)
		}
		if manifest.IsManifestList {
			t.Error("expected IsManifestList=false")
		}
		if manifest.ConfigMediaType != "application/vnd.oci.image.config.v1+json" {
			t.Errorf("expected config media type, got %q", manifest.ConfigMediaType)
		}
		if manifest.LayersCompressedSize != 10485760 {
			t.Errorf("expected compressed size 10485760, got %d", manifest.LayersCompressedSize)
		}
	})

	t.Run("server error", func(t *testing.T) {
		client, srv := newMockQuayClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "server error", http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := client.getManifest(context.Background(), "redhat", "ubi9", "sha256:abc123")
		if err == nil {
			t.Fatal("expected error for 500 response")
		}
	})
}

func TestGetVulnerabilities(t *testing.T) {
	t.Run("happy path with vulnerabilities", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/v1/repository/redhat/ubi9/manifest/sha256:abc123/security", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"status":"scanned","data":{"Layer":{"Features":[{"Name":"openssl","Version":"1.1.1","Vulnerabilities":[{"Name":"CVE-2023-0001","Severity":"Critical","FixedBy":"1.1.2"}]}]}}}`)
		})
		client, srv := newMockQuayClient(mux)
		defer srv.Close()

		result, err := client.getVulnerabilities(context.Background(), "redhat", "ubi9", "sha256:abc123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Status != "scanned" {
			t.Errorf("expected status 'scanned', got %q", result.Status)
		}
		if result.Data == nil || result.Data.Layer == nil {
			t.Fatal("expected non-nil data and layer")
		}
		if len(result.Data.Layer.Features) != 1 {
			t.Fatalf("expected 1 feature, got %d", len(result.Data.Layer.Features))
		}
		feat := result.Data.Layer.Features[0]
		if feat.Name != "openssl" {
			t.Errorf("expected feature 'openssl', got %q", feat.Name)
		}
		if len(feat.Vulnerabilities) != 1 {
			t.Fatalf("expected 1 vulnerability, got %d", len(feat.Vulnerabilities))
		}
		vuln := feat.Vulnerabilities[0]
		if vuln.Name != "CVE-2023-0001" {
			t.Errorf("expected CVE name, got %q", vuln.Name)
		}
		if vuln.Severity != "Critical" {
			t.Errorf("expected severity 'Critical', got %q", vuln.Severity)
		}
		if vuln.FixedBy != "1.1.2" {
			t.Errorf("expected fixedBy '1.1.2', got %q", vuln.FixedBy)
		}
	})

	t.Run("no vulnerabilities", func(t *testing.T) {
		client, srv := newMockQuayClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"status":"scanned","data":{"Layer":{"Features":[]}}}`)
		}))
		defer srv.Close()

		result, err := client.getVulnerabilities(context.Background(), "redhat", "ubi9", "sha256:abc123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Status != "scanned" {
			t.Errorf("expected status 'scanned', got %q", result.Status)
		}
		if len(result.Data.Layer.Features) != 0 {
			t.Errorf("expected 0 features, got %d", len(result.Data.Layer.Features))
		}
	})

	t.Run("unauthorized", func(t *testing.T) {
		client, srv := newMockQuayClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "unauthorized", http.StatusForbidden)
		}))
		defer srv.Close()

		_, err := client.getVulnerabilities(context.Background(), "redhat", "ubi9", "sha256:abc123")
		if err == nil {
			t.Fatal("expected error for 403 response")
		}
		if !strings.Contains(err.Error(), "403") {
			t.Errorf("expected error to contain '403', got %q", err.Error())
		}
	})
}

func TestGetLabels(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/v1/repository/redhat/ubi9/manifest/sha256:abc123/labels", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"labels":[{"key":"maintainer","value":"Red Hat","source_type":"manifest"},{"key":"version","value":"9.3","source_type":"manifest"}]}`)
		})
		client, srv := newMockQuayClient(mux)
		defer srv.Close()

		labels, err := client.getLabels(context.Background(), "redhat", "ubi9", "sha256:abc123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(labels) != 2 {
			t.Fatalf("expected 2 labels, got %d", len(labels))
		}
		if labels[0].Key != "maintainer" {
			t.Errorf("expected key 'maintainer', got %q", labels[0].Key)
		}
		if labels[0].Value != "Red Hat" {
			t.Errorf("expected value 'Red Hat', got %q", labels[0].Value)
		}
		if labels[0].SourceType != "manifest" {
			t.Errorf("expected source_type 'manifest', got %q", labels[0].SourceType)
		}
		if labels[1].Key != "version" {
			t.Errorf("expected key 'version', got %q", labels[1].Key)
		}
	})

	t.Run("empty labels", func(t *testing.T) {
		client, srv := newMockQuayClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"labels":[]}`)
		}))
		defer srv.Close()

		labels, err := client.getLabels(context.Background(), "redhat", "ubi9", "sha256:abc123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(labels) != 0 {
			t.Errorf("expected 0 labels, got %d", len(labels))
		}
	})

	t.Run("server error", func(t *testing.T) {
		client, srv := newMockQuayClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "internal error", http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := client.getLabels(context.Background(), "redhat", "ubi9", "sha256:abc123")
		if err == nil {
			t.Fatal("expected error for 500 response")
		}
	})
}
