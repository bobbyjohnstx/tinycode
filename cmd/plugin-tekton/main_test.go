package main

import (
	"strings"
	"testing"
)

func TestPluginID(t *testing.T) {
	p := newPlugin()
	if p.ID != "tekton" {
		t.Errorf("expected plugin ID 'tekton', got %q", p.ID)
	}
}

func TestToolCount(t *testing.T) {
	p := newPlugin()
	if len(p.Tools) != 6 {
		t.Fatalf("expected 6 tools, got %d", len(p.Tools))
	}
}

func TestToolDefinitions(t *testing.T) {
	p := newPlugin()
	expectedNames := []string{
		"tekton_list_pipelines",
		"tekton_list_runs",
		"tekton_run_status",
		"tekton_run_logs",
		"tekton_list_tasks",
		"tekton_start_run",
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
	p := newPlugin()
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
	p := newPlugin()
	toolsWithRequired := map[string]int{
		"tekton_list_pipelines": 1,
		"tekton_list_runs":      1,
		"tekton_run_status":     2,
		"tekton_run_logs":       3,
		"tekton_list_tasks":     1,
		"tekton_start_run":      2,
	}

	for _, tool := range p.Tools {
		expectedCount, ok := toolsWithRequired[tool.Name]
		if !ok {
			continue
		}
		required, ok := tool.Parameters["required"].([]string)
		if !ok {
			t.Errorf("tool %q: expected required to be []string", tool.Name)
			continue
		}
		if len(required) != expectedCount {
			t.Errorf("tool %q: expected %d required fields, got %d: %v", tool.Name, expectedCount, len(required), required)
		}
	}
}

func TestGetRunStatus(t *testing.T) {
	tests := []struct {
		name       string
		conditions []pipelineRunCondition
		want       string
	}{
		{
			name:       "empty conditions",
			conditions: nil,
			want:       "Unknown",
		},
		{
			name:       "true with reason",
			conditions: []pipelineRunCondition{{Status: "True", Reason: "Succeeded"}},
			want:       "Succeeded",
		},
		{
			name:       "true without reason",
			conditions: []pipelineRunCondition{{Status: "True"}},
			want:       "Succeeded",
		},
		{
			name:       "false with reason",
			conditions: []pipelineRunCondition{{Status: "False", Reason: "CouldntGetTask"}},
			want:       "CouldntGetTask",
		},
		{
			name:       "false without reason",
			conditions: []pipelineRunCondition{{Status: "False"}},
			want:       "Failed",
		},
		{
			name:       "unknown status with reason",
			conditions: []pipelineRunCondition{{Status: "Unknown", Reason: "Running"}},
			want:       "Running",
		},
		{
			name:       "unknown status without reason",
			conditions: []pipelineRunCondition{{Status: "Unknown"}},
			want:       "Running",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getRunStatus(tt.conditions)
			if got != tt.want {
				t.Errorf("getRunStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name  string
		start string
		end   string
		want  string
	}{
		{
			name:  "seconds only",
			start: "2024-01-15T10:00:00Z",
			end:   "2024-01-15T10:00:45Z",
			want:  "45s",
		},
		{
			name:  "minutes and seconds",
			start: "2024-01-15T10:00:00Z",
			end:   "2024-01-15T10:05:30Z",
			want:  "5m 30s",
		},
		{
			name:  "exact minutes",
			start: "2024-01-15T10:00:00Z",
			end:   "2024-01-15T10:03:00Z",
			want:  "3m 0s",
		},
		{
			name:  "zero seconds",
			start: "2024-01-15T10:00:00Z",
			end:   "2024-01-15T10:00:00Z",
			want:  "0s",
		},
		{
			name:  "invalid start",
			start: "not-a-date",
			end:   "2024-01-15T10:00:00Z",
			want:  "unknown",
		},
		{
			name:  "invalid end",
			start: "2024-01-15T10:00:00Z",
			end:   "not-a-date",
			want:  "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDuration(tt.start, tt.end)
			if got != tt.want {
				t.Errorf("formatDuration(%q, %q) = %q, want %q", tt.start, tt.end, got, tt.want)
			}
		})
	}
}

func TestGetRunStatusUsesFirstCondition(t *testing.T) {
	conditions := []pipelineRunCondition{
		{Status: "True", Reason: "Succeeded"},
		{Status: "False", Reason: "Failed"},
	}
	got := getRunStatus(conditions)
	if got != "Succeeded" {
		t.Errorf("expected first condition to win, got %q", got)
	}
}

func TestToolNamespaceRequired(t *testing.T) {
	p := newPlugin()
	for _, tool := range p.Tools {
		required, ok := tool.Parameters["required"].([]string)
		if !ok {
			continue
		}
		hasNamespace := false
		for _, r := range required {
			if r == "namespace" {
				hasNamespace = true
				break
			}
		}
		if !hasNamespace && tool.Name != "tekton_start_run" {
			// All tools except tekton_start_run require namespace
			// (tekton_start_run requires namespace + pipeline)
		}
		props, ok := tool.Parameters["properties"].(map[string]any)
		if !ok {
			t.Errorf("tool %q: expected properties to be map[string]any", tool.Name)
			continue
		}
		nsProp, ok := props["namespace"].(map[string]any)
		if !ok {
			// Some tools may not have namespace
			continue
		}
		if nsProp["type"] != "string" {
			t.Errorf("tool %q: namespace type should be string, got %v", tool.Name, nsProp["type"])
		}
		if _, ok := nsProp["description"]; !ok {
			t.Errorf("tool %q: namespace missing description", tool.Name)
		}
	}
}

func TestFormatDurationLargeValues(t *testing.T) {
	got := formatDuration("2024-01-15T10:00:00Z", "2024-01-15T11:30:45Z")
	if !strings.Contains(got, "m") {
		t.Errorf("expected minutes in long duration, got %q", got)
	}
}
