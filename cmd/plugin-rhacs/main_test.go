package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

func TestPluginID(t *testing.T) {
	p := newPlugin(options{})
	if p.ID != "rhacs" {
		t.Errorf("expected plugin ID 'rhacs', got %q", p.ID)
	}
}

func TestToolCount(t *testing.T) {
	p := newPlugin(options{})
	if len(p.Tools) != 7 {
		t.Fatalf("expected 7 tools, got %d", len(p.Tools))
	}
}

func TestToolNames(t *testing.T) {
	p := newPlugin(options{})
	expected := []string{
		"rhacs_image_scan",
		"rhacs_image_check",
		"rhacs_deployment_check",
		"rhacs_violations",
		"rhacs_risk",
		"rhacs_compliance_scan",
		"rhacs_compliance_status",
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

func TestConfiguredToolCount(t *testing.T) {
	p := newPlugin(options{CentralURL: "https://central.example.com", APIToken: "test-token"})
	if len(p.Tools) != 7 {
		t.Fatalf("expected 7 tools when configured, got %d", len(p.Tools))
	}
	for i, tool := range p.Tools {
		if tool.Execute == nil {
			t.Errorf("configured tool[%d] %q: expected non-nil Execute", i, tool.Name)
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
			name:  "both fields",
			input: map[string]any{"centralUrl": "https://central.example.com", "apiToken": "tok123"},
			want:  options{CentralURL: "https://central.example.com", APIToken: "tok123"},
		},
		{
			name:  "wrong types ignored",
			input: map[string]any{"centralUrl": 123, "apiToken": true},
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

func TestFormatVulns(t *testing.T) {
	t.Run("no vulnerabilities", func(t *testing.T) {
		data := &imageScanResult{}
		data.Image.Name.FullName = "registry.io/myapp:latest"
		result := formatVulns(data)
		if !strings.Contains(result, "registry.io/myapp:latest") {
			t.Error("expected image name in output")
		}
		if !strings.Contains(result, "No vulnerabilities found") {
			t.Error("expected 'No vulnerabilities found'")
		}
	})

	t.Run("empty image name", func(t *testing.T) {
		data := &imageScanResult{}
		result := formatVulns(data)
		if !strings.Contains(result, "unknown") {
			t.Error("expected 'unknown' for empty image name")
		}
	})

	t.Run("vulnerabilities sorted by CVSS", func(t *testing.T) {
		data := &imageScanResult{}
		data.Image.Name.FullName = "myimage:v1"
		data.Components = []struct {
			Name    string `json:"name"`
			Version string `json:"version"`
			Vulns   []struct {
				CVE      string  `json:"cve"`
				Severity string  `json:"severity"`
				CVSS     float64 `json:"cvss"`
				Link     string  `json:"link"`
				FixedBy  string  `json:"fixedBy"`
			} `json:"vulns"`
		}{
			{
				Name:    "openssl",
				Version: "1.1.1",
				Vulns: []struct {
					CVE      string  `json:"cve"`
					Severity string  `json:"severity"`
					CVSS     float64 `json:"cvss"`
					Link     string  `json:"link"`
					FixedBy  string  `json:"fixedBy"`
				}{
					{CVE: "CVE-2023-0001", Severity: "LOW", CVSS: 3.5, FixedBy: "1.1.2"},
					{CVE: "CVE-2023-0002", Severity: "CRITICAL", CVSS: 9.8, FixedBy: "1.1.3"},
				},
			},
		}
		result := formatVulns(data)
		if !strings.Contains(result, "Vulnerabilities found: 2") {
			t.Error("expected 2 vulnerabilities")
		}
		idx9 := strings.Index(result, "CVE-2023-0002")
		idx3 := strings.Index(result, "CVE-2023-0001")
		if idx9 < 0 || idx3 < 0 || idx9 > idx3 {
			t.Error("expected higher CVSS vulnerability listed first")
		}
	})

	t.Run("missing fields use defaults", func(t *testing.T) {
		data := &imageScanResult{}
		data.Components = []struct {
			Name    string `json:"name"`
			Version string `json:"version"`
			Vulns   []struct {
				CVE      string  `json:"cve"`
				Severity string  `json:"severity"`
				CVSS     float64 `json:"cvss"`
				Link     string  `json:"link"`
				FixedBy  string  `json:"fixedBy"`
			} `json:"vulns"`
		}{
			{
				Vulns: []struct {
					CVE      string  `json:"cve"`
					Severity string  `json:"severity"`
					CVSS     float64 `json:"cvss"`
					Link     string  `json:"link"`
					FixedBy  string  `json:"fixedBy"`
				}{
					{CVSS: 5.0},
				},
			},
		}
		result := formatVulns(data)
		if !strings.Contains(result, "unknown") {
			t.Error("expected 'unknown' for missing component/CVE fields")
		}
		if !strings.Contains(result, "UNKNOWN") {
			t.Error("expected 'UNKNOWN' for missing severity")
		}
		if !strings.Contains(result, "no fix available") {
			t.Error("expected 'no fix available' for missing fixedBy")
		}
	})
}

func TestFormatAlerts(t *testing.T) {
	t.Run("no alerts", func(t *testing.T) {
		result := formatAlerts(nil)
		if result != "No active violations found." {
			t.Errorf("expected empty message, got %q", result)
		}
	})

	t.Run("multiple alerts", func(t *testing.T) {
		alerts := []alert{
			{
				Policy:     alertPolicy{Name: "No-Root", Severity: "CRITICAL"},
				Deployment: alertDeployment{Name: "myapp", Namespace: "default"},
				State:      "ACTIVE",
			},
			{
				Policy:     alertPolicy{Name: "Resource-Limits", Severity: "HIGH"},
				Deployment: alertDeployment{Name: "backend", Namespace: "prod"},
				State:      "RESOLVED",
			},
		}
		result := formatAlerts(alerts)
		if !strings.Contains(result, "Active violations: 2") {
			t.Error("expected 2 active violations")
		}
		if !strings.Contains(result, "CRITICAL") {
			t.Error("expected CRITICAL severity")
		}
		if !strings.Contains(result, "default/myapp") {
			t.Error("expected deployment namespace/name")
		}
	})

	t.Run("missing fields", func(t *testing.T) {
		alerts := []alert{{}}
		result := formatAlerts(alerts)
		if !strings.Contains(result, "unknown policy") {
			t.Error("expected 'unknown policy' for missing policy name")
		}
		if !strings.Contains(result, "UNKNOWN") {
			t.Error("expected 'UNKNOWN' for missing severity")
		}
	})
}

func TestFormatPolicyAlerts(t *testing.T) {
	t.Run("no violations", func(t *testing.T) {
		result := formatPolicyAlerts(nil, "Image myapp:v1")
		if result != "Image myapp:v1 passed all policy checks." {
			t.Errorf("expected pass message, got %q", result)
		}
	})

	t.Run("with violations", func(t *testing.T) {
		alerts := []policyAlert{
			{Policy: policyViolation{Name: "No-Root", Severity: "CRITICAL", Description: "Running as root"}},
			{Policy: policyViolation{Name: "Limits", Severity: "HIGH"}},
		}
		result := formatPolicyAlerts(alerts, "Deployment")
		if !strings.Contains(result, "Deployment FAILED") {
			t.Error("expected FAILED prefix")
		}
		if !strings.Contains(result, "[CRITICAL] No-Root: Running as root") {
			t.Error("expected formatted policy with description")
		}
		if !strings.Contains(result, "[HIGH] Limits") {
			t.Error("expected formatted policy without description")
		}
	})
}

func TestFormatComplianceScanResult(t *testing.T) {
	t.Run("no profiles", func(t *testing.T) {
		result := formatComplianceScanResult(&complianceScanResult{})
		if result != "No compliance scan results available." {
			t.Errorf("expected empty message, got %q", result)
		}
	})

	t.Run("profiles with failing controls sorted by severity", func(t *testing.T) {
		data := &complianceScanResult{
			Profiles: []complianceScanProfile{
				{
					ProfileName: "cis-k8s",
					Passing:     8,
					Failing:     2,
					Controls: []complianceControl{
						{Name: "low-ctrl", Status: "FAIL", Severity: "LOW"},
						{Name: "critical-ctrl", Status: "FAIL", Severity: "CRITICAL", Remediation: "Fix it"},
						{Name: "passing-ctrl", Status: "PASS", Severity: "HIGH"},
					},
				},
			},
		}
		result := formatComplianceScanResult(data)
		if !strings.Contains(result, "Profile: cis-k8s") {
			t.Error("expected profile name")
		}
		if !strings.Contains(result, "Passing: 8/10 (80%)") {
			t.Error("expected passing percentage")
		}
		idxCritical := strings.Index(result, "critical-ctrl")
		idxLow := strings.Index(result, "low-ctrl")
		if idxCritical < 0 || idxLow < 0 || idxCritical > idxLow {
			t.Error("expected CRITICAL control listed before LOW")
		}
		if !strings.Contains(result, "Remediation: Fix it") {
			t.Error("expected remediation text")
		}
	})

	t.Run("all passing no failing controls listed", func(t *testing.T) {
		data := &complianceScanResult{
			Profiles: []complianceScanProfile{
				{
					ProfileName: "nist",
					Passing:     5,
					Failing:     0,
					Controls: []complianceControl{
						{Name: "ctrl-1", Status: "PASS", Severity: "HIGH"},
					},
				},
			},
		}
		result := formatComplianceScanResult(data)
		if !strings.Contains(result, "Passing: 5/5 (100%)") {
			t.Error("expected 100% passing")
		}
		if strings.Contains(result, "Failing Controls:") {
			t.Error("should not show Failing Controls section when none fail")
		}
	})
}

func TestSeverityOrder(t *testing.T) {
	if severityOrder["CRITICAL"] >= severityOrder["HIGH"] {
		t.Error("CRITICAL should have lower order (higher priority) than HIGH")
	}
	if severityOrder["HIGH"] >= severityOrder["MEDIUM"] {
		t.Error("HIGH should have lower order than MEDIUM")
	}
	if severityOrder["MEDIUM"] >= severityOrder["LOW"] {
		t.Error("MEDIUM should have lower order than LOW")
	}
}

func TestStubTool(t *testing.T) {
	tool := stubTool("test_tool", "A test tool", "Not available")
	if tool.Name != "test_tool" {
		t.Errorf("expected name 'test_tool', got %q", tool.Name)
	}
	if tool.Description != "A test tool" {
		t.Errorf("expected description 'A test tool', got %q", tool.Description)
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
