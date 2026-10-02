package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bobbyjohnstx/tinycode/internal/redhat"
	"github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

const defaultRHDPAPIURL = "https://catalog.demo.redhat.com/api/v1"

type options struct {
	SessionCookie string
	RHDPApiURL    string
}

func parseOptions(raw map[string]any) options {
	var opts options
	if v, ok := raw["sessionCookie"].(string); ok {
		opts.SessionCookie = v
	}
	if v, ok := raw["rhdpApiUrl"].(string); ok {
		opts.RHDPApiURL = v
	}
	return opts
}

type catalogItem struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	Category      string `json:"category"`
	Provider      string `json:"provider"`
	EstimatedTime string `json:"estimatedTime"`
}

type provisionCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type provisionStatus struct {
	OrderID     string                `json:"orderId"`
	Status      string                `json:"status"`
	StartedAt   string                `json:"startedAt"`
	ReadyAt     string                `json:"readyAt"`
	ConsoleURL  string                `json:"consoleUrl"`
	APIURL      string                `json:"apiUrl"`
	Credentials *provisionCredentials `json:"credentials"`
	ExpiresAt   string                `json:"expiresAt"`
	Error       string                `json:"error"`
}

type activeEnvironment struct {
	OrderID         string `json:"orderId"`
	CatalogItemName string `json:"catalogItemName"`
	Status          string `json:"status"`
	ConsoleURL      string `json:"consoleUrl"`
	ExpiresAt       string `json:"expiresAt"`
	StartedAt       string `json:"startedAt"`
}

type rhdpClient struct {
	api *redhat.APIClient
}

func newRHDPClient(apiURL, sessionCookie string) *rhdpClient {
	return &rhdpClient{
		api: redhat.NewAPIClient(redhat.APIClientConfig{
			BaseURL: apiURL,
			Headers: map[string]string{
				"Cookie": sessionCookie,
			},
		}),
	}
}

func (c *rhdpClient) searchCatalog(ctx context.Context, query, category string) ([]catalogItem, error) {
	params := map[string]string{"q": query}
	if category != "" {
		params["category"] = category
	}
	resp, err := c.api.Get(ctx, "/catalog/search", params)
	if err != nil {
		return nil, err
	}
	var result struct {
		Items []catalogItem `json:"items"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return result.Items, nil
}

func (c *rhdpClient) provision(ctx context.Context, catalogItemID string) (*provisionStatus, error) {
	resp, err := c.api.Post(ctx, "/orders", map[string]string{
		"catalog_item_id": catalogItemID,
	})
	if err != nil {
		return nil, err
	}
	var status provisionStatus
	if err := json.Unmarshal(resp.Data, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

func (c *rhdpClient) getStatus(ctx context.Context, orderID string) (*provisionStatus, error) {
	resp, err := c.api.Get(ctx, "/orders/"+orderID, nil)
	if err != nil {
		return nil, err
	}
	var status provisionStatus
	if err := json.Unmarshal(resp.Data, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

func (c *rhdpClient) listActive(ctx context.Context) ([]activeEnvironment, error) {
	resp, err := c.api.Get(ctx, "/environments", nil)
	if err != nil {
		return nil, err
	}
	var result struct {
		Environments []activeEnvironment `json:"environments"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return result.Environments, nil
}

func formatCatalogItem(item catalogItem) string {
	parts := []string{
		"Name: " + item.Name,
		"Description: " + item.Description,
		"Category: " + item.Category,
	}
	if item.EstimatedTime != "" {
		parts = append(parts, "Estimated Time: "+item.EstimatedTime)
	}
	return strings.Join(parts, " | ")
}

func formatProvisionStatus(status *provisionStatus) string {
	parts := []string{
		"Order: " + status.OrderID,
		"Status: " + status.Status,
		"Started: " + status.StartedAt,
	}
	if status.ConsoleURL != "" {
		parts = append(parts, "Console: "+status.ConsoleURL)
	}
	if status.APIURL != "" {
		parts = append(parts, "API: "+status.APIURL)
	}
	if status.Credentials != nil {
		parts = append(parts, "Username: "+status.Credentials.Username)
		consoleRef := "the provisioning console"
		if status.ConsoleURL != "" {
			consoleRef = status.ConsoleURL
		}
		parts = append(parts, fmt.Sprintf("Password: [REDACTED -- view credentials at %s]", consoleRef))
	}
	if status.ExpiresAt != "" {
		parts = append(parts, "Expires: "+status.ExpiresAt)
	}
	if status.Error != "" {
		parts = append(parts, "Error: "+status.Error)
	}
	return strings.Join(parts, " | ")
}

func formatActiveEnvironment(env activeEnvironment) string {
	parts := []string{
		"Name: " + env.CatalogItemName,
		"Status: " + env.Status,
		"Started: " + env.StartedAt,
	}
	if env.ConsoleURL != "" {
		parts = append(parts, "Console: "+env.ConsoleURL)
	}
	if env.ExpiresAt != "" {
		parts = append(parts, "Expires: "+env.ExpiresAt)
	}
	return strings.Join(parts, " | ")
}

const notConfiguredMsg = "RHDP Provisioner not configured. Set sessionCookie in plugin options (extract from browser session at demo.redhat.com — see docs/plugin-credentials.md)."

func stubTool(name, description string) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        name,
		Description: description,
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Execute: func(_ context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
			return notConfiguredMsg, nil
		},
	}
}

func buildTools(client *rhdpClient) []plugin.ToolDef {
	if client == nil {
		return []plugin.ToolDef{
			stubTool("rhdp_search", "Search the Red Hat Demo Platform catalog for available demo environments."),
			stubTool("rhdp_provision", "Request provisioning of an RHDP demo environment."),
			stubTool("rhdp_status", "Check the provisioning status of an RHDP demo environment order."),
			stubTool("rhdp_list_active", "List all active RHDP demo environments."),
		}
	}

	return []plugin.ToolDef{
		{
			Name:        "rhdp_search",
			Description: "Search the Red Hat Demo Platform catalog for available demo environments. Returns name, description, category, and estimated provisioning time.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":    map[string]any{"type": "string", "description": "Search query keyword(s)"},
					"category": map[string]any{"type": "string", "enum": []string{"workshop", "demo", "lab", "open-environment"}, "description": "Filter by catalog category"},
				},
				"required": []string{"query"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Query    string `json:"query"`
					Category string `json:"category"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				items, err := client.searchCatalog(ctx, input.Query, input.Category)
				if err != nil {
					return fmt.Sprintf("Catalog search failed: %v", err), nil
				}
				if len(items) == 0 {
					return "No catalog items found.", nil
				}
				var lines []string
				for _, item := range items {
					lines = append(lines, formatCatalogItem(item))
				}
				return strings.Join(lines, "\n"), nil
			},
		},
		{
			Name:        "rhdp_provision",
			Description: "Request provisioning of an RHDP demo environment.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"catalogItemId": map[string]any{"type": "string", "description": "ID of the catalog item to provision"},
				},
				"required": []string{"catalogItemId"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					CatalogItemID string `json:"catalogItemId"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				status, err := client.provision(ctx, input.CatalogItemID)
				if err != nil {
					return fmt.Sprintf("Provisioning failed: %v", err), nil
				}
				return formatProvisionStatus(status), nil
			},
		},
		{
			Name:        "rhdp_status",
			Description: "Check the provisioning status of an RHDP demo environment order. Returns status, connection details, credentials, and expiry.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"orderId": map[string]any{"type": "string", "description": "Order ID to check status for"},
				},
				"required": []string{"orderId"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct{ OrderID string `json:"orderId"` }
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				status, err := client.getStatus(ctx, input.OrderID)
				if err != nil {
					return fmt.Sprintf("Status check failed: %v", err), nil
				}
				return formatProvisionStatus(status), nil
			},
		},
		{
			Name:        "rhdp_list_active",
			Description: "List all active RHDP demo environments. Returns name, status, console URL, and expiry for each environment.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Execute: func(ctx context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				envs, err := client.listActive(ctx)
				if err != nil {
					return fmt.Sprintf("Failed to list environments: %v", err), nil
				}
				if len(envs) == 0 {
					return "No active environments.", nil
				}
				var lines []string
				for _, env := range envs {
					lines = append(lines, formatActiveEnvironment(env))
				}
				return strings.Join(lines, "\n"), nil
			},
		},
	}
}

func newPlugin(opts options) plugin.Plugin {
	var client *rhdpClient
	if opts.SessionCookie != "" {
		apiURL := opts.RHDPApiURL
		if apiURL == "" {
			apiURL = defaultRHDPAPIURL
		}
		client = newRHDPClient(apiURL, opts.SessionCookie)
	}

	return plugin.Plugin{
		ID:    "rhdp-provisioner",
		Tools: buildTools(client),
	}
}

func main() {
	plugin.RunWithOptions(func(params plugin.InitializeParams) (plugin.Plugin, error) {
		opts := parseOptions(params.Options)
		return newPlugin(opts), nil
	})
}
