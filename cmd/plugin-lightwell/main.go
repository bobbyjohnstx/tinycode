package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/bobbyjohnstx/tinycode-go/internal/redhat"
	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

const lightwellBaseURL = "https://packages.redhat.com/lightwell"

type options struct {
	ServiceAccountToken string
}

func parseOptions(raw map[string]any) options {
	var opts options
	if v, ok := raw["serviceAccountToken"].(string); ok {
		opts.ServiceAccountToken = v
	}
	return opts
}

// lightwellClient wraps the API client for Lightwell endpoints.
type lightwellClient struct {
	api *redhat.APIClient
}

func newLightwellClient(baseURL, token string) *lightwellClient {
	return &lightwellClient{
		api: redhat.NewAPIClient(redhat.APIClientConfig{
			BaseURL: baseURL,
			TokenFn: func(_ context.Context) (string, error) { return token, nil },
		}),
	}
}

// API response types

type packageCVE struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	FixedIn  string `json:"fixedIn"`
}

type packageCheckResult struct {
	Found            bool         `json:"found"`
	Ecosystem        string       `json:"ecosystem"`
	Name             string       `json:"name"`
	Version          string       `json:"version"`
	LightwellVersion string       `json:"lightwellVersion"`
	PatchAvailable   bool         `json:"patchAvailable"`
	CVECount         int          `json:"cveCount"`
	CVEs             []packageCVE `json:"cves"`
}

type osvVulnerability struct {
	ID       string `json:"id"`
	Summary  string `json:"summary"`
	Severity string `json:"severity"`
}

type osvResult struct {
	Vulnerabilities []osvVulnerability `json:"vulnerabilities"`
}

type attestation struct {
	Type     string `json:"type"`
	Verified bool   `json:"verified"`
	Issuer   string `json:"issuer"`
}

type provenanceResult struct {
	Verified     bool          `json:"verified"`
	BuildType    string        `json:"buildType"`
	Builder      string        `json:"builder"`
	SourceURI    string        `json:"sourceUri"`
	Digest       string        `json:"digest"`
	SLSALevel    string        `json:"slsaLevel"`
	Attestations []attestation `json:"attestations"`
}

func (c *lightwellClient) checkPackage(ctx context.Context, ecosystem, name, version string) (*packageCheckResult, error) {
	path := fmt.Sprintf("/api/v1/packages/%s/%s/%s", ecosystem, url.PathEscape(name), url.PathEscape(version))
	resp, err := c.api.Get(ctx, path, nil)
	if err != nil {
		return nil, err
	}
	var result packageCheckResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *lightwellClient) queryOsv(ctx context.Context, ecosystem, name string) (*osvResult, error) {
	path := fmt.Sprintf("/api/v1/osv/%s/%s", ecosystem, url.PathEscape(name))
	resp, err := c.api.Get(ctx, path, nil)
	if err != nil {
		return nil, err
	}
	var result osvResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *lightwellClient) getProvenance(ctx context.Context, ecosystem, name, version string) (*provenanceResult, error) {
	path := fmt.Sprintf("/api/v1/provenance/%s/%s/%s", ecosystem, url.PathEscape(name), url.PathEscape(version))
	resp, err := c.api.Get(ctx, path, nil)
	if err != nil {
		return nil, err
	}
	var result provenanceResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Formatting helpers

func formatPackageResult(r *packageCheckResult) string {
	ecosystem := r.Ecosystem
	if ecosystem == "" {
		ecosystem = "unknown"
	}
	name := r.Name
	if name == "" {
		name = "unknown"
	}
	version := r.Version
	if version == "" {
		version = "unknown"
	}

	lines := []string{
		fmt.Sprintf("Package: %s/%s@%s", ecosystem, name, version),
		fmt.Sprintf("Found in Lightwell: %s", boolYesNo(r.Found)),
	}

	if r.Found {
		if r.LightwellVersion != "" {
			lines = append(lines, fmt.Sprintf("Lightwell Version (.rhlw): %s", r.LightwellVersion))
		}
		lines = append(lines, fmt.Sprintf("Patch Available: %s", boolYesNo(r.PatchAvailable)))
		lines = append(lines, fmt.Sprintf("CVE Count: %d", r.CVECount))

		if len(r.CVEs) > 0 {
			lines = append(lines, "", "CVEs:")
			for _, cve := range r.CVEs {
				id := cve.ID
				if id == "" {
					id = "unknown"
				}
				severity := cve.Severity
				if severity == "" {
					severity = "UNKNOWN"
				}
				fixedIn := cve.FixedIn
				if fixedIn == "" {
					fixedIn = "no fix"
				}
				lines = append(lines, fmt.Sprintf("  - %s (%s) | Fixed in: %s", id, severity, fixedIn))
			}
		}
	}

	return strings.Join(lines, "\n")
}

func formatOsvResult(ecosystem, name string, vulns []osvVulnerability) string {
	if len(vulns) == 0 {
		return fmt.Sprintf("No known vulnerabilities found for %s/%s.", ecosystem, name)
	}

	lines := []string{
		fmt.Sprintf("Vulnerabilities for %s/%s: %d found", ecosystem, name, len(vulns)),
		"",
	}
	for _, v := range vulns {
		id := v.ID
		if id == "" {
			id = "unknown"
		}
		severity := v.Severity
		if severity == "" {
			severity = "UNKNOWN"
		}
		summary := v.Summary
		if summary == "" {
			summary = "No summary"
		}
		lines = append(lines, fmt.Sprintf("- %s (%s): %s", id, severity, summary))
	}

	return strings.Join(lines, "\n")
}

func formatProvenanceResult(r *provenanceResult) string {
	lines := []string{
		fmt.Sprintf("Provenance Verified: %s", boolYesNo(r.Verified)),
		fmt.Sprintf("SLSA Level: %s", strOrUnknown(r.SLSALevel)),
		fmt.Sprintf("Build Type: %s", strOrUnknown(r.BuildType)),
		fmt.Sprintf("Builder: %s", strOrUnknown(r.Builder)),
		fmt.Sprintf("Source URI: %s", strOrUnknown(r.SourceURI)),
		fmt.Sprintf("Digest: %s", strOrUnknown(r.Digest)),
	}

	if len(r.Attestations) > 0 {
		lines = append(lines, "", "Attestations:")
		for _, att := range r.Attestations {
			t := att.Type
			if t == "" {
				t = "unknown"
			}
			verified := "unverified"
			if att.Verified {
				verified = "verified"
			}
			issuer := att.Issuer
			if issuer == "" {
				issuer = "unknown issuer"
			}
			lines = append(lines, fmt.Sprintf("  - %s: %s (issuer: %s)", t, verified, issuer))
		}
	}

	return strings.Join(lines, "\n")
}

func boolYesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

func strOrUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}

// Dependency file parsing

type parsedDep struct {
	name    string
	version string
}

var depBlockRe = regexp.MustCompile(`(?s)<dependency>\s*(.*?)</dependency>`)
var groupRe = regexp.MustCompile(`<groupId>\s*([^<]+)\s*</groupId>`)
var artifactRe = regexp.MustCompile(`<artifactId>\s*([^<]+)\s*</artifactId>`)
var versionRe = regexp.MustCompile(`<version>\s*([^<]+)\s*</version>`)

func parsePomXml(content string) []parsedDep {
	var deps []parsedDep
	matches := depBlockRe.FindAllStringSubmatch(content, -1)
	for _, m := range matches {
		block := m[1]
		gm := groupRe.FindStringSubmatch(block)
		am := artifactRe.FindStringSubmatch(block)
		vm := versionRe.FindStringSubmatch(block)
		if gm != nil && am != nil && vm != nil {
			deps = append(deps, parsedDep{
				name:    strings.TrimSpace(gm[1]) + ":" + strings.TrimSpace(am[1]),
				version: strings.TrimSpace(vm[1]),
			})
		}
	}
	return deps
}

var reqEqRe = regexp.MustCompile(`^([a-zA-Z0-9_.-]+)==(.+)$`)
var reqGeRe = regexp.MustCompile(`^([a-zA-Z0-9_.-]+)>=(.+)$`)

func parseRequirementsTxt(content string) []parsedDep {
	var deps []parsedDep
	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
			continue
		}
		if m := reqEqRe.FindStringSubmatch(line); m != nil {
			deps = append(deps, parsedDep{name: m[1], version: m[2]})
			continue
		}
		if m := reqGeRe.FindStringSubmatch(line); m != nil {
			deps = append(deps, parsedDep{name: m[1], version: m[2]})
		}
	}
	return deps
}

func parseDeps(content, fileType string) ([]parsedDep, string) {
	switch fileType {
	case "pom.xml":
		return parsePomXml(content), ""
	case "requirements.txt":
		return parseRequirementsTxt(content), ""
	default:
		return nil, fmt.Sprintf("Unsupported file type: %s. Supported types: pom.xml, requirements.txt", fileType)
	}
}

// Config checking

func checkConfigContent(content, fileType string) string {
	lightwellURL := "packages.redhat.com/lightwell"
	configured := strings.Contains(content, lightwellURL)

	lines := []string{
		fmt.Sprintf("Config file type: %s", fileType),
		fmt.Sprintf("Lightwell repos configured: %s", boolYesNo(configured)),
	}

	if !configured {
		lines = append(lines, "", "Suggestions:")
		switch fileType {
		case "settings.xml":
			lines = append(lines, "  - Add a <repository> entry pointing to https://packages.redhat.com/lightwell/maven")
			lines = append(lines, "  - Add a <mirror> element for Lightwell in your <mirrors> section")
		case "build.gradle":
			lines = append(lines, "  - Add maven { url 'https://packages.redhat.com/lightwell/maven' } to repositories block")
		case "pip.conf":
			lines = append(lines, "  - Set index-url = https://packages.redhat.com/lightwell/pypi/simple/ in [global] section")
		default:
			lines = append(lines, fmt.Sprintf("  - Configure your build tool to use https://%s as a package repository", lightwellURL))
		}
	} else {
		lines = append(lines, "", "Lightwell repository URL detected in configuration.")
	}

	return strings.Join(lines, "\n")
}

// Containerfile scanner ecosystem mapping

var ecosystemMap = map[string]string{
	"pip":   "python",
	"npm":   "npm",
	"maven": "java",
}

var lightwellEcosystems = map[redhat.DependencySource]bool{
	redhat.DepPip:   true,
	redhat.DepNpm:   true,
	redhat.DepMaven: true,
}

func buildTools(client *lightwellClient) []plugin.ToolDef {
	notConfigured := "Lightwell plugin not configured. Set serviceAccountToken in plugin options."

	if client == nil {
		return []plugin.ToolDef{
			stubTool("lightwell_check_package", "Check a single package against Red Hat Lightwell remediated/validated repos. Returns patch availability, .rhlw version, and CVE count.", notConfigured),
			stubTool("lightwell_check_deps", "Parse a dependency manifest file and check each dependency against Lightwell repos. Returns a summary table with patch availability and CVE counts.", notConfigured),
			stubTool("lightwell_osv", "Query OSV vulnerability data for a package. Returns a list of known vulnerabilities with severity and summaries.", notConfigured),
			stubTool("lightwell_provenance", "Verify SLSA build provenance for a Lightwell artifact. Returns attestation status, build type, and SLSA level.", notConfigured),
			stubTool("lightwell_config_check", "Analyze build tool configuration content to check if it is configured to use Lightwell repos. Returns configured status and suggestions.", notConfigured),
			stubTool("lightwell_scan_containerfile", "Parse a Containerfile/Dockerfile and check extracted dependencies (pip, npm, maven) against Lightwell repos for patches and vulnerabilities.", notConfigured),
		}
	}

	return []plugin.ToolDef{
		{
			Name:        "lightwell_check_package",
			Description: "Check a single package against Red Hat Lightwell remediated/validated repos. Returns patch availability, .rhlw version, and CVE count.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"ecosystem": map[string]any{"type": "string", "enum": []string{"java", "python"}, "description": "Package ecosystem (java or python)"},
					"name":      map[string]any{"type": "string", "description": "Package name (groupId:artifactId for java, package name for python)"},
					"version":   map[string]any{"type": "string", "description": "Package version"},
				},
				"required": []string{"ecosystem", "name", "version"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Ecosystem string `json:"ecosystem"`
					Name      string `json:"name"`
					Version   string `json:"version"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				result, err := client.checkPackage(ctx, input.Ecosystem, input.Name, input.Version)
				if err != nil {
					return fmt.Sprintf("Failed to check package: %v", err), nil
				}
				return formatPackageResult(result), nil
			},
		},
		{
			Name:        "lightwell_check_deps",
			Description: "Parse a dependency manifest file and check each dependency against Lightwell repos. Returns a summary table with patch availability and CVE counts.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"content":  map[string]any{"type": "string", "description": "File contents of the dependency manifest"},
					"fileType": map[string]any{"type": "string", "enum": []string{"pom.xml", "requirements.txt", "build.gradle", "Pipfile.lock"}, "description": "Type of dependency file"},
				},
				"required": []string{"content", "fileType"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Content  string `json:"content"`
					FileType string `json:"fileType"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}

				deps, errMsg := parseDeps(input.Content, input.FileType)
				if errMsg != "" {
					return errMsg, nil
				}
				if len(deps) == 0 {
					return fmt.Sprintf("No dependencies found in %s content.", input.FileType), nil
				}

				ecosystem := "python"
				if input.FileType == "pom.xml" || input.FileType == "build.gradle" {
					ecosystem = "java"
				}

				lines := []string{
					fmt.Sprintf("Dependencies checked: %d", len(deps)),
					fmt.Sprintf("Ecosystem: %s", ecosystem),
					"",
				}

				patchCount := 0
				cveTotal := 0

				for _, dep := range deps {
					result, err := client.checkPackage(ctx, ecosystem, dep.name, dep.version)
					if err != nil {
						lines = append(lines, fmt.Sprintf("  %s@%s | ERROR | Could not check", dep.name, dep.version))
						continue
					}
					status := "NOT FOUND"
					if result.Found {
						status = "OK"
						if result.PatchAvailable {
							status = "PATCH"
						}
					}
					cveTotal += result.CVECount
					if result.PatchAvailable {
						patchCount++
					}
					lines = append(lines, fmt.Sprintf("  %s@%s | %s | CVEs: %d", dep.name, dep.version, status, result.CVECount))
				}

				lines = append(lines, "")
				lines = append(lines, fmt.Sprintf("Summary: %d patches available, %d total CVEs across %d dependencies", patchCount, cveTotal, len(deps)))

				return strings.Join(lines, "\n"), nil
			},
		},
		{
			Name:        "lightwell_osv",
			Description: "Query OSV vulnerability data for a package. Returns a list of known vulnerabilities with severity and summaries.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"ecosystem": map[string]any{"type": "string", "enum": []string{"java", "python"}, "description": "Package ecosystem (java or python)"},
					"name":      map[string]any{"type": "string", "description": "Package name"},
				},
				"required": []string{"ecosystem", "name"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Ecosystem string `json:"ecosystem"`
					Name      string `json:"name"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				result, err := client.queryOsv(ctx, input.Ecosystem, input.Name)
				if err != nil {
					return fmt.Sprintf("Failed to query OSV: %v", err), nil
				}
				return formatOsvResult(input.Ecosystem, input.Name, result.Vulnerabilities), nil
			},
		},
		{
			Name:        "lightwell_provenance",
			Description: "Verify SLSA build provenance for a Lightwell artifact. Returns attestation status, build type, and SLSA level.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"ecosystem": map[string]any{"type": "string", "enum": []string{"java", "python"}, "description": "Package ecosystem (java or python)"},
					"name":      map[string]any{"type": "string", "description": "Package name"},
					"version":   map[string]any{"type": "string", "description": "Package version"},
				},
				"required": []string{"ecosystem", "name", "version"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Ecosystem string `json:"ecosystem"`
					Name      string `json:"name"`
					Version   string `json:"version"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				result, err := client.getProvenance(ctx, input.Ecosystem, input.Name, input.Version)
				if err != nil {
					return fmt.Sprintf("Failed to verify provenance: %v", err), nil
				}
				return formatProvenanceResult(result), nil
			},
		},
		{
			Name:        "lightwell_config_check",
			Description: "Analyze build tool configuration content to check if it is configured to use Lightwell repos. Returns configured status and suggestions.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"content":  map[string]any{"type": "string", "description": "File contents of the build tool configuration"},
					"fileType": map[string]any{"type": "string", "enum": []string{"settings.xml", "build.gradle", "pip.conf"}, "description": "Type of configuration file"},
				},
				"required": []string{"content", "fileType"},
			},
			Execute: func(_ context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Content  string `json:"content"`
					FileType string `json:"fileType"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				return checkConfigContent(input.Content, input.FileType), nil
			},
		},
		{
			Name:        "lightwell_scan_containerfile",
			Description: "Parse a Containerfile/Dockerfile and check extracted dependencies (pip, npm, maven) against Lightwell repos for patches and vulnerabilities.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"content": map[string]any{"type": "string", "description": "Content of the Containerfile or Dockerfile"},
				},
				"required": []string{"content"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct{ Content string `json:"content"` }
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}

				parsed := redhat.ParseContainerfile(input.Content)
				deps := redhat.ExtractDependencies(parsed)

				var lightwellDeps []redhat.Dependency
				for _, d := range deps {
					if lightwellEcosystems[d.Source] {
						lightwellDeps = append(lightwellDeps, d)
					}
				}

				if len(lightwellDeps) == 0 {
					return "No Lightwell-checkable dependencies found in Containerfile. (Only pip, npm, and maven dependencies are checked.)", nil
				}

				lines := []string{
					fmt.Sprintf("Dependencies found in Containerfile: %d total, %d checkable", len(deps), len(lightwellDeps)),
					"",
				}

				patchCount := 0
				cveTotal := 0

				for _, dep := range lightwellDeps {
					eco := ecosystemMap[string(dep.Source)]
					if eco == "" {
						eco = string(dep.Source)
					}
					version := dep.Version
					if version == "" {
						version = "latest"
					}

					result, err := client.checkPackage(ctx, eco, dep.Name, version)
					if err != nil {
						lines = append(lines, fmt.Sprintf("  %s@%s (%s) | ERROR | Could not check", dep.Name, version, dep.Source))
						continue
					}
					status := "NOT FOUND"
					if result.Found {
						status = "OK"
						if result.PatchAvailable {
							status = "PATCH"
						}
					}
					cveTotal += result.CVECount
					if result.PatchAvailable {
						patchCount++
					}
					lines = append(lines, fmt.Sprintf("  %s@%s (%s) | %s | CVEs: %d", dep.Name, version, dep.Source, status, result.CVECount))
				}

				var skipped []redhat.Dependency
				for _, d := range deps {
					if !lightwellEcosystems[d.Source] {
						skipped = append(skipped, d)
					}
				}
				if len(skipped) > 0 {
					var parts []string
					for _, d := range skipped {
						parts = append(parts, fmt.Sprintf("%s (%s)", d.Name, d.Source))
					}
					lines = append(lines, "")
					lines = append(lines, fmt.Sprintf("Skipped (not Lightwell-relevant): %s", strings.Join(parts, ", ")))
				}

				lines = append(lines, "")
				lines = append(lines, fmt.Sprintf("Summary: %d patches available, %d total CVEs across %d dependencies", patchCount, cveTotal, len(lightwellDeps)))

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
	var client *lightwellClient
	if opts.ServiceAccountToken != "" {
		client = newLightwellClient(lightwellBaseURL, opts.ServiceAccountToken)
	}

	return plugin.Plugin{
		ID:    "lightwell",
		Tools: buildTools(client),
	}
}

func main() {
	opts := options{}
	plugin.Run(newPlugin(opts))
}
