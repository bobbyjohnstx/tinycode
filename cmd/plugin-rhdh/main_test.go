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
	if p.ID != "rhdh" {
		t.Errorf("expected plugin ID 'rhdh', got %q", p.ID)
	}
}

func TestToolCountUnconfigured(t *testing.T) {
	p := newPlugin(options{})
	if len(p.Tools) != 5 {
		t.Fatalf("expected 5 unconfigured tools, got %d", len(p.Tools))
	}
}

func TestToolCountConfigured(t *testing.T) {
	p := newPlugin(options{BaseURL: "https://rhdh.example.com"})
	if len(p.Tools) != 5 {
		t.Fatalf("expected 5 configured tools, got %d", len(p.Tools))
	}
}

func TestToolDefinitions(t *testing.T) {
	p := newPlugin(options{BaseURL: "https://rhdh.example.com"})
	expectedNames := []string{
		"rhdh_catalog_search",
		"rhdh_catalog_entity",
		"rhdh_api_spec",
		"rhdh_techdocs",
		"rhdh_dependencies",
	}

	if len(p.Tools) != len(expectedNames) {
		t.Fatalf("expected %d tools, got %d", len(expectedNames), len(p.Tools))
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
	p := newPlugin(options{BaseURL: "https://rhdh.example.com"})
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

func TestParseOptions(t *testing.T) {
	t.Run("full options", func(t *testing.T) {
		opts := parseOptions(map[string]any{
			"baseUrl":  "https://rhdh.example.com",
			"apiToken": "tok-123",
		})
		if opts.BaseURL != "https://rhdh.example.com" {
			t.Errorf("expected BaseURL, got %q", opts.BaseURL)
		}
		if opts.APIToken != "tok-123" {
			t.Errorf("expected APIToken 'tok-123', got %q", opts.APIToken)
		}
	})

	t.Run("empty map", func(t *testing.T) {
		opts := parseOptions(map[string]any{})
		if opts.BaseURL != "" || opts.APIToken != "" {
			t.Errorf("expected empty options, got %+v", opts)
		}
	})

	t.Run("wrong types", func(t *testing.T) {
		opts := parseOptions(map[string]any{
			"baseUrl":  42,
			"apiToken": true,
		})
		if opts.BaseURL != "" || opts.APIToken != "" {
			t.Errorf("expected empty options for wrong types, got %+v", opts)
		}
	})
}

func TestFormatEntity(t *testing.T) {
	t.Run("full entity", func(t *testing.T) {
		entity := &catalogEntity{
			Kind: "Component",
			Metadata: &struct {
				Name        string            `json:"name,omitempty"`
				Namespace   string            `json:"namespace,omitempty"`
				Description string            `json:"description,omitempty"`
				Title       string            `json:"title,omitempty"`
				Annotations map[string]string `json:"annotations,omitempty"`
				Tags        []string          `json:"tags,omitempty"`
				Links       []struct {
					URL   string `json:"url,omitempty"`
					Title string `json:"title,omitempty"`
				} `json:"links,omitempty"`
			}{
				Name:        "my-service",
				Namespace:   "default",
				Description: "A test service",
				Title:       "My Service",
				Tags:        []string{"go", "api"},
				Links: []struct {
					URL   string `json:"url,omitempty"`
					Title string `json:"title,omitempty"`
				}{
					{URL: "https://github.com/example", Title: "GitHub"},
				},
			},
			Spec: &struct {
				Type       string `json:"type,omitempty"`
				Lifecycle  string `json:"lifecycle,omitempty"`
				Owner      string `json:"owner,omitempty"`
				System     string `json:"system,omitempty"`
				Definition string `json:"definition,omitempty"`
			}{
				Type:      "service",
				Lifecycle: "production",
				Owner:     "team-alpha",
				System:    "platform",
			},
			Relations: []struct {
				Type      string `json:"type,omitempty"`
				TargetRef string `json:"targetRef,omitempty"`
			}{
				{Type: "dependsOn", TargetRef: "component:default/db"},
			},
		}
		result := formatEntity(entity)
		if !strings.Contains(result, "Component: my-service") {
			t.Errorf("expected kind+name, got %q", result)
		}
		if !strings.Contains(result, "Namespace: default") {
			t.Errorf("expected namespace, got %q", result)
		}
		if !strings.Contains(result, "Title: My Service") {
			t.Errorf("expected title, got %q", result)
		}
		if !strings.Contains(result, "Description: A test service") {
			t.Errorf("expected description, got %q", result)
		}
		if !strings.Contains(result, "Type: service") {
			t.Errorf("expected type, got %q", result)
		}
		if !strings.Contains(result, "Lifecycle: production") {
			t.Errorf("expected lifecycle, got %q", result)
		}
		if !strings.Contains(result, "Owner: team-alpha") {
			t.Errorf("expected owner, got %q", result)
		}
		if !strings.Contains(result, "Tags: go, api") {
			t.Errorf("expected tags, got %q", result)
		}
		if !strings.Contains(result, "GitHub: https://github.com/example") {
			t.Errorf("expected link, got %q", result)
		}
		if !strings.Contains(result, "dependsOn -> component:default/db") {
			t.Errorf("expected relation, got %q", result)
		}
	})

	t.Run("nil metadata and spec", func(t *testing.T) {
		entity := &catalogEntity{Kind: ""}
		result := formatEntity(entity)
		if !strings.Contains(result, "Unknown: unknown") {
			t.Errorf("expected fallbacks for nil metadata, got %q", result)
		}
	})
}

func TestFormatEntityList(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		result := formatEntityList(nil)
		if !strings.Contains(result, "No entities found") {
			t.Errorf("expected empty message, got %q", result)
		}
	})

	t.Run("multiple with lifecycle", func(t *testing.T) {
		entities := []catalogEntity{
			{
				Kind: "Component",
				Metadata: &struct {
					Name        string            `json:"name,omitempty"`
					Namespace   string            `json:"namespace,omitempty"`
					Description string            `json:"description,omitempty"`
					Title       string            `json:"title,omitempty"`
					Annotations map[string]string `json:"annotations,omitempty"`
					Tags        []string          `json:"tags,omitempty"`
					Links       []struct {
						URL   string `json:"url,omitempty"`
						Title string `json:"title,omitempty"`
					} `json:"links,omitempty"`
				}{
					Name:        "svc-a",
					Namespace:   "default",
					Description: "Service A desc",
				},
				Spec: &struct {
					Type       string `json:"type,omitempty"`
					Lifecycle  string `json:"lifecycle,omitempty"`
					Owner      string `json:"owner,omitempty"`
					System     string `json:"system,omitempty"`
					Definition string `json:"definition,omitempty"`
				}{
					Lifecycle: "production",
				},
			},
			{
				Kind: "API",
				Metadata: &struct {
					Name        string            `json:"name,omitempty"`
					Namespace   string            `json:"namespace,omitempty"`
					Description string            `json:"description,omitempty"`
					Title       string            `json:"title,omitempty"`
					Annotations map[string]string `json:"annotations,omitempty"`
					Tags        []string          `json:"tags,omitempty"`
					Links       []struct {
						URL   string `json:"url,omitempty"`
						Title string `json:"title,omitempty"`
					} `json:"links,omitempty"`
				}{
					Name:      "api-b",
					Namespace: "team-ns",
				},
			},
		}
		result := formatEntityList(entities)
		if !strings.Contains(result, "Entities found: 2") {
			t.Errorf("expected count, got %q", result)
		}
		if !strings.Contains(result, "Component:default/svc-a [production]") {
			t.Errorf("expected entity with lifecycle, got %q", result)
		}
		if !strings.Contains(result, "API:team-ns/api-b") {
			t.Errorf("expected entity without lifecycle, got %q", result)
		}
		if !strings.Contains(result, "Service A desc") {
			t.Errorf("expected description, got %q", result)
		}
	})
}

func TestFormatDependencies(t *testing.T) {
	t.Run("no relations", func(t *testing.T) {
		entity := &catalogEntity{
			Kind: "Component",
			Metadata: &struct {
				Name        string            `json:"name,omitempty"`
				Namespace   string            `json:"namespace,omitempty"`
				Description string            `json:"description,omitempty"`
				Title       string            `json:"title,omitempty"`
				Annotations map[string]string `json:"annotations,omitempty"`
				Tags        []string          `json:"tags,omitempty"`
				Links       []struct {
					URL   string `json:"url,omitempty"`
					Title string `json:"title,omitempty"`
				} `json:"links,omitempty"`
			}{Name: "svc"},
		}
		result := formatDependencies(entity)
		if !strings.Contains(result, "No dependencies found") {
			t.Errorf("expected no-deps message, got %q", result)
		}
	})

	t.Run("grouped relations", func(t *testing.T) {
		entity := &catalogEntity{
			Kind: "Component",
			Metadata: &struct {
				Name        string            `json:"name,omitempty"`
				Namespace   string            `json:"namespace,omitempty"`
				Description string            `json:"description,omitempty"`
				Title       string            `json:"title,omitempty"`
				Annotations map[string]string `json:"annotations,omitempty"`
				Tags        []string          `json:"tags,omitempty"`
				Links       []struct {
					URL   string `json:"url,omitempty"`
					Title string `json:"title,omitempty"`
				} `json:"links,omitempty"`
			}{Name: "svc"},
			Relations: []struct {
				Type      string `json:"type,omitempty"`
				TargetRef string `json:"targetRef,omitempty"`
			}{
				{Type: "dependsOn", TargetRef: "component:default/db"},
				{Type: "dependsOn", TargetRef: "component:default/cache"},
				{Type: "consumesApi", TargetRef: "api:default/auth-api"},
			},
		}
		result := formatDependencies(entity)
		if !strings.Contains(result, "Dependencies for Component:svc") {
			t.Errorf("expected header, got %q", result)
		}
		if !strings.Contains(result, "dependsOn:") {
			t.Errorf("expected dependsOn group, got %q", result)
		}
		if !strings.Contains(result, "consumesApi:") {
			t.Errorf("expected consumesApi group, got %q", result)
		}
		if !strings.Contains(result, "component:default/db") {
			t.Errorf("expected db target, got %q", result)
		}
	})
}

// --- httptest mock tests ---

func newMockRhdhClient(handler http.Handler) (*rhdhClient, *httptest.Server) {
	srv := httptest.NewServer(handler)
	return &rhdhClient{
		api: redhat.NewAPIClient(redhat.APIClientConfig{BaseURL: srv.URL}),
	}, srv
}

func TestSearchEntities(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/catalog/entities", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			filter := r.URL.Query().Get("filter")
			if !strings.Contains(filter, "kind=Component") {
				t.Errorf("expected filter to contain kind=Component, got %q", filter)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `[{"kind":"Component","metadata":{"name":"my-svc","namespace":"default","description":"A service"},"spec":{"type":"service","lifecycle":"production","owner":"team-a"}}]`)
		})
		client, srv := newMockRhdhClient(mux)
		defer srv.Close()

		entities, err := client.searchEntities(context.Background(), map[string]string{"kind": "Component"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(entities) != 1 {
			t.Fatalf("expected 1 entity, got %d", len(entities))
		}
		e := entities[0]
		if e.Kind != "Component" {
			t.Errorf("expected kind 'Component', got %q", e.Kind)
		}
		if e.Metadata == nil {
			t.Fatal("expected non-nil metadata")
		}
		if e.Metadata.Name != "my-svc" {
			t.Errorf("expected name 'my-svc', got %q", e.Metadata.Name)
		}
		if e.Metadata.Description != "A service" {
			t.Errorf("expected description, got %q", e.Metadata.Description)
		}
		if e.Spec == nil {
			t.Fatal("expected non-nil spec")
		}
		if e.Spec.Lifecycle != "production" {
			t.Errorf("expected lifecycle 'production', got %q", e.Spec.Lifecycle)
		}
	})

	t.Run("empty filter returns all", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/catalog/entities", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("filter") != "" {
				t.Errorf("expected no filter param for empty map, got %q", r.URL.Query().Get("filter"))
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `[]`)
		})
		client, srv := newMockRhdhClient(mux)
		defer srv.Close()

		entities, err := client.searchEntities(context.Background(), map[string]string{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(entities) != 0 {
			t.Errorf("expected 0 entities, got %d", len(entities))
		}
	})

	t.Run("server error", func(t *testing.T) {
		client, srv := newMockRhdhClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "internal error", http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := client.searchEntities(context.Background(), map[string]string{"kind": "Component"})
		if err == nil {
			t.Fatal("expected error for 500 response")
		}
		if !strings.Contains(err.Error(), "500") {
			t.Errorf("expected error to contain '500', got %q", err.Error())
		}
	})
}

func TestGetEntity(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/catalog/entities/by-name/Component/default/my-svc", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"kind":"Component","metadata":{"name":"my-svc","namespace":"default","description":"My service","title":"My Service","tags":["go","api"],"links":[{"url":"https://github.com/example","title":"GitHub"}]},"spec":{"type":"service","lifecycle":"production","owner":"team-a","system":"platform"},"relations":[{"type":"dependsOn","targetRef":"component:default/db"}]}`)
		})
		client, srv := newMockRhdhClient(mux)
		defer srv.Close()

		entity, err := client.getEntity(context.Background(), "Component", "default", "my-svc")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if entity.Kind != "Component" {
			t.Errorf("expected kind 'Component', got %q", entity.Kind)
		}
		if entity.Metadata == nil {
			t.Fatal("expected non-nil metadata")
		}
		if entity.Metadata.Name != "my-svc" {
			t.Errorf("expected name 'my-svc', got %q", entity.Metadata.Name)
		}
		if entity.Metadata.Title != "My Service" {
			t.Errorf("expected title 'My Service', got %q", entity.Metadata.Title)
		}
		if len(entity.Metadata.Tags) != 2 {
			t.Errorf("expected 2 tags, got %d", len(entity.Metadata.Tags))
		}
		if len(entity.Metadata.Links) != 1 {
			t.Errorf("expected 1 link, got %d", len(entity.Metadata.Links))
		}
		if entity.Spec == nil {
			t.Fatal("expected non-nil spec")
		}
		if entity.Spec.Owner != "team-a" {
			t.Errorf("expected owner 'team-a', got %q", entity.Spec.Owner)
		}
		if len(entity.Relations) != 1 {
			t.Fatalf("expected 1 relation, got %d", len(entity.Relations))
		}
		if entity.Relations[0].Type != "dependsOn" {
			t.Errorf("expected relation type 'dependsOn', got %q", entity.Relations[0].Type)
		}
	})

	t.Run("not found", func(t *testing.T) {
		client, srv := newMockRhdhClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		}))
		defer srv.Close()

		_, err := client.getEntity(context.Background(), "Component", "default", "nonexistent")
		if err == nil {
			t.Fatal("expected error for 404 response")
		}
		if !strings.Contains(err.Error(), "404") {
			t.Errorf("expected error to contain '404', got %q", err.Error())
		}
	})
}

func TestGetTechDocs(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/techdocs/static/docs/default/Component/my-svc/index.html", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<html><body><h1>My Service Docs</h1><p>Documentation content here.</p></body></html>`)
		})
		client, srv := newMockRhdhClient(mux)
		defer srv.Close()

		content, err := client.getTechDocs(context.Background(), "default", "Component", "my-svc")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(content, "My Service Docs") {
			t.Errorf("expected docs content, got %q", content)
		}
		if !strings.Contains(content, "<html>") {
			t.Errorf("expected HTML content, got %q", content)
		}
	})

	t.Run("not found", func(t *testing.T) {
		client, srv := newMockRhdhClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "not found", http.StatusNotFound)
		}))
		defer srv.Close()

		_, err := client.getTechDocs(context.Background(), "default", "Component", "nonexistent")
		if err == nil {
			t.Fatal("expected error for 404 response")
		}
		if !strings.Contains(err.Error(), "404") {
			t.Errorf("expected error to contain '404', got %q", err.Error())
		}
	})

	t.Run("server error", func(t *testing.T) {
		client, srv := newMockRhdhClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "internal error", http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := client.getTechDocs(context.Background(), "default", "Component", "my-svc")
		if err == nil {
			t.Fatal("expected error for 500 response")
		}
	})
}
