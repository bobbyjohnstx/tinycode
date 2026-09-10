package main

import (
	"testing"
)

func TestPluginID(t *testing.T) {
	p := newPlugin(options{})
	if p.ID != "rhoai-pipelines" {
		t.Errorf("got %q, want %q", p.ID, "rhoai-pipelines")
	}
}

func TestToolDefinitions_Unconfigured(t *testing.T) {
	p := newPlugin(options{})
	if len(p.Tools) != 4 {
		t.Fatalf("got %d tools, want 4", len(p.Tools))
	}
	wantNames := []string{
		"rhoai_pipeline_list",
		"rhoai_pipeline_run",
		"rhoai_pipeline_status",
		"rhoai_pipeline_create",
	}
	for i, want := range wantNames {
		if p.Tools[i].Name != want {
			t.Errorf("tool[%d] name = %q, want %q", i, p.Tools[i].Name, want)
		}
		if p.Tools[i].Execute == nil {
			t.Errorf("tool[%d] Execute is nil", i)
		}
	}
}

func TestToolDefinitions_Configured(t *testing.T) {
	p := newPlugin(options{PipelinesURL: "http://localhost:8080"})
	if len(p.Tools) != 4 {
		t.Fatalf("got %d tools, want 4", len(p.Tools))
	}
	for i, tool := range p.Tools {
		if tool.Description == "" {
			t.Errorf("tool[%d] description is empty", i)
		}
		params := tool.Parameters
		if params["type"] != "object" {
			t.Errorf("tool[%d] params type = %v, want %q", i, params["type"], "object")
		}
	}
}

func TestToolSchemas(t *testing.T) {
	p := newPlugin(options{PipelinesURL: "http://localhost:8080"})
	for _, tool := range p.Tools {
		params := tool.Parameters
		if params["type"] != "object" {
			t.Errorf("%s: params type = %v, want %q", tool.Name, params["type"], "object")
		}
		if _, ok := params["properties"].(map[string]any); !ok {
			t.Errorf("%s: properties is not map[string]any", tool.Name)
		}
	}
}

func TestParseOptions(t *testing.T) {
	tests := []struct {
		name         string
		raw          map[string]any
		pipelinesURL string
		namespace    string
		token        string
	}{
		{
			name:         "extracts all fields",
			raw:          map[string]any{"pipelinesUrl": "http://p:8080", "namespace": "ns1", "token": "tok"},
			pipelinesURL: "http://p:8080",
			namespace:    "ns1",
			token:        "tok",
		},
		{
			name:         "empty map",
			raw:          map[string]any{},
			pipelinesURL: "",
			namespace:    "",
			token:        "",
		},
		{
			name:         "wrong types ignored",
			raw:          map[string]any{"pipelinesUrl": 42},
			pipelinesURL: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := parseOptions(tt.raw)
			if opts.PipelinesURL != tt.pipelinesURL {
				t.Errorf("PipelinesURL = %q, want %q", opts.PipelinesURL, tt.pipelinesURL)
			}
			if opts.Namespace != tt.namespace {
				t.Errorf("Namespace = %q, want %q", opts.Namespace, tt.namespace)
			}
			if opts.Token != tt.token {
				t.Errorf("Token = %q, want %q", opts.Token, tt.token)
			}
		})
	}
}

func TestFormatPipelines(t *testing.T) {
	tests := []struct {
		name      string
		pipelines []pipeline
		contains  []string
	}{
		{
			name:      "empty list",
			pipelines: nil,
			contains:  []string{"No pipelines found"},
		},
		{
			name: "single pipeline",
			pipelines: []pipeline{
				{PipelineID: "p-1", DisplayName: "Train Model", Description: "ML training", CreatedAt: "2026-01-01"},
			},
			contains: []string{"Pipelines: 1", "p-1", "Train Model", "ML training", "2026-01-01"},
		},
		{
			name: "no description",
			pipelines: []pipeline{
				{PipelineID: "p-2", DisplayName: "Test"},
			},
			contains: []string{"No description"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatPipelines(tt.pipelines)
			for _, want := range tt.contains {
				if !stringContains(got, want) {
					t.Errorf("output missing %q in:\n%s", want, got)
				}
			}
		})
	}
}

func TestTaskStatusIcon(t *testing.T) {
	tests := []struct {
		state string
		want  string
	}{
		{"SUCCEEDED", "DONE"},
		{"FAILED", "FAIL"},
		{"RUNNING", "RUN"},
		{"PENDING", "WAIT"},
		{"SKIPPED", "SKIP"},
		{"CANCELLED", "CANCEL"},
		{"UNKNOWN", "UNKNOWN"},
	}
	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			if got := taskStatusIcon(tt.state); got != tt.want {
				t.Errorf("taskStatusIcon(%q) = %q, want %q", tt.state, got, tt.want)
			}
		})
	}
}

func TestFormatRunStatus(t *testing.T) {
	t.Run("basic run", func(t *testing.T) {
		detail := &pipelineRunDetail{
			pipelineRun: pipelineRun{
				RunID:       "r-1",
				DisplayName: "Training Run",
				State:       "SUCCEEDED",
				CreatedAt:   "2026-01-01",
				FinishedAt:  "2026-01-02",
			},
			Tasks: []pipelineTask{
				{TaskID: "t-1", DisplayName: "preprocess", State: "SUCCEEDED"},
				{TaskID: "t-2", DisplayName: "train", State: "FAILED"},
			},
		}
		got := formatRunStatus(detail)
		for _, want := range []string{"Training Run", "r-1", "SUCCEEDED", "2026-01-01", "2026-01-02", "Tasks:", "[DONE] preprocess", "[FAIL] train"} {
			if !stringContains(got, want) {
				t.Errorf("output missing %q in:\n%s", want, got)
			}
		}
	})

	t.Run("run with error", func(t *testing.T) {
		detail := &pipelineRunDetail{
			pipelineRun: pipelineRun{
				RunID:       "r-2",
				DisplayName: "Failed Run",
				State:       "FAILED",
				CreatedAt:   "2026-01-01",
				Error:       "out of memory",
			},
		}
		got := formatRunStatus(detail)
		if !stringContains(got, "out of memory") {
			t.Errorf("output missing error message")
		}
	})

	t.Run("task falls back to task ID", func(t *testing.T) {
		detail := &pipelineRunDetail{
			pipelineRun: pipelineRun{
				RunID:       "r-3",
				DisplayName: "Run",
				State:       "RUNNING",
				CreatedAt:   "2026-01-01",
			},
			Tasks: []pipelineTask{
				{TaskID: "task-abc", State: "RUNNING"},
			},
		}
		got := formatRunStatus(detail)
		if !stringContains(got, "task-abc") {
			t.Errorf("expected task ID fallback")
		}
	})
}

func stringContains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
