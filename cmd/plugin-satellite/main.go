package main

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/redhat"
	"github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

type options struct {
	SatelliteURL string
	Username     string
	Password     string
	Token        string
}

func parseOptions(raw map[string]any) options {
	var opts options
	if v, ok := raw["satelliteUrl"].(string); ok {
		opts.SatelliteURL = v
	}
	if v, ok := raw["username"].(string); ok {
		opts.Username = v
	}
	if v, ok := raw["password"].(string); ok {
		opts.Password = v
	}
	if v, ok := raw["token"].(string); ok {
		opts.Token = v
	}
	return opts
}

type satelliteClient struct {
	api        *redhat.APIClient
	baseURL    string
	authHeader string
}

func newSatelliteClient(baseURL string, opts options) *satelliteClient {
	cfg := redhat.APIClientConfig{
		BaseURL: baseURL,
	}
	var authHeader string
	if opts.Username != "" && opts.Password != "" {
		cred := base64.StdEncoding.EncodeToString([]byte(opts.Username + ":" + opts.Password))
		authHeader = "Basic " + cred
		cfg.Headers = map[string]string{"Authorization": authHeader}
	} else if opts.Token != "" {
		authHeader = "Bearer " + opts.Token
		cfg.TokenFn = func(_ context.Context) (string, error) { return opts.Token, nil }
	}
	return &satelliteClient{
		api:        redhat.NewAPIClient(cfg),
		baseURL:    strings.TrimRight(baseURL, "/"),
		authHeader: authHeader,
	}
}

type host struct {
	ID                  int    `json:"id"`
	Name                string `json:"name"`
	OperatingsystemName string `json:"operatingsystem_name"`
	EnvironmentName     string `json:"environment_name"`
	GlobalStatusLabel   string `json:"global_status_label"`
}

type erratum struct {
	ErrataID string `json:"errata_id"`
	Title    string `json:"title"`
	Type     string `json:"type"`
	Severity string `json:"severity"`
}

type contentView struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Label         string `json:"label"`
	Composite     bool   `json:"composite"`
	LastPublished string `json:"last_published"`
}

type repository struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Label       string `json:"label"`
	ContentType string `json:"content_type"`
}

type task struct {
	ID        string `json:"id"`
	Action    string `json:"action"`
	State     string `json:"state"`
	Result    string `json:"result"`
	StartedAt string `json:"started_at"`
	EndedAt   string `json:"ended_at"`
}

type smartProxy struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Features []struct {
		Name string `json:"name"`
	} `json:"features"`
}

type serviceStatus struct {
	Status string `json:"status"`
}

func (c *satelliteClient) getServices(ctx context.Context) (string, error) {
	lines := []string{"Satellite Services Health", ""}

	statusResp, err := c.api.Get(ctx, "/api/v2/status", nil)
	if err == nil {
		var st struct {
			SatelliteVersion string `json:"satellite_version"`
			Version          string `json:"version"`
			Result           string `json:"result"`
		}
		if json.Unmarshal(statusResp.Data, &st) == nil {
			if st.SatelliteVersion != "" {
				lines = append(lines, fmt.Sprintf("Satellite version: %s (Foreman %s)", st.SatelliteVersion, st.Version))
			}
			lines = append(lines, fmt.Sprintf("Status: %s", st.Result), "")
		}
	}

	resp, err := c.api.Get(ctx, "/api/v2/ping", nil)
	if err != nil {
		return "", err
	}
	var result struct {
		Results struct {
			Foreman struct {
				Database serviceStatus `json:"database"`
				Cache    struct {
					Servers []serviceStatus `json:"servers"`
				} `json:"cache"`
			} `json:"foreman"`
			Katello struct {
				Services map[string]struct {
					Status     string `json:"status"`
					Message    string `json:"message"`
					DurationMs string `json:"duration_ms"`
				} `json:"services"`
				Status string `json:"status"`
			} `json:"katello"`
		} `json:"results"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return "", err
	}

	r := result.Results

	dbStatus := r.Foreman.Database.Status
	if dbStatus == "" {
		dbStatus = "unknown"
	}
	lines = append(lines, fmt.Sprintf("Database: %s", dbStatus))

	if len(r.Foreman.Cache.Servers) > 0 {
		lines = append(lines, fmt.Sprintf("Cache: %s", r.Foreman.Cache.Servers[0].Status))
	}

	lines = append(lines, fmt.Sprintf("Katello overall: %s", r.Katello.Status), "")

	for name, svc := range r.Katello.Services {
		entry := fmt.Sprintf("- %s: %s", name, svc.Status)
		if svc.Message != "" {
			entry += " (" + svc.Message + ")"
		}
		if svc.DurationMs != "" && svc.DurationMs != "0" {
			entry += fmt.Sprintf(" [%sms]", svc.DurationMs)
		}
		lines = append(lines, entry)
	}
	return strings.Join(lines, "\n"), nil
}

func (c *satelliteClient) listTasks(ctx context.Context, search string, perPage int) ([]task, int, error) {
	query := map[string]string{}
	if search != "" {
		query["search"] = search
	}
	if perPage <= 0 {
		perPage = 10
	}
	query["per_page"] = fmt.Sprintf("%d", perPage)

	resp, err := c.api.Get(ctx, "/foreman_tasks/api/tasks", query)
	if err != nil {
		return nil, 0, err
	}
	var result struct {
		Total   int    `json:"total"`
		Results []task `json:"results"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, 0, err
	}
	return result.Results, result.Total, nil
}

func (c *satelliteClient) runREXCommand(ctx context.Context, hostSearch, command string) (int, error) {
	body := map[string]any{
		"job_invocation": map[string]any{
			"job_template_id": 187,
			"targeting_type":  "static_query",
			"search_query":    hostSearch,
			"inputs":          map[string]string{"command": command},
		},
	}
	resp, err := c.api.Post(ctx, "/api/v2/job_invocations", body)
	if err != nil {
		return 0, err
	}
	var result struct{ ID int `json:"id"` }
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return 0, err
	}
	return result.ID, nil
}

func (c *satelliteClient) getREXJobStatus(ctx context.Context, jobID int) (string, error) {
	resp, err := c.api.Get(ctx, fmt.Sprintf("/api/v2/job_invocations/%d", jobID), nil)
	if err != nil {
		return "", err
	}
	var result struct {
		ID          int    `json:"id"`
		Description string `json:"description"`
		StatusLabel string `json:"status_label"`
		Succeeded   int    `json:"succeeded"`
		Failed      int    `json:"failed"`
		Pending     int    `json:"pending"`
		Total       int    `json:"total"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return "", err
	}

	lines := []string{
		fmt.Sprintf("REX Job #%d", result.ID),
		fmt.Sprintf("Command: %s", result.Description),
		fmt.Sprintf("Status: %s", result.StatusLabel),
		fmt.Sprintf("Hosts: %d total, %d succeeded, %d failed, %d pending",
			result.Total, result.Succeeded, result.Failed, result.Pending),
	}

	if result.StatusLabel == "succeeded" || result.StatusLabel == "failed" {
		tiResp, err := c.api.Get(ctx, fmt.Sprintf("/api/v2/job_invocations/%d/template_invocations", jobID), nil)
		if err == nil {
			var ti struct {
				Results []struct {
					HostID   int    `json:"host_id"`
					HostName string `json:"host_name"`
				} `json:"results"`
			}
			if json.Unmarshal(tiResp.Data, &ti) == nil {
				for _, h := range ti.Results {
					outResp, err := c.api.Get(ctx, fmt.Sprintf("/api/v2/job_invocations/%d/hosts/%d", jobID, h.HostID), nil)
					if err != nil {
						continue
					}
					var out struct {
						Output []struct {
							OutputType string `json:"output_type"`
							Output     string `json:"output"`
						} `json:"output"`
					}
					if json.Unmarshal(outResp.Data, &out) == nil {
						lines = append(lines, "", fmt.Sprintf("--- %s ---", h.HostName))
						for _, o := range out.Output {
							if o.OutputType == "stdout" || o.OutputType == "stderr" {
								lines = append(lines, strings.TrimRight(o.Output, "\n"))
							}
						}
					}
				}
			}
		}
	}

	return strings.Join(lines, "\n"), nil
}

func (c *satelliteClient) listSmartProxies(ctx context.Context) ([]smartProxy, error) {
	resp, err := c.api.Get(ctx, "/api/v2/smart_proxies", nil)
	if err != nil {
		return nil, err
	}
	var result struct{ Results []smartProxy `json:"results"` }
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return result.Results, nil
}

func (c *satelliteClient) listRepositories(ctx context.Context, contentViewID int, search string) ([]repository, error) {
	query := map[string]string{}
	if search != "" {
		query["search"] = "name ~ " + search
	}
	path := "/katello/api/v2/repositories"
	if contentViewID > 0 {
		path = fmt.Sprintf("/katello/api/v2/content_views/%d/repositories", contentViewID)
	}
	query["per_page"] = "50"
	resp, err := c.api.Get(ctx, path, query)
	if err != nil {
		return nil, err
	}
	var result struct{ Results []repository `json:"results"` }
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return result.Results, nil
}

func formatRepositories(repos []repository, label string) string {
	if len(repos) == 0 {
		return fmt.Sprintf("No repositories found (%s).", label)
	}
	lines := []string{fmt.Sprintf("Repositories (%s): %d", label, len(repos)), ""}
	for _, r := range repos {
		entry := fmt.Sprintf("- #%d %s [%s]", r.ID, r.Name, r.ContentType)
		lines = append(lines, entry)
	}
	return strings.Join(lines, "\n")
}

func formatTasks(tasks []task, total int) string {
	if len(tasks) == 0 {
		return fmt.Sprintf("No matching tasks found (total: %d).", total)
	}
	lines := []string{fmt.Sprintf("Tasks: %d shown of %d total", len(tasks), total), ""}
	for _, t := range tasks {
		entry := fmt.Sprintf("- [%s/%s] %s", t.State, t.Result, t.Action)
		if t.StartedAt != "" {
			entry += " (started: " + t.StartedAt + ")"
		}
		if t.EndedAt != "" {
			entry += " (ended: " + t.EndedAt + ")"
		}
		lines = append(lines, entry)
	}
	return strings.Join(lines, "\n")
}

func formatSmartProxies(proxies []smartProxy) string {
	if len(proxies) == 0 {
		return "No smart proxies found."
	}
	lines := []string{fmt.Sprintf("Smart proxies: %d", len(proxies)), ""}
	for _, sp := range proxies {
		lines = append(lines, fmt.Sprintf("- #%d %s", sp.ID, sp.Name))
		lines = append(lines, fmt.Sprintf("  URL: %s", sp.URL))
		features := make([]string, 0, len(sp.Features))
		for _, f := range sp.Features {
			features = append(features, f.Name)
		}
		if len(features) > 0 {
			lines = append(lines, fmt.Sprintf("  Features: %s", strings.Join(features, ", ")))
		} else {
			lines = append(lines, "  Features: (none)")
		}
	}
	return strings.Join(lines, "\n")
}


func (c *satelliteClient) listHosts(ctx context.Context, search string) ([]host, error) {
	var query map[string]string
	if search != "" {
		query = map[string]string{"search": search}
	}
	resp, err := c.api.Get(ctx, "/api/v2/hosts", query)
	if err != nil {
		return nil, err
	}
	var result struct{ Results []host `json:"results"` }
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return result.Results, nil
}

func (c *satelliteClient) getHostFacts(ctx context.Context, hostname string, factSearch string) (string, error) {
	query := map[string]string{"per_page": "50"}
	if factSearch != "" {
		query["search"] = "name ~ " + factSearch
	}
	resp, err := c.api.Get(ctx, fmt.Sprintf("/api/v2/hosts/%s/facts", hostname), query)
	if err != nil {
		return "", err
	}
	var result struct {
		Total   int                       `json:"total"`
		Results map[string]map[string]any `json:"results"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return "", err
	}

	lines := []string{fmt.Sprintf("Facts for %s (showing %d of %d):", hostname, len(result.Results), result.Total), ""}
	for _, facts := range result.Results {
		for k, v := range facts {
			if v == nil {
				continue
			}
			lines = append(lines, fmt.Sprintf("  %s = %v", k, v))
		}
	}
	if len(lines) == 2 {
		return fmt.Sprintf("No facts found for %s.", hostname), nil
	}
	return strings.Join(lines, "\n"), nil
}

type probeResult struct {
	Port    string
	Service string
	Status  string
	Detail  string
}

func (c *satelliteClient) healthCheck(ctx context.Context) string {
	parsed, err := url.Parse(c.baseURL)
	if err != nil {
		return fmt.Sprintf("Failed to parse base URL: %v", err)
	}
	hostname := parsed.Hostname()

	httpClient := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	probes := []struct {
		port    string
		service string
		path    string
	}{
		{"443", "httpd (Satellite API)", "/api/v2/status"},
		{"9090", "foreman-proxy (Smart Proxy)", "/version"},
		{"23443", "tomcat (Candlepin)", "/candlepin/status"},
	}

	var results []probeResult
	for _, p := range probes {
		target := fmt.Sprintf("https://%s:%s%s", hostname, p.port, p.path)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			results = append(results, probeResult{p.port, p.service, "ERROR", err.Error()})
			continue
		}
		if c.authHeader != "" {
			req.Header.Set("Authorization", c.authHeader)
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			results = append(results, probeResult{p.port, p.service, "DOWN", err.Error()})
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		detail := ""
		if p.port == "443" {
			var st struct {
				SatelliteVersion string `json:"satellite_version"`
				Version          string `json:"version"`
			}
			if json.Unmarshal(body, &st) == nil && st.SatelliteVersion != "" {
				detail = fmt.Sprintf("Satellite %s (Foreman %s)", st.SatelliteVersion, st.Version)
			}
		} else if p.port == "9090" {
			var v struct {
				Version string `json:"version"`
			}
			if json.Unmarshal(body, &v) == nil && v.Version != "" {
				detail = "proxy v" + v.Version
			}
		} else if p.port == "23443" {
			var cs struct {
				Mode    string `json:"mode"`
				Version string `json:"version"`
			}
			if json.Unmarshal(body, &cs) == nil && cs.Version != "" {
				detail = fmt.Sprintf("Candlepin %s (%s)", cs.Version, cs.Mode)
			}
		}

		status := "UP"
		if resp.StatusCode >= 400 {
			status = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		results = append(results, probeResult{p.port, p.service, status, detail})
	}

	lines := []string{"Satellite Health Check", fmt.Sprintf("Host: %s", hostname), ""}
	allUp := true
	for _, r := range results {
		icon := "OK"
		if r.Status != "UP" {
			icon = "FAIL"
			allUp = false
		}
		entry := fmt.Sprintf("  :%s  %s — %s [%s]", r.Port, r.Service, r.Status, icon)
		if r.Detail != "" {
			entry += "  " + r.Detail
		}
		lines = append(lines, entry)
	}

	lines = append(lines, "")
	httpd := results[0]
	proxy := results[1]
	candlepin := results[2]

	if allUp {
		lines = append(lines, "All services reachable. Use satellite_services for detailed health.")
	} else if httpd.Status == "UP" && candlepin.Status != "UP" {
		lines = append(lines, "Note: port 23443 (Candlepin direct) is often firewalled — this may be normal.")
		lines = append(lines, "Use satellite_services to check Candlepin health through the API proxy.")
	} else if httpd.Status == "DOWN" && proxy.Status == "UP" {
		lines = append(lines, "Diagnosis: httpd is down but smart proxy is running.")
		lines = append(lines, "The Satellite API is unreachable — other plugin tools will fail.")
		lines = append(lines, "Likely causes: httpd config error, SSL/TLS issue, crypto policy, or certificate problem.")
		lines = append(lines, "Investigate on the satellite host: systemctl status httpd, journalctl -u httpd, /var/log/httpd/")
	} else if httpd.Status == "DOWN" && proxy.Status == "DOWN" {
		if candlepin.Status == "UP" {
			lines = append(lines, "Diagnosis: httpd and smart proxy are down, but Candlepin (Tomcat) is running.")
			lines = append(lines, "The satellite server is up but multiple services have failed.")
		} else {
			lines = append(lines, "Diagnosis: all probed services are unreachable.")
			lines = append(lines, "The satellite server may be down, rebooting, or there is a network issue.")
		}
	} else if httpd.Status == "UP" {
		lines = append(lines, "API is reachable. Use satellite_services for detailed health.")
	}

	return strings.Join(lines, "\n")
}

func (c *satelliteClient) listErrata(ctx context.Context, search, errataType string) ([]erratum, error) {
	query := map[string]string{}
	if search != "" {
		query["search"] = search
	}
	if errataType != "" {
		query["type"] = errataType
	}
	var qp map[string]string
	if len(query) > 0 {
		qp = query
	}
	resp, err := c.api.Get(ctx, "/katello/api/v2/errata", qp)
	if err != nil {
		return nil, err
	}
	var result struct{ Results []erratum `json:"results"` }
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return result.Results, nil
}

func (c *satelliteClient) listContentViews(ctx context.Context) ([]contentView, error) {
	resp, err := c.api.Get(ctx, "/katello/api/v2/content_views", nil)
	if err != nil {
		return nil, err
	}
	var result struct{ Results []contentView `json:"results"` }
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return result.Results, nil
}

func formatHosts(hosts []host) string {
	if len(hosts) == 0 {
		return "No hosts found."
	}
	lines := []string{fmt.Sprintf("Hosts: %d", len(hosts)), ""}
	for _, h := range hosts {
		name := h.Name
		if name == "" {
			name = "unknown"
		}
		entry := "- " + name
		if h.OperatingsystemName != "" {
			entry += fmt.Sprintf(" (%s)", h.OperatingsystemName)
		}
		if h.EnvironmentName != "" {
			entry += fmt.Sprintf(" [%s]", h.EnvironmentName)
		}
		if h.GlobalStatusLabel != "" {
			entry += " — " + h.GlobalStatusLabel
		}
		lines = append(lines, entry)
	}
	return strings.Join(lines, "\n")
}

func formatErrata(errata []erratum) string {
	if len(errata) == 0 {
		return "No errata found."
	}
	lines := []string{fmt.Sprintf("Errata: %d", len(errata)), ""}
	for _, e := range errata {
		id := e.ErrataID
		if id == "" {
			id = "unknown"
		}
		title := e.Title
		if title == "" {
			title = "untitled"
		}
		entry := fmt.Sprintf("- %s: %s", id, title)
		if e.Type != "" {
			entry += fmt.Sprintf(" [%s]", e.Type)
		}
		if e.Severity != "" {
			entry += fmt.Sprintf(" (%s)", e.Severity)
		}
		lines = append(lines, entry)
	}
	return strings.Join(lines, "\n")
}

func formatContentViews(views []contentView) string {
	if len(views) == 0 {
		return "No content views found."
	}
	lines := []string{fmt.Sprintf("Content views: %d", len(views)), ""}
	for _, v := range views {
		name := v.Name
		if name == "" {
			name = "unknown"
		}
		entry := "- " + name
		if v.Label != "" {
			entry += fmt.Sprintf(" (%s)", v.Label)
		}
		if v.Composite {
			entry += " [composite]"
		}
		if v.LastPublished != "" {
			entry += " — last published: " + v.LastPublished
		}
		lines = append(lines, entry)
	}
	return strings.Join(lines, "\n")
}

func buildTools(client *satelliteClient) []plugin.ToolDef {
	return []plugin.ToolDef{
		{
			Name:        "satellite_health_check",
			Description: "Probe Satellite connectivity on ports 443 (httpd/API), 9090 (smart proxy), and 23443 (Candlepin). Works even when httpd is down — use this first when the API is unreachable.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Execute: func(ctx context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				return client.healthCheck(ctx), nil
			},
		},
		{
			Name:        "satellite_hosts",
			Description: "Search managed hosts in Satellite with name, OS, environment, and status.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"search": map[string]any{"type": "string", "description": "Search query to filter hosts"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct{ Search string `json:"search"` }
				json.Unmarshal(args, &input)
				hosts, err := client.listHosts(ctx, input.Search)
				if err != nil {
					return fmt.Sprintf("Failed to list hosts: %v", err), nil
				}
				return formatHosts(hosts), nil
			},
		},
		{
			Name:        "satellite_host_facts",
			Description: "Get system facts for a managed host (CPU, memory, OS, networking). Useful for determining tuning profiles and hardware specs.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"hostname": map[string]any{"type": "string", "description": "FQDN of the host (from satellite_hosts output)"},
					"search":   map[string]any{"type": "string", "description": "Filter facts by name (e.g. 'memory', 'processor', 'network')"},
				},
				"required": []string{"hostname"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Hostname string `json:"hostname"`
					Search   string `json:"search"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				result, err := client.getHostFacts(ctx, input.Hostname, input.Search)
				if err != nil {
					return fmt.Sprintf("Failed to get host facts: %v", err), nil
				}
				return result, nil
			},
		},
		{
			Name:        "satellite_errata",
			Description: "Search available errata in Satellite with ID, title, type, and severity.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"search": map[string]any{"type": "string", "description": "Search query to filter errata"},
					"type":   map[string]any{"type": "string", "description": "Filter by errata type (security, bugfix, enhancement)"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Search string `json:"search"`
					Type   string `json:"type"`
				}
				json.Unmarshal(args, &input)
				errata, err := client.listErrata(ctx, input.Search, input.Type)
				if err != nil {
					return fmt.Sprintf("Failed to list errata: %v", err), nil
				}
				return formatErrata(errata), nil
			},
		},
		{
			Name:        "satellite_content_views",
			Description: "List content views in Satellite with name, label, composite flag, and last published date.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Execute: func(ctx context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				views, err := client.listContentViews(ctx)
				if err != nil {
					return fmt.Sprintf("Failed to list content views: %v", err), nil
				}
				return formatContentViews(views), nil
			},
		},
		{
			Name:        "satellite_services",
			Description: "Check Satellite service health — database, cache, candlepin, pulp, foreman_tasks, katello_events.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Execute: func(ctx context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				result, err := client.getServices(ctx)
				if err != nil {
					return fmt.Sprintf("Failed to check services: %v", err), nil
				}
				return result, nil
			},
		},
		{
			Name:        "satellite_tasks",
			Description: "List Foreman tasks with optional search filter. Use 'result = error' to find failed tasks, 'state = running' for active.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"search":  map[string]any{"type": "string", "description": "Foreman task search query (e.g. 'result = error', 'state = running')"},
					"perPage": map[string]any{"type": "integer", "description": "Number of results to return (default: 10)"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Search  string `json:"search"`
					PerPage int    `json:"perPage"`
				}
				json.Unmarshal(args, &input)
				tasks, total, err := client.listTasks(ctx, input.Search, input.PerPage)
				if err != nil {
					return fmt.Sprintf("Failed to list tasks: %v", err), nil
				}
				return formatTasks(tasks, total), nil
			},
		},
		{
			Name:        "satellite_proxies",
			Description: "List smart proxies (capsules) with URL and registered features like TFTP, DHCP, DNS, Puppet, Pulpcore.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Execute: func(ctx context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				proxies, err := client.listSmartProxies(ctx)
				if err != nil {
					return fmt.Sprintf("Failed to list smart proxies: %v", err), nil
				}
				return formatSmartProxies(proxies), nil
			},
		},
		{
			Name:        "satellite_repositories",
			Description: "List repositories in the organization or in a specific content view. Use contentViewId to see what repos are included in a content view.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"contentViewId": map[string]any{"type": "integer", "description": "Filter to repos in this content view (omit for all org repos)"},
					"search":        map[string]any{"type": "string", "description": "Filter repos by name substring"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					ContentViewID int    `json:"contentViewId"`
					Search        string `json:"search"`
				}
				json.Unmarshal(args, &input)
				label := "all"
				if input.ContentViewID > 0 {
					label = fmt.Sprintf("content view #%d", input.ContentViewID)
				}
				repos, err := client.listRepositories(ctx, input.ContentViewID, input.Search)
				if err != nil {
					return fmt.Sprintf("Failed to list repositories: %v", err), nil
				}
				return formatRepositories(repos, label), nil
			},
		},
		{
			Name:        "satellite_rex_run",
			Description: "Run a shell command on a managed host via Satellite Remote Execution (REX). Returns the job ID — use satellite_rex_result to get output.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"host":    map[string]any{"type": "string", "description": "Host search query (e.g. 'name = satellite.example.com')"},
					"command": map[string]any{"type": "string", "description": "Shell command to execute on the host"},
				},
				"required": []string{"host", "command"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Host    string `json:"host"`
					Command string `json:"command"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				jobID, err := client.runREXCommand(ctx, input.Host, input.Command)
				if err != nil {
					return fmt.Sprintf("Failed to invoke REX job: %v", err), nil
				}
				return fmt.Sprintf("REX job submitted. Job ID: %d\nUse satellite_rex_result with jobId=%d to check status and get output.", jobID, jobID), nil
			},
		},
		{
			Name:        "satellite_rex_result",
			Description: "Get the status and output of a Remote Execution job. Poll until status is 'succeeded' or 'failed'.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"jobId": map[string]any{"type": "integer", "description": "REX job ID from satellite_rex_run"},
				},
				"required": []string{"jobId"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct{ JobID int `json:"jobId"` }
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				result, err := client.getREXJobStatus(ctx, input.JobID)
				if err != nil {
					return fmt.Sprintf("Failed to get job status: %v", err), nil
				}
				return result, nil
			},
		},
	}
}

func unconfiguredTools() []plugin.ToolDef {
	msg := "Satellite plugin not configured. Set satelliteUrl in plugin options."
	return []plugin.ToolDef{
		stubTool("satellite_health_check", "Probe Satellite connectivity on multiple ports.", msg),
		stubTool("satellite_hosts", "Search managed hosts in Satellite.", msg),
		stubTool("satellite_host_facts", "Get system facts for a managed host.", msg),
		stubTool("satellite_errata", "Search available errata in Satellite.", msg),
		stubTool("satellite_content_views", "List content views in Satellite.", msg),
		stubTool("satellite_services", "Check Satellite service health.", msg),
		stubTool("satellite_tasks", "List Foreman tasks.", msg),
		stubTool("satellite_proxies", "List smart proxies.", msg),
		stubTool("satellite_repositories", "List repositories.", msg),
		stubTool("satellite_rex_run", "Run a command on a managed host via REX.", msg),
		stubTool("satellite_rex_result", "Get REX job status and output.", msg),
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

func newPlugin(opts options) plugin.Plugin {
	hasAuth := opts.Token != "" || (opts.Username != "" && opts.Password != "")
	if opts.SatelliteURL == "" || !hasAuth {
		return plugin.Plugin{
			ID:    "satellite",
			Tools: unconfiguredTools(),
		}
	}

	client := newSatelliteClient(opts.SatelliteURL, opts)

	return plugin.Plugin{
		ID:    "satellite",
		Tools: buildTools(client),
	}
}

func main() {
	plugin.RunWithOptions(func(params plugin.InitializeParams) (plugin.Plugin, error) {
		opts := parseOptions(params.Options)
		return newPlugin(opts), nil
	})
}
