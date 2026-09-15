package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bobbyjohnstx/tinycode-go/internal/redhat"
)

func TestPluginID(t *testing.T) {
	p := newPlugin(options{})
	if p.ID != "aap-bridge" {
		t.Errorf("got %q, want %q", p.ID, "aap-bridge")
	}
}

func TestToolDefinitions_Unconfigured(t *testing.T) {
	p := newPlugin(options{})
	if len(p.Tools) != 8 {
		t.Fatalf("got %d tools, want 8 (6 stub + lint + health)", len(p.Tools))
	}
	wantNames := []string{
		"aap_list_templates", "aap_launch_job", "aap_job_status",
		"aap_job_output", "aap_list_inventories", "aap_hub_search",
		"aap_lint_playbook", "aap_health",
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
	p := newPlugin(options{ControllerURL: "http://aap:8080", OAuthToken: "tok"})
	if len(p.Tools) != 8 {
		t.Fatalf("got %d tools, want 8", len(p.Tools))
	}
	if p.Hooks.ShellEnv == nil {
		t.Error("expected ShellEnv hook when configured")
	}
}

func TestToolSchemas(t *testing.T) {
	p := newPlugin(options{ControllerURL: "http://aap:8080", OAuthToken: "tok"})
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
		name          string
		raw           map[string]any
		controllerURL string
		oauthToken    string
	}{
		{
			name:          "extracts all fields",
			raw:           map[string]any{"controllerUrl": "http://aap:8080", "oauthToken": "tok-abc"},
			controllerURL: "http://aap:8080",
			oauthToken:    "tok-abc",
		},
		{
			name:          "empty map",
			raw:           map[string]any{},
			controllerURL: "",
			oauthToken:    "",
		},
		{
			name:          "wrong types ignored",
			raw:           map[string]any{"controllerUrl": 123},
			controllerURL: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := parseOptions(tt.raw)
			if opts.ControllerURL != tt.controllerURL {
				t.Errorf("ControllerURL = %q, want %q", opts.ControllerURL, tt.controllerURL)
			}
			if opts.OAuthToken != tt.oauthToken {
				t.Errorf("OAuthToken = %q, want %q", opts.OAuthToken, tt.oauthToken)
			}
		})
	}
}

func TestFormatTemplates(t *testing.T) {
	tests := []struct {
		name      string
		templates []jobTemplate
		contains  []string
	}{
		{
			name:      "empty list",
			templates: nil,
			contains:  []string{"No job templates found"},
		},
		{
			name: "single template",
			templates: []jobTemplate{
				{ID: 1, Name: "Deploy App", Description: "deploys the app", Status: "successful"},
			},
			contains: []string{"Job templates: 1", "#1", "Deploy App", "deploys the app", "successful"},
		},
		{
			name: "no name falls back to unknown",
			templates: []jobTemplate{
				{ID: 2},
			},
			contains: []string{"unknown"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatTemplates(tt.templates)
			for _, want := range tt.contains {
				if !stringContains(got, want) {
					t.Errorf("output missing %q in:\n%s", want, got)
				}
			}
		})
	}
}

func TestFormatJob(t *testing.T) {
	j := &job{
		ID:       42,
		Name:     "deploy-prod",
		Status:   "successful",
		Started:  "2026-01-01T00:00:00Z",
		Finished: "2026-01-01T00:05:00Z",
		Elapsed:  300.5,
	}
	got := formatJob(j)
	for _, want := range []string{"Job #42", "deploy-prod", "successful", "2026-01-01T00:00:00Z", "300.5s"} {
		if !stringContains(got, want) {
			t.Errorf("output missing %q in:\n%s", want, got)
		}
	}
}

func TestFormatInventories(t *testing.T) {
	tests := []struct {
		name     string
		invs     []inventory
		contains []string
	}{
		{
			name:     "empty list",
			invs:     nil,
			contains: []string{"No inventories found"},
		},
		{
			name: "single inventory",
			invs: []inventory{
				{ID: 1, Name: "prod", Description: "production hosts", TotalHosts: 10, HostsWithActiveFailures: 2},
			},
			contains: []string{"Inventories: 1", "#1", "prod", "production hosts", "10 hosts", "2 failures"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatInventories(tt.invs)
			for _, want := range tt.contains {
				if !stringContains(got, want) {
					t.Errorf("output missing %q in:\n%s", want, got)
				}
			}
		})
	}
}

func TestFormatCollections(t *testing.T) {
	tests := []struct {
		name     string
		colls    []collection
		contains []string
	}{
		{
			name:     "empty list",
			colls:    nil,
			contains: []string{"No collections found"},
		},
		{
			name: "with namespace and version",
			colls: []collection{
				{
					Namespace:     &struct{ Name string `json:"name"` }{Name: "ansible"},
					Name:          "netcommon",
					Description:   "network common",
					LatestVersion: &struct{ Version string `json:"version"` }{Version: "5.1.0"},
				},
			},
			contains: []string{"Collections: 1", "ansible.netcommon", "v5.1.0", "network common"},
		},
		{
			name: "nil namespace and version",
			colls: []collection{
				{Name: "test"},
			},
			contains: []string{"unknown.test", "v?"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatCollections(tt.colls)
			for _, want := range tt.contains {
				if !stringContains(got, want) {
					t.Errorf("output missing %q in:\n%s", want, got)
				}
			}
		})
	}
}

func stringContains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// --- httptest-based client method tests ---

func newMockAapClient(handler http.Handler) (*aapClient, *httptest.Server) {
	srv := httptest.NewServer(handler)
	return &aapClient{
		api:       redhat.NewAPIClient(redhat.APIClientConfig{BaseURL: srv.URL}),
		apiPrefix: defaultAPIPrefix,
	}, srv
}

func TestListTemplates(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/v2/job_templates/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"results":[{"id":10,"name":"Deploy","description":"deploy app","status":"successful"},{"id":11,"name":"Cleanup","description":"","status":"failed"}]}`)
		})
		client, srv := newMockAapClient(mux)
		defer srv.Close()

		templates, err := client.listTemplates(context.Background(), "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(templates) != 2 {
			t.Fatalf("got %d templates, want 2", len(templates))
		}
		if templates[0].ID != 10 || templates[0].Name != "Deploy" {
			t.Errorf("template[0] = %+v, want ID=10 Name=Deploy", templates[0])
		}
		if templates[1].Status != "failed" {
			t.Errorf("template[1].Status = %q, want %q", templates[1].Status, "failed")
		}
	})

	t.Run("with search parameter", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/v2/job_templates/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("search") != "deploy" {
				t.Errorf("expected search=deploy, got %q", r.URL.Query().Get("search"))
			}
			fmt.Fprint(w, `{"results":[{"id":10,"name":"Deploy"}]}`)
		})
		client, srv := newMockAapClient(mux)
		defer srv.Close()

		templates, err := client.listTemplates(context.Background(), "deploy")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(templates) != 1 {
			t.Fatalf("got %d templates, want 1", len(templates))
		}
	})

	t.Run("server error", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/v2/job_templates/", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "internal error", http.StatusInternalServerError)
		})
		client, srv := newMockAapClient(mux)
		defer srv.Close()

		_, err := client.listTemplates(context.Background(), "")
		if err == nil {
			t.Fatal("expected error for 500 response")
		}
	})
}

func TestLaunchJob(t *testing.T) {
	t.Run("happy path with job field", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/v2/job_templates/10/launch/", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			fmt.Fprint(w, `{"job":42,"id":42}`)
		})
		client, srv := newMockAapClient(mux)
		defer srv.Close()

		jobID, err := client.launchJob(context.Background(), 10, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if jobID != 42 {
			t.Errorf("got jobID=%d, want 42", jobID)
		}
	})

	t.Run("falls back to id field", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/v2/job_templates/5/launch/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"id":99}`)
		})
		client, srv := newMockAapClient(mux)
		defer srv.Close()

		jobID, err := client.launchJob(context.Background(), 5, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if jobID != 99 {
			t.Errorf("got jobID=%d, want 99", jobID)
		}
	})

	t.Run("server error", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/v2/job_templates/1/launch/", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "forbidden", http.StatusForbidden)
		})
		client, srv := newMockAapClient(mux)
		defer srv.Close()

		_, err := client.launchJob(context.Background(), 1, "")
		if err == nil {
			t.Fatal("expected error for 403 response")
		}
	})
}

func TestGetJobStatus(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/v2/jobs/42/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"id":42,"name":"deploy-prod","status":"successful","started":"2026-01-01T00:00:00Z","finished":"2026-01-01T00:05:00Z","failed":false,"elapsed":300.5}`)
		})
		client, srv := newMockAapClient(mux)
		defer srv.Close()

		j, err := client.getJobStatus(context.Background(), 42)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if j.ID != 42 {
			t.Errorf("got ID=%d, want 42", j.ID)
		}
		if j.Status != "successful" {
			t.Errorf("got Status=%q, want %q", j.Status, "successful")
		}
		if j.Elapsed != 300.5 {
			t.Errorf("got Elapsed=%f, want 300.5", j.Elapsed)
		}
	})

	t.Run("server error", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/v2/jobs/1/", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "not found", http.StatusNotFound)
		})
		client, srv := newMockAapClient(mux)
		defer srv.Close()

		_, err := client.getJobStatus(context.Background(), 1)
		if err == nil {
			t.Fatal("expected error for 404 response")
		}
	})
}

func TestGetJobOutput(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/v2/jobs/42/stdout/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("format") != "txt" {
				t.Errorf("expected format=txt, got %q", r.URL.Query().Get("format"))
			}
			fmt.Fprint(w, "PLAY [all] ***\nok: [host1]")
		})
		client, srv := newMockAapClient(mux)
		defer srv.Close()

		output, err := client.getJobOutput(context.Background(), 42)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !stringContains(output, "PLAY [all]") {
			t.Errorf("output missing expected content: %q", output)
		}
	})

	t.Run("server error", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/v2/jobs/1/stdout/", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "gone", http.StatusGone)
		})
		client, srv := newMockAapClient(mux)
		defer srv.Close()

		_, err := client.getJobOutput(context.Background(), 1)
		if err == nil {
			t.Fatal("expected error for 410 response")
		}
	})
}

func TestListInventories(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/v2/inventories/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"results":[{"id":1,"name":"prod","description":"production hosts","total_hosts":10,"hosts_with_active_failures":2}]}`)
		})
		client, srv := newMockAapClient(mux)
		defer srv.Close()

		invs, err := client.listInventories(context.Background(), "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(invs) != 1 {
			t.Fatalf("got %d inventories, want 1", len(invs))
		}
		if invs[0].Name != "prod" || invs[0].TotalHosts != 10 {
			t.Errorf("inventory = %+v, want Name=prod TotalHosts=10", invs[0])
		}
	})

	t.Run("server error", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/v2/inventories/", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		})
		client, srv := newMockAapClient(mux)
		defer srv.Close()

		_, err := client.listInventories(context.Background(), "")
		if err == nil {
			t.Fatal("expected error for 401 response")
		}
	})
}

func TestSearchCollections(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/v2/collections/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("keyword") != "network" {
				t.Errorf("expected keyword=network, got %q", r.URL.Query().Get("keyword"))
			}
			fmt.Fprint(w, `{"results":[{"namespace":{"name":"ansible"},"name":"netcommon","description":"network common","latest_version":{"version":"5.1.0"}}]}`)
		})
		client, srv := newMockAapClient(mux)
		defer srv.Close()

		colls, err := client.searchCollections(context.Background(), "network")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(colls) != 1 {
			t.Fatalf("got %d collections, want 1", len(colls))
		}
		if colls[0].Name != "netcommon" {
			t.Errorf("collection name = %q, want %q", colls[0].Name, "netcommon")
		}
		if colls[0].Namespace == nil || colls[0].Namespace.Name != "ansible" {
			t.Errorf("collection namespace unexpected: %+v", colls[0].Namespace)
		}
	})

	t.Run("server error", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/v2/collections/", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "bad gateway", http.StatusBadGateway)
		})
		client, srv := newMockAapClient(mux)
		defer srv.Close()

		_, err := client.searchCollections(context.Background(), "anything")
		if err == nil {
			t.Fatal("expected error for 502 response")
		}
	})
}
