package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/bobbyjohnstx/tinycode/internal/redhat"
	"github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

type options struct {
	BaseURL  string
	APIToken string
}

func parseOptions(raw map[string]any) options {
	var opts options
	if v, ok := raw["baseUrl"].(string); ok {
		opts.BaseURL = v
	}
	if v, ok := raw["apiToken"].(string); ok {
		opts.APIToken = v
	}
	return opts
}

type rhdhClient struct {
	api *redhat.APIClient
}

func newRhdhClient(baseURL, token string) *rhdhClient {
	var tokenFn func(context.Context) (string, error)
	if token != "" {
		tokenFn = func(_ context.Context) (string, error) { return token, nil }
	}
	return &rhdhClient{
		api: redhat.NewAPIClient(redhat.APIClientConfig{
			BaseURL: baseURL,
			TokenFn: tokenFn,
		}),
	}
}

type catalogEntity struct {
	APIVersion string `json:"apiVersion,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Metadata   *struct {
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
	} `json:"metadata,omitempty"`
	Spec *struct {
		Type       string `json:"type,omitempty"`
		Lifecycle  string `json:"lifecycle,omitempty"`
		Owner      string `json:"owner,omitempty"`
		System     string `json:"system,omitempty"`
		Definition string `json:"definition,omitempty"`
	} `json:"spec,omitempty"`
	Relations []struct {
		Type      string `json:"type,omitempty"`
		TargetRef string `json:"targetRef,omitempty"`
	} `json:"relations,omitempty"`
}

func (c *rhdhClient) searchEntities(ctx context.Context, filter map[string]string) ([]catalogEntity, error) {
	query := map[string]string{}
	var filterParts []string
	for k, v := range filter {
		filterParts = append(filterParts, k+"="+v)
	}
	if len(filterParts) > 0 {
		query["filter"] = strings.Join(filterParts, ",")
	}

	resp, err := c.api.Get(ctx, "/api/catalog/entities", query)
	if err != nil {
		return nil, err
	}
	var entities []catalogEntity
	if err := json.Unmarshal(resp.Data, &entities); err != nil {
		return nil, fmt.Errorf("parsing entities: %w", err)
	}
	return entities, nil
}

func (c *rhdhClient) getEntity(ctx context.Context, kind, namespace, name string) (*catalogEntity, error) {
	path := fmt.Sprintf("/api/catalog/entities/by-name/%s/%s/%s",
		url.PathEscape(kind), url.PathEscape(namespace), url.PathEscape(name))
	resp, err := c.api.Get(ctx, path, nil)
	if err != nil {
		return nil, err
	}
	var entity catalogEntity
	if err := json.Unmarshal(resp.Data, &entity); err != nil {
		return nil, fmt.Errorf("parsing entity: %w", err)
	}
	return &entity, nil
}

func (c *rhdhClient) getTechDocs(ctx context.Context, namespace, kind, name string) (string, error) {
	path := fmt.Sprintf("/api/techdocs/static/docs/%s/%s/%s/index.html",
		url.PathEscape(namespace), url.PathEscape(kind), url.PathEscape(name))
	resp, err := c.api.Get(ctx, path, nil)
	if err != nil {
		return "", err
	}
	return string(resp.Data), nil
}

func formatEntity(entity *catalogEntity) string {
	var lines []string

	kind := "Unknown"
	if entity.Kind != "" {
		kind = entity.Kind
	}
	name := "unknown"
	if entity.Metadata != nil && entity.Metadata.Name != "" {
		name = entity.Metadata.Name
	}
	lines = append(lines, fmt.Sprintf("%s: %s", kind, name))

	if entity.Metadata != nil {
		if entity.Metadata.Namespace != "" {
			lines = append(lines, fmt.Sprintf("Namespace: %s", entity.Metadata.Namespace))
		}
		if entity.Metadata.Title != "" {
			lines = append(lines, fmt.Sprintf("Title: %s", entity.Metadata.Title))
		}
		if entity.Metadata.Description != "" {
			lines = append(lines, fmt.Sprintf("Description: %s", entity.Metadata.Description))
		}
	}

	if entity.Spec != nil {
		if entity.Spec.Type != "" {
			lines = append(lines, fmt.Sprintf("Type: %s", entity.Spec.Type))
		}
		if entity.Spec.Lifecycle != "" {
			lines = append(lines, fmt.Sprintf("Lifecycle: %s", entity.Spec.Lifecycle))
		}
		if entity.Spec.Owner != "" {
			lines = append(lines, fmt.Sprintf("Owner: %s", entity.Spec.Owner))
		}
		if entity.Spec.System != "" {
			lines = append(lines, fmt.Sprintf("System: %s", entity.Spec.System))
		}
	}

	if entity.Metadata != nil && len(entity.Metadata.Tags) > 0 {
		lines = append(lines, fmt.Sprintf("Tags: %s", strings.Join(entity.Metadata.Tags, ", ")))
	}

	if entity.Metadata != nil && len(entity.Metadata.Links) > 0 {
		lines = append(lines, "", "Links:")
		for _, link := range entity.Metadata.Links {
			title := link.Title
			if title == "" {
				title = "link"
			}
			lines = append(lines, fmt.Sprintf("- %s: %s", title, link.URL))
		}
	}

	if len(entity.Relations) > 0 {
		lines = append(lines, "", "Relations:")
		for _, rel := range entity.Relations {
			relType := rel.Type
			if relType == "" {
				relType = "unknown"
			}
			target := rel.TargetRef
			if target == "" {
				target = "unknown"
			}
			lines = append(lines, fmt.Sprintf("- %s -> %s", relType, target))
		}
	}

	return strings.Join(lines, "\n")
}

func formatEntityList(entities []catalogEntity) string {
	if len(entities) == 0 {
		return "No entities found matching the search criteria."
	}
	lines := []string{fmt.Sprintf("Entities found: %d", len(entities)), ""}
	for _, entity := range entities {
		kind := "Unknown"
		if entity.Kind != "" {
			kind = entity.Kind
		}
		name := "unknown"
		ns := "default"
		desc := ""
		if entity.Metadata != nil {
			if entity.Metadata.Name != "" {
				name = entity.Metadata.Name
			}
			if entity.Metadata.Namespace != "" {
				ns = entity.Metadata.Namespace
			}
			if entity.Metadata.Description != "" {
				desc = " - " + entity.Metadata.Description
			}
		}
		lifecycle := ""
		if entity.Spec != nil && entity.Spec.Lifecycle != "" {
			lifecycle = " [" + entity.Spec.Lifecycle + "]"
		}
		lines = append(lines, fmt.Sprintf("- %s:%s/%s%s%s", kind, ns, name, lifecycle, desc))
	}
	return strings.Join(lines, "\n")
}

func formatDependencies(entity *catalogEntity) string {
	name := "unknown"
	kind := "Unknown"
	if entity.Metadata != nil && entity.Metadata.Name != "" {
		name = entity.Metadata.Name
	}
	if entity.Kind != "" {
		kind = entity.Kind
	}

	if len(entity.Relations) == 0 {
		return fmt.Sprintf("No dependencies found for %s:%s.", kind, name)
	}

	grouped := map[string][]string{}
	for _, rel := range entity.Relations {
		relType := rel.Type
		if relType == "" {
			relType = "unknown"
		}
		target := rel.TargetRef
		if target == "" {
			target = "unknown"
		}
		grouped[relType] = append(grouped[relType], target)
	}

	lines := []string{fmt.Sprintf("Dependencies for %s:%s", kind, name), ""}
	for relType, targets := range grouped {
		lines = append(lines, relType+":")
		for _, t := range targets {
			lines = append(lines, "  - "+t)
		}
	}
	return strings.Join(lines, "\n")
}

func buildTools(client *rhdhClient) []plugin.ToolDef {
	return []plugin.ToolDef{
		{
			Name:        "rhdh_catalog_search",
			Description: "Search the RHDH software catalog by name, kind (Component/API/System/Group), or lifecycle stage.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":     map[string]any{"type": "string", "description": "Search by entity name (substring match)"},
					"kind":      map[string]any{"type": "string", "description": "Filter by entity kind: Component, API, System, or Group"},
					"lifecycle": map[string]any{"type": "string", "description": "Filter by lifecycle stage (e.g. production, experimental)"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Query     string `json:"query"`
					Kind      string `json:"kind"`
					Lifecycle string `json:"lifecycle"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				filter := map[string]string{}
				if input.Kind != "" {
					filter["kind"] = input.Kind
				}
				if input.Lifecycle != "" {
					filter["spec.lifecycle"] = input.Lifecycle
				}
				if input.Query != "" {
					filter["metadata.name"] = input.Query
				}
				entities, err := client.searchEntities(ctx, filter)
				if err != nil {
					return fmt.Sprintf("Failed to search catalog: %v", err), nil
				}
				return formatEntityList(entities), nil
			},
		},
		{
			Name:        "rhdh_catalog_entity",
			Description: "Get full entity details from the RHDH catalog: metadata, spec, relations, and links.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind":      map[string]any{"type": "string", "description": "Entity kind (e.g. Component, API, System, Group)"},
					"name":      map[string]any{"type": "string", "description": "Entity name"},
					"namespace": map[string]any{"type": "string", "description": "Entity namespace (defaults to 'default')"},
				},
				"required": []string{"kind", "name"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Kind      string `json:"kind"`
					Name      string `json:"name"`
					Namespace string `json:"namespace"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				ns := input.Namespace
				if ns == "" {
					ns = "default"
				}
				entity, err := client.getEntity(ctx, input.Kind, ns, input.Name)
				if err != nil {
					return fmt.Sprintf("Failed to get entity: %v", err), nil
				}
				return formatEntity(entity), nil
			},
		},
		{
			Name:        "rhdh_api_spec",
			Description: "Fetch the OpenAPI or AsyncAPI specification for an API entity in the RHDH catalog.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":      map[string]any{"type": "string", "description": "API entity name"},
					"namespace": map[string]any{"type": "string", "description": "API entity namespace (defaults to 'default')"},
				},
				"required": []string{"name"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Name      string `json:"name"`
					Namespace string `json:"namespace"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				ns := input.Namespace
				if ns == "" {
					ns = "default"
				}
				entity, err := client.getEntity(ctx, "API", ns, input.Name)
				if err != nil {
					return fmt.Sprintf("Failed to get API spec: %v", err), nil
				}
				if entity.Spec == nil || entity.Spec.Definition == "" {
					return fmt.Sprintf("No API specification found for API:%s/%s.", ns, input.Name), nil
				}
				entityName := input.Name
				if entity.Metadata != nil && entity.Metadata.Name != "" {
					entityName = entity.Metadata.Name
				}
				specType := "unknown"
				if entity.Spec.Type != "" {
					specType = entity.Spec.Type
				}
				lines := []string{
					fmt.Sprintf("API Spec for %s", entityName),
					fmt.Sprintf("Type: %s", specType),
					"",
					entity.Spec.Definition,
				}
				return strings.Join(lines, "\n"), nil
			},
		},
		{
			Name:        "rhdh_techdocs",
			Description: "Fetch TechDocs content for a component in the RHDH catalog.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind":      map[string]any{"type": "string", "description": "Entity kind (e.g. Component)"},
					"name":      map[string]any{"type": "string", "description": "Entity name"},
					"namespace": map[string]any{"type": "string", "description": "Entity namespace (defaults to 'default')"},
				},
				"required": []string{"kind", "name"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Kind      string `json:"kind"`
					Name      string `json:"name"`
					Namespace string `json:"namespace"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				ns := input.Namespace
				if ns == "" {
					ns = "default"
				}
				content, err := client.getTechDocs(ctx, ns, input.Kind, input.Name)
				if err != nil {
					return fmt.Sprintf("Failed to get TechDocs: %v", err), nil
				}
				if content == "" {
					return fmt.Sprintf("No TechDocs found for %s:%s/%s.", input.Kind, ns, input.Name), nil
				}
				return fmt.Sprintf("TechDocs for %s:%s/%s\n\n%s", input.Kind, ns, input.Name, content), nil
			},
		},
		{
			Name:        "rhdh_dependencies",
			Description: "Get the dependency graph for an entity: consumesApi, providesApi, dependsOn, dependencyOf relations.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind":      map[string]any{"type": "string", "description": "Entity kind (e.g. Component, API, System)"},
					"name":      map[string]any{"type": "string", "description": "Entity name"},
					"namespace": map[string]any{"type": "string", "description": "Entity namespace (defaults to 'default')"},
				},
				"required": []string{"kind", "name"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Kind      string `json:"kind"`
					Name      string `json:"name"`
					Namespace string `json:"namespace"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				ns := input.Namespace
				if ns == "" {
					ns = "default"
				}
				entity, err := client.getEntity(ctx, input.Kind, ns, input.Name)
				if err != nil {
					return fmt.Sprintf("Failed to get dependencies: %v", err), nil
				}
				return formatDependencies(entity), nil
			},
		},
	}
}

func unconfiguredTools() []plugin.ToolDef {
	msg := "RHDH plugin not configured. Set baseUrl in plugin options."
	return []plugin.ToolDef{
		{
			Name:        "rhdh_catalog_search",
			Description: "Search the RHDH software catalog by name, kind, or lifecycle.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":     map[string]any{"type": "string", "description": "Search by entity name"},
					"kind":      map[string]any{"type": "string", "description": "Filter by entity kind"},
					"lifecycle": map[string]any{"type": "string", "description": "Filter by lifecycle stage"},
				},
			},
			Execute: func(_ context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				return msg, nil
			},
		},
		{
			Name:        "rhdh_catalog_entity",
			Description: "Get full entity details from the RHDH catalog.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind":      map[string]any{"type": "string", "description": "Entity kind"},
					"name":      map[string]any{"type": "string", "description": "Entity name"},
					"namespace": map[string]any{"type": "string", "description": "Entity namespace"},
				},
				"required": []string{"kind", "name"},
			},
			Execute: func(_ context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				return msg, nil
			},
		},
		{
			Name:        "rhdh_api_spec",
			Description: "Fetch API specification for an API entity in the RHDH catalog.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":      map[string]any{"type": "string", "description": "API entity name"},
					"namespace": map[string]any{"type": "string", "description": "API entity namespace"},
				},
				"required": []string{"name"},
			},
			Execute: func(_ context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				return msg, nil
			},
		},
		{
			Name:        "rhdh_techdocs",
			Description: "Fetch TechDocs content for a component in the RHDH catalog.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind":      map[string]any{"type": "string", "description": "Entity kind"},
					"name":      map[string]any{"type": "string", "description": "Entity name"},
					"namespace": map[string]any{"type": "string", "description": "Entity namespace"},
				},
				"required": []string{"kind", "name"},
			},
			Execute: func(_ context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				return msg, nil
			},
		},
		{
			Name:        "rhdh_dependencies",
			Description: "Get the dependency graph for an entity.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind":      map[string]any{"type": "string", "description": "Entity kind"},
					"name":      map[string]any{"type": "string", "description": "Entity name"},
					"namespace": map[string]any{"type": "string", "description": "Entity namespace"},
				},
				"required": []string{"kind", "name"},
			},
			Execute: func(_ context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				return msg, nil
			},
		},
	}
}

func newPlugin(opts options) plugin.Plugin {
	if opts.BaseURL == "" {
		return plugin.Plugin{
			ID:    "rhdh",
			Tools: unconfiguredTools(),
		}
	}

	client := newRhdhClient(opts.BaseURL, opts.APIToken)
	return plugin.Plugin{
		ID:    "rhdh",
		Tools: buildTools(client),
	}
}

func main() {
	plugin.RunWithOptions(func(params plugin.InitializeParams) (plugin.Plugin, error) {
		opts := parseOptions(params.Options)
		return newPlugin(opts), nil
	})
}
