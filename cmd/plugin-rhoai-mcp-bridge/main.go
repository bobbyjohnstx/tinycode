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
	McpServerURL string
	OAuthToken   string
}

func parseOptions(raw map[string]any) options {
	var opts options
	if v, ok := raw["mcpServerUrl"].(string); ok {
		opts.McpServerURL = v
	}
	if v, ok := raw["oauthToken"].(string); ok {
		opts.OAuthToken = v
	}
	return opts
}

// --- MCP JSON-RPC client ---

type mcpClient struct {
	api *redhat.APIClient
}

type mcpToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func newMcpClient(serverURL, token string) *mcpClient {
	var tokenFn func(context.Context) (string, error)
	if token != "" {
		tokenFn = func(_ context.Context) (string, error) { return token, nil }
	}
	api := redhat.NewAPIClient(redhat.APIClientConfig{
		BaseURL: strings.TrimRight(serverURL, "/"),
		TokenFn: tokenFn,
	})
	return &mcpClient{api: api}
}

func (c *mcpClient) rpcCall(ctx context.Context, method string, params any) (json.RawMessage, error) {
	body := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
	}
	if params != nil {
		body["params"] = params
	}
	resp, err := c.api.Post(ctx, "", body)
	if err != nil {
		return nil, err
	}
	var rpcResp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(resp.Data, &rpcResp); err != nil {
		return nil, fmt.Errorf("parsing RPC response: %w", err)
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("MCP error: %s", rpcResp.Error.Message)
	}
	return rpcResp.Result, nil
}

func (c *mcpClient) listTools(ctx context.Context) ([]mcpToolDef, error) {
	result, err := c.rpcCall(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}
	var toolsResult struct {
		Tools []mcpToolDef `json:"tools"`
	}
	if err := json.Unmarshal(result, &toolsResult); err != nil {
		return nil, fmt.Errorf("parsing tools list: %w", err)
	}
	return toolsResult.Tools, nil
}

func (c *mcpClient) callTool(ctx context.Context, name string, args map[string]any) (string, error) {
	params := map[string]any{"name": name}
	if args != nil {
		params["arguments"] = args
	}
	result, err := c.rpcCall(ctx, "tools/call", params)
	if err != nil {
		return "", err
	}
	var toolResult struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(result, &toolResult); err != nil {
		return "", fmt.Errorf("parsing tool result: %w", err)
	}
	if toolResult.IsError {
		var texts []string
		for _, c := range toolResult.Content {
			if c.Text != "" {
				texts = append(texts, c.Text)
			}
		}
		return "", fmt.Errorf("MCP tool error: %s", strings.Join(texts, "\n"))
	}
	var texts []string
	for _, c := range toolResult.Content {
		if c.Text != "" {
			texts = append(texts, c.Text)
		}
	}
	return strings.Join(texts, "\n"), nil
}

// --- tool builders ---

func buildMcpTools(client *mcpClient) []plugin.ToolDef {
	return []plugin.ToolDef{
		{
			Name:        "rhoai_mcp_list",
			Description: "List available tools on the remote MCP server.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Execute: func(ctx context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				tools, err := client.listTools(ctx)
				if err != nil {
					return fmt.Sprintf("Failed to list MCP tools: %v", err), nil
				}
				if len(tools) == 0 {
					return "No tools available on the MCP server.", nil
				}
				lines := []string{fmt.Sprintf("Available MCP Tools: %d", len(tools)), ""}
				for _, t := range tools {
					desc := t.Description
					if desc == "" {
						desc = "No description"
					}
					lines = append(lines, fmt.Sprintf("  %s — %s", t.Name, desc))
				}
				return strings.Join(lines, "\n"), nil
			},
		},
		{
			Name:        "rhoai_mcp_call",
			Description: "Call a tool on the remote MCP server by name with optional JSON arguments.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"tool": map[string]any{"type": "string", "description": "Name of the MCP tool to call"},
					"args": map[string]any{"type": "string", "description": "JSON string of tool arguments"},
				},
				"required": []string{"tool"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Tool string `json:"tool"`
					Args string `json:"args"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				var toolArgs map[string]any
				if input.Args != "" {
					if err := json.Unmarshal([]byte(input.Args), &toolArgs); err != nil {
						return fmt.Sprintf("Invalid JSON arguments: %v", err), nil
					}
				}
				result, err := client.callTool(ctx, input.Tool, toolArgs)
				if err != nil {
					return fmt.Sprintf("MCP tool call failed: %v", err), nil
				}
				return result, nil
			},
		},
	}
}

func unconfiguredMcpTools() []plugin.ToolDef {
	msg := "MCP bridge not configured. Set mcpServerUrl in plugin options."
	return []plugin.ToolDef{
		{
			Name:        "rhoai_mcp_list",
			Description: "List available tools on the remote MCP server.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Execute: func(_ context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				return msg, nil
			},
		},
		{
			Name:        "rhoai_mcp_call",
			Description: "Call a tool on the remote MCP server.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"tool": map[string]any{"type": "string", "description": "Tool name"},
				},
				"required": []string{"tool"},
			},
			Execute: func(_ context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				return msg, nil
			},
		},
	}
}

func newPlugin(opts options) plugin.Plugin {
	if opts.McpServerURL == "" {
		return plugin.Plugin{
			ID:    "rhoai-mcp-bridge",
			Tools: unconfiguredMcpTools(),
		}
	}

	client := newMcpClient(opts.McpServerURL, opts.OAuthToken)
	return plugin.Plugin{
		ID:    "rhoai-mcp-bridge",
		Tools: buildMcpTools(client),
	}
}

func main() {
	opts := options{}
	plugin.Run(newPlugin(opts))
}
