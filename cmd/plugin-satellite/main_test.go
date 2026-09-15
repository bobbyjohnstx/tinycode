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
	if p.ID != "satellite" {
		t.Errorf("got %q, want %q", p.ID, "satellite")
	}
}

func TestToolDefinitions_Unconfigured(t *testing.T) {
	p := newPlugin(options{})
	if len(p.Tools) != 11 {
		t.Fatalf("got %d tools, want 11", len(p.Tools))
	}
	wantNames := []string{"satellite_health_check", "satellite_hosts", "satellite_host_facts", "satellite_errata", "satellite_content_views", "satellite_services", "satellite_tasks", "satellite_proxies", "satellite_repositories", "satellite_rex_run", "satellite_rex_result"}
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
	p := newPlugin(options{SatelliteURL: "http://sat:8080", Token: "tok"})
	if len(p.Tools) != 11 {
		t.Fatalf("got %d tools, want 11", len(p.Tools))
	}
}

func TestToolDefinitions_ConfiguredBasicAuth(t *testing.T) {
	p := newPlugin(options{SatelliteURL: "http://sat:8080", Username: "admin", Password: "pass"})
	if len(p.Tools) != 11 {
		t.Fatalf("got %d tools, want 11", len(p.Tools))
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
		name         string
		raw          map[string]any
		satelliteURL string
		username     string
		password     string
		token        string
	}{
		{
			name:         "extracts all fields",
			raw:          map[string]any{"satelliteUrl": "http://sat:8080", "token": "tok-abc"},
			satelliteURL: "http://sat:8080",
			token:        "tok-abc",
		},
		{
			name:         "basic auth fields",
			raw:          map[string]any{"satelliteUrl": "http://sat:8080", "username": "admin", "password": "secret"},
			satelliteURL: "http://sat:8080",
			username:     "admin",
			password:     "secret",
		},
		{
			name:         "empty map",
			raw:          map[string]any{},
			satelliteURL: "",
			token:        "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := parseOptions(tt.raw)
			if opts.SatelliteURL != tt.satelliteURL {
				t.Errorf("SatelliteURL = %q, want %q", opts.SatelliteURL, tt.satelliteURL)
			}
			if opts.Username != tt.username {
				t.Errorf("Username = %q, want %q", opts.Username, tt.username)
			}
			if opts.Password != tt.password {
				t.Errorf("Password = %q, want %q", opts.Password, tt.password)
			}
			if opts.Token != tt.token {
				t.Errorf("Token = %q, want %q", opts.Token, tt.token)
			}
		})
	}
}

func TestFormatHosts(t *testing.T) {
	tests := []struct {
		name     string
		hosts    []host
		contains []string
	}{
		{
			name:     "empty list",
			hosts:    nil,
			contains: []string{"No hosts found"},
		},
		{
			name: "single host",
			hosts: []host{
				{ID: 1, Name: "web01.example.com", OperatingsystemName: "RHEL 9.2", EnvironmentName: "production", GlobalStatusLabel: "OK"},
			},
			contains: []string{"Hosts: 1", "web01.example.com", "RHEL 9.2", "production", "OK"},
		},
		{
			name: "host with empty name",
			hosts: []host{
				{ID: 2},
			},
			contains: []string{"unknown"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatHosts(tt.hosts)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q in:\n%s", want, got)
				}
			}
		})
	}
}

func TestFormatErrata(t *testing.T) {
	tests := []struct {
		name     string
		errata   []erratum
		contains []string
	}{
		{
			name:     "empty list",
			errata:   nil,
			contains: []string{"No errata found"},
		},
		{
			name: "single erratum",
			errata: []erratum{
				{ErrataID: "RHSA-2026:0001", Title: "Critical kernel update", Type: "security", Severity: "Critical"},
			},
			contains: []string{"Errata: 1", "RHSA-2026:0001", "Critical kernel update", "security", "Critical"},
		},
		{
			name: "empty ID and title",
			errata: []erratum{
				{},
			},
			contains: []string{"unknown", "untitled"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatErrata(tt.errata)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q in:\n%s", want, got)
				}
			}
		})
	}
}

func newMockClient(handler http.Handler) (*satelliteClient, *httptest.Server) {
	srv := httptest.NewServer(handler)
	return &satelliteClient{
		api: redhat.NewAPIClient(redhat.APIClientConfig{BaseURL: srv.URL}),
	}, srv
}

func TestHealthCheck_OutputFormat(t *testing.T) {
	client := &satelliteClient{
		api:     redhat.NewAPIClient(redhat.APIClientConfig{BaseURL: "https://satellite.example.com"}),
		baseURL: "https://satellite.example.com",
	}

	result := client.healthCheck(context.Background())
	for _, want := range []string{"Satellite Health Check", "Host: satellite.example.com", ":443", ":9090", ":23443", "httpd", "foreman-proxy", "tomcat"} {
		if !strings.Contains(result, want) {
			t.Errorf("output missing %q in:\n%s", want, result)
		}
	}
}

func TestHealthCheck_HttpdDown(t *testing.T) {
	client := &satelliteClient{
		api:     redhat.NewAPIClient(redhat.APIClientConfig{BaseURL: "https://127.0.0.1:1"}),
		baseURL: "https://127.0.0.1:1",
	}

	result := client.healthCheck(context.Background())
	for _, want := range []string{"Satellite Health Check", "DOWN"} {
		if !strings.Contains(result, want) {
			t.Errorf("output missing %q in:\n%s", want, result)
		}
	}
	if strings.Contains(result, "All services reachable") {
		t.Errorf("should not say all services reachable when ports are down:\n%s", result)
	}
}

func TestGetREXJobStatus_WithOutput(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/job_invocations/3", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":3,"description":"hostname -f","status_label":"succeeded","succeeded":1,"failed":0,"pending":0,"total":1}`)
	})
	mux.HandleFunc("/api/v2/job_invocations/3/template_invocations", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"results":[{"host_id":4,"host_name":"rhel81.example.com"}]}`)
	})
	mux.HandleFunc("/api/v2/job_invocations/3/hosts/4", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"output":[{"output_type":"stdout","output":"rhel81.example.com\n"},{"output_type":"stderr","output":""},{"output_type":"debug","output":"debug noise"}]}`)
	})

	client, srv := newMockClient(mux)
	defer srv.Close()

	result, err := client.getREXJobStatus(context.Background(), 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{"REX Job #3", "Status: succeeded", "--- rhel81.example.com ---", "rhel81.example.com"} {
		if !strings.Contains(result, want) {
			t.Errorf("output missing %q in:\n%s", want, result)
		}
	}
	if strings.Contains(result, "debug noise") {
		t.Errorf("debug output should be filtered out:\n%s", result)
	}
}

func TestGetREXJobStatus_Running(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/job_invocations/1", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":1,"description":"cat /etc/hosts","status_label":"running","succeeded":0,"failed":0,"pending":1,"total":1}`)
	})

	client, srv := newMockClient(mux)
	defer srv.Close()

	result, err := client.getREXJobStatus(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Status: running") {
		t.Errorf("expected running status in:\n%s", result)
	}
	if strings.Contains(result, "---") {
		t.Errorf("running job should not have host output:\n%s", result)
	}
}

func TestGetREXJobStatus_MultipleHosts(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/job_invocations/5", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":5,"description":"uptime","status_label":"succeeded","succeeded":2,"failed":0,"pending":0,"total":2}`)
	})
	mux.HandleFunc("/api/v2/job_invocations/5/template_invocations", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"results":[{"host_id":1,"host_name":"web01.example.com"},{"host_id":2,"host_name":"web02.example.com"}]}`)
	})
	mux.HandleFunc("/api/v2/job_invocations/5/hosts/1", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"output":[{"output_type":"stdout","output":"up 3 days\n"}]}`)
	})
	mux.HandleFunc("/api/v2/job_invocations/5/hosts/2", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"output":[{"output_type":"stdout","output":"up 7 days\n"}]}`)
	})

	client, srv := newMockClient(mux)
	defer srv.Close()

	result, err := client.getREXJobStatus(context.Background(), 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"--- web01.example.com ---", "up 3 days", "--- web02.example.com ---", "up 7 days"} {
		if !strings.Contains(result, want) {
			t.Errorf("output missing %q in:\n%s", want, result)
		}
	}
}

func TestRunREXCommand(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/job_invocations", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		fmt.Fprint(w, `{"id":42}`)
	})

	client, srv := newMockClient(mux)
	defer srv.Close()

	jobID, err := client.runREXCommand(context.Background(), "name = web01", "hostname")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if jobID != 42 {
		t.Errorf("got job ID %d, want 42", jobID)
	}
}

func TestGetServices(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/status", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"satellite_version":"6.15.4","version":"3.9.1","result":"ok"}`)
	})
	mux.HandleFunc("/api/v2/ping", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"results":{"foreman":{"database":{"status":"ok"},"cache":{"servers":[{"status":"ok"}]}},"katello":{"services":{"candlepin":{"status":"ok","duration_ms":"12"},"pulp3":{"status":"FAIL","message":"No route to host"}},"status":"FAIL"}}}`)
	})

	client, srv := newMockClient(mux)
	defer srv.Close()

	result, err := client.getServices(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"Satellite version: 6.15.4", "Database: ok", "Katello overall: FAIL", "candlepin: ok", "pulp3: FAIL", "No route to host"} {
		if !strings.Contains(result, want) {
			t.Errorf("output missing %q in:\n%s", want, result)
		}
	}
}

func TestListHosts(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/hosts", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"results":[{"id":1,"name":"web01.example.com","operatingsystem_name":"RHEL 9.2","global_status_label":"OK"}]}`)
	})

	client, srv := newMockClient(mux)
	defer srv.Close()

	hosts, err := client.listHosts(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("got %d hosts, want 1", len(hosts))
	}
	if hosts[0].Name != "web01.example.com" {
		t.Errorf("host name = %q, want %q", hosts[0].Name, "web01.example.com")
	}
}

func TestListErrata(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/katello/api/v2/errata", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"results":[{"errata_id":"RHSA-2026:0001","title":"Kernel fix","type":"security","severity":"Critical"}]}`)
	})

	client, srv := newMockClient(mux)
	defer srv.Close()

	errata, err := client.listErrata(context.Background(), "", "security")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errata) != 1 {
		t.Fatalf("got %d errata, want 1", len(errata))
	}
	if errata[0].ErrataID != "RHSA-2026:0001" {
		t.Errorf("errata ID = %q, want %q", errata[0].ErrataID, "RHSA-2026:0001")
	}
}

func TestListContentViews(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/katello/api/v2/content_views", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"results":[{"id":1,"name":"RHEL-Base","label":"rhel-base","composite":false,"last_published":"2026-01-01"}]}`)
	})

	client, srv := newMockClient(mux)
	defer srv.Close()

	views, err := client.listContentViews(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(views) != 1 || views[0].Name != "RHEL-Base" {
		t.Errorf("got %+v, want 1 view named RHEL-Base", views)
	}
}

func TestListRepositories(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/katello/api/v2/repositories", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"results":[{"id":10,"name":"rhel-9-baseos-rpms","label":"rhel-9-baseos","content_type":"yum"}]}`)
	})

	client, srv := newMockClient(mux)
	defer srv.Close()

	repos, err := client.listRepositories(context.Background(), 0, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 1 || repos[0].Name != "rhel-9-baseos-rpms" {
		t.Errorf("got %+v", repos)
	}
}

func TestListRepositories_ByContentView(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/katello/api/v2/content_views/5/repositories", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"results":[{"id":11,"name":"appstream","label":"appstream","content_type":"yum"}]}`)
	})

	client, srv := newMockClient(mux)
	defer srv.Close()

	repos, err := client.listRepositories(context.Background(), 5, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 1 || repos[0].Name != "appstream" {
		t.Errorf("got %+v", repos)
	}
}

func TestListTasks(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/foreman_tasks/api/tasks", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"total":50,"results":[{"id":"abc","action":"Sync repo","state":"stopped","result":"success","started_at":"2026-01-01","ended_at":"2026-01-02"}]}`)
	})

	client, srv := newMockClient(mux)
	defer srv.Close()

	tasks, total, err := client.listTasks(context.Background(), "", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 50 {
		t.Errorf("total = %d, want 50", total)
	}
	if len(tasks) != 1 || tasks[0].Action != "Sync repo" {
		t.Errorf("got %+v", tasks)
	}
}

func TestGetHostFacts(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/hosts/satellite.example.com/facts", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"total":469,"results":{"satellite.example.com":{"processorcount":"8","memorysize":"61.30 GiB","operatingsystem":"RedHat"}}}`)
	})

	client, srv := newMockClient(mux)
	defer srv.Close()

	result, err := client.getHostFacts(context.Background(), "satellite.example.com", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"satellite.example.com", "processorcount = 8", "memorysize = 61.30 GiB"} {
		if !strings.Contains(result, want) {
			t.Errorf("output missing %q in:\n%s", want, result)
		}
	}
}

func TestGetHostFacts_NoResults(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/hosts/unknown.example.com/facts", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"total":0,"results":{}}`)
	})

	client, srv := newMockClient(mux)
	defer srv.Close()

	result, err := client.getHostFacts(context.Background(), "unknown.example.com", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "No facts found") {
		t.Errorf("expected 'No facts found' in:\n%s", result)
	}
}

func TestListSmartProxies(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/smart_proxies", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"results":[{"id":1,"name":"satellite.example.com","url":"https://satellite.example.com:9090","features":[{"name":"TFTP"},{"name":"DNS"}]}]}`)
	})

	client, srv := newMockClient(mux)
	defer srv.Close()

	proxies, err := client.listSmartProxies(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(proxies) != 1 || proxies[0].Name != "satellite.example.com" {
		t.Errorf("got %+v", proxies)
	}
	if len(proxies[0].Features) != 2 {
		t.Errorf("got %d features, want 2", len(proxies[0].Features))
	}
}

func TestFormatContentViews(t *testing.T) {
	tests := []struct {
		name     string
		views    []contentView
		contains []string
	}{
		{
			name:     "empty list",
			views:    nil,
			contains: []string{"No content views found"},
		},
		{
			name: "single view",
			views: []contentView{
				{ID: 1, Name: "RHEL-Base", Label: "rhel-base", Composite: false, LastPublished: "2026-01-01"},
			},
			contains: []string{"Content views: 1", "RHEL-Base", "rhel-base", "last published: 2026-01-01"},
		},
		{
			name: "composite view",
			views: []contentView{
				{ID: 2, Name: "Combined", Label: "combined", Composite: true},
			},
			contains: []string{"Combined", "composite"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatContentViews(tt.views)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q in:\n%s", want, got)
				}
			}
		})
	}
}
