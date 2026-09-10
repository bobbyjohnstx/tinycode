package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bobbyjohnstx/tinycode-go/internal/redhat"
	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

type options struct {
	SatelliteURL string
	Token        string
}

func parseOptions(raw map[string]any) options {
	var opts options
	if v, ok := raw["satelliteUrl"].(string); ok {
		opts.SatelliteURL = v
	}
	if v, ok := raw["token"].(string); ok {
		opts.Token = v
	}
	return opts
}

type satelliteClient struct {
	api *redhat.APIClient
}

func newSatelliteClient(baseURL, token string) *satelliteClient {
	return &satelliteClient{
		api: redhat.NewAPIClient(redhat.APIClientConfig{
			BaseURL: baseURL,
			TokenFn: func(_ context.Context) (string, error) { return token, nil },
		}),
	}
}

type host struct {
	ID                  int    `json:"id"`
	Name                string `json:"name"`
	OperatingsystemName string `json:"operatingsystem_name"`
	EnvironmentName     string `json:"environment_name"`
	GlobalStatusLabel   string `json:"global_status_label"`
}

type erratum struct {
	ErrataID string `json:"errata_id"`
	Title    string `json:"title"`
	Type     string `json:"type"`
	Severity string `json:"severity"`
}

type contentView struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Label         string `json:"label"`
	Composite     bool   `json:"composite"`
	LastPublished string `json:"last_published"`
}

func (c *satelliteClient) queryLightspeed(ctx context.Context, question string) (string, error) {
	resp, err := c.api.Post(ctx, "/api/v2/lightspeed/chats", map[string]string{"question": question})
	if err != nil {
		return "", err
	}
	var result struct{ Answer string `json:"answer"` }
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return "", err
	}
	if result.Answer == "" {
		return "No response received.", nil
	}
	return result.Answer, nil
}

func (c *satelliteClient) listHosts(ctx context.Context, search string) ([]host, error) {
	var query map[string]string
	if search != "" {
		query = map[string]string{"search": search}
	}
	resp, err := c.api.Get(ctx, "/api/v2/hosts", query)
	if err != nil {
		return nil, err
	}
	var result struct{ Results []host `json:"results"` }
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return result.Results, nil
}

func (c *satelliteClient) listErrata(ctx context.Context, search, errataType string) ([]erratum, error) {
	query := map[string]string{}
	if search != "" {
		query["search"] = search
	}
	if errataType != "" {
		query["type"] = errataType
	}
	var qp map[string]string
	if len(query) > 0 {
		qp = query
	}
	resp, err := c.api.Get(ctx, "/api/v2/errata", qp)
	if err != nil {
		return nil, err
	}
	var result struct{ Results []erratum `json:"results"` }
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return result.Results, nil
}

func (c *satelliteClient) listContentViews(ctx context.Context) ([]contentView, error) {
	resp, err := c.api.Get(ctx, "/api/v2/content_views", nil)
	if err != nil {
		return nil, err
	}
	var result struct{ Results []contentView `json:"results"` }
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return result.Results, nil
}

func formatHosts(hosts []host) string {
	if len(hosts) == 0 {
		return "No hosts found."
	}
	lines := []string{fmt.Sprintf("Hosts: %d", len(hosts)), ""}
	for _, h := range hosts {
		name := h.Name
		if name == "" {
			name = "unknown"
		}
		entry := "- " + name
		if h.OperatingsystemName != "" {
			entry += fmt.Sprintf(" (%s)", h.OperatingsystemName)
		}
		if h.EnvironmentName != "" {
			entry += fmt.Sprintf(" [%s]", h.EnvironmentName)
		}
		if h.GlobalStatusLabel != "" {
			entry += " — " + h.GlobalStatusLabel
		}
		lines = append(lines, entry)
	}
	return strings.Join(lines, "\n")
}

func formatErrata(errata []erratum) string {
	if len(errata) == 0 {
		return "No errata found."
	}
	lines := []string{fmt.Sprintf("Errata: %d", len(errata)), ""}
	for _, e := range errata {
		id := e.ErrataID
		if id == "" {
			id = "unknown"
		}
		title := e.Title
		if title == "" {
			title = "untitled"
		}
		entry := fmt.Sprintf("- %s: %s", id, title)
		if e.Type != "" {
			entry += fmt.Sprintf(" [%s]", e.Type)
		}
		if e.Severity != "" {
			entry += fmt.Sprintf(" (%s)", e.Severity)
		}
		lines = append(lines, entry)
	}
	return strings.Join(lines, "\n")
}

func formatContentViews(views []contentView) string {
	if len(views) == 0 {
		return "No content views found."
	}
	lines := []string{fmt.Sprintf("Content views: %d", len(views)), ""}
	for _, v := range views {
		name := v.Name
		if name == "" {
			name = "unknown"
		}
		entry := "- " + name
		if v.Label != "" {
			entry += fmt.Sprintf(" (%s)", v.Label)
		}
		if v.Composite {
			entry += " [composite]"
		}
		if v.LastPublished != "" {
			entry += " — last published: " + v.LastPublished
		}
		lines = append(lines, entry)
	}
	return strings.Join(lines, "\n")
}

func buildTools(client *satelliteClient) []plugin.ToolDef {
	return []plugin.ToolDef{
		{
			Name:        "satellite_query",
			Description: "Ask Satellite Lightspeed a question about RHEL, host management, errata, or content views.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"question": map[string]any{"type": "string", "description": "The question to ask Lightspeed"},
				},
				"required": []string{"question"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct{ Question string `json:"question"` }
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				result, err := client.queryLightspeed(ctx, input.Question)
				if err != nil {
					return fmt.Sprintf("Failed to query Lightspeed: %v", err), nil
				}
				return result, nil
			},
		},
		{
			Name:        "satellite_hosts",
			Description: "Search managed hosts in Satellite with name, OS, environment, and status.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"search": map[string]any{"type": "string", "description": "Search query to filter hosts"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct{ Search string `json:"search"` }
				json.Unmarshal(args, &input)
				hosts, err := client.listHosts(ctx, input.Search)
				if err != nil {
					return fmt.Sprintf("Failed to list hosts: %v", err), nil
				}
				return formatHosts(hosts), nil
			},
		},
		{
			Name:        "satellite_errata",
			Description: "Search available errata in Satellite with ID, title, type, and severity.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"search": map[string]any{"type": "string", "description": "Search query to filter errata"},
					"type":   map[string]any{"type": "string", "description": "Filter by errata type (security, bugfix, enhancement)"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Search string `json:"search"`
					Type   string `json:"type"`
				}
				json.Unmarshal(args, &input)
				errata, err := client.listErrata(ctx, input.Search, input.Type)
				if err != nil {
					return fmt.Sprintf("Failed to list errata: %v", err), nil
				}
				return formatErrata(errata), nil
			},
		},
		{
			Name:        "satellite_content_views",
			Description: "List content views in Satellite with name, label, composite flag, and last published date.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Execute: func(ctx context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				views, err := client.listContentViews(ctx)
				if err != nil {
					return fmt.Sprintf("Failed to list content views: %v", err), nil
				}
				return formatContentViews(views), nil
			},
		},
	}
}

func unconfiguredTools() []plugin.ToolDef {
	msg := "Satellite plugin not configured. Set satelliteUrl in plugin options."
	return []plugin.ToolDef{
		stubTool("satellite_query", "Ask Satellite Lightspeed a question.", msg),
		stubTool("satellite_hosts", "Search managed hosts in Satellite.", msg),
		stubTool("satellite_errata", "Search available errata in Satellite.", msg),
		stubTool("satellite_content_views", "List content views in Satellite.", msg),
	}
}

func stubTool(name, description, message string) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        name,
		Description: description,
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Execute: func(_ context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
			return message, nil
		},
	}
}

func newPlugin(opts options) plugin.Plugin {
	if opts.SatelliteURL == "" || opts.Token == "" {
		return plugin.Plugin{
			ID:    "satellite-lightspeed",
			Tools: unconfiguredTools(),
		}
	}

	client := newSatelliteClient(opts.SatelliteURL, opts.Token)

	return plugin.Plugin{
		ID:    "satellite-lightspeed",
		Tools: buildTools(client),
	}
}

func main() {
	opts := options{}
	plugin.Run(newPlugin(opts))
}
