package redhat

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func newTestMlflowClient(t *testing.T, handler http.HandlerFunc) (*MlflowClient, *httptest.Server) {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	api := NewAPIClient(APIClientConfig{BaseURL: ts.URL})
	return NewMlflowClient(api), ts
}

func TestNewMlflowClient(t *testing.T) {
	api := NewAPIClient(APIClientConfig{BaseURL: "http://localhost"})
	client := NewMlflowClient(api)
	if client == nil {
		t.Fatal("expected non-nil MlflowClient")
	}
	if client.api != api {
		t.Error("expected api field to match")
	}
}

func TestMlflowClient_CreateExperiment(t *testing.T) {
	var gotPath string
	var gotBody map[string]string

	client, _ := newTestMlflowClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"experiment_id":"exp-123"}`))
	})

	id, err := client.CreateExperiment(context.Background(), "my-experiment")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "exp-123" {
		t.Errorf("got experiment_id %q, want %q", id, "exp-123")
	}
	if gotPath != "/api/2.0/mlflow/experiments/create" {
		t.Errorf("got path %q, want %q", gotPath, "/api/2.0/mlflow/experiments/create")
	}
	if gotBody["name"] != "my-experiment" {
		t.Errorf("got body name %q, want %q", gotBody["name"], "my-experiment")
	}
}

func TestMlflowClient_CreateExperiment_Error(t *testing.T) {
	client, _ := newTestMlflowClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	})

	_, err := client.CreateExperiment(context.Background(), "fail")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestMlflowClient_GetExperimentByName(t *testing.T) {
	var gotPath string
	var gotQuery string

	client, _ := newTestMlflowClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query().Get("experiment_name")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"experiment":{"experiment_id":"exp-456"}}`))
	})

	id, err := client.GetExperimentByName(context.Background(), "test-exp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "exp-456" {
		t.Errorf("got experiment_id %q, want %q", id, "exp-456")
	}
	if gotPath != "/api/2.0/mlflow/experiments/get-by-name" {
		t.Errorf("got path %q, want %q", gotPath, "/api/2.0/mlflow/experiments/get-by-name")
	}
	if gotQuery != "test-exp" {
		t.Errorf("got query experiment_name %q, want %q", gotQuery, "test-exp")
	}
}

func TestMlflowClient_CreateRun(t *testing.T) {
	t.Run("without tags", func(t *testing.T) {
		var gotBody map[string]any

		client, _ := newTestMlflowClient(t, func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &gotBody)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"run":{"info":{"run_id":"run-789"}}}`))
		})

		id, err := client.CreateRun(context.Background(), "exp-123", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != "run-789" {
			t.Errorf("got run_id %q, want %q", id, "run-789")
		}
		if gotBody["experiment_id"] != "exp-123" {
			t.Errorf("got experiment_id %v, want %q", gotBody["experiment_id"], "exp-123")
		}
		if _, hasTags := gotBody["tags"]; hasTags {
			t.Error("expected no tags key when tags is nil")
		}
	})

	t.Run("with tags", func(t *testing.T) {
		var gotBody map[string]any

		client, _ := newTestMlflowClient(t, func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &gotBody)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"run":{"info":{"run_id":"run-abc"}}}`))
		})

		tags := []RunTag{{Key: "env", Value: "prod"}, {Key: "team", Value: "ml"}}
		id, err := client.CreateRun(context.Background(), "exp-456", tags)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != "run-abc" {
			t.Errorf("got run_id %q, want %q", id, "run-abc")
		}
		rawTags, ok := gotBody["tags"]
		if !ok {
			t.Fatal("expected tags in body")
		}
		tagSlice, ok := rawTags.([]any)
		if !ok || len(tagSlice) != 2 {
			t.Fatalf("expected 2 tags, got %v", rawTags)
		}
	})
}

func TestMlflowClient_LogMetric(t *testing.T) {
	t.Run("without step", func(t *testing.T) {
		var gotBody map[string]any
		var gotPath string

		client, _ := newTestMlflowClient(t, func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &gotBody)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{}`))
		})

		err := client.LogMetric(context.Background(), "run-1", "accuracy", 0.95, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotPath != "/api/2.0/mlflow/runs/log-metric" {
			t.Errorf("got path %q, want %q", gotPath, "/api/2.0/mlflow/runs/log-metric")
		}
		if gotBody["run_id"] != "run-1" {
			t.Errorf("got run_id %v, want %q", gotBody["run_id"], "run-1")
		}
		if gotBody["key"] != "accuracy" {
			t.Errorf("got key %v, want %q", gotBody["key"], "accuracy")
		}
		if gotBody["value"] != 0.95 {
			t.Errorf("got value %v, want %v", gotBody["value"], 0.95)
		}
		if _, hasStep := gotBody["step"]; hasStep {
			t.Error("expected no step key when step is nil")
		}
	})

	t.Run("with step", func(t *testing.T) {
		var gotBody map[string]any

		client, _ := newTestMlflowClient(t, func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &gotBody)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{}`))
		})

		step := 5
		err := client.LogMetric(context.Background(), "run-1", "loss", 0.1, &step)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		gotStep, ok := gotBody["step"]
		if !ok {
			t.Fatal("expected step in body")
		}
		if gotStep != float64(5) {
			t.Errorf("got step %v, want %v", gotStep, 5)
		}
	})
}

func TestMlflowClient_LogParam(t *testing.T) {
	var gotBody map[string]string
	var gotPath string

	client, _ := newTestMlflowClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	})

	err := client.LogParam(context.Background(), "run-1", "lr", "0.001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/api/2.0/mlflow/runs/log-param" {
		t.Errorf("got path %q, want %q", gotPath, "/api/2.0/mlflow/runs/log-param")
	}
	if gotBody["run_id"] != "run-1" {
		t.Errorf("got run_id %q, want %q", gotBody["run_id"], "run-1")
	}
	if gotBody["key"] != "lr" {
		t.Errorf("got key %q, want %q", gotBody["key"], "lr")
	}
	if gotBody["value"] != "0.001" {
		t.Errorf("got value %q, want %q", gotBody["value"], "0.001")
	}
}

func TestMlflowClient_EndRun(t *testing.T) {
	var gotBody map[string]any
	var gotPath string

	client, _ := newTestMlflowClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	})

	err := client.EndRun(context.Background(), "run-1", "FINISHED")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/api/2.0/mlflow/runs/update" {
		t.Errorf("got path %q, want %q", gotPath, "/api/2.0/mlflow/runs/update")
	}
	if gotBody["run_id"] != "run-1" {
		t.Errorf("got run_id %v, want %q", gotBody["run_id"], "run-1")
	}
	if gotBody["status"] != "FINISHED" {
		t.Errorf("got status %v, want %q", gotBody["status"], "FINISHED")
	}
	if _, hasEndTime := gotBody["end_time"]; !hasEndTime {
		t.Error("expected end_time in body")
	}
}

func TestMlflowClient_ErrorPropagation(t *testing.T) {
	var callCount atomic.Int32

	client, _ := newTestMlflowClient(t, func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad request"}`))
	})

	ctx := context.Background()

	_, err := client.CreateExperiment(ctx, "fail")
	if err == nil {
		t.Error("CreateExperiment: expected error")
	}

	_, err = client.GetExperimentByName(ctx, "fail")
	if err == nil {
		t.Error("GetExperimentByName: expected error")
	}

	_, err = client.CreateRun(ctx, "exp-1", nil)
	if err == nil {
		t.Error("CreateRun: expected error")
	}

	err = client.LogMetric(ctx, "run-1", "x", 1.0, nil)
	if err == nil {
		t.Error("LogMetric: expected error")
	}

	err = client.LogParam(ctx, "run-1", "x", "y")
	if err == nil {
		t.Error("LogParam: expected error")
	}

	err = client.EndRun(ctx, "run-1", "FAILED")
	if err == nil {
		t.Error("EndRun: expected error")
	}

	if c := callCount.Load(); c != 6 {
		t.Errorf("expected 6 calls, got %d", c)
	}
}
