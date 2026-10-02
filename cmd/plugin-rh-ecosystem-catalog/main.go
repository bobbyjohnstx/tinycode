package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

const pyxisBase = "https://catalog.redhat.com/api/containers/v1"

var httpClient = &http.Client{Timeout: 30 * time.Second}

func pyxisGet(path string, params map[string]string) (json.RawMessage, error) {
	u, err := url.Parse(pyxisBase + path)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Pyxis API %d: %s", resp.StatusCode, string(body))
	}
	return json.RawMessage(body), nil
}

type pyxisDisplayData struct {
	Name             string `json:"name"`
	ShortDescription string `json:"short_description"`
	LongDescription  string `json:"long_description"`
}

type pyxisRepo struct {
	Repository            string            `json:"repository"`
	Registry              string            `json:"registry"`
	Description           string            `json:"description"`
	DisplayData           *pyxisDisplayData `json:"display_data"`
	ApplicationCategories []string          `json:"application_categories"`
	BuildCategories       []string          `json:"build_categories"`
	LastUpdateDate        string            `json:"last_update_date"`
}

type pyxisOperatorBundle struct {
	CSVDisplayName string          `json:"csv_display_name"`
	CSVDescription string          `json:"csv_description"`
	Package        string          `json:"package"`
	Version        string          `json:"version"`
	OCPVersion     string          `json:"ocp_version"`
	Organization   string          `json:"organization"`
	ChannelName    string          `json:"channel_name"`
	Capabilities   json.RawMessage `json:"capabilities"`
}

type pyxisResponse[T any] struct {
	Data     []T `json:"data"`
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	Total    int `json:"total"`
}

func formatRepo(repo pyxisRepo) string {
	registry := repo.Registry
	if registry == "" {
		registry = "registry.redhat.com"
	}
	repository := repo.Repository
	if repository == "" {
		repository = "unknown"
	}
	name := repository
	if repo.DisplayData != nil && repo.DisplayData.Name != "" {
		name = repo.DisplayData.Name
	}
	parts := []string{
		fmt.Sprintf("Image: %s/%s", registry, repository),
		fmt.Sprintf("Name: %s", name),
	}
	if repo.DisplayData != nil && repo.DisplayData.ShortDescription != "" {
		parts = append(parts, "Description: "+repo.DisplayData.ShortDescription)
	}
	if len(repo.ApplicationCategories) > 0 {
		parts = append(parts, "Categories: "+strings.Join(repo.ApplicationCategories, ", "))
	}
	if repo.LastUpdateDate != "" {
		parts = append(parts, "Updated: "+strings.SplitN(repo.LastUpdateDate, "T", 2)[0])
	}
	return strings.Join(parts, " | ")
}

func formatOperator(op pyxisOperatorBundle) string {
	name := op.CSVDisplayName
	if name == "" {
		name = op.Package
	}
	if name == "" {
		name = "unknown"
	}
	pkg := op.Package
	if pkg == "" {
		pkg = "unknown"
	}
	version := op.Version
	if version == "" {
		version = "unknown"
	}
	parts := []string{
		"Operator: " + name,
		"Package: " + pkg,
		"Version: " + version,
	}
	if op.OCPVersion != "" {
		parts = append(parts, "OCP: "+op.OCPVersion)
	}
	if op.Organization != "" {
		parts = append(parts, "Source: "+op.Organization)
	}
	if op.ChannelName != "" {
		parts = append(parts, "Channel: "+op.ChannelName)
	}
	return strings.Join(parts, " | ")
}

func buildTools() []plugin.ToolDef {
	return []plugin.ToolDef{
		{
			Name:        "ecosystem_search",
			Description: "Search the Red Hat Ecosystem Catalog for certified container images by repository name (e.g., ubi9, nodejs-18, httpd-24). Uses the Pyxis API.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"repository": map[string]any{"type": "string", "description": "Container image repository name to look up (e.g., ubi9, nodejs-18, httpd-24)"},
					"page_size":  map[string]any{"type": "integer", "description": "Number of results (default 10, max 50)"},
				},
				"required": []string{"repository"},
			},
			Execute: func(_ context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Repository string `json:"repository"`
					PageSize   int    `json:"page_size"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				pageSize := input.PageSize
				if pageSize <= 0 {
					pageSize = 10
				}
				pageSize = int(math.Min(float64(pageSize), 50))

				data, err := pyxisGet("/repositories", map[string]string{
					"page_size": fmt.Sprintf("%d", pageSize),
					"filter":    fmt.Sprintf("repository==%s", input.Repository),
					"include":   "data.repository,data.registry,data.display_data,data.application_categories,data.last_update_date",
				})
				if err != nil {
					return fmt.Sprintf("Search failed: %v", err), nil
				}
				var result pyxisResponse[pyxisRepo]
				if err := json.Unmarshal(data, &result); err != nil {
					return fmt.Sprintf("Search failed: %v", err), nil
				}
				if len(result.Data) == 0 {
					return fmt.Sprintf("No container image found for repository %q. Try an exact repository name like ubi9, nodejs-18, httpd-24, or postgresql-16.", input.Repository), nil
				}
				var lines []string
				for _, r := range result.Data {
					lines = append(lines, formatRepo(r))
				}
				return strings.Join(lines, "\n"), nil
			},
		},
		{
			Name:        "ecosystem_operator",
			Description: "Search certified operators in the Red Hat Ecosystem Catalog by package name. Returns operator name, version, supported OCP versions, and source catalog.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"package":   map[string]any{"type": "string", "description": "Operator package name (e.g., amq-streams, elasticsearch-operator, grafana-operator)"},
					"page_size": map[string]any{"type": "integer", "description": "Number of results (default 5, max 20)"},
				},
				"required": []string{"package"},
			},
			Execute: func(_ context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Package  string `json:"package"`
					PageSize int    `json:"page_size"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				pageSize := input.PageSize
				if pageSize <= 0 {
					pageSize = 5
				}
				pageSize = int(math.Min(float64(pageSize), 20))

				data, err := pyxisGet("/operators/bundles", map[string]string{
					"page_size": fmt.Sprintf("%d", pageSize),
					"filter":    fmt.Sprintf("package==%s", input.Package),
					"include":   "data.csv_display_name,data.package,data.version,data.ocp_version,data.organization,data.channel_name,data.capabilities",
					"sort_by":   "creation_date[desc]",
				})
				if err != nil {
					return fmt.Sprintf("Operator lookup failed: %v", err), nil
				}
				var result pyxisResponse[pyxisOperatorBundle]
				if err := json.Unmarshal(data, &result); err != nil {
					return fmt.Sprintf("Operator lookup failed: %v", err), nil
				}
				if len(result.Data) == 0 {
					return fmt.Sprintf("No operator found for package %q. Try an exact package name like amq-streams, elasticsearch-operator, or grafana-operator.", input.Package), nil
				}
				var lines []string
				for _, op := range result.Data {
					lines = append(lines, formatOperator(op))
				}
				return strings.Join(lines, "\n"), nil
			},
		},
		{
			Name:        "ecosystem_browse",
			Description: "Browse the Red Hat Ecosystem Catalog — list recent certified container images or operators. Useful for discovering what's available.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"type":      map[string]any{"type": "string", "enum": []string{"containers", "operators"}, "description": "What to browse"},
					"page_size": map[string]any{"type": "integer", "description": "Number of results (default 10, max 50)"},
					"page":      map[string]any{"type": "integer", "description": "Page number for pagination (default 0)"},
				},
				"required": []string{"type"},
			},
			Execute: func(_ context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Type     string `json:"type"`
					PageSize int    `json:"page_size"`
					Page     int    `json:"page"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				pageSize := input.PageSize
				if pageSize <= 0 {
					pageSize = 10
				}
				pageSize = int(math.Min(float64(pageSize), 50))
				page := input.Page

				if input.Type == "containers" {
					data, err := pyxisGet("/repositories", map[string]string{
						"page_size": fmt.Sprintf("%d", pageSize),
						"page":      fmt.Sprintf("%d", page),
						"sort_by":   "last_update_date[desc]",
						"include":   "data.repository,data.registry,data.display_data,data.application_categories,data.last_update_date",
					})
					if err != nil {
						return fmt.Sprintf("Browse failed: %v", err), nil
					}
					var result pyxisResponse[pyxisRepo]
					if err := json.Unmarshal(data, &result); err != nil {
						return fmt.Sprintf("Browse failed: %v", err), nil
					}
					if len(result.Data) == 0 {
						return "No results.", nil
					}
					header := fmt.Sprintf("Showing %d of %d certified container images (page %d):\n", len(result.Data), result.Total, page)
					var lines []string
					for _, r := range result.Data {
						lines = append(lines, formatRepo(r))
					}
					return header + strings.Join(lines, "\n"), nil
				}

				data, err := pyxisGet("/operators/bundles", map[string]string{
					"page_size": fmt.Sprintf("%d", pageSize),
					"page":      fmt.Sprintf("%d", page),
					"sort_by":   "creation_date[desc]",
					"include":   "data.csv_display_name,data.package,data.version,data.ocp_version,data.organization,data.channel_name",
				})
				if err != nil {
					return fmt.Sprintf("Browse failed: %v", err), nil
				}
				var result pyxisResponse[pyxisOperatorBundle]
				if err := json.Unmarshal(data, &result); err != nil {
					return fmt.Sprintf("Browse failed: %v", err), nil
				}
				if len(result.Data) == 0 {
					return "No results.", nil
				}
				header := fmt.Sprintf("Showing %d of %d operator bundles (page %d):\n", len(result.Data), result.Total, page)
				var lines []string
				for _, op := range result.Data {
					lines = append(lines, formatOperator(op))
				}
				return header + strings.Join(lines, "\n"), nil
			},
		},
	}
}

func newPlugin() plugin.Plugin {
	return plugin.Plugin{
		ID:    "rh-ecosystem-catalog",
		Tools: buildTools(),
	}
}

func main() {
	plugin.Run(newPlugin())
}
