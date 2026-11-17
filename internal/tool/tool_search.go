package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bobbyjohnstx/tinycode/internal/llm"
)

const (
	// DeferredToolCountThreshold is the minimum number of deferred tools
	// before deferral becomes active and schemas are omitted.
	DeferredToolCountThreshold = 20

	// DeferredSchemaSizeBytes is the minimum total schema JSON size (in bytes)
	// of deferred tools before deferral becomes active.
	DeferredSchemaSizeBytes = 30 * 1024

	// MaxToolSearchResults is the maximum number of tools returned by a
	// single tool_search call.
	MaxToolSearchResults = 10
)

// NewToolSearchDef creates the tool_search builtin tool. The returned Def
// closes over the registry so it can search and activate deferred tools.
func NewToolSearchDef(registry *Registry) *Def {
	return &Def{
		ID:          "tool_search",
		Description: "Search for and activate deferred tools by name or keyword. Some tools are deferred to save context. Returns matching tool schemas and activates them for the rest of the session.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search query to match against tool names and descriptions",
				},
			},
			"required": []string{"query"},
		},
		Execute: func(_ context.Context, _ *Context, rawArgs json.RawMessage) (*ExecuteResult, error) {
			var args struct {
				Query string `json:"query"`
			}
			if err := json.Unmarshal(rawArgs, &args); err != nil {
				return &ExecuteResult{Output: fmt.Sprintf("Invalid arguments: %v", err), IsError: true}, nil
			}
			if args.Query == "" {
				return &ExecuteResult{Output: "query is required", IsError: true}, nil
			}

			results := registry.SearchDeferred(args.Query)
			if len(results) == 0 {
				return &ExecuteResult{Output: "No matching deferred tools found."}, nil
			}

			// Activate matched tools.
			names := make([]string, len(results))
			for i, t := range results {
				names[i] = t.Function.Name
			}
			registry.ActivateTools(names...)

			b, err := json.MarshalIndent(results, "", "  ")
			if err != nil {
				return &ExecuteResult{Output: fmt.Sprintf("Error marshaling results: %v", err), IsError: true}, nil
			}
			return &ExecuteResult{Output: fmt.Sprintf("Found and activated %d tool(s):\n%s", len(results), string(b))}, nil
		},
	}
}

// SearchDeferred returns deferred tools whose name or description contains
// the query string (case-insensitive). At most MaxToolSearchResults are returned.
func (r *Registry) SearchDeferred(query string) []llm.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	q := strings.ToLower(query)
	var results []llm.Tool

	for _, name := range r.order {
		def := r.tools[name]
		if !def.Deferred || r.disabled[name] {
			continue
		}
		nameLower := strings.ToLower(name)
		descLower := strings.ToLower(def.Description)
		if strings.Contains(nameLower, q) || strings.Contains(descLower, q) {
			results = append(results, def.ToLLMTool())
			if len(results) >= MaxToolSearchResults {
				break
			}
		}
	}

	return results
}

// ActivateTools marks deferred tools as activated so they appear in ToolDefs
// and can be executed for the rest of the session.
func (r *Registry) ActivateTools(names ...string) {
	for _, name := range names {
		r.activated.Mark(name)
	}
}

// deferredSchemaSize returns the total JSON byte size of all tool parameter
// schemas in the given slice.
func deferredSchemaSize(tools []llm.Tool) int {
	var total int
	for _, t := range tools {
		if t.Function.Parameters != nil {
			b, err := json.Marshal(t.Function.Parameters)
			if err == nil {
				total += len(b)
			}
		}
	}
	return total
}
