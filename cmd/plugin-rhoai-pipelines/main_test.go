package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/redhat"
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

// --- httptest mock-based tests (issue #152) ---

func newMockPipelineClient(handler http.Handler) (*pipelineClient, *httptest.Server) {
	srv := httptest.NewServer(handler)
	return &pipelineClient{
		api:       redhat.NewAPIClient(redhat.APIClientConfig{BaseURL: srv.URL}),
		apiPrefix: "/apis/v2beta1",
	}, srv
}

func TestListPipelines_Mock(t *testing.T) {
	client, srv := newMockPipelineClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/apis/v2beta1/pipelines") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"pipelines":[
			{"pipeline_id":"p-1","display_name":"Train","description":"ML training","created_at":"2026-01-01"},
			{"pipeline_id":"p-2","display_name":"Eval","description":"Evaluation","created_at":"2026-01-02"}
		]}`))
	}))
	defer srv.Close()

	pipelines, err := client.listPipelines(context.Background(), "")
	if err != nil {
		t.Fatalf("listPipelines() error: %v", err)
	}
	if len(pipelines) != 2 {
		t.Fatalf("got %d pipelines, want 2", len(pipelines))
	}
	if pipelines[0].PipelineID != "p-1" {
		t.Errorf("pipeline[0].PipelineID = %q, want %q", pipelines[0].PipelineID, "p-1")
	}
	if pipelines[0].DisplayName != "Train" {
		t.Errorf("pipeline[0].DisplayName = %q, want %q", pipelines[0].DisplayName, "Train")
	}
	if pipelines[1].PipelineID != "p-2" {
		t.Errorf("pipeline[1].PipelineID = %q, want %q", pipelines[1].PipelineID, "p-2")
	}
}

func TestListPipelines_WithNamespace(t *testing.T) {
	var gotNamespace string
	client, srv := newMockPipelineClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotNamespace = r.URL.Query().Get("namespace")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"pipelines":[]}`))
	}))
	defer srv.Close()

	_, err := client.listPipelines(context.Background(), "my-ns")
	if err != nil {
		t.Fatalf("listPipelines() error: %v", err)
	}
	if gotNamespace != "my-ns" {
		t.Errorf("namespace query param = %q, want %q", gotNamespace, "my-ns")
	}
}

func TestListPipelines_Empty(t *testing.T) {
	client, srv := newMockPipelineClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"pipelines":[]}`))
	}))
	defer srv.Close()

	pipelines, err := client.listPipelines(context.Background(), "")
	if err != nil {
		t.Fatalf("listPipelines() error: %v", err)
	}
	if len(pipelines) != 0 {
		t.Errorf("got %d pipelines, want 0", len(pipelines))
	}
}

func TestCreateRun_Mock(t *testing.T) {
	var gotPath string
	client, srv := newMockPipelineClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"run_id":"run-abc","display_name":"test-run","state":"PENDING","created_at":"2026-01-01"}`))
	}))
	defer srv.Close()

	run, err := client.createRun(context.Background(), "p-1", nil)
	if err != nil {
		t.Fatalf("createRun() error: %v", err)
	}
	if gotPath != "/apis/v2beta1/runs" {
		t.Errorf("path = %q, want %q", gotPath, "/apis/v2beta1/runs")
	}
	if run.RunID != "run-abc" {
		t.Errorf("RunID = %q, want %q", run.RunID, "run-abc")
	}
	if run.State != "PENDING" {
		t.Errorf("State = %q, want %q", run.State, "PENDING")
	}
}

func TestCreateRun_WithParams(t *testing.T) {
	client, srv := newMockPipelineClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"run_id":"run-xyz","state":"PENDING"}`))
	}))
	defer srv.Close()

	params := map[string]any{"learning_rate": 0.01, "epochs": 10}
	run, err := client.createRun(context.Background(), "p-1", params)
	if err != nil {
		t.Fatalf("createRun() error: %v", err)
	}
	if run.RunID != "run-xyz" {
		t.Errorf("RunID = %q, want %q", run.RunID, "run-xyz")
	}
}

func TestGetRunStatus_Mock(t *testing.T) {
	var gotPath string
	client, srv := newMockPipelineClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"run_id":"run-abc",
			"display_name":"Training Run",
			"state":"SUCCEEDED",
			"created_at":"2026-01-01",
			"finished_at":"2026-01-02",
			"tasks":[
				{"run_id":"run-abc","task_id":"t-1","display_name":"preprocess","state":"SUCCEEDED"},
				{"run_id":"run-abc","task_id":"t-2","display_name":"train","state":"SUCCEEDED"}
			]
		}`))
	}))
	defer srv.Close()

	detail, err := client.getRunStatus(context.Background(), "run-abc")
	if err != nil {
		t.Fatalf("getRunStatus() error: %v", err)
	}
	if gotPath != "/apis/v2beta1/runs/run-abc" {
		t.Errorf("path = %q, want %q", gotPath, "/apis/v2beta1/runs/run-abc")
	}
	if detail.RunID != "run-abc" {
		t.Errorf("RunID = %q, want %q", detail.RunID, "run-abc")
	}
	if detail.State != "SUCCEEDED" {
		t.Errorf("State = %q, want %q", detail.State, "SUCCEEDED")
	}
	if len(detail.Tasks) != 2 {
		t.Fatalf("got %d tasks, want 2", len(detail.Tasks))
	}
	if detail.Tasks[0].DisplayName != "preprocess" {
		t.Errorf("task[0].DisplayName = %q, want %q", detail.Tasks[0].DisplayName, "preprocess")
	}
	if detail.Tasks[1].State != "SUCCEEDED" {
		t.Errorf("task[1].State = %q, want %q", detail.Tasks[1].State, "SUCCEEDED")
	}
}

func TestCreatePipeline_Mock(t *testing.T) {
	var gotPath string
	client, srv := newMockPipelineClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"pipeline_id":"p-new","display_name":"New Pipeline"}`))
	}))
	defer srv.Close()

	p, err := client.createPipeline(context.Background(), "apiVersion: v2beta1\nkind: Pipeline")
	if err != nil {
		t.Fatalf("createPipeline() error: %v", err)
	}
	if gotPath != "/apis/v2beta1/pipelines" {
		t.Errorf("path = %q, want %q", gotPath, "/apis/v2beta1/pipelines")
	}
	if p.PipelineID != "p-new" {
		t.Errorf("PipelineID = %q, want %q", p.PipelineID, "p-new")
	}
	if p.DisplayName != "New Pipeline" {
		t.Errorf("DisplayName = %q, want %q", p.DisplayName, "New Pipeline")
	}
}

func TestPipelineClient_ServerError(t *testing.T) {
	client, srv := newMockPipelineClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal server error"}`))
	}))
	defer srv.Close()

	_, err := client.listPipelines(context.Background(), "")
	if err == nil {
		t.Error("expected error for 500 response, got nil")
	}
}

func TestPipelineClient_InvalidJSON(t *testing.T) {
	client, srv := newMockPipelineClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`not valid json`))
	}))
	defer srv.Close()

	_, err := client.listPipelines(context.Background(), "")
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}
