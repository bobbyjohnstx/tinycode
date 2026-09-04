package plugin

import (
	"context"
	"encoding/json"
)

// ToolDef defines a tool that a plugin exposes to the tinycode session.
type ToolDef struct {
	Name        string
	Description string
	Parameters  map[string]any
	Execute     func(ctx context.Context, args json.RawMessage, tc ToolContext) (string, error)
}
