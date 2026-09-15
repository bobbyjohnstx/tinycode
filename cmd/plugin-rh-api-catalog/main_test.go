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
	if p.ID != "rh-api-catalog" {
		t.Errorf("got %q, want %q", p.ID, "rh-api-catalog")
	}
}

func TestToolDefinitions(t *testing.T) {
	p := newPlugin(options{})
	if len(p.Tools) != 3 {
		t.Fatalf("got %d tools, want 3", len(p.Tools))
	}
	wantNames := []string{"rh_api_list", "rh_api_spec", "rh_api_endpoints"}
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
		name        string
		raw         map[string]any
		token       string
		catalogPath string
	}{
		{
			name:        "extracts all fields",
			raw:         map[string]any{"consoleOfflineToken": "tok-abc", "catalogPath": "/tmp/specs"},
			token:       "tok-abc",
			catalogPath: "/tmp/specs",
		},
		{
			name:  "empty map",
			raw:   map[string]any{},
			token: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := parseOptions(tt.raw)
			if opts.ConsoleOfflineToken != tt.token {
				t.Errorf("ConsoleOfflineToken = %q, want %q", opts.ConsoleOfflineToken, tt.token)
			}
			if opts.CatalogPath != tt.catalogPath {
				t.Errorf("CatalogPath = %q, want %q", opts.CatalogPath, tt.catalogPath)
			}
		})
	}
}

func TestSearchCatalog(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		wantMin int
	}{
		{
			name:    "search for cost",
			query:   "cost",
			wantMin: 1,
		},
		{
			name:    "search for vulnerability",
			query:   "vulnerability",
			wantMin: 1,
		},
		{
			name:    "no match",
			query:   "zzz-nonexistent-zzz",
			wantMin: 0,
		},
		{
			name:    "platform filter ocp",
			query:   "ocp",
			wantMin: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := searchCatalog(tt.query)
			if len(results) < tt.wantMin {
				t.Errorf("searchCatalog(%q) returned %d results, want at least %d", tt.query, len(results), tt.wantMin)
			}
		})
	}
}

func TestFindCatalogEntry(t *testing.T) {
	t.Run("existing entry", func(t *testing.T) {
		entry := findCatalogEntry("cost-management")
		if entry == nil {
			t.Fatal("expected non-nil entry")
		}
		if entry.Name != "cost-management" {
			t.Errorf("got name %q", entry.Name)
		}
	})

	t.Run("non-existent entry", func(t *testing.T) {
		entry := findCatalogEntry("nonexistent")
		if entry != nil {
			t.Errorf("expected nil, got %v", entry)
		}
	})
}

func TestParseOpenAPIEndpoints(t *testing.T) {
	spec := map[string]any{
		"paths": map[string]any{
			"/api/v1/users": map[string]any{
				"get": map[string]any{
					"summary": "List users",
					"parameters": []any{
						map[string]any{"name": "limit"},
						map[string]any{"name": "offset"},
					},
				},
				"post": map[string]any{
					"description": "Create user",
				},
			},
		},
	}

	endpoints := parseOpenAPIEndpoints(spec)
	if len(endpoints) != 2 {
		t.Fatalf("got %d endpoints, want 2", len(endpoints))
	}

	var getEndpoint *apiEndpoint
	for i := range endpoints {
		if endpoints[i].Method == "GET" {
			getEndpoint = &endpoints[i]
		}
	}
	if getEndpoint == nil {
		t.Fatal("missing GET endpoint")
	}
	if getEndpoint.Description != "List users" {
		t.Errorf("GET description = %q", getEndpoint.Description)
	}
	if len(getEndpoint.Params) != 2 {
		t.Errorf("GET params count = %d, want 2", len(getEndpoint.Params))
	}
}

func TestFormatEndpoints(t *testing.T) {
	endpoints := []apiEndpoint{
		{Method: "GET", Path: "/users", Description: "List users", Params: []string{"limit"}},
		{Method: "POST", Path: "/users", Description: "Create user"},
	}

	t.Run("no filter", func(t *testing.T) {
		got := formatEndpoints(endpoints, "")
		if !strings.Contains(got, "GET /users") {
			t.Error("missing GET endpoint")
		}
		if !strings.Contains(got, "POST /users") {
			t.Error("missing POST endpoint")
		}
	})

	t.Run("filter by method", func(t *testing.T) {
		got := formatEndpoints(endpoints, "POST")
		if !strings.Contains(got, "POST") {
			t.Error("missing filtered POST endpoint")
		}
	})

	t.Run("no match", func(t *testing.T) {
		got := formatEndpoints(endpoints, "DELETE")
		if !strings.Contains(got, "No matching endpoints") {
			t.Errorf("expected no match message, got: %q", got)
		}
	})
}

func TestFormatSpec(t *testing.T) {
	t.Run("small spec", func(t *testing.T) {
		spec := map[string]any{"info": map[string]any{"title": "Test API"}}
		got := formatSpec(spec)
		if !strings.Contains(got, "Test API") {
			t.Errorf("output missing title")
		}
	})

	t.Run("large spec truncated", func(t *testing.T) {
		spec := map[string]any{"data": strings.Repeat("x", 10000)}
		got := formatSpec(spec)
		if !strings.Contains(got, "truncated") {
			t.Error("expected truncation message")
		}
	})
}

func newMockConsoleClient(handler http.Handler) (*redhat.APIClient, *httptest.Server) {
	srv := httptest.NewServer(handler)
	return redhat.NewAPIClient(redhat.APIClientConfig{BaseURL: srv.URL}), srv
}

func TestResolveSpec_ViaClient(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/cost-management/v1/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		fmt.Fprint(w, `{"openapi":"3.0.0","info":{"title":"Cost Management","version":"1.0"},"paths":{}}`)
	})
	client, srv := newMockConsoleClient(mux)
	defer srv.Close()

	spec, err := resolveSpec(context.Background(), client, "", "cost-management")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec == nil {
		t.Fatal("expected non-nil spec")
	}
	info, ok := spec["info"].(map[string]any)
	if !ok {
		t.Fatal("expected info object in spec")
	}
	if info["title"] != "Cost Management" {
		t.Errorf("title = %v, want %q", info["title"], "Cost Management")
	}
}

func TestResolveSpec_ViaClient_Error(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	})
	client, srv := newMockConsoleClient(mux)
	defer srv.Close()

	_, err := resolveSpec(context.Background(), client, "", "cost-management")
	if err == nil {
		t.Fatal("expected error for server error response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error = %q, want to contain '500'", err.Error())
	}
}

func TestResolveSpec_UnknownAPI(t *testing.T) {
	client, srv := newMockConsoleClient(http.NewServeMux())
	defer srv.Close()

	_, err := resolveSpec(context.Background(), client, "", "nonexistent-api")
	if err == nil {
		t.Fatal("expected error for unknown API")
	}
	if !strings.Contains(err.Error(), "unknown API") {
		t.Errorf("error = %q, want to contain 'unknown API'", err.Error())
	}
}
