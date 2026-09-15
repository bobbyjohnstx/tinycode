package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestPluginID(t *testing.T) {
	p := newPlugin()
	if p.ID != "rh-ecosystem-catalog" {
		t.Errorf("got %q, want %q", p.ID, "rh-ecosystem-catalog")
	}
}

func TestToolDefinitions(t *testing.T) {
	p := newPlugin()
	if len(p.Tools) != 3 {
		t.Fatalf("got %d tools, want 3", len(p.Tools))
	}
	wantNames := []string{"ecosystem_search", "ecosystem_operator", "ecosystem_browse"}
	for i, want := range wantNames {
		if p.Tools[i].Name != want {
			t.Errorf("tool[%d] name = %q, want %q", i, p.Tools[i].Name, want)
		}
		if p.Tools[i].Execute == nil {
			t.Errorf("tool[%d] Execute is nil", i)
		}
	}
}

func TestToolSchemas(t *testing.T) {
	p := newPlugin()
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

func TestFormatRepo(t *testing.T) {
	tests := []struct {
		name     string
		repo     pyxisRepo
		contains []string
	}{
		{
			name: "full repo",
			repo: pyxisRepo{
				Repository: "ubi9/ubi",
				Registry:   "registry.access.redhat.com",
				DisplayData: &pyxisDisplayData{
					Name:             "UBI 9",
					ShortDescription: "Universal Base Image 9",
				},
				ApplicationCategories: []string{"base-image"},
				LastUpdateDate:        "2026-01-15T00:00:00Z",
			},
			contains: []string{"registry.access.redhat.com/ubi9/ubi", "UBI 9", "Universal Base Image 9", "base-image", "2026-01-15"},
		},
		{
			name: "minimal repo",
			repo: pyxisRepo{
				Repository: "test/app",
			},
			contains: []string{"registry.redhat.com/test/app"},
		},
		{
			name:     "empty repo",
			repo:     pyxisRepo{},
			contains: []string{"registry.redhat.com/unknown"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatRepo(tt.repo)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q in: %q", want, got)
				}
			}
		})
	}
}

func TestFormatOperator(t *testing.T) {
	tests := []struct {
		name     string
		op       pyxisOperatorBundle
		contains []string
	}{
		{
			name: "full operator",
			op: pyxisOperatorBundle{
				CSVDisplayName: "AMQ Streams",
				Package:        "amq-streams",
				Version:        "2.5.0",
				OCPVersion:     "4.14",
				Organization:   "Red Hat",
				ChannelName:    "stable",
			},
			contains: []string{"AMQ Streams", "amq-streams", "2.5.0", "OCP: 4.14", "Red Hat", "stable"},
		},
		{
			name: "minimal operator",
			op: pyxisOperatorBundle{
				Package: "test-op",
			},
			contains: []string{"test-op", "unknown"},
		},
		{
			name:     "empty operator",
			op:       pyxisOperatorBundle{},
			contains: []string{"unknown"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatOperator(tt.op)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q in: %q", want, got)
				}
			}
		})
	}
}

// urlRewriter redirects HTTP requests to a test server while preserving the path.
type urlRewriter struct {
	target *url.URL
	rt     http.RoundTripper
}

func (u *urlRewriter) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme = u.target.Scheme
	req.URL.Host = u.target.Host
	return u.rt.RoundTrip(req)
}

func withMockPyxis(t *testing.T, handler http.Handler) {
	t.Helper()
	srv := httptest.NewServer(handler)
	target, _ := url.Parse(srv.URL)
	old := httpClient
	httpClient = &http.Client{
		Transport: &urlRewriter{target: target, rt: http.DefaultTransport},
	}
	t.Cleanup(func() {
		httpClient = old
		srv.Close()
	})
}

func TestPyxisGet_HappyPath(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/containers/v1/repositories", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"repository":"ubi9/ubi","registry":"registry.access.redhat.com"}],"total":1,"page":0,"page_size":10}`))
	})
	withMockPyxis(t, mux)

	data, err := pyxisGet("/repositories", map[string]string{"filter": "repository==ubi9/ubi"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result pyxisResponse[pyxisRepo]
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(result.Data) != 1 {
		t.Fatalf("got %d repos, want 1", len(result.Data))
	}
	if result.Data[0].Repository != "ubi9/ubi" {
		t.Errorf("repository = %q, want %q", result.Data[0].Repository, "ubi9/ubi")
	}
}

func TestPyxisGet_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	})
	withMockPyxis(t, mux)

	_, err := pyxisGet("/repositories", nil)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error = %q, want to contain '500'", err.Error())
	}
}

func TestPyxisGet_OperatorBundles(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/containers/v1/operators/bundles", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"csv_display_name":"AMQ Streams","package":"amq-streams","version":"2.5.0"}],"total":1,"page":0,"page_size":5}`))
	})
	withMockPyxis(t, mux)

	data, err := pyxisGet("/operators/bundles", map[string]string{"filter": "package==amq-streams"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result pyxisResponse[pyxisOperatorBundle]
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(result.Data) != 1 {
		t.Fatalf("got %d bundles, want 1", len(result.Data))
	}
	if result.Data[0].Package != "amq-streams" {
		t.Errorf("package = %q, want %q", result.Data[0].Package, "amq-streams")
	}
}
