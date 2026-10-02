package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func newFindingsContext() *Context {
	findings := make([]Finding, 0)
	return &Context{
		SessionID: "test-session",
		Findings:  &findings,
	}
}

func TestReportFindings_ValidInput(t *testing.T) {
	def := ReportFindingsTool()
	args, _ := json.Marshal(reportFindingsArgs{
		Findings: []Finding{
			{File: "main.go", Line: 10, Severity: "critical", Summary: "nil pointer"},
			{File: "util.go", Severity: "low", Summary: "unused variable"},
		},
		Level: "high",
	})

	tc := newFindingsContext()
	result, err := def.Execute(context.Background(), tc, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "2 findings") {
		t.Errorf("expected finding count in output, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "1 critical") {
		t.Errorf("expected severity breakdown, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "high effort level") {
		t.Errorf("expected effort level in output, got: %s", result.Output)
	}
	if len(*tc.Findings) != 2 {
		t.Errorf("expected 2 stored findings, got %d", len(*tc.Findings))
	}
}

func TestReportFindings_EmptyArray(t *testing.T) {
	def := ReportFindingsTool()
	args, _ := json.Marshal(reportFindingsArgs{Findings: []Finding{}})

	tc := newFindingsContext()
	result, err := def.Execute(context.Background(), tc, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "0 findings") {
		t.Errorf("expected 0 count, got: %s", result.Output)
	}
}

func TestReportFindings_MaxCapEnforcement(t *testing.T) {
	def := ReportFindingsTool()
	findings := make([]Finding, 51)
	for i := range findings {
		findings[i] = Finding{File: "f.go", Summary: "issue"}
	}
	args, _ := json.Marshal(reportFindingsArgs{Findings: findings})

	tc := newFindingsContext()
	result, err := def.Execute(context.Background(), tc, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for exceeding max findings")
	}
	if !strings.Contains(result.Output, "exceeds maximum") {
		t.Errorf("expected max cap message, got: %s", result.Output)
	}
}

func TestReportFindings_InvalidSeverity(t *testing.T) {
	def := ReportFindingsTool()
	args, _ := json.Marshal(reportFindingsArgs{
		Findings: []Finding{
			{File: "main.go", Severity: "urgent", Summary: "something"},
		},
	})

	tc := newFindingsContext()
	result, err := def.Execute(context.Background(), tc, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected validation error")
	}
	if !strings.Contains(result.Output, "invalid severity") {
		t.Errorf("expected severity validation message, got: %s", result.Output)
	}
}

func TestReportFindings_MissingRequiredFields(t *testing.T) {
	def := ReportFindingsTool()
	args, _ := json.Marshal(reportFindingsArgs{
		Findings: []Finding{
			{File: "", Summary: ""},
		},
	})

	tc := newFindingsContext()
	result, err := def.Execute(context.Background(), tc, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected validation error")
	}
	if !strings.Contains(result.Output, "file is required") {
		t.Errorf("expected file validation message, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "summary is required") {
		t.Errorf("expected summary validation message, got: %s", result.Output)
	}
}

func TestReportFindings_Accumulates(t *testing.T) {
	def := ReportFindingsTool()
	tc := newFindingsContext()

	// First call
	args1, _ := json.Marshal(reportFindingsArgs{
		Findings: []Finding{
			{File: "a.go", Summary: "first"},
		},
	})
	result, err := def.Execute(context.Background(), tc, args1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}

	// Second call
	args2, _ := json.Marshal(reportFindingsArgs{
		Findings: []Finding{
			{File: "b.go", Summary: "second"},
			{File: "c.go", Summary: "third"},
		},
	})
	result, err = def.Execute(context.Background(), tc, args2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}

	if len(*tc.Findings) != 3 {
		t.Errorf("expected 3 accumulated findings, got %d", len(*tc.Findings))
	}
	if (*tc.Findings)[0].Summary != "first" {
		t.Errorf("expected first finding preserved, got: %s", (*tc.Findings)[0].Summary)
	}
}

func TestReportFindings_BadJSON(t *testing.T) {
	def := ReportFindingsTool()
	tc := newFindingsContext()
	result, err := def.Execute(context.Background(), tc, json.RawMessage(`not json`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for bad JSON")
	}
}

func TestReportFindings_OptionalSeverity(t *testing.T) {
	def := ReportFindingsTool()
	args, _ := json.Marshal(reportFindingsArgs{
		Findings: []Finding{
			{File: "main.go", Summary: "no severity specified"},
		},
	})

	tc := newFindingsContext()
	result, err := def.Execute(context.Background(), tc, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "1 findings") {
		t.Errorf("expected finding count, got: %s", result.Output)
	}
}
