package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/bobbyjohnstx/tinycode-go/internal/redhat"
	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

const defaultAPIPrefix = "/api/v2"

type options struct {
	ControllerURL string
	OAuthToken    string
	Username      string
	Password      string
	APIPrefix     string
}

func parseOptions(raw map[string]any) options {
	var opts options
	if v, ok := raw["controllerUrl"].(string); ok {
		opts.ControllerURL = v
	}
	if v, ok := raw["oauthToken"].(string); ok {
		opts.OAuthToken = v
	}
	if v, ok := raw["username"].(string); ok {
		opts.Username = v
	}
	if v, ok := raw["password"].(string); ok {
		opts.Password = v
	}
	if v, ok := raw["apiPrefix"].(string); ok {
		opts.APIPrefix = v
	}
	return opts
}

type aapClient struct {
	api       *redhat.APIClient
	apiPrefix string
}

func newAapClient(controllerURL, token, username, password, apiPrefix string) *aapClient {
	if apiPrefix == "" {
		apiPrefix = defaultAPIPrefix
	}
	cfg := redhat.APIClientConfig{BaseURL: controllerURL}
	if username != "" && password != "" {
		cfg.BasicAuth = &redhat.BasicAuthConfig{Username: username, Password: password}
	} else if token != "" {
		cfg.TokenFn = func(_ context.Context) (string, error) { return token, nil }
	}
	return &aapClient{
		api:       redhat.NewAPIClient(cfg),
		apiPrefix: apiPrefix,
	}
}

type jobTemplate struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	LastJobRun  string `json:"last_job_run"`
	Status      string `json:"status"`
}

type job struct {
	ID       int     `json:"id"`
	Name     string  `json:"name"`
	Status   string  `json:"status"`
	Started  string  `json:"started"`
	Finished string  `json:"finished"`
	Failed   bool    `json:"failed"`
	Elapsed  float64 `json:"elapsed"`
}

type inventory struct {
	ID                       int    `json:"id"`
	Name                     string `json:"name"`
	Description              string `json:"description"`
	TotalHosts               int    `json:"total_hosts"`
	HostsWithActiveFailures  int    `json:"hosts_with_active_failures"`
}

type collection struct {
	Namespace     *struct{ Name string `json:"name"` } `json:"namespace"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	LatestVersion *struct{ Version string `json:"version"` } `json:"latest_version"`
}

func (c *aapClient) listTemplates(ctx context.Context, search string) ([]jobTemplate, error) {
	var query map[string]string
	if search != "" {
		query = map[string]string{"search": search}
	}
	resp, err := c.api.Get(ctx, c.apiPrefix+"/job_templates/", query)
	if err != nil {
		return nil, err
	}
	var result struct{ Results []jobTemplate `json:"results"` }
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return result.Results, nil
}

func (c *aapClient) launchJob(ctx context.Context, templateID int, extraVars string) (int, error) {
	var body any
	if extraVars != "" {
		body = map[string]string{"extra_vars": extraVars}
	}
	resp, err := c.api.Post(ctx, fmt.Sprintf(c.apiPrefix+"/job_templates/%d/launch/", templateID), body)
	if err != nil {
		return 0, err
	}
	var result struct {
		Job int `json:"job"`
		ID  int `json:"id"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return 0, err
	}
	id := result.Job
	if id == 0 {
		id = result.ID
	}
	return id, nil
}

func (c *aapClient) getJobStatus(ctx context.Context, jobID int) (*job, error) {
	resp, err := c.api.Get(ctx, fmt.Sprintf(c.apiPrefix+"/jobs/%d/", jobID), nil)
	if err != nil {
		return nil, err
	}
	var j job
	if err := json.Unmarshal(resp.Data, &j); err != nil {
		return nil, err
	}
	return &j, nil
}

func (c *aapClient) getJobOutput(ctx context.Context, jobID int) (string, error) {
	resp, err := c.api.Get(ctx, fmt.Sprintf(c.apiPrefix+"/jobs/%d/stdout/?format=txt", jobID), nil)
	if err != nil {
		return "", err
	}
	return string(resp.Data), nil
}

func (c *aapClient) listInventories(ctx context.Context, search string) ([]inventory, error) {
	var query map[string]string
	if search != "" {
		query = map[string]string{"search": search}
	}
	resp, err := c.api.Get(ctx, c.apiPrefix+"/inventories/", query)
	if err != nil {
		return nil, err
	}
	var result struct{ Results []inventory `json:"results"` }
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return result.Results, nil
}

func (c *aapClient) searchCollections(ctx context.Context, keyword string) ([]collection, error) {
	resp, err := c.api.Get(ctx, c.apiPrefix+"/collections/", map[string]string{"keyword": keyword})
	if err != nil {
		return nil, err
	}
	var result struct{ Results []collection `json:"results"` }
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return result.Results, nil
}

func formatTemplates(templates []jobTemplate) string {
	if len(templates) == 0 {
		return "No job templates found."
	}
	lines := []string{fmt.Sprintf("Job templates: %d", len(templates)), ""}
	for _, t := range templates {
		name := t.Name
		if name == "" {
			name = "unknown"
		}
		entry := fmt.Sprintf("- #%d %s", t.ID, name)
		if t.Description != "" {
			entry += " — " + t.Description
		}
		if t.Status != "" {
			entry += fmt.Sprintf(" [%s]", t.Status)
		}
		lines = append(lines, entry)
	}
	return strings.Join(lines, "\n")
}

func formatJob(j *job) string {
	lines := []string{
		fmt.Sprintf("Job #%d", j.ID),
		fmt.Sprintf("Name: %s", j.Name),
		fmt.Sprintf("Status: %s", j.Status),
	}
	if j.Started != "" {
		lines = append(lines, "Started: "+j.Started)
	}
	if j.Finished != "" {
		lines = append(lines, "Finished: "+j.Finished)
	}
	if j.Elapsed > 0 {
		lines = append(lines, fmt.Sprintf("Elapsed: %.1fs", j.Elapsed))
	}
	return strings.Join(lines, "\n")
}

func formatInventories(invs []inventory) string {
	if len(invs) == 0 {
		return "No inventories found."
	}
	lines := []string{fmt.Sprintf("Inventories: %d", len(invs)), ""}
	for _, inv := range invs {
		name := inv.Name
		if name == "" {
			name = "unknown"
		}
		entry := fmt.Sprintf("- #%d %s", inv.ID, name)
		if inv.Description != "" {
			entry += " — " + inv.Description
		}
		entry += fmt.Sprintf(" (%d hosts, %d failures)", inv.TotalHosts, inv.HostsWithActiveFailures)
		lines = append(lines, entry)
	}
	return strings.Join(lines, "\n")
}

func formatCollections(colls []collection) string {
	if len(colls) == 0 {
		return "No collections found."
	}
	lines := []string{fmt.Sprintf("Collections: %d", len(colls)), ""}
	for _, c := range colls {
		ns := "unknown"
		if c.Namespace != nil && c.Namespace.Name != "" {
			ns = c.Namespace.Name
		}
		name := c.Name
		if name == "" {
			name = "unknown"
		}
		ver := "?"
		if c.LatestVersion != nil && c.LatestVersion.Version != "" {
			ver = c.LatestVersion.Version
		}
		entry := fmt.Sprintf("- %s.%s v%s", ns, name, ver)
		if c.Description != "" {
			entry += " — " + c.Description
		}
		lines = append(lines, entry)
	}
	return strings.Join(lines, "\n")
}

type lintViolation struct {
	Type      string `json:"type"`
	CheckName string `json:"check_name"`
	Severity  string `json:"severity"`
	Desc      string `json:"description"`
	Location  struct {
		Path  string `json:"path"`
		Lines struct {
			Begin int `json:"begin"`
		} `json:"lines"`
	} `json:"location"`
}

func buildHealthTool(client *aapClient) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "aap_health",
		Description: "Check health of AAP services (Controller API, EDA controller).",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Execute: func(ctx context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
			var lines []string

			if client == nil {
				lines = append(lines, "[SKIP] Controller API (not configured)")
				lines = append(lines, "[SKIP] EDA Controller (not configured)")
				return "Service Health:\n" + strings.Join(lines, "\n"), nil
			}

			// Check Controller API via ping endpoint
			if _, err := client.api.Get(ctx, client.apiPrefix+"/ping/", nil); err != nil {
				lines = append(lines, fmt.Sprintf("[DOWN] Controller API: %v", err))
			} else {
				lines = append(lines, "[OK] Controller API")
			}

			// Check EDA controller via config endpoint
			if _, err := client.api.Get(ctx, client.apiPrefix+"/config/", nil); err != nil {
				lines = append(lines, fmt.Sprintf("[DOWN] EDA Controller: %v", err))
			} else {
				lines = append(lines, "[OK] EDA Controller")
			}

			return "Service Health:\n" + strings.Join(lines, "\n"), nil
		},
	}
}

func buildTools(client *aapClient) []plugin.ToolDef {
	notConfigured := "AAP plugin not configured. Set controllerUrl in plugin options."

	if client == nil {
		return []plugin.ToolDef{
			stubTool("aap_list_templates", "List job templates from AAP Controller.", notConfigured),
			stubTool("aap_launch_job", "Launch a job template on AAP Controller.", notConfigured),
			stubTool("aap_job_status", "Check the status of a job on AAP Controller.", notConfigured),
			stubTool("aap_job_output", "Get the stdout output of a job.", notConfigured),
			stubTool("aap_list_inventories", "List inventories from AAP Controller.", notConfigured),
			stubTool("aap_hub_search", "Search Automation Hub for Ansible collections.", notConfigured),
		}
	}

	return []plugin.ToolDef{
		{
			Name:        "aap_list_templates",
			Description: "List job templates from AAP Controller with name, description, and last run status.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"search": map[string]any{"type": "string", "description": "Filter templates by name"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct{ Search string `json:"search"` }
				json.Unmarshal(args, &input)
				templates, err := client.listTemplates(ctx, input.Search)
				if err != nil {
					return fmt.Sprintf("Failed to list templates: %v", err), nil
				}
				return formatTemplates(templates), nil
			},
		},
		{
			Name:        "aap_launch_job",
			Description: "Launch a job template on AAP Controller. Returns the job ID.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"templateId": map[string]any{"type": "integer", "description": "The job template ID to launch"},
					"extraVars":  map[string]any{"type": "string", "description": "Extra variables as JSON or YAML string"},
				},
				"required": []string{"templateId"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					TemplateID int    `json:"templateId"`
					ExtraVars  string `json:"extraVars"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				jobID, err := client.launchJob(ctx, input.TemplateID, input.ExtraVars)
				if err != nil {
					return fmt.Sprintf("Failed to launch job: %v", err), nil
				}
				return fmt.Sprintf("Job launched successfully. Job ID: %d", jobID), nil
			},
		},
		{
			Name:        "aap_job_status",
			Description: "Check the status of a job on AAP Controller (pending, running, successful, failed).",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"jobId": map[string]any{"type": "integer", "description": "The job ID to check"},
				},
				"required": []string{"jobId"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct{ JobID int `json:"jobId"` }
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				j, err := client.getJobStatus(ctx, input.JobID)
				if err != nil {
					return fmt.Sprintf("Failed to get job status: %v", err), nil
				}
				return formatJob(j), nil
			},
		},
		{
			Name:        "aap_job_output",
			Description: "Get the stdout output of a job from AAP Controller.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"jobId": map[string]any{"type": "integer", "description": "The job ID to get output for"},
				},
				"required": []string{"jobId"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct{ JobID int `json:"jobId"` }
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				output, err := client.getJobOutput(ctx, input.JobID)
				if err != nil {
					return fmt.Sprintf("Failed to get job output: %v", err), nil
				}
				if strings.TrimSpace(output) == "" {
					return "No output available for this job.", nil
				}
				return output, nil
			},
		},
		{
			Name:        "aap_list_inventories",
			Description: "List inventories from AAP Controller with host counts.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"search": map[string]any{"type": "string", "description": "Filter inventories by name"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct{ Search string `json:"search"` }
				json.Unmarshal(args, &input)
				invs, err := client.listInventories(ctx, input.Search)
				if err != nil {
					return fmt.Sprintf("Failed to list inventories: %v", err), nil
				}
				return formatInventories(invs), nil
			},
		},
		{
			Name:        "aap_hub_search",
			Description: "Search Automation Hub for Ansible collections by keyword.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"keyword": map[string]any{"type": "string", "description": "Search keyword for collections"},
				},
				"required": []string{"keyword"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct{ Keyword string `json:"keyword"` }
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				colls, err := client.searchCollections(ctx, input.Keyword)
				if err != nil {
					return fmt.Sprintf("Failed to search collections: %v", err), nil
				}
				return formatCollections(colls), nil
			},
		},
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

func lintTool() plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "aap_lint_playbook",
		Description: "Lint an Ansible playbook or role using ansible-lint. Returns structured violations with rule, severity, line number, and fix suggestions.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"filePath": map[string]any{"type": "string", "description": "Path to the playbook YAML file to lint"},
				"profile":  map[string]any{"type": "string", "description": "Lint profile: production (default), shared, basic, or safety"},
			},
			"required": []string{"filePath"},
		},
		Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
			var input struct {
				FilePath string `json:"filePath"`
				Profile  string `json:"profile"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return "", fmt.Errorf("parsing args: %w", err)
			}

			if _, err := exec.LookPath("ansible-lint"); err != nil {
				return "ansible-lint not found. Install with: pip install ansible-lint (included in ansible-dev-tools)", nil
			}

			profile := input.Profile
			if profile == "" {
				profile = "production"
			}

			cmd := exec.CommandContext(ctx, "ansible-lint", input.FilePath, "--format", "json", "-p", profile)
			out, err := cmd.CombinedOutput()

			if err == nil {
				return fmt.Sprintf("Playbook %s passed all lint checks (profile: %s).", input.FilePath, profile), nil
			}

			output := strings.TrimSpace(string(out))
			if !strings.HasPrefix(output, "[") {
				return fmt.Sprintf("ansible-lint error: %s", output), nil
			}

			var violations []lintViolation
			if err := json.Unmarshal([]byte(output), &violations); err != nil {
				return fmt.Sprintf("Lint failed: %s", output), nil
			}

			lines := []string{fmt.Sprintf("Violations found: %d (profile: %s)", len(violations), profile), ""}
			for _, v := range violations {
				path := v.Location.Path
				if path == "" {
					path = "unknown"
				}
				line := v.Location.Lines.Begin
				header := fmt.Sprintf("- [%s] %s at %s:%d", v.Severity, v.CheckName, path, line)
				if v.Desc != "" {
					header += "\n  " + v.Desc
				}
				lines = append(lines, header)
			}
			return strings.Join(lines, "\n"), nil
		},
	}
}

func newPlugin(opts options) plugin.Plugin {
	var client *aapClient
	if opts.ControllerURL != "" && (opts.OAuthToken != "" || (opts.Username != "" && opts.Password != "")) {
		client = newAapClient(opts.ControllerURL, opts.OAuthToken, opts.Username, opts.Password, opts.APIPrefix)
	}

	tools := buildTools(client)
	tools = append(tools, lintTool())
	tools = append(tools, buildHealthTool(client))

	p := plugin.Plugin{
		ID:    "aap-bridge",
		Tools: tools,
	}

	if client != nil {
		p.Hooks = plugin.HookHandlers{
			ShellEnv: func(_ context.Context, _ plugin.ShellEnvInput) (*plugin.ShellEnvOutput, error) {
				return &plugin.ShellEnvOutput{
					Env: map[string]string{
						"CONTROLLER_HOST":        opts.ControllerURL,
						"CONTROLLER_OAUTH_TOKEN": opts.OAuthToken,
					},
				}, nil
			},
		}
	}

	return p
}

func main() {
	plugin.RunWithOptions(func(params plugin.InitializeParams) (plugin.Plugin, error) {
		opts := parseOptions(params.Options)
		return newPlugin(opts), nil
	})
}
