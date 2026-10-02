package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/redhat"
)

func TestPluginID(t *testing.T) {
	p := newPlugin(options{})
	if p.ID != "rhdp-provisioner" {
		t.Errorf("got %q, want %q", p.ID, "rhdp-provisioner")
	}
}

func TestToolDefinitions_Unconfigured(t *testing.T) {
	p := newPlugin(options{})
	if len(p.Tools) != 4 {
		t.Fatalf("got %d tools, want 4", len(p.Tools))
	}
	wantNames := []string{"rhdp_search", "rhdp_provision", "rhdp_status", "rhdp_list_active"}
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
		name       string
		raw        map[string]any
		token      string
		rhdpApiURL string
	}{
		{
			name:       "extracts all fields",
			raw:        map[string]any{"sessionCookie": "tok-abc", "rhdpApiUrl": "http://custom:8080"},
			token:      "tok-abc",
			rhdpApiURL: "http://custom:8080",
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
			if opts.SessionCookie != tt.token {
				t.Errorf("ConsoleOfflineToken = %q, want %q", opts.SessionCookie, tt.token)
			}
			if opts.RHDPApiURL != tt.rhdpApiURL {
				t.Errorf("RHDPApiURL = %q, want %q", opts.RHDPApiURL, tt.rhdpApiURL)
			}
		})
	}
}

func TestFormatCatalogItem(t *testing.T) {
	item := catalogItem{
		ID:            "item-1",
		Name:          "OpenShift Workshop",
		Description:   "Hands-on OpenShift workshop",
		Category:      "workshop",
		Provider:      "Red Hat",
		EstimatedTime: "90 minutes",
	}
	got := formatCatalogItem(item)
	for _, want := range []string{"OpenShift Workshop", "Hands-on OpenShift workshop", "workshop", "90 minutes"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q in: %q", want, got)
		}
	}
}

func TestFormatProvisionStatus(t *testing.T) {
	tests := []struct {
		name     string
		status   *provisionStatus
		contains []string
	}{
		{
			name: "basic status",
			status: &provisionStatus{
				OrderID:   "ord-123",
				Status:    "provisioning",
				StartedAt: "2026-01-01T00:00:00Z",
			},
			contains: []string{"ord-123", "provisioning", "2026-01-01T00:00:00Z"},
		},
		{
			name: "ready with credentials",
			status: &provisionStatus{
				OrderID:    "ord-456",
				Status:     "ready",
				StartedAt:  "2026-01-01T00:00:00Z",
				ConsoleURL: "https://console.example.com",
				APIURL:     "https://api.example.com:6443",
				Credentials: &provisionCredentials{
					Username: "admin",
					Password: "secret123",
				},
				ExpiresAt: "2026-01-03T00:00:00Z",
			},
			contains: []string{"ord-456", "ready", "https://console.example.com", "admin", "REDACTED", "Expires"},
		},
		{
			name: "with error",
			status: &provisionStatus{
				OrderID:   "ord-789",
				Status:    "failed",
				StartedAt: "2026-01-01T00:00:00Z",
				Error:     "quota exceeded",
			},
			contains: []string{"failed", "quota exceeded"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatProvisionStatus(tt.status)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q in: %q", want, got)
				}
			}
		})
	}
}

func TestFormatActiveEnvironment(t *testing.T) {
	env := activeEnvironment{
		OrderID:         "ord-123",
		CatalogItemName: "OpenShift Workshop",
		Status:          "running",
		ConsoleURL:      "https://console.example.com",
		ExpiresAt:       "2026-01-03T00:00:00Z",
		StartedAt:       "2026-01-01T00:00:00Z",
	}
	got := formatActiveEnvironment(env)
	for _, want := range []string{"OpenShift Workshop", "running", "https://console.example.com", "Expires"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q in: %q", want, got)
		}
	}
}

func newMockRHDPClient(handler http.Handler) (*rhdpClient, *httptest.Server) {
	srv := httptest.NewServer(handler)
	return &rhdpClient{
		api: redhat.NewAPIClient(redhat.APIClientConfig{BaseURL: srv.URL}),
	}, srv
}

func TestRHDPClient_SearchCatalog(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/catalog/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Query().Get("q") != "openshift" {
			http.Error(w, "unexpected query", http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, `{"items":[{"id":"item-1","name":"OpenShift Workshop","description":"Hands-on OCP workshop","category":"workshop","estimatedTime":"90 minutes"}]}`)
	})
	client, srv := newMockRHDPClient(mux)
	defer srv.Close()

	items, err := client.searchCatalog(context.Background(), "openshift", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	if items[0].Name != "OpenShift Workshop" {
		t.Errorf("name = %q, want %q", items[0].Name, "OpenShift Workshop")
	}
	if items[0].Category != "workshop" {
		t.Errorf("category = %q, want %q", items[0].Category, "workshop")
	}
}

func TestRHDPClient_SearchCatalog_Error(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	})
	client, srv := newMockRHDPClient(mux)
	defer srv.Close()

	_, err := client.searchCatalog(context.Background(), "test", "")
	if err == nil {
		t.Fatal("expected error for server error response")
	}
}

func TestRHDPClient_Provision(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/orders", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		fmt.Fprint(w, `{"orderId":"ord-123","status":"provisioning","startedAt":"2026-01-01T00:00:00Z"}`)
	})
	client, srv := newMockRHDPClient(mux)
	defer srv.Close()

	status, err := client.provision(context.Background(), "item-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.OrderID != "ord-123" {
		t.Errorf("orderID = %q, want %q", status.OrderID, "ord-123")
	}
	if status.Status != "provisioning" {
		t.Errorf("status = %q, want %q", status.Status, "provisioning")
	}
}

func TestRHDPClient_Provision_Error(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	})
	client, srv := newMockRHDPClient(mux)
	defer srv.Close()

	_, err := client.provision(context.Background(), "item-1")
	if err == nil {
		t.Fatal("expected error for server error response")
	}
}

func TestRHDPClient_GetStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/orders/ord-123", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		fmt.Fprint(w, `{"orderId":"ord-123","status":"ready","startedAt":"2026-01-01T00:00:00Z","consoleUrl":"https://console.example.com","apiUrl":"https://api.example.com:6443","expiresAt":"2026-01-03T00:00:00Z"}`)
	})
	client, srv := newMockRHDPClient(mux)
	defer srv.Close()

	status, err := client.getStatus(context.Background(), "ord-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Status != "ready" {
		t.Errorf("status = %q, want %q", status.Status, "ready")
	}
	if status.ConsoleURL != "https://console.example.com" {
		t.Errorf("consoleURL = %q, want %q", status.ConsoleURL, "https://console.example.com")
	}
}

func TestRHDPClient_GetStatus_Error(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})
	client, srv := newMockRHDPClient(mux)
	defer srv.Close()

	_, err := client.getStatus(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}

func TestRHDPClient_ListActive(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/environments", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		fmt.Fprint(w, `{"environments":[{"orderId":"ord-1","catalogItemName":"Workshop A","status":"running","consoleUrl":"https://console.example.com","expiresAt":"2026-01-03T00:00:00Z","startedAt":"2026-01-01T00:00:00Z"}]}`)
	})
	client, srv := newMockRHDPClient(mux)
	defer srv.Close()

	envs, err := client.listActive(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(envs) != 1 {
		t.Fatalf("got %d envs, want 1", len(envs))
	}
	if envs[0].CatalogItemName != "Workshop A" {
		t.Errorf("name = %q, want %q", envs[0].CatalogItemName, "Workshop A")
	}
	if envs[0].Status != "running" {
		t.Errorf("status = %q, want %q", envs[0].Status, "running")
	}
}

func TestRHDPClient_ListActive_Error(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	})
	client, srv := newMockRHDPClient(mux)
	defer srv.Close()

	_, err := client.listActive(context.Background())
	if err == nil {
		t.Fatal("expected error for server error response")
	}
}
