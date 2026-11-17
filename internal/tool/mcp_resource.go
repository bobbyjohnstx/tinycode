package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// MCPResourceLister is the interface the mcp_resource tool uses to list and
// read MCP resources. The mcp.Service satisfies this interface.
type MCPResourceLister interface {
	ListResources(ctx context.Context) ([]MCPResourceInfo, error)
	ListResourceTemplates(ctx context.Context) ([]MCPResourceTemplateInfo, error)
	ReadResource(ctx context.Context, uri string) (string, error)
}

// MCPResourceInfo describes a single MCP resource for tool output.
type MCPResourceInfo struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// MCPResourceTemplateInfo describes a single MCP resource template for tool output.
type MCPResourceTemplateInfo struct {
	URITemplate string `json:"uriTemplate"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// MCPResourceTool returns the mcp_resource tool definition. The svc parameter
// provides access to the MCP service's resource methods.
func MCPResourceTool(svc MCPResourceLister) *Def {
	return &Def{
		ID:          "mcp_resource",
		Description: "List and read MCP server resources. Use action 'list' to see available resources, 'templates' to see resource templates, or 'read' to read a resource by URI.",
		Permission:  "read",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"enum":        []string{"list", "templates", "read"},
					"description": "Operation: list resources, list templates, or read a resource",
				},
				"uri": map[string]any{
					"type":        "string",
					"description": "Resource URI (required when action is read)",
				},
			},
			"required": []string{"action"},
		},
		Execute: func(ctx context.Context, tc *Context, rawArgs json.RawMessage) (*ExecuteResult, error) {
			return executeMCPResource(ctx, svc, rawArgs)
		},
	}
}

func executeMCPResource(ctx context.Context, svc MCPResourceLister, rawArgs json.RawMessage) (*ExecuteResult, error) {
	var args struct {
		Action string `json:"action"`
		URI    string `json:"uri"`
	}
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Invalid arguments: %v", err), IsError: true}, nil
	}

	switch args.Action {
	case "list":
		resources, err := svc.ListResources(ctx)
		if err != nil {
			return &ExecuteResult{Output: fmt.Sprintf("Error listing resources: %v", err), IsError: true}, nil
		}
		if len(resources) == 0 {
			return &ExecuteResult{Output: "No resources available."}, nil
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "%d resources:\n", len(resources))
		for _, r := range resources {
			fmt.Fprintf(&sb, "  %s — %s", r.URI, r.Name)
			if r.Description != "" {
				fmt.Fprintf(&sb, " (%s)", r.Description)
			}
			if r.MimeType != "" {
				fmt.Fprintf(&sb, " [%s]", r.MimeType)
			}
			sb.WriteByte('\n')
		}
		return &ExecuteResult{Output: sb.String()}, nil

	case "templates":
		templates, err := svc.ListResourceTemplates(ctx)
		if err != nil {
			return &ExecuteResult{Output: fmt.Sprintf("Error listing resource templates: %v", err), IsError: true}, nil
		}
		if len(templates) == 0 {
			return &ExecuteResult{Output: "No resource templates available."}, nil
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "%d resource templates:\n", len(templates))
		for _, t := range templates {
			fmt.Fprintf(&sb, "  %s — %s", t.URITemplate, t.Name)
			if t.Description != "" {
				fmt.Fprintf(&sb, " (%s)", t.Description)
			}
			if t.MimeType != "" {
				fmt.Fprintf(&sb, " [%s]", t.MimeType)
			}
			sb.WriteByte('\n')
		}
		return &ExecuteResult{Output: sb.String()}, nil

	case "read":
		if args.URI == "" {
			return &ExecuteResult{Output: "uri is required when action is read", IsError: true}, nil
		}
		content, err := svc.ReadResource(ctx, args.URI)
		if err != nil {
			return &ExecuteResult{Output: fmt.Sprintf("Error reading resource %s: %v", args.URI, err), IsError: true}, nil
		}
		return &ExecuteResult{Output: content}, nil

	default:
		return &ExecuteResult{
			Output:  fmt.Sprintf("invalid action %q: must be list, templates, or read", args.Action),
			IsError: true,
		}, nil
	}
}
