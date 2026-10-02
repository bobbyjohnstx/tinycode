package main

import (
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/redhat"
)

func TestToolDefinitions(t *testing.T) {
	tools := buildTools()
	if len(tools) != 3 {
		t.Fatalf("got %d tools, want 3", len(tools))
	}
	wantNames := []string{"container_lint", "bootc_validate", "container_base_suggest"}
	for i, want := range wantNames {
		if tools[i].Name != want {
			t.Errorf("tool[%d] name = %q, want %q", i, tools[i].Name, want)
		}
		if tools[i].Description == "" {
			t.Errorf("tool[%d] description is empty", i)
		}
		if tools[i].Execute == nil {
			t.Errorf("tool[%d] Execute is nil", i)
		}
	}
}

func TestToolSchemas(t *testing.T) {
	tools := buildTools()
	for _, tool := range tools {
		params := tool.Parameters
		if params["type"] != "object" {
			t.Errorf("%s: params type = %v, want %q", tool.Name, params["type"], "object")
		}
		if _, ok := params["properties"].(map[string]any); !ok {
			t.Errorf("%s: properties is not map[string]any", tool.Name)
		}
	}
}

func TestIsUbiImage(t *testing.T) {
	tests := []struct {
		image string
		want  bool
	}{
		{"registry.access.redhat.com/ubi9/ubi:9.5", true},
		{"registry.redhat.io/ubi9/ubi-minimal:9.5", true},
		{"docker.io/library/ubuntu:22.04", false},
		{"quay.io/centos/centos:stream9", false},
		{"registry.access.redhat.com/ubi8/openjdk-17:1.18", true},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.image, func(t *testing.T) {
			if got := isUbiImage(tt.image); got != tt.want {
				t.Errorf("isUbiImage(%q) = %v, want %v", tt.image, got, tt.want)
			}
		})
	}
}

func TestRunLintRules(t *testing.T) {
	tests := []struct {
		name          string
		containerfile string
		wantRules     []string
	}{
		{
			name:          "non-UBI base image",
			containerfile: "FROM ubuntu:22.04\nRUN echo hello",
			wantRules:     []string{"non-ubi-base", "missing-user-directive"},
		},
		{
			name:          "latest tag",
			containerfile: "FROM registry.access.redhat.com/ubi9/ubi:latest\nUSER 1001",
			wantRules:     []string{"latest-tag"},
		},
		{
			name:          "no tag implies latest",
			containerfile: "FROM registry.access.redhat.com/ubi9/ubi\nUSER 1001",
			wantRules:     []string{"latest-tag"},
		},
		{
			name:          "root user without subsequent non-root",
			containerfile: "FROM registry.access.redhat.com/ubi9/ubi:9.5\nUSER root\nRUN yum update",
			wantRules:     []string{"root-user"},
		},
		{
			name:          "root user with subsequent non-root is OK",
			containerfile: "FROM registry.access.redhat.com/ubi9/ubi:9.5\nUSER root\nRUN yum update\nUSER 1001",
			wantRules:     []string{},
		},
		{
			name:          "missing labels",
			containerfile: "FROM registry.access.redhat.com/ubi9/ubi:9.5\nUSER 1001",
			wantRules:     []string{"missing-labels"},
		},
		{
			name:          "hardcoded secret in ENV",
			containerfile: "FROM registry.access.redhat.com/ubi9/ubi:9.5\nENV MY_PASSWORD=secret123\nUSER 1001",
			wantRules:     []string{"hardcoded-secret"},
		},
		{
			name:          "hardcoded secret in ARG",
			containerfile: "FROM registry.access.redhat.com/ubi9/ubi:9.5\nARG API_TOKEN\nUSER 1001",
			wantRules:     []string{"hardcoded-secret"},
		},
		{
			name:          "three consecutive RUN instructions",
			containerfile: "FROM registry.access.redhat.com/ubi9/ubi:9.5\nRUN echo 1\nRUN echo 2\nRUN echo 3\nUSER 1001",
			wantRules:     []string{"run-layer-chaining"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed := redhat.ParseContainerfile(tt.containerfile)
			warnings := runLintRules(parsed)
			gotRules := map[string]bool{}
			for _, w := range warnings {
				gotRules[w.rule] = true
			}
			for _, want := range tt.wantRules {
				if !gotRules[want] {
					t.Errorf("expected rule %q to fire, got rules: %v", want, ruleNames(warnings))
				}
			}
		})
	}
}

func ruleNames(warnings []lintWarning) []string {
	var names []string
	for _, w := range warnings {
		names = append(names, w.rule)
	}
	return names
}

func TestFormatWarnings(t *testing.T) {
	t.Run("no warnings", func(t *testing.T) {
		got := formatWarnings(nil)
		if !strings.Contains(got, "No issues found") {
			t.Errorf("expected 'No issues found', got: %q", got)
		}
	})

	t.Run("grouped by severity", func(t *testing.T) {
		warnings := []lintWarning{
			{rule: "root-user", severity: "error", line: 5, message: "root user", suggestion: "fix it"},
			{rule: "latest-tag", severity: "warning", line: 1, message: "latest tag", suggestion: "pin version"},
			{rule: "missing-labels", severity: "info", line: 1, message: "missing labels", suggestion: "add labels"},
		}
		got := formatWarnings(warnings)
		if !strings.Contains(got, "ERRORS (1)") {
			t.Error("missing ERRORS header")
		}
		if !strings.Contains(got, "WARNINGS (1)") {
			t.Error("missing WARNINGS header")
		}
		if !strings.Contains(got, "INFOS (1)") {
			t.Error("missing INFOS header")
		}
	})
}

func TestValidateBootc(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantPass bool
		contains []string
	}{
		{
			name:     "bootc-compatible",
			content:  "FROM registry.redhat.io/rhel9/rhel-bootc:9.5\nLABEL bootc.diskimage-builder=\"true\"\nCOPY myservice.service /etc/systemd/system/",
			wantPass: true,
			contains: []string{"PASS"},
		},
		{
			name:     "non-bootc base",
			content:  "FROM registry.access.redhat.com/ubi9/ubi:9.5\nRUN echo hello",
			wantPass: false,
			contains: []string{"FAIL", "bootc base image"},
		},
		{
			name:     "empty content",
			content:  "",
			wantPass: false,
			contains: []string{"FAIL"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateBootc(tt.content)
			if tt.wantPass && !strings.HasPrefix(got, "PASS") {
				t.Errorf("expected PASS, got:\n%s", got)
			}
			if !tt.wantPass && !strings.HasPrefix(got, "FAIL") {
				t.Errorf("expected FAIL, got:\n%s", got)
			}
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q in:\n%s", want, got)
				}
			}
		})
	}
}

func TestSuggestBaseImage(t *testing.T) {
	tests := []struct {
		useCase  string
		contains string
	}{
		{"Java microservice", "openjdk"},
		{"Python API", "python"},
		{"Node.js application", "nodejs"},
		{"Go CLI tool", "ubi-minimal"},
		{"golang binary", "ubi-minimal"},
		{"minimal container", "ubi-minimal"},
		{"micro static binary", "ubi-micro"},
		{"general purpose", "ubi:9.5"},
	}
	for _, tt := range tests {
		t.Run(tt.useCase, func(t *testing.T) {
			got := suggestBaseImage(tt.useCase)
			if !strings.Contains(strings.ToLower(got), tt.contains) {
				t.Errorf("suggestBaseImage(%q) missing %q in:\n%s", tt.useCase, tt.contains, got)
			}
		})
	}
}
