package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bobbyjohnstx/tinycode-go/internal/redhat"
	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

type options struct {
	ThanosURL string
	Token     string
}

func parseOptions(raw map[string]any) options {
	var opts options
	if v, ok := raw["thanosUrl"].(string); ok {
		opts.ThanosURL = v
	}
	if v, ok := raw["token"].(string); ok {
		opts.Token = v
	}
	return opts
}

// ACM domain types

type managedCluster struct {
	Name     string            `json:"name"`
	Status   string            `json:"status"`
	Version  string            `json:"version"`
	Provider string            `json:"provider"`
	Labels   map[string]string `json:"labels"`
}

type clusterDetail struct {
	Cluster managedCluster `json:"cluster"`
	Addons  []addonStatus  `json:"addons"`
}

type addonStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type policy struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Compliant string `json:"compliant"`
	Severity  string `json:"severity"`
}

type violation struct {
	Policy   string `json:"policy"`
	Cluster  string `json:"cluster"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
}

type acmApplication struct {
	Name       string   `json:"name"`
	Namespace  string   `json:"namespace"`
	Clusters   []string `json:"clusters"`
	SyncStatus string   `json:"syncStatus"`
}

// K8s resource types for oc get

type managedClusterCondition struct {
	Type   string `json:"type"`
	Status string `json:"status"`
}

type managedClusterResource struct {
	Metadata struct {
		Name   string            `json:"name"`
		Labels map[string]string `json:"labels,omitempty"`
	} `json:"metadata"`
	Status *struct {
		Conditions []managedClusterCondition `json:"conditions,omitempty"`
		Version    *struct {
			Kubernetes string `json:"kubernetes,omitempty"`
		} `json:"version,omitempty"`
	} `json:"status,omitempty"`
}

type addonResource struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Status *struct {
		Conditions []struct {
			Type   string `json:"type"`
			Status string `json:"status"`
		} `json:"conditions,omitempty"`
	} `json:"status,omitempty"`
}

type policyResource struct {
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Spec *struct {
		Severity string `json:"severity,omitempty"`
	} `json:"spec,omitempty"`
	Status *struct {
		Compliant string `json:"compliant,omitempty"`
		Status    []struct {
			ClusterName string `json:"clustername"`
			Compliant   string `json:"compliant"`
		} `json:"status,omitempty"`
		Details []struct {
			TemplateMeta *struct {
				Name string `json:"name"`
			} `json:"templateMeta,omitempty"`
			History []struct {
				Message string `json:"message"`
			} `json:"history,omitempty"`
		} `json:"details,omitempty"`
	} `json:"status,omitempty"`
}

type appResource struct {
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Spec *struct {
		Destination *struct {
			Name      string `json:"name,omitempty"`
			Namespace string `json:"namespace,omitempty"`
		} `json:"destination,omitempty"`
	} `json:"spec,omitempty"`
	Status *struct {
		Sync *struct {
			Status string `json:"status,omitempty"`
		} `json:"sync,omitempty"`
	} `json:"status,omitempty"`
}

func parseClusterStatus(conditions []managedClusterCondition) string {
	for _, c := range conditions {
		if c.Type == "ManagedClusterConditionAvailable" {
			if c.Status == "True" {
				return "Ready"
			}
			return "NotReady"
		}
	}
	return "Unknown"
}

func parseManagedCluster(r managedClusterResource) managedCluster {
	version := "unknown"
	if r.Status != nil && r.Status.Version != nil && r.Status.Version.Kubernetes != "" {
		version = r.Status.Version.Kubernetes
	}
	provider := "unknown"
	if cloud, ok := r.Metadata.Labels["cloud"]; ok {
		provider = cloud
	}
	var conditions []managedClusterCondition
	if r.Status != nil {
		conditions = r.Status.Conditions
	}
	labels := r.Metadata.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	return managedCluster{
		Name:     r.Metadata.Name,
		Status:   parseClusterStatus(conditions),
		Version:  version,
		Provider: provider,
		Labels:   labels,
	}
}

func listClusters(ctx context.Context, oc *redhat.OcClient, status string) ([]managedCluster, error) {
	result, err := oc.Get(ctx, "managedclusters", nil)
	if err != nil {
		return nil, err
	}
	var list struct {
		Items []managedClusterResource `json:"items"`
	}
	if err := json.Unmarshal(result, &list); err != nil {
		return nil, fmt.Errorf("parsing managed clusters: %w", err)
	}
	var clusters []managedCluster
	for _, item := range list.Items {
		c := parseManagedCluster(item)
		if status != "" && c.Status != status {
			continue
		}
		clusters = append(clusters, c)
	}
	return clusters, nil
}

func getClusterDetail(ctx context.Context, oc *redhat.OcClient, name string) (*clusterDetail, error) {
	result, err := oc.Get(ctx, "managedcluster/"+name, nil)
	if err != nil {
		return nil, err
	}
	var resource managedClusterResource
	if err := json.Unmarshal(result, &resource); err != nil {
		return nil, fmt.Errorf("parsing managed cluster: %w", err)
	}
	cluster := parseManagedCluster(resource)

	addonResult, err := oc.Get(ctx, "managedclusteraddons", &redhat.OcGetOptions{Namespace: name})
	if err != nil {
		return nil, fmt.Errorf("listing addons: %w", err)
	}
	var addonList struct {
		Items []addonResource `json:"items"`
	}
	if err := json.Unmarshal(addonResult, &addonList); err != nil {
		return nil, fmt.Errorf("parsing addons: %w", err)
	}
	var addons []addonStatus
	for _, addon := range addonList.Items {
		status := "Unavailable"
		if addon.Status != nil {
			for _, c := range addon.Status.Conditions {
				if c.Type == "Available" && c.Status == "True" {
					status = "Available"
					break
				}
			}
		}
		addons = append(addons, addonStatus{Name: addon.Metadata.Name, Status: status})
	}
	return &clusterDetail{Cluster: cluster, Addons: addons}, nil
}

func listPolicies(ctx context.Context, oc *redhat.OcClient, namespace string) ([]policy, error) {
	var getOpts *redhat.OcGetOptions
	if namespace != "" {
		getOpts = &redhat.OcGetOptions{Namespace: namespace}
	}
	result, err := oc.Get(ctx, "policies.policy.open-cluster-management.io", getOpts)
	if err != nil {
		return nil, err
	}
	var list struct {
		Items []policyResource `json:"items"`
	}
	if err := json.Unmarshal(result, &list); err != nil {
		return nil, fmt.Errorf("parsing policies: %w", err)
	}
	var policies []policy
	for _, p := range list.Items {
		compliant := "Unknown"
		if p.Status != nil && p.Status.Compliant != "" {
			compliant = p.Status.Compliant
		}
		severity := "low"
		if p.Spec != nil && p.Spec.Severity != "" {
			severity = p.Spec.Severity
		}
		policies = append(policies, policy{
			Name:      p.Metadata.Name,
			Namespace: p.Metadata.Namespace,
			Compliant: compliant,
			Severity:  severity,
		})
	}
	return policies, nil
}

func listViolations(ctx context.Context, oc *redhat.OcClient, cluster string) ([]violation, error) {
	result, err := oc.Get(ctx, "policies.policy.open-cluster-management.io", nil)
	if err != nil {
		return nil, err
	}
	var list struct {
		Items []policyResource `json:"items"`
	}
	if err := json.Unmarshal(result, &list); err != nil {
		return nil, fmt.Errorf("parsing policies: %w", err)
	}
	var violations []violation
	for _, p := range list.Items {
		if p.Status == nil || p.Status.Compliant != "NonCompliant" {
			continue
		}
		for _, cs := range p.Status.Status {
			if cs.Compliant != "NonCompliant" {
				continue
			}
			if cluster != "" && cs.ClusterName != cluster {
				continue
			}
			message := "Policy violation detected"
			if len(p.Status.Details) > 0 && len(p.Status.Details[0].History) > 0 {
				message = p.Status.Details[0].History[0].Message
			}
			severity := "low"
			if p.Spec != nil && p.Spec.Severity != "" {
				severity = p.Spec.Severity
			}
			violations = append(violations, violation{
				Policy:   p.Metadata.Name,
				Cluster:  cs.ClusterName,
				Message:  message,
				Severity: severity,
			})
		}
	}
	return violations, nil
}

func listApplications(ctx context.Context, oc *redhat.OcClient, cluster string) ([]acmApplication, error) {
	result, err := oc.Get(ctx, "applications.argoproj.io", nil)
	if err != nil {
		return nil, err
	}
	var list struct {
		Items []appResource `json:"items"`
	}
	if err := json.Unmarshal(result, &list); err != nil {
		return nil, fmt.Errorf("parsing applications: %w", err)
	}
	var apps []acmApplication
	for _, a := range list.Items {
		var clusters []string
		if a.Spec != nil && a.Spec.Destination != nil && a.Spec.Destination.Name != "" {
			clusters = []string{a.Spec.Destination.Name}
		}
		syncStatus := "Unknown"
		if a.Status != nil && a.Status.Sync != nil && a.Status.Sync.Status != "" {
			syncStatus = a.Status.Sync.Status
		}
		if cluster != "" {
			found := false
			for _, c := range clusters {
				if c == cluster {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if clusters == nil {
			clusters = []string{}
		}
		apps = append(apps, acmApplication{
			Name:       a.Metadata.Name,
			Namespace:  a.Metadata.Namespace,
			Clusters:   clusters,
			SyncStatus: syncStatus,
		})
	}
	return apps, nil
}

func truncateWithMessage(data any, count, max int, label string) (string, error) {
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", err
	}
	if count > max {
		return string(out) + fmt.Sprintf("\n\n(Showing %d of %d %s. Use filters or increase limit.)", max, count, label), nil
	}
	return string(out), nil
}

func buildTools(oc *redhat.OcClient) []plugin.ToolDef {
	return []plugin.ToolDef{
		{
			Name:        "acm_clusters",
			Description: "List managed clusters with status, version, and cloud provider. Optionally filter by Ready or NotReady status.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"status": map[string]any{"type": "string", "enum": []string{"Ready", "NotReady"}, "description": "Filter clusters by status"},
					"limit":  map[string]any{"type": "integer", "description": "Maximum number of clusters to return (default: 50)"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Status string `json:"status"`
					Limit  int    `json:"limit"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				clusters, err := listClusters(ctx, oc, input.Status)
				if err != nil {
					return fmt.Sprintf("Error listing clusters: %v", err), nil
				}
				if len(clusters) == 0 {
					return "No managed clusters found.", nil
				}
				max := input.Limit
				if max <= 0 {
					max = 50
				}
				total := len(clusters)
				if total > max {
					clusters = clusters[:max]
				}
				result, _ := truncateWithMessage(clusters, total, max, "clusters")
				return result, nil
			},
		},
		{
			Name:        "acm_cluster_detail",
			Description: "Get detailed information for a single managed cluster including installed add-ons and their status.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string", "description": "Name of the managed cluster"},
				},
				"required": []string{"name"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Name string `json:"name"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				detail, err := getClusterDetail(ctx, oc, input.Name)
				if err != nil {
					return fmt.Sprintf("Error getting cluster detail: %v", err), nil
				}
				out, _ := json.MarshalIndent(detail, "", "  ")
				return string(out), nil
			},
		},
		{
			Name:        "acm_policies",
			Description: "List governance policies across the ACM hub. Optionally filter by namespace.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"namespace": map[string]any{"type": "string", "description": "Filter policies by namespace"},
					"limit":     map[string]any{"type": "integer", "description": "Maximum number of policies to return (default: 50)"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Namespace string `json:"namespace"`
					Limit     int    `json:"limit"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				policies, err := listPolicies(ctx, oc, input.Namespace)
				if err != nil {
					return fmt.Sprintf("Error listing policies: %v", err), nil
				}
				if len(policies) == 0 {
					return "No governance policies found.", nil
				}
				max := input.Limit
				if max <= 0 {
					max = 50
				}
				total := len(policies)
				if total > max {
					policies = policies[:max]
				}
				result, _ := truncateWithMessage(policies, total, max, "policies")
				return result, nil
			},
		},
		{
			Name:        "acm_violations",
			Description: "List active policy violations across managed clusters. Optionally filter by cluster or severity.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"cluster":  map[string]any{"type": "string", "description": "Filter violations by cluster name"},
					"severity": map[string]any{"type": "string", "description": "Filter violations by severity (e.g. high, medium, low)"},
					"limit":    map[string]any{"type": "integer", "description": "Maximum number of violations to return (default: 50)"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Cluster  string `json:"cluster"`
					Severity string `json:"severity"`
					Limit    int    `json:"limit"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				violations, err := listViolations(ctx, oc, input.Cluster)
				if err != nil {
					return fmt.Sprintf("Error listing violations: %v", err), nil
				}
				if input.Severity != "" {
					var filtered []violation
					for _, v := range violations {
						if v.Severity == input.Severity {
							filtered = append(filtered, v)
						}
					}
					violations = filtered
				}
				if len(violations) == 0 {
					return "No active policy violations.", nil
				}
				max := input.Limit
				if max <= 0 {
					max = 50
				}
				total := len(violations)
				if total > max {
					violations = violations[:max]
				}
				result, _ := truncateWithMessage(violations, total, max, "violations")
				return result, nil
			},
		},
		{
			Name:        "acm_applications",
			Description: "List ACM-managed applications and their sync status. Optionally filter by target cluster.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"cluster": map[string]any{"type": "string", "description": "Filter applications by target cluster name"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Cluster string `json:"cluster"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				apps, err := listApplications(ctx, oc, input.Cluster)
				if err != nil {
					return fmt.Sprintf("Error listing applications: %v", err), nil
				}
				if len(apps) == 0 {
					return "No ACM-managed applications found.", nil
				}
				out, _ := json.MarshalIndent(apps, "", "  ")
				return string(out), nil
			},
		},
		{
			Name:        "acm_app_deploy",
			Description: "Deploy an ApplicationSet or Application manifest to the ACM hub.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"yaml": map[string]any{"type": "string", "description": "YAML manifest content for the ApplicationSet or Application"},
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
				result, err := oc.Apply(ctx, input.YAML)
				if err != nil {
					return fmt.Sprintf("Error deploying application: %v", err), nil
				}
				if result == "" {
					return "Applied successfully", nil
				}
				return result, nil
			},
		},
	}
}

func buildObservabilityTool(client *redhat.PromQLClient) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "acm_observability",
		Description: "Run a PromQL query against the ACM Thanos observability endpoint.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "PromQL query to execute"},
				"time":  map[string]any{"type": "string", "description": "Evaluation timestamp (RFC3339 or Unix timestamp)"},
			},
			"required": []string{"query"},
		},
		Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
			var input struct {
				Query string `json:"query"`
				Time  string `json:"time"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return "", fmt.Errorf("parsing args: %w", err)
			}
			result, err := client.InstantQuery(ctx, input.Query, input.Time)
			if err != nil {
				return fmt.Sprintf("Error querying ACM observability: %v", err), nil
			}
			out, _ := json.MarshalIndent(result, "", "  ")
			return string(out), nil
		},
	}
}

func unconfiguredObservabilityTool() plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "acm_observability",
		Description: "Run a PromQL query against the ACM Thanos observability endpoint. Requires thanosUrl to be configured.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "PromQL query to execute"},
				"time":  map[string]any{"type": "string", "description": "Evaluation timestamp (RFC3339 or Unix timestamp)"},
			},
			"required": []string{"query"},
		},
		Execute: func(_ context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
			return "ACM observability not configured. Set thanosUrl in plugin options.", nil
		},
	}
}

func newPlugin(opts options) plugin.Plugin {
	oc := redhat.NewOcClient()
	tools := buildTools(oc)

	if opts.ThanosURL != "" && opts.Token != "" {
		tokenFn := func(_ context.Context) (string, error) { return opts.Token, nil }
		client := redhat.NewPromQLClient(redhat.PromQLClientConfig{
			BaseURL: opts.ThanosURL,
			TokenFn: tokenFn,
		})
		tools = append(tools, buildObservabilityTool(client))
	} else {
		tools = append(tools, unconfiguredObservabilityTool())
	}

	return plugin.Plugin{
		ID:    "rhacm",
		Tools: tools,
	}
}

func main() {
	plugin.RunWithOptions(func(params plugin.InitializeParams) (plugin.Plugin, error) {
		opts := parseOptions(params.Options)
		return newPlugin(opts), nil
	})
}
