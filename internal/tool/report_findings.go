package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Finding represents a single code review finding.
type Finding struct {
	File            string `json:"file"`
	Line            int    `json:"line,omitempty"`
	Severity        string `json:"severity,omitempty"`
	Category        string `json:"category,omitempty"`
	Summary         string `json:"summary"`
	FailureScenario string `json:"failure_scenario,omitempty"`
}

var validSeverities = map[string]bool{
	"critical": true,
	"high":     true,
	"medium":   true,
	"low":      true,
	"info":     true,
}

type reportFindingsArgs struct {
	Findings []Finding `json:"findings"`
	Level    string    `json:"level"`
}

// ReportFindingsTool returns a tool that stores structured code review findings
// on the session. Findings accumulate across multiple calls.
func ReportFindingsTool() *Def {
	return &Def{
		ID:          "report_findings",
		Description: "Report code review findings as a structured list",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"findings": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"file": map[string]any{
								"type":        "string",
								"description": "File path of the finding",
							},
							"line": map[string]any{
								"type":        "integer",
								"description": "Line number of the finding",
							},
							"severity": map[string]any{
								"type":        "string",
								"enum":        []string{"critical", "high", "medium", "low", "info"},
								"description": "Severity level",
							},
							"category": map[string]any{
								"type":        "string",
								"description": "Finding category (e.g. bug, security, performance)",
							},
							"summary": map[string]any{
								"type":        "string",
								"description": "Short description of the finding",
							},
							"failure_scenario": map[string]any{
								"type":        "string",
								"description": "What breaks and how",
							},
						},
						"required": []string{"file", "summary"},
					},
					"maxItems": 50,
					"description": "List of code review findings",
				},
				"level": map[string]any{
					"type":        "string",
					"description": "Review effort level",
				},
			},
			"required": []string{"findings"},
		},
		Execute: executeReportFindings,
	}
}

const maxFindingsPerCall = 50

func executeReportFindings(_ context.Context, tc *Context, rawArgs json.RawMessage) (*ExecuteResult, error) {
	var args reportFindingsArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Invalid arguments: %v", err), IsError: true}, nil
	}

	if len(args.Findings) > maxFindingsPerCall {
		return &ExecuteResult{
			Output:  fmt.Sprintf("Too many findings: %d exceeds maximum of %d per call", len(args.Findings), maxFindingsPerCall),
			IsError: true,
		}, nil
	}

	// Validate each finding.
	var errors []string
	for i, f := range args.Findings {
		if f.File == "" {
			errors = append(errors, fmt.Sprintf("findings[%d]: file is required", i))
		}
		if f.Summary == "" {
			errors = append(errors, fmt.Sprintf("findings[%d]: summary is required", i))
		}
		if f.Severity != "" && !validSeverities[f.Severity] {
			errors = append(errors, fmt.Sprintf("findings[%d]: invalid severity %q (must be critical/high/medium/low/info)", i, f.Severity))
		}
	}
	if len(errors) > 0 {
		return &ExecuteResult{
			Output:  "Validation errors:\n" + strings.Join(errors, "\n"),
			IsError: true,
		}, nil
	}

	// Append findings to the shared session slice.
	if tc.Findings == nil {
		return &ExecuteResult{
			Output:  "Findings storage not initialized",
			IsError: true,
		}, nil
	}
	*tc.Findings = append(*tc.Findings, args.Findings...)

	// Build summary with severity counts.
	counts := make(map[string]int)
	for _, f := range args.Findings {
		if f.Severity != "" {
			counts[f.Severity]++
		}
	}

	var parts []string
	for _, sev := range []string{"critical", "high", "medium", "low", "info"} {
		if n := counts[sev]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, sev))
		}
	}

	summary := fmt.Sprintf("Reported %d findings", len(args.Findings))
	if len(parts) > 0 {
		summary += " (" + strings.Join(parts, ", ") + ")"
	}
	if args.Level != "" {
		summary += fmt.Sprintf(" at %s effort level", args.Level)
	}

	if tc.Bus != nil {
		tc.Bus.Publish("session.findings.reported", map[string]any{
			"sessionID": tc.SessionID,
			"count":     len(args.Findings),
			"total":     len(*tc.Findings),
		})
	}

	return &ExecuteResult{Output: summary}, nil
}
