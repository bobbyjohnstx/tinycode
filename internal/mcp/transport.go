package mcp

import (
	"context"
	"encoding/json"
)

type Transport interface {
	Connect(ctx context.Context) error
	ListTools(ctx context.Context) ([]MCPTool, error)
	CallTool(ctx context.Context, name string, args json.RawMessage) (string, error)
	ListResources(ctx context.Context) ([]MCPResource, error)
	ListResourceTemplates(ctx context.Context) ([]MCPResourceTemplate, error)
	ReadResource(ctx context.Context, uri string) (string, error)
	Close() error
}
