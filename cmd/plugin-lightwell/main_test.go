package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode-go/internal/redhat"
	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

func TestPluginID(t *testing.T) {
	p := newPlugin(options{})
	if p.ID != "lightwell" {
		t.Errorf("expected plugin ID 'lightwell', got %q", p.ID)
	}
}

func TestToolCountUnconfigured(t *testing.T) {
	p := newPlugin(options{})
	if len(p.Tools) != 6 {
		t.Fatalf("expected 6 stub tools when unconfigured, got %d", len(p.Tools))
	}
}

func TestToolCountConfigured(t *testing.T) {
	p := newPlugin(options{ServiceAccountToken: "test-token"})
	if len(p.Tools) != 6 {
		t.Fatalf("expected 6 tools when configured, got %d", len(p.Tools))
	}
}

func TestToolNames(t *testing.T) {
	p := newPlugin(options{})
	expected := []string{
		"lightwell_check_package",
		"lightwell_check_deps",
		"lightwell_osv",
		"lightwell_provenance",
		"lightwell_config_check",
		"lightwell_scan_containerfile",
	}
	for i, want := range expected {
		if p.Tools[i].Name != want {
			t.Errorf("tool[%d]: expected name %q, got %q", i, want, p.Tools[i].Name)
		}
	}
}

func TestToolDefinitions(t *testing.T) {
	p := newPlugin(options{})
	for i, tool := range p.Tools {
		if tool.Description == "" {
			t.Errorf("tool[%d] %q: expected non-empty description", i, tool.Name)
		}
		if tool.Execute == nil {
			t.Errorf("tool[%d] %q: expected non-nil Execute", i, tool.Name)
		}
		typ, ok := tool.Parameters["type"]
		if !ok || typ != "object" {
			t.Errorf("tool[%d] %q: expected parameters type 'object', got %v", i, tool.Name, typ)
		}
	}
}

func TestUnconfiguredStubTools(t *testing.T) {
	p := newPlugin(options{})
	tc := plugin.ToolContext{}
	for _, tool := range p.Tools {
		result, err := tool.Execute(context.Background(), json.RawMessage(`{}`), tc)
		if err != nil {
			t.Errorf("tool %q: unexpected error: %v", tool.Name, err)
		}
		if !strings.Contains(result, "not configured") {
			t.Errorf("tool %q: expected 'not configured' message, got %q", tool.Name, result)
		}
	}
}

func TestParseOptions(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]any
		want  options
	}{
		{
			name:  "empty",
			input: map[string]any{},
			want:  options{},
		},
		{
			name:  "with token",
			input: map[string]any{"serviceAccountToken": "sa-token-123"},
			want:  options{ServiceAccountToken: "sa-token-123"},
		},
		{
			name:  "wrong type ignored",
			input: map[string]any{"serviceAccountToken": 123},
			want:  options{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseOptions(tt.input)
			if got != tt.want {
				t.Errorf("parseOptions() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestBoolYesNo(t *testing.T) {
	if boolYesNo(true) != "Yes" {
		t.Error("expected 'Yes' for true")
	}
	if boolYesNo(false) != "No" {
		t.Error("expected 'No' for false")
	}
}

func TestStrOrUnknown(t *testing.T) {
	if strOrUnknown("") != "unknown" {
		t.Error("expected 'unknown' for empty string")
	}
	if strOrUnknown("hello") != "hello" {
		t.Error("expected 'hello' for non-empty string")
	}
}

func TestFormatPackageResult(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		r := &packageCheckResult{
			Ecosystem: "python",
			Name:      "requests",
			Version:   "2.28.0",
			Found:     false,
		}
		result := formatPackageResult(r)
		if !strings.Contains(result, "python/requests@2.28.0") {
			t.Error("expected package identifier in output")
		}
		if !strings.Contains(result, "Found in Lightwell: No") {
			t.Error("expected 'Found in Lightwell: No'")
		}
	})

	t.Run("found with CVEs", func(t *testing.T) {
		r := &packageCheckResult{
			Ecosystem:        "java",
			Name:             "org.apache:commons-text",
			Version:          "1.9",
			Found:            true,
			LightwellVersion: "1.9.0.rhlw1",
			PatchAvailable:   true,
			CVECount:         2,
			CVEs: []packageCVE{
				{ID: "CVE-2022-42889", Severity: "CRITICAL", FixedIn: "1.10"},
				{ID: "CVE-2022-12345", Severity: "HIGH"},
			},
		}
		result := formatPackageResult(r)
		if !strings.Contains(result, "Found in Lightwell: Yes") {
			t.Error("expected found")
		}
		if !strings.Contains(result, "1.9.0.rhlw1") {
			t.Error("expected Lightwell version")
		}
		if !strings.Contains(result, "Patch Available: Yes") {
			t.Error("expected patch available")
		}
		if !strings.Contains(result, "CVE-2022-42889 (CRITICAL)") {
			t.Error("expected CVE with severity")
		}
		if !strings.Contains(result, "no fix") {
			t.Error("expected 'no fix' for CVE without fixedIn")
		}
	})

	t.Run("empty fields use defaults", func(t *testing.T) {
		r := &packageCheckResult{Found: true, CVECount: 1, CVEs: []packageCVE{{}}}
		result := formatPackageResult(r)
		if !strings.Contains(result, "unknown/unknown@unknown") {
			t.Error("expected 'unknown' defaults for empty fields")
		}
	})
}

func TestFormatOsvResult(t *testing.T) {
	t.Run("no vulnerabilities", func(t *testing.T) {
		result := formatOsvResult("python", "requests", nil)
		if !strings.Contains(result, "No known vulnerabilities found for python/requests") {
			t.Errorf("expected no vulns message, got %q", result)
		}
	})

	t.Run("with vulnerabilities", func(t *testing.T) {
		vulns := []osvVulnerability{
			{ID: "GHSA-1234", Severity: "HIGH", Summary: "RCE in parser"},
			{ID: "GHSA-5678", Severity: "MEDIUM", Summary: "XSS in output"},
		}
		result := formatOsvResult("java", "jackson-core", vulns)
		if !strings.Contains(result, "java/jackson-core: 2 found") {
			t.Error("expected vulnerability count")
		}
		if !strings.Contains(result, "GHSA-1234 (HIGH): RCE in parser") {
			t.Error("expected formatted vulnerability")
		}
	})

	t.Run("missing fields", func(t *testing.T) {
		vulns := []osvVulnerability{{}}
		result := formatOsvResult("python", "pkg", vulns)
		if !strings.Contains(result, "unknown (UNKNOWN): No summary") {
			t.Error("expected defaults for empty vulnerability fields")
		}
	})
}

func TestFormatProvenanceResult(t *testing.T) {
	t.Run("with attestations", func(t *testing.T) {
		r := &provenanceResult{
			Verified:  true,
			SLSALevel: "L3",
			BuildType: "tekton",
			Builder:   "tekton-chains",
			SourceURI: "https://github.com/example/repo",
			Digest:    "sha256:abc123",
			Attestations: []attestation{
				{Type: "cosign", Verified: true, Issuer: "sigstore"},
				{Type: "intoto", Verified: false, Issuer: ""},
			},
		}
		result := formatProvenanceResult(r)
		if !strings.Contains(result, "Provenance Verified: Yes") {
			t.Error("expected verified yes")
		}
		if !strings.Contains(result, "SLSA Level: L3") {
			t.Error("expected SLSA level")
		}
		if !strings.Contains(result, "cosign: verified (issuer: sigstore)") {
			t.Error("expected verified attestation")
		}
		if !strings.Contains(result, "intoto: unverified (issuer: unknown issuer)") {
			t.Error("expected unverified attestation with unknown issuer")
		}
	})

	t.Run("empty fields", func(t *testing.T) {
		r := &provenanceResult{}
		result := formatProvenanceResult(r)
		if !strings.Contains(result, "Provenance Verified: No") {
			t.Error("expected verified no")
		}
		if !strings.Contains(result, "SLSA Level: unknown") {
			t.Error("expected unknown SLSA level")
		}
	})
}

func TestParsePomXml(t *testing.T) {
	t.Run("valid pom with dependencies", func(t *testing.T) {
		content := `<?xml version="1.0"?>
<project>
  <dependencies>
    <dependency>
      <groupId>org.apache.commons</groupId>
      <artifactId>commons-text</artifactId>
      <version>1.9</version>
    </dependency>
    <dependency>
      <groupId>com.google.guava</groupId>
      <artifactId>guava</artifactId>
      <version>31.1-jre</version>
    </dependency>
  </dependencies>
</project>`
		deps := parsePomXml(content)
		if len(deps) != 2 {
			t.Fatalf("expected 2 deps, got %d", len(deps))
		}
		if deps[0].name != "org.apache.commons:commons-text" {
			t.Errorf("expected 'org.apache.commons:commons-text', got %q", deps[0].name)
		}
		if deps[0].version != "1.9" {
			t.Errorf("expected version '1.9', got %q", deps[0].version)
		}
		if deps[1].name != "com.google.guava:guava" {
			t.Errorf("expected 'com.google.guava:guava', got %q", deps[1].name)
		}
	})

	t.Run("empty content", func(t *testing.T) {
		deps := parsePomXml("")
		if len(deps) != 0 {
			t.Errorf("expected 0 deps for empty content, got %d", len(deps))
		}
	})

	t.Run("dependency without version skipped", func(t *testing.T) {
		content := `<dependency>
      <groupId>org.example</groupId>
      <artifactId>no-version</artifactId>
    </dependency>`
		deps := parsePomXml(content)
		if len(deps) != 0 {
			t.Errorf("expected 0 deps for dep without version, got %d", len(deps))
		}
	})
}

func TestParseRequirementsTxt(t *testing.T) {
	t.Run("valid requirements", func(t *testing.T) {
		content := `# This is a comment
requests==2.28.0
flask>=2.0.0

# Another comment
-e git+https://github.com/example/repo
numpy==1.23.4
`
		deps := parseRequirementsTxt(content)
		if len(deps) != 3 {
			t.Fatalf("expected 3 deps, got %d", len(deps))
		}
		if deps[0].name != "requests" || deps[0].version != "2.28.0" {
			t.Errorf("dep[0]: expected requests==2.28.0, got %s==%s", deps[0].name, deps[0].version)
		}
		if deps[1].name != "flask" || deps[1].version != "2.0.0" {
			t.Errorf("dep[1]: expected flask>=2.0.0, got %s>=%s", deps[1].name, deps[1].version)
		}
		if deps[2].name != "numpy" || deps[2].version != "1.23.4" {
			t.Errorf("dep[2]: expected numpy==1.23.4, got %s==%s", deps[2].name, deps[2].version)
		}
	})

	t.Run("empty content", func(t *testing.T) {
		deps := parseRequirementsTxt("")
		if len(deps) != 0 {
			t.Errorf("expected 0 deps, got %d", len(deps))
		}
	})

	t.Run("only comments and blanks", func(t *testing.T) {
		content := "# comment\n\n# another\n"
		deps := parseRequirementsTxt(content)
		if len(deps) != 0 {
			t.Errorf("expected 0 deps, got %d", len(deps))
		}
	})
}

func TestParseDeps(t *testing.T) {
	t.Run("pom.xml", func(t *testing.T) {
		content := `<dependency><groupId>g</groupId><artifactId>a</artifactId><version>1</version></dependency>`
		deps, errMsg := parseDeps(content, "pom.xml")
		if errMsg != "" {
			t.Errorf("unexpected error: %q", errMsg)
		}
		if len(deps) != 1 {
			t.Errorf("expected 1 dep, got %d", len(deps))
		}
	})

	t.Run("requirements.txt", func(t *testing.T) {
		deps, errMsg := parseDeps("pkg==1.0", "requirements.txt")
		if errMsg != "" {
			t.Errorf("unexpected error: %q", errMsg)
		}
		if len(deps) != 1 {
			t.Errorf("expected 1 dep, got %d", len(deps))
		}
	})

	t.Run("unsupported file type", func(t *testing.T) {
		deps, errMsg := parseDeps("content", "package.json")
		if errMsg == "" {
			t.Error("expected error for unsupported file type")
		}
		if !strings.Contains(errMsg, "Unsupported file type") {
			t.Errorf("expected unsupported message, got %q", errMsg)
		}
		if deps != nil {
			t.Error("expected nil deps for unsupported type")
		}
	})
}

func TestCheckConfigContent(t *testing.T) {
	t.Run("configured", func(t *testing.T) {
		content := `<repository>https://packages.redhat.com/lightwell/maven</repository>`
		result := checkConfigContent(content, "settings.xml")
		if !strings.Contains(result, "Lightwell repos configured: Yes") {
			t.Error("expected configured detection")
		}
		if !strings.Contains(result, "Lightwell repository URL detected") {
			t.Error("expected positive detection message")
		}
	})

	t.Run("not configured settings.xml", func(t *testing.T) {
		result := checkConfigContent("<settings></settings>", "settings.xml")
		if !strings.Contains(result, "Lightwell repos configured: No") {
			t.Error("expected not configured")
		}
		if !strings.Contains(result, "<repository>") {
			t.Error("expected settings.xml-specific suggestion")
		}
	})

	t.Run("not configured build.gradle", func(t *testing.T) {
		result := checkConfigContent("repositories {}", "build.gradle")
		if !strings.Contains(result, "maven {") {
			t.Error("expected build.gradle-specific suggestion")
		}
	})

	t.Run("not configured pip.conf", func(t *testing.T) {
		result := checkConfigContent("[global]\nindex-url = https://pypi.org", "pip.conf")
		if !strings.Contains(result, "index-url") {
			t.Error("expected pip.conf-specific suggestion")
		}
	})

	t.Run("unknown file type not configured", func(t *testing.T) {
		result := checkConfigContent("", "unknown.cfg")
		if !strings.Contains(result, "Configure your build tool") {
			t.Error("expected generic suggestion for unknown file type")
		}
	})
}

func TestStubTool(t *testing.T) {
	tool := stubTool("test_tool", "A test description", "Not available")
	if tool.Name != "test_tool" {
		t.Errorf("expected name 'test_tool', got %q", tool.Name)
	}
	if tool.Description != "A test description" {
		t.Errorf("expected matching description, got %q", tool.Description)
	}
	tc := plugin.ToolContext{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`), tc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Not available" {
		t.Errorf("expected 'Not available', got %q", result)
	}
}

func TestEcosystemMap(t *testing.T) {
	if ecosystemMap["pip"] != "python" {
		t.Errorf("expected pip -> python, got %q", ecosystemMap["pip"])
	}
	if ecosystemMap["npm"] != "npm" {
		t.Errorf("expected npm -> npm, got %q", ecosystemMap["npm"])
	}
	if ecosystemMap["maven"] != "java" {
		t.Errorf("expected maven -> java, got %q", ecosystemMap["maven"])
	}
}

func newMockLightwellClient(handler http.Handler) (*lightwellClient, *httptest.Server) {
	srv := httptest.NewServer(handler)
	return &lightwellClient{
		api: redhat.NewAPIClient(redhat.APIClientConfig{BaseURL: srv.URL}),
	}, srv
}

func TestLightwellClient_CheckPackage(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/packages/python/requests/2.28.0", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		fmt.Fprint(w, `{"found":true,"ecosystem":"python","name":"requests","version":"2.28.0","lightwellVersion":"2.28.0.rhlw1","patchAvailable":true,"cveCount":1,"cves":[{"id":"CVE-2023-32681","severity":"MODERATE","fixedIn":"2.31.0"}]}`)
	})
	client, srv := newMockLightwellClient(mux)
	defer srv.Close()

	result, err := client.checkPackage(context.Background(), "python", "requests", "2.28.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Found {
		t.Error("expected Found to be true")
	}
	if result.LightwellVersion != "2.28.0.rhlw1" {
		t.Errorf("lightwellVersion = %q, want %q", result.LightwellVersion, "2.28.0.rhlw1")
	}
	if !result.PatchAvailable {
		t.Error("expected PatchAvailable to be true")
	}
	if result.CVECount != 1 {
		t.Errorf("cveCount = %d, want 1", result.CVECount)
	}
	if len(result.CVEs) != 1 || result.CVEs[0].ID != "CVE-2023-32681" {
		t.Errorf("unexpected CVEs: %+v", result.CVEs)
	}
}

func TestLightwellClient_CheckPackage_Error(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	})
	client, srv := newMockLightwellClient(mux)
	defer srv.Close()

	_, err := client.checkPackage(context.Background(), "python", "nonexistent", "0.0.0")
	if err == nil {
		t.Fatal("expected error for server error response")
	}
}

func TestLightwellClient_QueryOsv(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/osv/python/requests", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		fmt.Fprint(w, `{"vulnerabilities":[{"id":"GHSA-j8r2-6x86-q33q","severity":"MODERATE","summary":"Unintended leak of Proxy-Authorization header"},{"id":"PYSEC-2023-74","severity":"HIGH","summary":"Session fixation vulnerability"}]}`)
	})
	client, srv := newMockLightwellClient(mux)
	defer srv.Close()

	result, err := client.queryOsv(context.Background(), "python", "requests")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Vulnerabilities) != 2 {
		t.Fatalf("got %d vulns, want 2", len(result.Vulnerabilities))
	}
	if result.Vulnerabilities[0].ID != "GHSA-j8r2-6x86-q33q" {
		t.Errorf("vuln[0].ID = %q", result.Vulnerabilities[0].ID)
	}
}

func TestLightwellClient_QueryOsv_Error(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	})
	client, srv := newMockLightwellClient(mux)
	defer srv.Close()

	_, err := client.queryOsv(context.Background(), "python", "nonexistent")
	if err == nil {
		t.Fatal("expected error for server error response")
	}
}

func TestLightwellClient_GetProvenance(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/provenance/python/requests/2.28.0", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		fmt.Fprint(w, `{"verified":true,"buildType":"tekton","builder":"tekton-chains","sourceUri":"https://github.com/psf/requests","digest":"sha256:abc123","slsaLevel":"L3","attestations":[{"type":"cosign","verified":true,"issuer":"sigstore"}]}`)
	})
	client, srv := newMockLightwellClient(mux)
	defer srv.Close()

	result, err := client.getProvenance(context.Background(), "python", "requests", "2.28.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Verified {
		t.Error("expected Verified to be true")
	}
	if result.SLSALevel != "L3" {
		t.Errorf("slsaLevel = %q, want %q", result.SLSALevel, "L3")
	}
	if result.BuildType != "tekton" {
		t.Errorf("buildType = %q, want %q", result.BuildType, "tekton")
	}
	if len(result.Attestations) != 1 {
		t.Fatalf("got %d attestations, want 1", len(result.Attestations))
	}
	if result.Attestations[0].Type != "cosign" || !result.Attestations[0].Verified {
		t.Errorf("attestation = %+v", result.Attestations[0])
	}
}

func TestLightwellClient_GetProvenance_Error(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})
	client, srv := newMockLightwellClient(mux)
	defer srv.Close()

	_, err := client.getProvenance(context.Background(), "python", "nonexistent", "0.0.0")
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}
