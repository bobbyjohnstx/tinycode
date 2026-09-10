package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/bobbyjohnstx/tinycode-go/internal/redhat"
	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

type lintWarning struct {
	rule       string
	severity   string
	line       int
	message    string
	suggestion string
}

// UBI image prefixes for base image validation.
var ubiPrefixes = []string{
	"registry.access.redhat.com/ubi",
	"registry.redhat.io/ubi",
}

func isUbiImage(image string) bool {
	for _, prefix := range ubiPrefixes {
		if strings.HasPrefix(image, prefix) {
			return true
		}
	}
	return false
}

var secretPattern = regexp.MustCompile(`(?i)password|secret|token|key`)

var requiredLabels = []string{"name", "version", "summary"}

func runLintRules(parsed redhat.ParsedContainerfile) []lintWarning {
	var warnings []lintWarning

	// non-ubi-base
	for _, stage := range parsed.Stages {
		from := stage.From
		if !isUbiImage(from.Image) {
			warnings = append(warnings, lintWarning{
				rule:       "non-ubi-base",
				severity:   "warning",
				line:       from.LineNumber,
				message:    fmt.Sprintf("Base image %q is not a Red Hat UBI image.", from.Image),
				suggestion: "Use a UBI base image such as registry.access.redhat.com/ubi9/ubi:9.5 for RHEL compatibility and support.",
			})
		}
	}

	// latest-tag
	for _, stage := range parsed.Stages {
		from := stage.From
		if from.Tag == "latest" || (from.Tag == "" && from.Digest == "") {
			msg := "FROM uses no tag (implicit latest)."
			if from.Tag == "latest" {
				msg = "FROM uses :latest tag."
			}
			warnings = append(warnings, lintWarning{
				rule:       "latest-tag",
				severity:   "warning",
				line:       from.LineNumber,
				message:    msg,
				suggestion: "Pin to a specific version tag for reproducible builds (e.g. :9.5 or :1.20).",
			})
		}
	}

	// root-user
	for _, stage := range parsed.Stages {
		var userInstrs []redhat.Instruction
		for _, instr := range stage.Instructions {
			if instr.Type == redhat.InstrUser {
				userInstrs = append(userInstrs, instr)
			}
		}
		for idx, u := range userInstrs {
			if u.User == "root" || u.User == "0" {
				hasSubsequentNonRoot := false
				for _, later := range userInstrs[idx+1:] {
					if later.User != "root" && later.User != "0" {
						hasSubsequentNonRoot = true
						break
					}
				}
				if !hasSubsequentNonRoot {
					warnings = append(warnings, lintWarning{
						rule:       "root-user",
						severity:   "error",
						line:       u.LineNumber,
						message:    "USER root without subsequent non-root USER in this stage.",
						suggestion: "Add USER 1001 (or another non-root user) after root operations.",
					})
				}
			}
		}
	}

	// missing-labels
	allLabels := map[string]bool{}
	for _, stage := range parsed.Stages {
		for _, instr := range stage.Instructions {
			if instr.Type == redhat.InstrLabel {
				allLabels[strings.ToLower(instr.Key)] = true
			}
		}
	}
	var missing []string
	for _, l := range requiredLabels {
		if !allLabels[l] {
			missing = append(missing, l)
		}
	}
	if len(missing) > 0 {
		line := 1
		if len(parsed.Stages) > 0 {
			line = parsed.Stages[0].From.LineNumber
		}
		warnings = append(warnings, lintWarning{
			rule:       "missing-labels",
			severity:   "info",
			line:       line,
			message:    fmt.Sprintf("Missing recommended labels: %s.", strings.Join(missing, ", ")),
			suggestion: `Add LABEL name="app-name" version="1.0" summary="description" for OCP compatibility.`,
		})
	}

	// run-layer-chaining
	for _, stage := range parsed.Stages {
		consecutiveRuns := 0
		firstRunLine := 0
		for _, instr := range stage.Instructions {
			if instr.Type == redhat.InstrRun {
				consecutiveRuns++
				if consecutiveRuns == 1 {
					firstRunLine = instr.LineNumber
				}
				if consecutiveRuns == 3 {
					warnings = append(warnings, lintWarning{
						rule:       "run-layer-chaining",
						severity:   "info",
						line:       firstRunLine,
						message:    "3+ consecutive RUN instructions create unnecessary layers.",
						suggestion: "Combine related RUN commands with && to reduce image layers.",
					})
				}
			} else {
				consecutiveRuns = 0
			}
		}
	}

	// hardcoded-secret
	for _, stage := range parsed.Stages {
		for _, instr := range stage.Instructions {
			if instr.Type == redhat.InstrEnv && secretPattern.MatchString(instr.Key) {
				warnings = append(warnings, lintWarning{
					rule:       "hardcoded-secret",
					severity:   "error",
					line:       instr.LineNumber,
					message:    fmt.Sprintf("ENV %q may contain a secret exposed in the image layer.", instr.Key),
					suggestion: "Use --mount=type=secret or runtime environment variables instead of build-time ENV for secrets.",
				})
			}
			if instr.Type == redhat.InstrArg && secretPattern.MatchString(instr.ArgName) {
				warnings = append(warnings, lintWarning{
					rule:       "hardcoded-secret",
					severity:   "error",
					line:       instr.LineNumber,
					message:    fmt.Sprintf("ARG %q may contain a secret visible in build history.", instr.ArgName),
					suggestion: "Use --mount=type=secret or runtime environment variables instead of build-time ARG for secrets.",
				})
			}
		}
	}
	for _, arg := range parsed.GlobalArgs {
		if secretPattern.MatchString(arg.ArgName) {
			warnings = append(warnings, lintWarning{
				rule:       "hardcoded-secret",
				severity:   "error",
				line:       arg.LineNumber,
				message:    fmt.Sprintf("ARG %q may contain a secret visible in build history.", arg.ArgName),
				suggestion: "Use --mount=type=secret or runtime environment variables instead of build-time ARG for secrets.",
			})
		}
	}

	// missing-user-directive
	if len(parsed.Stages) > 0 {
		finalStage := parsed.Stages[len(parsed.Stages)-1]
		hasUser := false
		for _, instr := range finalStage.Instructions {
			if instr.Type == redhat.InstrUser {
				hasUser = true
				break
			}
		}
		if !hasUser {
			warnings = append(warnings, lintWarning{
				rule:       "missing-user-directive",
				severity:   "warning",
				line:       finalStage.From.LineNumber,
				message:    "No USER instruction in the final stage. Container will run as root.",
				suggestion: "Add USER 1001 to run the container as a non-root user.",
			})
		}
	}

	sort.Slice(warnings, func(i, j int) bool {
		return warnings[i].line < warnings[j].line
	})

	return warnings
}

func formatWarnings(warnings []lintWarning) string {
	if len(warnings) == 0 {
		return "No issues found. Containerfile follows Red Hat best practices."
	}

	grouped := map[string][]lintWarning{
		"error":   {},
		"warning": {},
		"info":    {},
	}
	for _, w := range warnings {
		grouped[w.severity] = append(grouped[w.severity], w)
	}

	var lines []string
	for _, severity := range []string{"error", "warning", "info"} {
		group := grouped[severity]
		if len(group) == 0 {
			continue
		}
		lines = append(lines, fmt.Sprintf("--- %sS (%d) ---", strings.ToUpper(severity), len(group)))
		for _, w := range group {
			lines = append(lines, fmt.Sprintf("[%s] Line %d: %s — %s", strings.ToUpper(w.severity), w.line, w.rule, w.message))
			lines = append(lines, fmt.Sprintf("  → %s", w.suggestion))
		}
		lines = append(lines, "")
	}

	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

func validateBootc(content string) string {
	parsed := redhat.ParseContainerfile(content)

	type checkResult struct {
		check  string
		passed bool
		detail string
	}

	var results []checkResult

	// Check bootc base image
	if len(parsed.Stages) == 0 {
		results = append(results, checkResult{
			check:  "bootc base image",
			passed: false,
			detail: `Base image "" is not a recognized bootc base (expected registry.redhat.io/rhel*/rhel-bootc:*).`,
		})
	} else {
		finalStage := parsed.Stages[len(parsed.Stages)-1]
		baseImage := finalStage.From.Image
		fullRef := baseImage
		if finalStage.From.Tag != "" {
			fullRef += ":" + finalStage.From.Tag
		}

		isBootcBase := strings.HasPrefix(baseImage, "registry.redhat.io/rhel") &&
			strings.Contains(fullRef, "rhel-bootc")
		if isBootcBase {
			results = append(results, checkResult{
				check:  "bootc base image",
				passed: true,
				detail: fmt.Sprintf("Base image %s is bootc-compatible.", fullRef),
			})
		} else {
			results = append(results, checkResult{
				check:  "bootc base image",
				passed: false,
				detail: fmt.Sprintf("Base image %q is not a recognized bootc base (expected registry.redhat.io/rhel*/rhel-bootc:*).", fullRef),
			})
		}
	}

	// Check bootc.diskimage-builder label
	hasBootcLabel := false
	for _, stage := range parsed.Stages {
		for _, instr := range stage.Instructions {
			if instr.Type == redhat.InstrLabel && instr.Key == "bootc.diskimage-builder" {
				hasBootcLabel = true
			}
		}
	}
	if hasBootcLabel {
		results = append(results, checkResult{
			check:  "bootc.diskimage-builder label",
			passed: true,
			detail: "Found bootc.diskimage-builder label.",
		})
	} else {
		results = append(results, checkResult{
			check:  "bootc.diskimage-builder label",
			passed: false,
			detail: `Missing LABEL bootc.diskimage-builder. Add LABEL bootc.diskimage-builder="true" for bootc compatibility.`,
		})
	}

	// Check systemd unit files
	hasSystemdFiles := false
	for _, stage := range parsed.Stages {
		for _, instr := range stage.Instructions {
			if (instr.Type == redhat.InstrCopy || instr.Type == redhat.InstrAdd) &&
				strings.HasPrefix(instr.Dest, "/etc/systemd/") {
				hasSystemdFiles = true
			}
		}
	}
	if hasSystemdFiles {
		results = append(results, checkResult{
			check:  "systemd unit files",
			passed: true,
			detail: "Found systemd unit file(s) copied to /etc/systemd/.",
		})
	} else {
		results = append(results, checkResult{
			check:  "systemd unit files",
			passed: false,
			detail: "No systemd unit files detected. Bootc images typically include systemd services.",
		})
	}

	allPassed := true
	for _, r := range results {
		if !r.passed {
			allPassed = false
			break
		}
	}

	var lines []string
	if allPassed {
		lines = append(lines, "PASS: Containerfile is bootc-compatible.")
	} else {
		lines = append(lines, "FAIL: Containerfile has bootc compatibility issues.")
	}
	lines = append(lines, "")
	for _, r := range results {
		status := "PASS"
		if !r.passed {
			status = "FAIL"
		}
		lines = append(lines, fmt.Sprintf("[%s] %s: %s", status, r.check, r.detail))
	}

	return strings.Join(lines, "\n")
}

type baseSuggestion struct {
	image     string
	rationale string
}

const registry = "registry.access.redhat.com/ubi9"

var baseImageLookup = []struct {
	keywords   []string
	suggestion baseSuggestion
}{
	{
		keywords: []string{"java", "quarkus"},
		suggestion: baseSuggestion{
			image:     registry + "/openjdk-21-runtime:1.20",
			rationale: "OpenJDK 21 runtime optimized for Java/Quarkus workloads on UBI 9.",
		},
	},
	{
		keywords: []string{"python"},
		suggestion: baseSuggestion{
			image:     registry + "/python-312:1",
			rationale: "Python 3.12 runtime on UBI 9 with pip and virtualenv pre-installed.",
		},
	},
	{
		keywords: []string{"node", "nodejs"},
		suggestion: baseSuggestion{
			image:     registry + "/nodejs-22:1",
			rationale: "Node.js 22 runtime on UBI 9 for server-side JavaScript workloads.",
		},
	},
	{
		keywords: []string{"go", "golang"},
		suggestion: baseSuggestion{
			image:     registry + "/ubi-minimal:9.5",
			rationale: "Go compiles to static binaries. Use ubi-minimal as a lightweight runtime base.",
		},
	},
	{
		keywords: []string{"minimal"},
		suggestion: baseSuggestion{
			image:     registry + "/ubi-minimal:9.5",
			rationale: "Minimal UBI image with microdnf. Suitable for compiled or minimal apps.",
		},
	},
	{
		keywords: []string{"micro"},
		suggestion: baseSuggestion{
			image:     registry + "/ubi-micro:9.5",
			rationale: "Smallest UBI image with no package manager. Best for static binaries.",
		},
	},
}

var defaultSuggestion = baseSuggestion{
	image:     registry + "/ubi:9.5",
	rationale: "General-purpose UBI 9 base image with dnf and full RHEL userspace.",
}

func suggestBaseImage(useCase string) string {
	lower := strings.ToLower(useCase)
	for _, entry := range baseImageLookup {
		for _, kw := range entry.keywords {
			if strings.Contains(lower, kw) {
				return fmt.Sprintf("Suggested image: %s\nRationale: %s\n\nFROM %s",
					entry.suggestion.image, entry.suggestion.rationale, entry.suggestion.image)
			}
		}
	}
	return fmt.Sprintf("Suggested image: %s\nRationale: %s\n\nFROM %s",
		defaultSuggestion.image, defaultSuggestion.rationale, defaultSuggestion.image)
}

func buildTools() []plugin.ToolDef {
	return []plugin.ToolDef{
		{
			Name:        "container_lint",
			Description: "Lint a Containerfile/Dockerfile for Red Hat best practices including UBI base images, security, and layer optimization.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"content": map[string]any{"type": "string", "description": "Containerfile/Dockerfile content to lint"},
				},
				"required": []string{"content"},
			},
			Execute: func(_ context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct{ Content string `json:"content"` }
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				parsed := redhat.ParseContainerfile(input.Content)
				warnings := runLintRules(parsed)
				return formatWarnings(warnings), nil
			},
		},
		{
			Name:        "bootc_validate",
			Description: "Validate whether a Containerfile is compatible with bootc (bootable container) requirements.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"content": map[string]any{"type": "string", "description": "Containerfile/Dockerfile content to validate for bootc compatibility"},
				},
				"required": []string{"content"},
			},
			Execute: func(_ context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct{ Content string `json:"content"` }
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				return validateBootc(input.Content), nil
			},
		},
		{
			Name:        "container_base_suggest",
			Description: "Suggest a Red Hat UBI base image for a given use case or technology stack.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"useCase": map[string]any{"type": "string", "description": "Description of the use case or technology (e.g. 'Java microservice', 'Python API', 'Go CLI tool')"},
				},
				"required": []string{"useCase"},
			},
			Execute: func(_ context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct{ UseCase string `json:"useCase"` }
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				return suggestBaseImage(input.UseCase), nil
			},
		},
	}
}

func main() {
	p := plugin.Plugin{
		ID:    "container-linter",
		Tools: buildTools(),
	}
	plugin.Run(p)
}
