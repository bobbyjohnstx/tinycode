package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/bobbyjohnstx/tinycode-go/internal/redhat"
	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

type apiEntry struct {
	Name        string
	Description string
	Version     string
	BasePath    string
	Platform    string // "ocp", "rhel", or "both"
}

var apiCatalog = []apiEntry{
	{Name: "cost-management", Description: "Cost Management API for OpenShift and cloud resources", Version: "v1", BasePath: "/api/cost-management/v1", Platform: "both"},
	{Name: "insights", Description: "Red Hat Insights advisor API", Version: "v1", BasePath: "/api/insights/v1", Platform: "both"},
	{Name: "vulnerability", Description: "Vulnerability assessment and CVE management", Version: "v1", BasePath: "/api/vulnerability/v1", Platform: "rhel"},
	{Name: "patch", Description: "Content and patch management for RHEL systems", Version: "v3", BasePath: "/api/patch/v3", Platform: "rhel"},
	{Name: "inventory", Description: "Host-based Inventory for managed systems", Version: "v1", BasePath: "/api/inventory/v1", Platform: "both"},
	{Name: "notifications", Description: "Notifications service for event delivery", Version: "v1", BasePath: "/api/notifications/v1", Platform: "both"},
	{Name: "integrations", Description: "Third-party integrations for notifications", Version: "v1", BasePath: "/api/integrations/v1", Platform: "both"},
	{Name: "rbac", Description: "Role-based access control", Version: "v1", BasePath: "/api/rbac/v1", Platform: "both"},
	{Name: "sources", Description: "Cloud source management", Version: "v3", BasePath: "/api/sources/v3.1", Platform: "both"},
	{Name: "image-builder", Description: "RHEL image composition service", Version: "v1", BasePath: "/api/image-builder/v1", Platform: "rhel"},
	{Name: "edge", Description: "Fleet Management for Edge Devices", Version: "v1", BasePath: "/api/edge/v1", Platform: "rhel"},
	{Name: "subscriptions", Description: "Subscription management and watch", Version: "v1", BasePath: "/api/rhsm-subscriptions/v1", Platform: "both"},
	{Name: "compliance", Description: "OpenSCAP compliance and policy management", Version: "v2", BasePath: "/api/compliance/v2", Platform: "both"},
	{Name: "ros", Description: "Resource Optimization for cloud instances", Version: "v1", BasePath: "/api/ros/v1", Platform: "rhel"},
	{Name: "malware-detection", Description: "Malware detection for RHEL systems", Version: "v1", BasePath: "/api/malware-detection/v1", Platform: "rhel"},
	{Name: "tasks", Description: "Tasks service for async job management", Version: "v1", BasePath: "/api/tasks/v1", Platform: "both"},
	{Name: "config-manager", Description: "Cloud connector configuration", Version: "v2", BasePath: "/api/config-manager/v2", Platform: "rhel"},
	{Name: "playbook-dispatcher", Description: "Playbook execution dispatcher for Remediations", Version: "v1", BasePath: "/api/playbook-dispatcher/v1", Platform: "rhel"},
	{Name: "remediations", Description: "Remediation playbook generation", Version: "v1", BasePath: "/api/remediations/v1", Platform: "rhel"},
	{Name: "drift", Description: "System comparison and baseline drift", Version: "v1", BasePath: "/api/drift/v1", Platform: "rhel"},
	{Name: "policies", Description: "Custom alert policies for system events", Version: "v1", BasePath: "/api/policies/v1", Platform: "rhel"},
	{Name: "gathering", Description: "Insights Operator gathering conditions", Version: "v1", BasePath: "/api/gathering/v1", Platform: "ocp"},
	{Name: "ocp-vulnerability", Description: "Container vulnerability analysis for OCP", Version: "v1", BasePath: "/api/ocp-vulnerability/v1", Platform: "ocp"},
	{Name: "content-sources", Description: "Custom RPM repository management", Version: "v1", BasePath: "/api/content-sources/v1", Platform: "rhel"},
	{Name: "provisioning", Description: "Cloud resource provisioning", Version: "v1", BasePath: "/api/provisioning/v1", Platform: "both"},
}

type apiEndpoint struct {
	Method      string
	Path        string
	Description string
	Params      []string
}

func searchCatalog(query string) []apiEntry {
	lower := strings.ToLower(query)
	var results []apiEntry
	for _, e := range apiCatalog {
		if strings.Contains(strings.ToLower(e.Name), lower) ||
			strings.Contains(strings.ToLower(e.Description), lower) ||
			strings.Contains(strings.ToLower(e.Platform), lower) {
			results = append(results, e)
		}
	}
	return results
}

func findCatalogEntry(name string) *apiEntry {
	for i := range apiCatalog {
		if apiCatalog[i].Name == name {
			return &apiCatalog[i]
		}
	}
	return nil
}

func parseOpenAPIEndpoints(spec map[string]any) []apiEndpoint {
	pathsRaw, ok := spec["paths"]
	if !ok {
		return nil
	}
	paths, ok := pathsRaw.(map[string]any)
	if !ok {
		return nil
	}

	httpMethods := map[string]bool{
		"get": true, "post": true, "put": true, "patch": true, "delete": true,
	}

	var endpoints []apiEndpoint
	for path, methodsRaw := range paths {
		methods, ok := methodsRaw.(map[string]any)
		if !ok {
			continue
		}
		for method, detailRaw := range methods {
			if !httpMethods[method] {
				continue
			}
			detail, ok := detailRaw.(map[string]any)
			if !ok {
				continue
			}
			desc := ""
			if s, ok := detail["summary"].(string); ok && s != "" {
				desc = s
			} else if s, ok := detail["description"].(string); ok {
				desc = s
			}
			var params []string
			if paramsRaw, ok := detail["parameters"].([]any); ok {
				for _, pRaw := range paramsRaw {
					if p, ok := pRaw.(map[string]any); ok {
						if name, ok := p["name"].(string); ok {
							params = append(params, name)
						}
					}
				}
			}
			endpoints = append(endpoints, apiEndpoint{
				Method:      strings.ToUpper(method),
				Path:        path,
				Description: desc,
				Params:      params,
			})
		}
	}
	return endpoints
}

func formatEndpoints(endpoints []apiEndpoint, search string) string {
	filtered := endpoints
	if search != "" {
		lower := strings.ToLower(search)
		filtered = nil
		for _, ep := range endpoints {
			if strings.Contains(strings.ToLower(ep.Path), lower) ||
				strings.Contains(strings.ToLower(ep.Method), lower) ||
				strings.Contains(strings.ToLower(ep.Description), lower) {
				filtered = append(filtered, ep)
			}
		}
	}
	if len(filtered) == 0 {
		return "No matching endpoints found."
	}
	var lines []string
	for _, ep := range filtered {
		paramStr := ""
		if len(ep.Params) > 0 {
			paramStr = " [" + strings.Join(ep.Params, ", ") + "]"
		}
		desc := ep.Description
		if desc == "" {
			desc = "(no description)"
		}
		lines = append(lines, fmt.Sprintf("%s %s%s — %s", ep.Method, ep.Path, paramStr, desc))
	}
	return strings.Join(lines, "\n")
}

const maxSpecLength = 5000

func formatSpec(spec map[string]any) string {
	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return fmt.Sprintf("Failed to format spec: %v", err)
	}
	s := string(data)
	if len(s) > maxSpecLength {
		return s[:maxSpecLength] + fmt.Sprintf("\n\n... (truncated, full spec is %d characters)", len(s))
	}
	return s
}

type options struct {
	ConsoleOfflineToken string
	ClientID            string
	CatalogPath         string
}

func parseOptions(raw map[string]any) options {
	var opts options
	if v, ok := raw["consoleOfflineToken"].(string); ok {
		opts.ConsoleOfflineToken = v
	}
	if v, ok := raw["clientId"].(string); ok {
		opts.ClientID = v
	}
	if v, ok := raw["catalogPath"].(string); ok {
		opts.CatalogPath = v
	}
	return opts
}

func resolveSpec(ctx context.Context, consoleClient *redhat.APIClient, catalogPath, apiName string) (map[string]any, error) {
	if catalogPath != "" {
		data, err := os.ReadFile(catalogPath + "/" + apiName + ".json")
		if err == nil {
			var spec map[string]any
			if err := json.Unmarshal(data, &spec); err == nil {
				return spec, nil
			}
		}
	}
	if consoleClient != nil {
		entry := findCatalogEntry(apiName)
		if entry == nil {
			return nil, fmt.Errorf("unknown API %q", apiName)
		}
		resp, err := consoleClient.Get(ctx, entry.BasePath+"/openapi.json", nil)
		if err != nil {
			return nil, err
		}
		var spec map[string]any
		if err := json.Unmarshal(resp.Data, &spec); err != nil {
			return nil, err
		}
		return spec, nil
	}
	return nil, nil
}

const notAvailableMsg = "API spec not available. Configure consoleOfflineToken for live fetching or catalogPath for cached specs."

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

func buildTools(consoleClient *redhat.APIClient, catalogPath string) []plugin.ToolDef {
	hasSource := consoleClient != nil || catalogPath != ""

	return []plugin.ToolDef{
		{
			Name:        "rh_api_list",
			Description: "List all Red Hat console.redhat.com APIs with name, description, and version. Filter by keyword search.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"search": map[string]any{"type": "string", "description": "Filter APIs by keyword match on name or description"},
				},
			},
			Execute: func(_ context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct{ Search string `json:"search"` }
				json.Unmarshal(args, &input)

				results := apiCatalog
				if input.Search != "" {
					results = searchCatalog(input.Search)
				}
				if len(results) == 0 {
					return "No APIs found matching the search query.", nil
				}
				var lines []string
				for _, api := range results {
					lines = append(lines, fmt.Sprintf("%s (%s) [%s] — %s [%s]",
						api.Name, api.Version, api.Platform, api.Description, api.BasePath))
				}
				return strings.Join(lines, "\n"), nil
			},
		},
		{
			Name:        "rh_api_spec",
			Description: "Fetch the OpenAPI spec for a specific Red Hat console.redhat.com API. Returns formatted JSON, truncated if very large.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"api": map[string]any{"type": "string", "description": "API name (e.g., 'cost-management', 'insights')"},
				},
				"required": []string{"api"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				if !hasSource {
					return notAvailableMsg, nil
				}
				var input struct{ API string `json:"api"` }
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				entry := findCatalogEntry(input.API)
				if entry == nil {
					return fmt.Sprintf("Unknown API %q. Use rh_api_list to see available APIs.", input.API), nil
				}
				spec, err := resolveSpec(ctx, consoleClient, catalogPath, input.API)
				if err != nil {
					return fmt.Sprintf("Failed to fetch spec: %v", err), nil
				}
				if spec == nil {
					return notAvailableMsg, nil
				}
				return formatSpec(spec), nil
			},
		},
		{
			Name:        "rh_api_endpoints",
			Description: "List endpoints for a Red Hat console.redhat.com API without the full spec. Shows method, path, description, and parameters.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"api":    map[string]any{"type": "string", "description": "API name (e.g., 'cost-management', 'insights')"},
					"search": map[string]any{"type": "string", "description": "Filter endpoints by keyword match on path, method, or description"},
				},
				"required": []string{"api"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				if !hasSource {
					return notAvailableMsg, nil
				}
				var input struct {
					API    string `json:"api"`
					Search string `json:"search"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				entry := findCatalogEntry(input.API)
				if entry == nil {
					return fmt.Sprintf("Unknown API %q. Use rh_api_list to see available APIs.", input.API), nil
				}
				spec, err := resolveSpec(ctx, consoleClient, catalogPath, input.API)
				if err != nil {
					return fmt.Sprintf("Failed to fetch endpoints: %v", err), nil
				}
				if spec == nil {
					return notAvailableMsg, nil
				}
				endpoints := parseOpenAPIEndpoints(spec)
				return formatEndpoints(endpoints, input.Search), nil
			},
		},
	}
}

func newPlugin(opts options) plugin.Plugin {
	var consoleClient *redhat.APIClient
	if opts.ConsoleOfflineToken != "" {
		authCfg := redhat.ConsoleAuthConfig{
			OfflineToken: opts.ConsoleOfflineToken,
			ClientID:     opts.ClientID,
		}
		authClient := redhat.NewConsoleAuthClient(authCfg)
		consoleClient = redhat.NewConsoleAPIClient(authCfg, "", authClient)
	}

	return plugin.Plugin{
		ID:    "rh-api-catalog",
		Tools: buildTools(consoleClient, opts.CatalogPath),
	}
}

func main() {
	plugin.RunWithOptions(func(params plugin.InitializeParams) (plugin.Plugin, error) {
		opts := parseOptions(params.Options)
		return newPlugin(opts), nil
	})
}
