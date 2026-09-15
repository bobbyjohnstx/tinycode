package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strings"

	"github.com/bobbyjohnstx/tinycode-go/internal/redhat"
	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

type options struct {
	CentralURL string
	APIToken   string
	Username   string
	Password   string
}

func parseOptions(raw map[string]any) options {
	var opts options
	if v, ok := raw["centralUrl"].(string); ok {
		opts.CentralURL = v
	}
	if v, ok := raw["apiToken"].(string); ok {
		opts.APIToken = v
	}
	if v, ok := raw["username"].(string); ok {
		opts.Username = v
	}
	if v, ok := raw["password"].(string); ok {
		opts.Password = v
	}
	return opts
}

// centralClient wraps the API client for RHACS Central endpoints.
type centralClient struct {
	api *redhat.APIClient
}

func newCentralClient(cfg redhat.APIClientConfig) *centralClient {
	return &centralClient{
		api: redhat.NewAPIClient(cfg),
	}
}

// API response types

type imageScanResult struct {
	Image struct {
		Name struct {
			FullName string `json:"fullName"`
		} `json:"name"`
	} `json:"image"`
	Components []struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Vulns   []struct {
			CVE      string  `json:"cve"`
			Severity string  `json:"severity"`
			CVSS     float64 `json:"cvss"`
			Link     string  `json:"link"`
			FixedBy  string  `json:"fixedBy"`
		} `json:"vulns"`
	} `json:"components"`
}

type policyViolation struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
}

type alertPolicy struct {
	Name        string `json:"name"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
}

type alertDeployment struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	ClusterName string `json:"clusterName"`
}

type alert struct {
	ID         string          `json:"id"`
	Policy     alertPolicy     `json:"policy"`
	Deployment alertDeployment `json:"deployment"`
	State      string          `json:"state"`
	Time       string          `json:"time"`
}

type riskFactor struct {
	Message string `json:"message"`
}

type riskResultEntry struct {
	Name    string       `json:"name"`
	Factors []riskFactor `json:"factors"`
}

type riskResult struct {
	Subject struct {
		ID        string `json:"id"`
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
		Type      string `json:"type"`
	} `json:"subject"`
	Score   float64           `json:"score"`
	Results []riskResultEntry `json:"results"`
}

type complianceProfile struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	TotalControls   int    `json:"totalControls"`
	PassingControls int    `json:"passingControls"`
	FailingControls int    `json:"failingControls"`
	ProfileVersion  string `json:"profileVersion"`
}

type complianceControl struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Severity    string `json:"severity"`
	Remediation string `json:"remediation"`
}

type complianceScanProfile struct {
	ProfileName string              `json:"profileName"`
	Passing     int                 `json:"passing"`
	Failing     int                 `json:"failing"`
	Errors      int                 `json:"errors"`
	Controls    []complianceControl `json:"controls"`
}

type complianceScanResult struct {
	ScanConfigID string                  `json:"scanConfigId"`
	Profiles     []complianceScanProfile `json:"profiles"`
}

// Client methods

func (c *centralClient) scanImage(ctx context.Context, imageName string) (*imageScanResult, error) {
	resp, err := c.api.Post(ctx, "/v1/images/scan", map[string]string{"imageName": imageName})
	if err != nil {
		return nil, err
	}
	var result imageScanResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

type policyAlert struct {
	Policy policyViolation `json:"policy"`
}

func (c *centralClient) checkImage(ctx context.Context, imageName string) ([]policyAlert, error) {
	resp, err := c.api.Post(ctx, "/v1/images/check", map[string]string{"imageName": imageName})
	if err != nil {
		return nil, err
	}
	var result struct{ Alerts []policyAlert `json:"alerts"` }
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return result.Alerts, nil
}

func (c *centralClient) checkDeployment(ctx context.Context, yaml string) ([]policyAlert, error) {
	resp, err := c.api.Post(ctx, "/v1/deploymentcheck", map[string]string{"resources": yaml})
	if err != nil {
		return nil, err
	}
	var result struct{ Alerts []policyAlert `json:"alerts"` }
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return result.Alerts, nil
}

func (c *centralClient) listAlerts(ctx context.Context, query map[string]string) ([]alert, error) {
	resp, err := c.api.Get(ctx, "/v1/alerts", query)
	if err != nil {
		return nil, err
	}
	var result struct{ Alerts []alert `json:"alerts"` }
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return result.Alerts, nil
}

func (c *centralClient) getDeploymentRisk(ctx context.Context, deploymentID string) (*riskResult, error) {
	path := fmt.Sprintf("/v1/deployments/%s/risk", url.PathEscape(deploymentID))
	resp, err := c.api.Get(ctx, path, nil)
	if err != nil {
		return nil, err
	}
	var result riskResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *centralClient) getComplianceProfiles(ctx context.Context) ([]complianceProfile, error) {
	resp, err := c.api.Get(ctx, "/v2/compliance/profiles", nil)
	if err != nil {
		return nil, err
	}
	var result struct{ Profiles []complianceProfile `json:"profiles"` }
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return result.Profiles, nil
}

func (c *centralClient) runComplianceScan(ctx context.Context, scanConfigID string) error {
	path := fmt.Sprintf("/v2/compliance/scan/configurations/%s/run", url.PathEscape(scanConfigID))
	_, err := c.api.Post(ctx, path, map[string]any{})
	return err
}

func (c *centralClient) getComplianceResults(ctx context.Context, scanConfigID string) ([]complianceScanResult, error) {
	var query map[string]string
	if scanConfigID != "" {
		query = map[string]string{"scanConfigId": scanConfigID}
	}
	resp, err := c.api.Get(ctx, "/v2/compliance/results", query)
	if err != nil {
		return nil, err
	}
	var result struct{ Results []complianceScanResult `json:"results"` }
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return result.Results, nil
}

// Formatting helpers

type flatVuln struct {
	cve       string
	severity  string
	cvss      float64
	component string
	version   string
	fixedBy   string
}

func formatVulns(data *imageScanResult) string {
	var vulns []flatVuln
	for _, comp := range data.Components {
		compName := comp.Name
		if compName == "" {
			compName = "unknown"
		}
		compVersion := comp.Version
		if compVersion == "" {
			compVersion = "unknown"
		}
		for _, v := range comp.Vulns {
			cve := v.CVE
			if cve == "" {
				cve = "unknown"
			}
			severity := v.Severity
			if severity == "" {
				severity = "UNKNOWN"
			}
			fixedBy := v.FixedBy
			if fixedBy == "" {
				fixedBy = "no fix available"
			}
			vulns = append(vulns, flatVuln{
				cve:       cve,
				severity:  severity,
				cvss:      v.CVSS,
				component: compName,
				version:   compVersion,
				fixedBy:   fixedBy,
			})
		}
	}

	imageName := data.Image.Name.FullName
	if imageName == "" {
		imageName = "unknown"
	}

	if len(vulns) == 0 {
		return fmt.Sprintf("Image: %s\nNo vulnerabilities found.", imageName)
	}

	sort.Slice(vulns, func(i, j int) bool {
		return vulns[i].cvss > vulns[j].cvss
	})

	lines := []string{
		fmt.Sprintf("Image: %s", imageName),
		fmt.Sprintf("Vulnerabilities found: %d", len(vulns)),
		"",
	}
	for _, v := range vulns {
		lines = append(lines, fmt.Sprintf("- %s (%s, CVSS %.1f) in %s@%s | Fix: %s",
			v.cve, v.severity, v.cvss, v.component, v.version, v.fixedBy))
	}

	return strings.Join(lines, "\n")
}

func formatAlerts(alerts []alert) string {
	if len(alerts) == 0 {
		return "No active violations found."
	}

	lines := []string{
		fmt.Sprintf("Active violations: %d", len(alerts)),
		"",
	}
	for _, a := range alerts {
		policy := a.Policy.Name
		if policy == "" {
			policy = "unknown policy"
		}
		severity := a.Policy.Severity
		if severity == "" {
			severity = "UNKNOWN"
		}
		deployment := a.Deployment.Name
		if deployment == "" {
			deployment = "unknown"
		}
		namespace := a.Deployment.Namespace
		if namespace == "" {
			namespace = "unknown"
		}
		state := a.State
		if state == "" {
			state = "unknown"
		}
		lines = append(lines, fmt.Sprintf("- [%s] %s | Deployment: %s/%s | State: %s",
			severity, policy, namespace, deployment, state))
	}

	return strings.Join(lines, "\n")
}

func formatPolicyAlerts(alerts []policyAlert, prefix string) string {
	if len(alerts) == 0 {
		return prefix + " passed all policy checks."
	}

	lines := []string{prefix + " FAILED policy checks:", ""}
	for _, a := range alerts {
		name := a.Policy.Name
		if name == "" {
			name = "unknown"
		}
		severity := a.Policy.Severity
		if severity == "" {
			severity = "UNKNOWN"
		}
		entry := fmt.Sprintf("- [%s] %s", severity, name)
		if a.Policy.Description != "" {
			entry += ": " + a.Policy.Description
		}
		lines = append(lines, entry)
	}

	return strings.Join(lines, "\n")
}

var severityOrder = map[string]int{
	"CRITICAL": 0,
	"HIGH":     1,
	"MEDIUM":   2,
	"LOW":      3,
}

func formatComplianceScanResult(result *complianceScanResult) string {
	if len(result.Profiles) == 0 {
		return "No compliance scan results available."
	}

	var lines []string
	for _, profile := range result.Profiles {
		passing := profile.Passing
		failing := profile.Failing
		total := passing + failing + profile.Errors
		pct := 0
		if total > 0 {
			pct = int(math.Round(float64(passing) / float64(total) * 100))
		}

		profileName := profile.ProfileName
		if profileName == "" {
			profileName = "unknown"
		}
		lines = append(lines, fmt.Sprintf("Profile: %s", profileName))
		lines = append(lines, fmt.Sprintf("Passing: %d/%d (%d%%)", passing, total, pct))
		lines = append(lines, fmt.Sprintf("Failing: %d", failing))

		var failingControls []complianceControl
		for _, ctrl := range profile.Controls {
			if ctrl.Status == "FAIL" {
				failingControls = append(failingControls, ctrl)
			}
		}

		if len(failingControls) > 0 {
			sort.Slice(failingControls, func(i, j int) bool {
				aOrder, aOk := severityOrder[failingControls[i].Severity]
				if !aOk {
					aOrder = 4
				}
				bOrder, bOk := severityOrder[failingControls[j].Severity]
				if !bOk {
					bOrder = 4
				}
				return aOrder < bOrder
			})

			lines = append(lines, "", "Failing Controls:")
			for _, ctrl := range failingControls {
				severity := ctrl.Severity
				if severity == "" {
					severity = "UNKNOWN"
				}
				name := ctrl.Name
				if name == "" {
					name = "unknown"
				}
				lines = append(lines, fmt.Sprintf("- [%s] %s", severity, name))
				if ctrl.Remediation != "" {
					lines = append(lines, fmt.Sprintf("  Remediation: %s", ctrl.Remediation))
				}
			}
		}

		lines = append(lines, "")
	}

	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

func buildHealthTool(client *centralClient) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "rhacs_health",
		Description: "Check health of RHACS services (Central API, Scanner, Sensor connectivity).",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Execute: func(ctx context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
			var lines []string

			if client == nil {
				lines = append(lines, "[SKIP] Central API (not configured)")
				lines = append(lines, "[SKIP] Scanner (not configured)")
				lines = append(lines, "[SKIP] Sensor (not configured)")
				return "Service Health:\n" + strings.Join(lines, "\n"), nil
			}

			// Check Central API via /v1/metadata
			if _, err := client.api.Get(ctx, "/v1/metadata", nil); err != nil {
				lines = append(lines, fmt.Sprintf("[DOWN] Central API: %v", err))
			} else {
				lines = append(lines, "[OK] Central API")
			}

			// Check Scanner via image scan endpoint
			if _, err := client.api.Get(ctx, "/v1/integrationhealth", map[string]string{"type": "SCANNER"}); err != nil {
				lines = append(lines, fmt.Sprintf("[DOWN] Scanner: %v", err))
			} else {
				lines = append(lines, "[OK] Scanner")
			}

			// Check Sensor connectivity via cluster health
			if _, err := client.api.Get(ctx, "/v1/clusters", nil); err != nil {
				lines = append(lines, fmt.Sprintf("[DOWN] Sensor: %v", err))
			} else {
				lines = append(lines, "[OK] Sensor")
			}

			return "Service Health:\n" + strings.Join(lines, "\n"), nil
		},
	}
}

func buildTools(client *centralClient) []plugin.ToolDef {
	notConfigured := "RHACS plugin is not configured. Set centralUrl and apiToken in plugin options."

	if client == nil {
		return []plugin.ToolDef{
			stubTool("rhacs_image_scan", "Scan a container image for vulnerabilities via RHACS Central API. Returns CVE list with severity, CVSS score, and fixable status.", notConfigured),
			stubTool("rhacs_image_check", "Check a container image against RHACS deploy-time policies. Returns pass/fail with violated policy names.", notConfigured),
			stubTool("rhacs_deployment_check", "Check a deployment YAML against RHACS policies. Catches issues like privileged containers, missing resource limits, etc.", notConfigured),
			stubTool("rhacs_violations", "List active policy violations from RHACS. Can filter by namespace or severity.", notConfigured),
			stubTool("rhacs_risk", "Get the risk score and risk factors for a deployment from RHACS.", notConfigured),
			stubTool("rhacs_compliance_scan", "Run or get compliance scan results from RHACS. If scanConfigId is provided, triggers that scan config and returns results. Otherwise lists available scan configurations.", notConfigured),
			stubTool("rhacs_compliance_status", "Get compliance summary across all profiles from RHACS. Shows pass rate and top failing controls per profile.", notConfigured),
		}
	}

	return []plugin.ToolDef{
		{
			Name:        "rhacs_image_scan",
			Description: "Scan a container image for vulnerabilities via RHACS Central API. Returns CVE list with severity, CVSS score, and fixable status.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"image": map[string]any{"type": "string", "description": "Full container image reference (e.g. registry.io/repo/image:tag)"},
				},
				"required": []string{"image"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct{ Image string `json:"image"` }
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				result, err := client.scanImage(ctx, input.Image)
				if err != nil {
					return fmt.Sprintf("Failed to scan image: %v", err), nil
				}
				return formatVulns(result), nil
			},
		},
		{
			Name:        "rhacs_image_check",
			Description: "Check a container image against RHACS deploy-time policies. Returns pass/fail with violated policy names.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"image": map[string]any{"type": "string", "description": "Full container image reference (e.g. registry.io/repo/image:tag)"},
				},
				"required": []string{"image"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct{ Image string `json:"image"` }
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				alerts, err := client.checkImage(ctx, input.Image)
				if err != nil {
					return fmt.Sprintf("Failed to check image: %v", err), nil
				}
				if len(alerts) == 0 {
					return fmt.Sprintf("Image %s passed all deploy-time policy checks.", input.Image), nil
				}
				return formatPolicyAlerts(alerts, fmt.Sprintf("Image %s", input.Image)), nil
			},
		},
		{
			Name:        "rhacs_deployment_check",
			Description: "Check a deployment YAML against RHACS policies. Catches issues like privileged containers, missing resource limits, etc.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"yaml": map[string]any{"type": "string", "description": "Kubernetes deployment YAML content to check"},
				},
				"required": []string{"yaml"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct{ YAML string `json:"yaml"` }
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				alerts, err := client.checkDeployment(ctx, input.YAML)
				if err != nil {
					return fmt.Sprintf("Failed to check deployment: %v", err), nil
				}
				if len(alerts) == 0 {
					return "Deployment passed all policy checks.", nil
				}
				return formatPolicyAlerts(alerts, fmt.Sprintf("Deployment FAILED policy checks (%d violation(s))", len(alerts))), nil
			},
		},
		{
			Name:        "rhacs_violations",
			Description: "List active policy violations from RHACS. Can filter by namespace or severity.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"namespace": map[string]any{"type": "string", "description": "Filter violations by namespace"},
					"severity":  map[string]any{"type": "string", "description": "Filter by severity (CRITICAL_SEVERITY, HIGH_SEVERITY, MEDIUM_SEVERITY, LOW_SEVERITY)"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Namespace string `json:"namespace"`
					Severity  string `json:"severity"`
				}
				json.Unmarshal(args, &input)

				var filters []string
				if input.Namespace != "" {
					filters = append(filters, "Namespace:"+input.Namespace)
				}
				if input.Severity != "" {
					filters = append(filters, "Severity:"+input.Severity)
				}
				var query map[string]string
				if len(filters) > 0 {
					query = map[string]string{"query": strings.Join(filters, "+")}
				}

				alerts, err := client.listAlerts(ctx, query)
				if err != nil {
					return fmt.Sprintf("Failed to list violations: %v", err), nil
				}
				return formatAlerts(alerts), nil
			},
		},
		{
			Name:        "rhacs_risk",
			Description: "Get the risk score and risk factors for a deployment from RHACS.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"deploymentId": map[string]any{"type": "string", "description": "The RHACS deployment ID"},
				},
				"required": []string{"deploymentId"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct{ DeploymentID string `json:"deploymentId"` }
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				result, err := client.getDeploymentRisk(ctx, input.DeploymentID)
				if err != nil {
					return fmt.Sprintf("Failed to get risk: %v", err), nil
				}

				name := result.Subject.Name
				if name == "" {
					name = "unknown"
				}
				namespace := result.Subject.Namespace
				if namespace == "" {
					namespace = "unknown"
				}

				lines := []string{
					fmt.Sprintf("Deployment: %s/%s", namespace, name),
					fmt.Sprintf("Risk Score: %.0f", result.Score),
					"",
				}

				if len(result.Results) > 0 {
					lines = append(lines, "Risk Factors:")
					for _, r := range result.Results {
						rName := r.Name
						if rName == "" {
							rName = "unknown"
						}
						lines = append(lines, fmt.Sprintf("  %s:", rName))
						for _, f := range r.Factors {
							msg := f.Message
							if msg == "" {
								msg = "unknown factor"
							}
							lines = append(lines, fmt.Sprintf("    - %s", msg))
						}
					}
				} else {
					lines = append(lines, "No risk factors identified.")
				}

				return strings.Join(lines, "\n"), nil
			},
		},
		{
			Name:        "rhacs_compliance_scan",
			Description: "Run or get compliance scan results from RHACS. If scanConfigId is provided, triggers that scan config and returns results. Otherwise lists available scan configurations.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"scanConfigId": map[string]any{"type": "string", "description": "Scan configuration ID to trigger. If omitted, lists available configs."},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct{ ScanConfigID string `json:"scanConfigId"` }
				json.Unmarshal(args, &input)

				if input.ScanConfigID == "" {
					results, err := client.getComplianceResults(ctx, "")
					if err != nil {
						return fmt.Sprintf("Failed to run compliance scan: %v", err), nil
					}
					if len(results) == 0 {
						return "No compliance scan results available. Provide a scanConfigId to trigger a scan.", nil
					}
					lines := []string{"Available compliance scan results:", ""}
					for _, r := range results {
						configID := r.ScanConfigID
						if configID == "" {
							configID = "unknown"
						}
						lines = append(lines, fmt.Sprintf("- Scan Config: %s", configID))
						for _, p := range r.Profiles {
							passing := p.Passing
							failing := p.Failing
							total := passing + failing + p.Errors
							profileName := p.ProfileName
							if profileName == "" {
								profileName = "unknown"
							}
							lines = append(lines, fmt.Sprintf("  Profile: %s (%d/%d passing)", profileName, passing, total))
						}
					}
					return strings.Join(lines, "\n"), nil
				}

				if err := client.runComplianceScan(ctx, input.ScanConfigID); err != nil {
					return fmt.Sprintf("Failed to run compliance scan: %v", err), nil
				}
				results, err := client.getComplianceResults(ctx, input.ScanConfigID)
				if err != nil {
					return fmt.Sprintf("Failed to run compliance scan: %v", err), nil
				}

				var match *complianceScanResult
				for i := range results {
					if results[i].ScanConfigID == input.ScanConfigID {
						match = &results[i]
						break
					}
				}

				if match == nil {
					return fmt.Sprintf("Scan triggered for config %s but no results available yet.", input.ScanConfigID), nil
				}

				return formatComplianceScanResult(match), nil
			},
		},
		{
			Name:        "rhacs_compliance_status",
			Description: "Get compliance summary across all profiles from RHACS. Shows pass rate and top failing controls per profile.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"profileName": map[string]any{"type": "string", "description": "Filter results to a specific profile name"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct{ ProfileName string `json:"profileName"` }
				json.Unmarshal(args, &input)

				profiles, err := client.getComplianceProfiles(ctx)
				if err != nil {
					return fmt.Sprintf("Failed to get compliance status: %v", err), nil
				}

				if len(profiles) == 0 {
					return "No compliance profiles found.", nil
				}

				if input.ProfileName != "" {
					var filtered []complianceProfile
					for _, p := range profiles {
						if p.Name == input.ProfileName {
							filtered = append(filtered, p)
						}
					}
					if len(filtered) == 0 {
						return fmt.Sprintf("No compliance profile found matching %q.", input.ProfileName), nil
					}
					profiles = filtered
				}

				header := "Profile                                  | Passing | Total | Status"
				separator := strings.Repeat("-", len(header))
				lines := []string{"Compliance Status Summary", "", header, separator}

				for _, p := range profiles {
					total := p.TotalControls
					passing := p.PassingControls
					pct := 0
					if total > 0 {
						pct = int(math.Round(float64(passing) / float64(total) * 100))
					}
					name := p.Name
					if name == "" {
						name = "unknown"
					}
					status := "FAIL"
					if pct == 100 {
						status = "PASS"
					}
					lines = append(lines, fmt.Sprintf("%-40s | %-7s | %-5d | %s",
						name, fmt.Sprintf("%d%%", pct), total, status))
				}

				return strings.Join(lines, "\n"), nil
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

func newPlugin(opts options) plugin.Plugin {
	var client *centralClient
	if opts.CentralURL != "" {
		cfg := redhat.APIClientConfig{BaseURL: opts.CentralURL}
		if opts.Username != "" && opts.Password != "" {
			cfg.BasicAuth = &redhat.BasicAuthConfig{Username: opts.Username, Password: opts.Password}
		} else if opts.APIToken != "" {
			cfg.TokenFn = func(_ context.Context) (string, error) { return opts.APIToken, nil }
		}
		if cfg.BasicAuth != nil || cfg.TokenFn != nil {
			client = newCentralClient(cfg)
		}
	}

	tools := buildTools(client)
	tools = append(tools, buildHealthTool(client))

	return plugin.Plugin{
		ID:    "rhacs",
		Tools: tools,
	}
}

func main() {
	plugin.RunWithOptions(func(params plugin.InitializeParams) (plugin.Plugin, error) {
		opts := parseOptions(params.Options)
		return newPlugin(opts), nil
	})
}
