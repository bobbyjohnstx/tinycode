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
	if p.ID != "ocp-obs-logging" {
		t.Errorf("got %q, want %q", p.ID, "ocp-obs-logging")
	}
}

func TestToolDefinitions_Unconfigured(t *testing.T) {
	p := newPlugin(options{})
	if len(p.Tools) != 6 {
		t.Fatalf("got %d tools, want 6", len(p.Tools))
	}
	wantNames := []string{
		"obs_logs", "obs_traces", "obs_trace_detail",
		"obs_flow_collectors", "obs_dashboards", "logging_health",
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

func TestToolDefinitions_FullyConfigured(t *testing.T) {
	p := newPlugin(options{LokiURL: "http://loki:3100", TempoURL: "http://tempo:3200", Token: "tok"})
	if len(p.Tools) != 6 {
		t.Fatalf("got %d tools, want 6", len(p.Tools))
	}
}

func TestToolSchemas(t *testing.T) {
	p := newPlugin(options{})
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
		name    string
		raw     map[string]any
		lokiURL string
		tempoURL string
		token   string
	}{
		{
			name:     "extracts all fields",
			raw:      map[string]any{"lokiUrl": "http://loki:3100", "tempoUrl": "http://tempo:3200", "token": "tok"},
			lokiURL:  "http://loki:3100",
			tempoURL: "http://tempo:3200",
			token:    "tok",
		},
		{
			name:     "empty map",
			raw:      map[string]any{},
			lokiURL:  "",
			tempoURL: "",
			token:    "",
		},
		{
			name:    "wrong types ignored",
			raw:     map[string]any{"lokiUrl": 123},
			lokiURL: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := parseOptions(tt.raw)
			if opts.LokiURL != tt.lokiURL {
				t.Errorf("LokiURL = %q, want %q", opts.LokiURL, tt.lokiURL)
			}
			if opts.TempoURL != tt.tempoURL {
				t.Errorf("TempoURL = %q, want %q", opts.TempoURL, tt.tempoURL)
			}
			if opts.Token != tt.token {
				t.Errorf("Token = %q, want %q", opts.Token, tt.token)
			}
		})
	}
}

func TestFormatLogEntries(t *testing.T) {
	tests := []struct {
		name     string
		entries  []logEntry
		contains []string
	}{
		{
			name:     "empty entries",
			entries:  nil,
			contains: []string{"No log entries found"},
		},
		{
			name: "single entry",
			entries: []logEntry{
				{Timestamp: "1234567890", Line: "error happened", Labels: map[string]string{"namespace": "prod"}},
			},
			contains: []string{"Log entries: 1", "1234567890", "error happened", "namespace"},
		},
		{
			name: "multiple entries",
			entries: []logEntry{
				{Timestamp: "1", Line: "line1", Labels: map[string]string{}},
				{Timestamp: "2", Line: "line2", Labels: map[string]string{}},
			},
			contains: []string{"Log entries: 2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatLogEntries(tt.entries)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q in:\n%s", want, got)
				}
			}
		})
	}
}

func TestBuildLogQL(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		pod       string
		severity  string
		want      string
	}{
		{
			name: "no filters",
			want: `{job=~".+"}`,
		},
		{
			name:      "namespace only",
			namespace: "prod",
			want:      `{namespace="prod"}`,
		},
		{
			name:      "namespace and pod",
			namespace: "prod",
			pod:       "app-1",
			want:      `{namespace="prod", pod="app-1"}`,
		},
		{
			name:     "severity filter",
			severity: "error",
			want:     `{job=~".+"} |= "error"`,
		},
		{
			name:      "all filters",
			namespace: "ns",
			pod:       "pod-1",
			severity:  "warning",
			want:      `{namespace="ns", pod="pod-1"} |= "warning"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildLogQL(tt.namespace, tt.pod, tt.severity)
			if got != tt.want {
				t.Errorf("buildLogQL(%q, %q, %q) = %q, want %q",
					tt.namespace, tt.pod, tt.severity, got, tt.want)
			}
		})
	}
}

func TestFormatSpanTree(t *testing.T) {
	t.Run("single span", func(t *testing.T) {
		spans := []span{
			{ServiceName: "api", OperationName: "GET /users", Duration: 150},
		}
		got := formatSpanTree(spans, 0)
		if !strings.Contains(got, "[150ms] api:GET /users") {
			t.Errorf("unexpected output: %q", got)
		}
	})

	t.Run("nested spans", func(t *testing.T) {
		spans := []span{
			{
				ServiceName:   "api",
				OperationName: "GET /users",
				Duration:      200,
				Children: []span{
					{ServiceName: "db", OperationName: "SELECT", Duration: 50},
				},
			},
		}
		got := formatSpanTree(spans, 0)
		if !strings.Contains(got, "[200ms] api:GET /users") {
			t.Errorf("missing parent span")
		}
		if !strings.Contains(got, "  [50ms] db:SELECT") {
			t.Errorf("missing indented child span")
		}
	})

	t.Run("indentation", func(t *testing.T) {
		spans := []span{
			{ServiceName: "svc", OperationName: "op", Duration: 10},
		}
		got := formatSpanTree(spans, 2)
		if !strings.HasPrefix(got, "    ") {
			t.Errorf("expected 4-space indent for level 2, got: %q", got)
		}
	})
}

// --- httptest-based client method tests ---

func newMockLokiClient(handler http.Handler) (*lokiClient, *httptest.Server) {
	srv := httptest.NewServer(handler)
	return &lokiClient{
		api: redhat.NewAPIClient(redhat.APIClientConfig{BaseURL: srv.URL}),
	}, srv
}

func newMockTempoClient(handler http.Handler) (*tempoClient, *httptest.Server) {
	srv := httptest.NewServer(handler)
	return &tempoClient{
		api: redhat.NewAPIClient(redhat.APIClientConfig{BaseURL: srv.URL}),
	}, srv
}

func TestLokiQuery(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/loki/api/v1/query_range", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("query") != `{namespace="prod"}` {
				t.Errorf("unexpected query param: %q", r.URL.Query().Get("query"))
			}
			fmt.Fprint(w, `{"data":{"result":[{"stream":{"namespace":"prod","pod":"app-1"},"values":[["1234567890","error in handler"],["1234567891","connection reset"]]}]}}`)
		})
		client, srv := newMockLokiClient(mux)
		defer srv.Close()

		entries, err := client.query(context.Background(), `{namespace="prod"}`, 0, "", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(entries) != 2 {
			t.Fatalf("got %d entries, want 2", len(entries))
		}
		if entries[0].Timestamp != "1234567890" || entries[0].Line != "error in handler" {
			t.Errorf("entry[0] = %+v, unexpected", entries[0])
		}
		if entries[0].Labels["namespace"] != "prod" {
			t.Errorf("entry[0] labels = %+v, want namespace=prod", entries[0].Labels)
		}
	})

	t.Run("with limit and time range", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/loki/api/v1/query_range", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("limit") != "10" {
				t.Errorf("expected limit=10, got %q", r.URL.Query().Get("limit"))
			}
			if r.URL.Query().Get("start") != "2026-01-01T00:00:00Z" {
				t.Errorf("expected start param, got %q", r.URL.Query().Get("start"))
			}
			fmt.Fprint(w, `{"data":{"result":[]}}`)
		})
		client, srv := newMockLokiClient(mux)
		defer srv.Close()

		entries, err := client.query(context.Background(), `{job=~".+"}`, 10, "2026-01-01T00:00:00Z", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(entries) != 0 {
			t.Errorf("got %d entries, want 0", len(entries))
		}
	})

	t.Run("server error", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/loki/api/v1/query_range", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "bad request", http.StatusBadRequest)
		})
		client, srv := newMockLokiClient(mux)
		defer srv.Close()

		_, err := client.query(context.Background(), "bad query", 0, "", "")
		if err == nil {
			t.Fatal("expected error for 400 response")
		}
	})
}

func TestTempoSearchTraces(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("service.name") != "api-gateway" {
				t.Errorf("expected service.name=api-gateway, got %q", r.URL.Query().Get("service.name"))
			}
			fmt.Fprint(w, `{"traces":[{"traceID":"abc123","rootServiceName":"api-gateway","rootTraceName":"GET /users","durationMs":150,"spanCount":5}]}`)
		})
		client, srv := newMockTempoClient(mux)
		defer srv.Close()

		traces, err := client.searchTraces(context.Background(), "api-gateway", "", "", 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(traces) != 1 {
			t.Fatalf("got %d traces, want 1", len(traces))
		}
		if traces[0].TraceID != "abc123" || traces[0].DurationMs != 150 {
			t.Errorf("trace = %+v, unexpected", traces[0])
		}
	})

	t.Run("with filters", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("name") != "GET /users" {
				t.Errorf("expected name filter, got %q", r.URL.Query().Get("name"))
			}
			if r.URL.Query().Get("minDuration") != "500ms" {
				t.Errorf("expected minDuration=500ms, got %q", r.URL.Query().Get("minDuration"))
			}
			if r.URL.Query().Get("limit") != "5" {
				t.Errorf("expected limit=5, got %q", r.URL.Query().Get("limit"))
			}
			fmt.Fprint(w, `{"traces":[]}`)
		})
		client, srv := newMockTempoClient(mux)
		defer srv.Close()

		traces, err := client.searchTraces(context.Background(), "svc", "GET /users", "500ms", 5)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(traces) != 0 {
			t.Errorf("got %d traces, want 0", len(traces))
		}
	})

	t.Run("server error", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		})
		client, srv := newMockTempoClient(mux)
		defer srv.Close()

		_, err := client.searchTraces(context.Background(), "svc", "", "", 0)
		if err == nil {
			t.Fatal("expected error for 401 response")
		}
	})
}

func TestTempoGetTrace(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/traces/abc123", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"traceID":"abc123","spans":[{"traceID":"abc123","spanID":"s1","operationName":"GET /users","serviceName":"api","duration":150,"startTime":1234567890}]}`)
		})
		client, srv := newMockTempoClient(mux)
		defer srv.Close()

		detail, err := client.getTrace(context.Background(), "abc123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if detail.TraceID != "abc123" {
			t.Errorf("traceID = %q, want %q", detail.TraceID, "abc123")
		}
		if len(detail.Spans) != 1 || detail.Spans[0].OperationName != "GET /users" {
			t.Errorf("unexpected spans: %+v", detail.Spans)
		}
	})

	t.Run("server error", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/traces/bad-id", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "not found", http.StatusNotFound)
		})
		client, srv := newMockTempoClient(mux)
		defer srv.Close()

		_, err := client.getTrace(context.Background(), "bad-id")
		if err == nil {
			t.Fatal("expected error for 404 response")
		}
	})
}
