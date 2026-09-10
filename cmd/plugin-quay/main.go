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
	RegistryURL string
	APIToken    string
}

func parseOptions(raw map[string]any) options {
	var opts options
	if v, ok := raw["registryUrl"].(string); ok {
		opts.RegistryURL = v
	}
	if v, ok := raw["apiToken"].(string); ok {
		opts.APIToken = v
	}
	return opts
}

type quayClient struct {
	api *redhat.APIClient
}

func newQuayClient(registryURL, token string) *quayClient {
	var tokenFn func(context.Context) (string, error)
	if token != "" {
		tokenFn = func(_ context.Context) (string, error) { return token, nil }
	}
	return &quayClient{
		api: redhat.NewAPIClient(redhat.APIClientConfig{
			BaseURL: registryURL,
			TokenFn: tokenFn,
		}),
	}
}

func (c *quayClient) searchRepositories(ctx context.Context, query string) ([]quayRepository, error) {
	resp, err := c.api.Get(ctx, "/api/v1/find/repositories", map[string]string{"query": query})
	if err != nil {
		return nil, err
	}
	var result struct {
		Results []quayRepository `json:"results"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("parsing search results: %w", err)
	}
	return result.Results, nil
}

func (c *quayClient) listTags(ctx context.Context, namespace, name string) ([]quayTag, error) {
	path := fmt.Sprintf("/api/v1/repository/%s/%s/tag/", url.PathEscape(namespace), url.PathEscape(name))
	resp, err := c.api.Get(ctx, path, nil)
	if err != nil {
		return nil, err
	}
	var result struct {
		Tags []quayTag `json:"tags"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("parsing tags: %w", err)
	}
	return result.Tags, nil
}

func (c *quayClient) getManifest(ctx context.Context, namespace, name, digest string) (*quayManifest, error) {
	path := fmt.Sprintf("/api/v1/repository/%s/%s/manifest/%s",
		url.PathEscape(namespace), url.PathEscape(name), url.PathEscape(digest))
	resp, err := c.api.Get(ctx, path, nil)
	if err != nil {
		return nil, err
	}
	var manifest quayManifest
	if err := json.Unmarshal(resp.Data, &manifest); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}
	return &manifest, nil
}

func (c *quayClient) getVulnerabilities(ctx context.Context, namespace, name, digest string) (*quaySecurityResult, error) {
	path := fmt.Sprintf("/api/v1/repository/%s/%s/manifest/%s/security",
		url.PathEscape(namespace), url.PathEscape(name), url.PathEscape(digest))
	resp, err := c.api.Get(ctx, path, nil)
	if err != nil {
		return nil, err
	}
	var result quaySecurityResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("parsing vulnerabilities: %w", err)
	}
	return &result, nil
}

func (c *quayClient) getLabels(ctx context.Context, namespace, name, digest string) ([]quayLabel, error) {
	path := fmt.Sprintf("/api/v1/repository/%s/%s/manifest/%s/labels",
		url.PathEscape(namespace), url.PathEscape(name), url.PathEscape(digest))
	resp, err := c.api.Get(ctx, path, nil)
	if err != nil {
		return nil, err
	}
	var result struct {
		Labels []quayLabel `json:"labels"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("parsing labels: %w", err)
	}
	return result.Labels, nil
}

type quayRepository struct {
	Namespace   string `json:"namespace"`
	Name        string `json:"name"`
	Description string `json:"description"`
	StarCount   int    `json:"star_count"`
}

type quayTag struct {
	Name           string `json:"name"`
	ManifestDigest string `json:"manifest_digest"`
	Size           int64  `json:"size"`
	LastModified   string `json:"last_modified"`
}

type quayManifest struct {
	Digest               string `json:"digest"`
	IsManifestList       bool   `json:"is_manifest_list"`
	ManifestData         string `json:"manifest_data"`
	ConfigMediaType      string `json:"config_media_type"`
	LayersCompressedSize int64  `json:"layers_compressed_size"`
}

type quaySecurityResult struct {
	Status string `json:"status"`
	Data   *struct {
		Layer *struct {
			Features []quayFeature `json:"Features"`
		} `json:"Layer"`
	} `json:"data"`
}

type quayFeature struct {
	Name            string              `json:"Name"`
	Version         string              `json:"Version"`
	Vulnerabilities []quayVulnerability `json:"Vulnerabilities"`
}

type quayVulnerability struct {
	Name     string `json:"Name"`
	Severity string `json:"Severity"`
	FixedBy  string `json:"FixedBy"`
}

type quayLabel struct {
	Key        string `json:"key"`
	Value      string `json:"value"`
	SourceType string `json:"source_type"`
}

var severityOrder = map[string]int{
	"Critical":   0,
	"High":       1,
	"Medium":     2,
	"Low":        3,
	"Negligible": 4,
	"Unknown":    5,
}

func parseRepository(repository string) (namespace, name string, ok bool) {
	parts := strings.SplitN(repository, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func formatVulnerabilities(data *quaySecurityResult) string {
	var features []quayFeature
	if data.Data != nil && data.Data.Layer != nil {
		features = data.Data.Layer.Features
	}

	type vulnEntry struct {
		CVE      string
		Severity string
		Pkg      string
		Version  string
		FixedBy  string
	}
	var vulns []vulnEntry
	for _, f := range features {
		for _, v := range f.Vulnerabilities {
			cve := v.Name
			if cve == "" {
				cve = "unknown"
			}
			severity := v.Severity
			if severity == "" {
				severity = "Unknown"
			}
			pkg := f.Name
			if pkg == "" {
				pkg = "unknown"
			}
			version := f.Version
			if version == "" {
				version = "unknown"
			}
			fixedBy := v.FixedBy
			if fixedBy == "" {
				fixedBy = "no fix available"
			}
			vulns = append(vulns, vulnEntry{
				CVE: cve, Severity: severity, Pkg: pkg, Version: version, FixedBy: fixedBy,
			})
		}
	}

	status := data.Status
	if status == "" {
		status = "unknown"
	}
	if len(vulns) == 0 {
		return fmt.Sprintf("Scan status: %s\nNo vulnerabilities found.", status)
	}

	sort.Slice(vulns, func(i, j int) bool {
		oi := severityOrder[vulns[i].Severity]
		oj := severityOrder[vulns[j].Severity]
		if _, ok := severityOrder[vulns[i].Severity]; !ok {
			oi = 5
		}
		if _, ok := severityOrder[vulns[j].Severity]; !ok {
			oj = 5
		}
		return oi < oj
	})

	lines := []string{
		fmt.Sprintf("Scan status: %s", status),
		fmt.Sprintf("Vulnerabilities found: %d", len(vulns)),
		"",
	}
	for _, v := range vulns {
		lines = append(lines, fmt.Sprintf("- %s [%s] in %s@%s | Fix: %s", v.CVE, v.Severity, v.Pkg, v.Version, v.FixedBy))
	}
	return strings.Join(lines, "\n")
}

func buildTools(client *quayClient) []plugin.ToolDef {
	return []plugin.ToolDef{
		{
			Name:        "quay_search",
			Description: "Search Quay container registry for repositories by name or keyword. Returns repo name, description, star count, and last modified.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string", "description": "Search query for repository name or keyword"},
				},
				"required": []string{"query"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Query string `json:"query"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				repos, err := client.searchRepositories(ctx, input.Query)
				if err != nil {
					return fmt.Sprintf("Failed to search repositories: %v", err), nil
				}
				if len(repos) == 0 {
					return fmt.Sprintf("No repositories found matching %q.", input.Query), nil
				}
				lines := []string{fmt.Sprintf("Repositories matching %q: %d", input.Query, len(repos)), ""}
				for _, r := range repos {
					fullName := r.Name
					if r.Namespace != "" && r.Name != "" {
						fullName = r.Namespace + "/" + r.Name
					}
					desc := ""
					if r.Description != "" {
						desc = ": " + r.Description
					}
					lines = append(lines, fmt.Sprintf("- %s (%d stars)%s", fullName, r.StarCount, desc))
				}
				return strings.Join(lines, "\n"), nil
			},
		},
		{
			Name:        "quay_tags",
			Description: "List tags for a Quay repository. Returns tag name, digest, size, and last modified.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"repository": map[string]any{"type": "string", "description": "Repository in \"namespace/name\" format (e.g. \"redhat/ubi9\")"},
				},
				"required": []string{"repository"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Repository string `json:"repository"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				ns, name, ok := parseRepository(input.Repository)
				if !ok {
					return fmt.Sprintf("Invalid repository format %q. Expected \"namespace/name\" (e.g. \"redhat/ubi9\").", input.Repository), nil
				}
				tags, err := client.listTags(ctx, ns, name)
				if err != nil {
					return fmt.Sprintf("Failed to list tags: %v", err), nil
				}
				if len(tags) == 0 {
					return fmt.Sprintf("No tags found for %s.", input.Repository), nil
				}
				lines := []string{fmt.Sprintf("Tags for %s: %d", input.Repository, len(tags)), ""}
				for _, t := range tags {
					tagName := t.Name
					if tagName == "" {
						tagName = "unknown"
					}
					digest := "unknown"
					if t.ManifestDigest != "" && len(t.ManifestDigest) >= 19 {
						digest = t.ManifestDigest[:19]
					} else if t.ManifestDigest != "" {
						digest = t.ManifestDigest
					}
					size := "unknown size"
					if t.Size > 0 {
						size = fmt.Sprintf("%dMB", int(math.Round(float64(t.Size)/1024/1024)))
					}
					modified := t.LastModified
					if modified == "" {
						modified = "unknown"
					}
					lines = append(lines, fmt.Sprintf("- %s (%s) %s | Modified: %s", tagName, digest, size, modified))
				}
				return strings.Join(lines, "\n"), nil
			},
		},
		{
			Name:        "quay_manifest",
			Description: "Get manifest details for a specific image digest in a Quay repository. Returns layers, architecture, and config.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"repository": map[string]any{"type": "string", "description": "Repository in \"namespace/name\" format (e.g. \"redhat/ubi9\")"},
					"digest":     map[string]any{"type": "string", "description": "Manifest digest (e.g. sha256:abc123...)"},
				},
				"required": []string{"repository", "digest"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Repository string `json:"repository"`
					Digest     string `json:"digest"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				ns, name, ok := parseRepository(input.Repository)
				if !ok {
					return fmt.Sprintf("Invalid repository format %q. Expected \"namespace/name\" (e.g. \"redhat/ubi9\").", input.Repository), nil
				}
				manifest, err := client.getManifest(ctx, ns, name, input.Digest)
				if err != nil {
					return fmt.Sprintf("Failed to get manifest: %v", err), nil
				}
				digest := manifest.Digest
				if digest == "" {
					digest = input.Digest
				}
				manifestList := "no"
				if manifest.IsManifestList {
					manifestList = "yes"
				}
				configMediaType := manifest.ConfigMediaType
				if configMediaType == "" {
					configMediaType = "unknown"
				}
				lines := []string{
					fmt.Sprintf("Manifest: %s", digest),
					fmt.Sprintf("Manifest list: %s", manifestList),
					fmt.Sprintf("Config media type: %s", configMediaType),
				}
				if manifest.LayersCompressedSize > 0 {
					lines = append(lines, fmt.Sprintf("Compressed size: %dMB", int(math.Round(float64(manifest.LayersCompressedSize)/1024/1024))))
				}
				if manifest.ManifestData != "" {
					lines = append(lines, "", "Manifest data:", manifest.ManifestData)
				}
				return strings.Join(lines, "\n"), nil
			},
		},
		{
			Name:        "quay_vulnerabilities",
			Description: "Get Clair vulnerability scan results for a manifest in a Quay repository. Returns CVEs with severity, package, and fix version, sorted by severity.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"repository": map[string]any{"type": "string", "description": "Repository in \"namespace/name\" format (e.g. \"redhat/ubi9\")"},
					"digest":     map[string]any{"type": "string", "description": "Manifest digest (e.g. sha256:abc123...)"},
				},
				"required": []string{"repository", "digest"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Repository string `json:"repository"`
					Digest     string `json:"digest"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				ns, name, ok := parseRepository(input.Repository)
				if !ok {
					return fmt.Sprintf("Invalid repository format %q. Expected \"namespace/name\" (e.g. \"redhat/ubi9\").", input.Repository), nil
				}
				result, err := client.getVulnerabilities(ctx, ns, name, input.Digest)
				if err != nil {
					return fmt.Sprintf("Failed to get vulnerabilities: %v", err), nil
				}
				return formatVulnerabilities(result), nil
			},
		},
		{
			Name:        "quay_labels",
			Description: "Get labels on a manifest in a Quay repository.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"repository": map[string]any{"type": "string", "description": "Repository in \"namespace/name\" format (e.g. \"redhat/ubi9\")"},
					"digest":     map[string]any{"type": "string", "description": "Manifest digest (e.g. sha256:abc123...)"},
				},
				"required": []string{"repository", "digest"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Repository string `json:"repository"`
					Digest     string `json:"digest"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				ns, name, ok := parseRepository(input.Repository)
				if !ok {
					return fmt.Sprintf("Invalid repository format %q. Expected \"namespace/name\" (e.g. \"redhat/ubi9\").", input.Repository), nil
				}
				labels, err := client.getLabels(ctx, ns, name, input.Digest)
				if err != nil {
					return fmt.Sprintf("Failed to get labels: %v", err), nil
				}
				if len(labels) == 0 {
					return fmt.Sprintf("No labels found for %s@%s.", input.Repository, input.Digest), nil
				}
				lines := []string{fmt.Sprintf("Labels for %s@%s: %d", input.Repository, input.Digest, len(labels)), ""}
				for _, l := range labels {
					key := l.Key
					if key == "" {
						key = "unknown"
					}
					sourceType := l.SourceType
					if sourceType == "" {
						sourceType = "unknown"
					}
					lines = append(lines, fmt.Sprintf("- %s: %s (%s)", key, l.Value, sourceType))
				}
				return strings.Join(lines, "\n"), nil
			},
		},
	}
}

func unconfiguredTools() []plugin.ToolDef {
	msg := "Quay plugin is not configured. Set registryUrl in plugin options."
	return []plugin.ToolDef{
		{
			Name:        "quay_search",
			Description: "Search Quay container registry for repositories by name or keyword.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string", "description": "Search query for repository name or keyword"},
				},
				"required": []string{"query"},
			},
			Execute: func(_ context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				return msg, nil
			},
		},
		{
			Name:        "quay_tags",
			Description: "List tags for a Quay repository.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"repository": map[string]any{"type": "string", "description": "Repository in \"namespace/name\" format (e.g. \"redhat/ubi9\")"},
				},
				"required": []string{"repository"},
			},
			Execute: func(_ context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				return msg, nil
			},
		},
		{
			Name:        "quay_manifest",
			Description: "Get manifest details for a specific image digest in a Quay repository.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"repository": map[string]any{"type": "string", "description": "Repository in \"namespace/name\" format (e.g. \"redhat/ubi9\")"},
					"digest":     map[string]any{"type": "string", "description": "Manifest digest (e.g. sha256:abc123...)"},
				},
				"required": []string{"repository", "digest"},
			},
			Execute: func(_ context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				return msg, nil
			},
		},
		{
			Name:        "quay_vulnerabilities",
			Description: "Get Clair vulnerability scan results for a manifest in a Quay repository.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"repository": map[string]any{"type": "string", "description": "Repository in \"namespace/name\" format (e.g. \"redhat/ubi9\")"},
					"digest":     map[string]any{"type": "string", "description": "Manifest digest (e.g. sha256:abc123...)"},
				},
				"required": []string{"repository", "digest"},
			},
			Execute: func(_ context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				return msg, nil
			},
		},
		{
			Name:        "quay_labels",
			Description: "Get labels on a manifest in a Quay repository.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"repository": map[string]any{"type": "string", "description": "Repository in \"namespace/name\" format (e.g. \"redhat/ubi9\")"},
					"digest":     map[string]any{"type": "string", "description": "Manifest digest (e.g. sha256:abc123...)"},
				},
				"required": []string{"repository", "digest"},
			},
			Execute: func(_ context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				return msg, nil
			},
		},
	}
}

func newPlugin(opts options) plugin.Plugin {
	if opts.RegistryURL == "" {
		return plugin.Plugin{
			ID:    "quay",
			Tools: unconfiguredTools(),
		}
	}

	client := newQuayClient(opts.RegistryURL, opts.APIToken)
	return plugin.Plugin{
		ID:    "quay",
		Tools: buildTools(client),
	}
}

func main() {
	opts := options{}
	plugin.Run(newPlugin(opts))
}
