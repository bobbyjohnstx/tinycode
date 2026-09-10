package main

import (
	"strings"
	"testing"
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
