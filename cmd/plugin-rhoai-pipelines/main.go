package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bobbyjohnstx/tinycode/internal/redhat"
	"github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

type options struct {
	PipelinesURL string
	Namespace    string
	Token        string
	APIPrefix    string
}

func parseOptions(raw map[string]any) options {
	var opts options
	if v, ok := raw["pipelinesUrl"].(string); ok {
		opts.PipelinesURL = v
	}
	if v, ok := raw["namespace"].(string); ok {
		opts.Namespace = v
	}
	if v, ok := raw["token"].(string); ok {
		opts.Token = v
	}
	if v, ok := raw["apiPrefix"].(string); ok {
		opts.APIPrefix = v
	}
	if opts.APIPrefix == "" {
		opts.APIPrefix = "/apis/v2beta1"
	}
	return opts
}

// --- pipeline types ---

type pipeline struct {
	PipelineID  string `json:"pipeline_id"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	CreatedAt   string `json:"created_at"`
}

type pipelineTask struct {
	RunID       string `json:"run_id"`
	TaskID      string `json:"task_id"`
	DisplayName string `json:"display_name"`
	State       string `json:"state"`
}

type pipelineRun struct {
	RunID       string `json:"run_id"`
	DisplayName string `json:"display_name"`
	State       string `json:"state"`
	CreatedAt   string `json:"created_at"`
	FinishedAt  string `json:"finished_at,omitempty"`
	Error       string `json:"error,omitempty"`
}

type pipelineRunDetail struct {
	pipelineRun
	Tasks []pipelineTask `json:"tasks,omitempty"`
}

// --- pipeline client ---

type pipelineClient struct {
	api       *redhat.APIClient
	apiPrefix string
}

func newPipelineClient(apiURL, token, apiPrefix string) *pipelineClient {
	var tokenFn func(context.Context) (string, error)
	if token != "" {
		tokenFn = func(_ context.Context) (string, error) { return token, nil }
	}
	api := redhat.NewAPIClient(redhat.APIClientConfig{
		BaseURL: strings.TrimRight(apiURL, "/"),
		TokenFn: tokenFn,
	})
	return &pipelineClient{api: api, apiPrefix: apiPrefix}
}

func (c *pipelineClient) listPipelines(ctx context.Context, namespace string) ([]pipeline, error) {
	var query map[string]string
	if namespace != "" {
		query = map[string]string{"namespace": namespace}
	}
	resp, err := c.api.Get(ctx, c.apiPrefix+"/pipelines", query)
	if err != nil {
		return nil, err
	}
	var result struct {
		Pipelines []pipeline `json:"pipelines"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return result.Pipelines, nil
}

func (c *pipelineClient) createRun(ctx context.Context, pipelineID string, params map[string]any) (*pipelineRun, error) {
	body := map[string]any{
		"pipeline_id": pipelineID,
	}
	if params != nil {
		body["runtime_config"] = map[string]any{
			"parameters": params,
		}
	}
	resp, err := c.api.Post(ctx, c.apiPrefix+"/runs", body)
	if err != nil {
		return nil, err
	}
	var run pipelineRun
	if err := json.Unmarshal(resp.Data, &run); err != nil {
		return nil, err
	}
	return &run, nil
}

func (c *pipelineClient) getRunStatus(ctx context.Context, runID string) (*pipelineRunDetail, error) {
	resp, err := c.api.Get(ctx, "/apis/v2beta1/runs/"+runID, nil)
	if err != nil {
		return nil, err
	}
	var detail pipelineRunDetail
	if err := json.Unmarshal(resp.Data, &detail); err != nil {
		return nil, err
	}
	return &detail, nil
}

func (c *pipelineClient) createPipeline(ctx context.Context, yaml string) (*pipeline, error) {
	resp, err := c.api.Post(ctx, c.apiPrefix+"/pipelines", map[string]any{
		"pipeline_spec": yaml,
	})
	if err != nil {
		return nil, err
	}
	var p pipeline
	if err := json.Unmarshal(resp.Data, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// --- formatters ---

func formatPipelines(pipelines []pipeline) string {
	if len(pipelines) == 0 {
		return "No pipelines found."
	}
	lines := []string{fmt.Sprintf("Pipelines: %d", len(pipelines)), ""}
	for _, p := range pipelines {
		desc := p.Description
		if desc == "" {
			desc = "No description"
		}
		lines = append(lines, fmt.Sprintf("  [%s] %s", p.PipelineID, p.DisplayName))
		lines = append(lines, fmt.Sprintf("    %s | created: %s", desc, p.CreatedAt))
	}
	return strings.Join(lines, "\n")
}

func taskStatusIcon(state string) string {
	switch state {
	case "SUCCEEDED":
		return "DONE"
	case "FAILED":
		return "FAIL"
	case "RUNNING":
		return "RUN"
	case "PENDING":
		return "WAIT"
	case "SKIPPED":
		return "SKIP"
	case "CANCELLED":
		return "CANCEL"
	default:
		return state
	}
}

func formatRunStatus(detail *pipelineRunDetail) string {
	lines := []string{
		fmt.Sprintf("Run: %s (%s)", detail.DisplayName, detail.RunID),
		fmt.Sprintf("State: %s", detail.State),
		fmt.Sprintf("Created: %s", detail.CreatedAt),
	}
	if detail.FinishedAt != "" {
		lines = append(lines, fmt.Sprintf("Finished: %s", detail.FinishedAt))
	}
	if detail.Error != "" {
		lines = append(lines, fmt.Sprintf("Error: %s", detail.Error))
	}
	if len(detail.Tasks) > 0 {
		lines = append(lines, "", "Tasks:")
		for _, t := range detail.Tasks {
			name := t.DisplayName
			if name == "" {
				name = t.TaskID
			}
			lines = append(lines, fmt.Sprintf("  [%s] %s", taskStatusIcon(t.State), name))
		}
	}
	return strings.Join(lines, "\n")
}

// --- tool builders ---

func buildPipelineTools(client *pipelineClient, defaultNS string) []plugin.ToolDef {
	return []plugin.ToolDef{
		{
			Name:        "rhoai_pipeline_list",
			Description: "List pipelines registered in the RHOAI pipeline server.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"namespace": map[string]any{"type": "string", "description": "Namespace to filter pipelines"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Namespace string `json:"namespace"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				ns := input.Namespace
				if ns == "" {
					ns = defaultNS
				}
				pipelines, err := client.listPipelines(ctx, ns)
				if err != nil {
					return fmt.Sprintf("Failed to list pipelines: %v", err), nil
				}
				return formatPipelines(pipelines), nil
			},
		},
		{
			Name:        "rhoai_pipeline_run",
			Description: "Start a pipeline run with optional parameters.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pipelineId": map[string]any{"type": "string", "description": "Pipeline ID to run"},
					"params":     map[string]any{"type": "string", "description": "Optional JSON string of runtime parameters"},
				},
				"required": []string{"pipelineId"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					PipelineID string `json:"pipelineId"`
					Params     string `json:"params"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				var params map[string]any
				if input.Params != "" {
					if err := json.Unmarshal([]byte(input.Params), &params); err != nil {
						return fmt.Sprintf("Invalid params JSON: %v", err), nil
					}
				}
				run, err := client.createRun(ctx, input.PipelineID, params)
				if err != nil {
					return fmt.Sprintf("Failed to start pipeline run: %v", err), nil
				}
				return fmt.Sprintf("Pipeline run started.\nRun ID: %s\nState: %s", run.RunID, run.State), nil
			},
		},
		{
			Name:        "rhoai_pipeline_status",
			Description: "Get detailed status of a pipeline run including task states.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"runId": map[string]any{"type": "string", "description": "Pipeline run ID to check"},
				},
				"required": []string{"runId"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					RunID string `json:"runId"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				detail, err := client.getRunStatus(ctx, input.RunID)
				if err != nil {
					return fmt.Sprintf("Failed to get run status: %v", err), nil
				}
				return formatRunStatus(detail), nil
			},
		},
		{
			Name:        "rhoai_pipeline_create",
			Description: "Create a new pipeline from a YAML pipeline specification.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"yaml": map[string]any{"type": "string", "description": "Pipeline YAML specification"},
				},
				"required": []string{"yaml"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					YAML string `json:"yaml"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				p, err := client.createPipeline(ctx, input.YAML)
				if err != nil {
					return fmt.Sprintf("Failed to create pipeline: %v", err), nil
				}
				return fmt.Sprintf("Pipeline created.\nID: %s\nName: %s", p.PipelineID, p.DisplayName), nil
			},
		},
	}
}

func unconfiguredPipelineTools() []plugin.ToolDef {
	msg := "Pipelines plugin not configured. Set pipelinesUrl in plugin options."
	names := []struct {
		name string
		desc string
	}{
		{"rhoai_pipeline_list", "List pipelines."},
		{"rhoai_pipeline_run", "Start a pipeline run."},
		{"rhoai_pipeline_status", "Get pipeline run status."},
		{"rhoai_pipeline_create", "Create a pipeline."},
	}
	var tools []plugin.ToolDef
	for _, n := range names {
		name := n.name
		desc := n.desc
		tools = append(tools, plugin.ToolDef{
			Name:        name,
			Description: desc,
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Execute: func(_ context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				return msg, nil
			},
		})
	}
	return tools
}

func newPlugin(opts options) plugin.Plugin {
	if opts.PipelinesURL == "" {
		return plugin.Plugin{
			ID:    "rhoai-pipelines",
			Tools: unconfiguredPipelineTools(),
		}
	}

	client := newPipelineClient(opts.PipelinesURL, opts.Token, opts.APIPrefix)
	return plugin.Plugin{
		ID:    "rhoai-pipelines",
		Tools: buildPipelineTools(client, opts.Namespace),
	}
}

func main() {
	plugin.RunWithOptions(func(params plugin.InitializeParams) (plugin.Plugin, error) {
		opts := parseOptions(params.Options)
		return newPlugin(opts), nil
	})
}
