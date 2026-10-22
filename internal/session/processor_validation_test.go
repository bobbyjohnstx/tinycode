package session

import (
	"context"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/permission"
)

func TestDoomThreshold_DefaultIsThree(t *testing.T) {
	p := &Processor{}
	if got := p.doomThreshold(); got != defaultDoomThreshold {
		t.Errorf("doomThreshold() = %d, want %d", got, defaultDoomThreshold)
	}
}

func TestDoomThreshold_UsesConfiguredValue(t *testing.T) {
	p := &Processor{config: ProcessorConfig{DoomThreshold: 5}}
	if got := p.doomThreshold(); got != 5 {
		t.Errorf("doomThreshold() = %d, want 5", got)
	}
}

func TestDoomThreshold_GoalActiveRaises(t *testing.T) {
	p := &Processor{config: ProcessorConfig{DoomThreshold: 3, GoalActive: true}}
	if got := p.doomThreshold(); got != GoalDoomThreshold(3) {
		t.Errorf("doomThreshold() = %d, want %d", got, GoalDoomThreshold(3))
	}
}

func TestCanonicalToolArgs_WhitespaceAndKeyOrder(t *testing.T) {
	a := canonicalToolArgs(`{"b":1,"a":2}`)
	b := canonicalToolArgs(`{ "a" : 2 , "b" : 1 }`)
	if a != b {
		t.Fatalf("canonical mismatch: %q vs %q", a, b)
	}
	if !isDoomLoop([]ToolCallSignature{
		{Name: "read", Args: a},
		{Name: "read", Args: b},
		{Name: "read", Args: a},
	}, 3) {
		t.Fatal("expected doom after canonical compare")
	}
}

func TestMessageHasVisibleAnswer_TextOnly(t *testing.T) {
	if messageHasVisibleAnswer(&Message{Parts: []Part{
		ToolCallPart("c1", "read", `{"path":"a.go"}`),
	}}) {
		t.Fatal("tool-call-only must not count as visible answer")
	}
	if !messageHasVisibleAnswer(&Message{Parts: []Part{TextPart("hello")}}) {
		t.Fatal("text should count as visible answer")
	}
}

func TestCompactBreakerTripped(t *testing.T) {
	if CompactBreakerTripped(2) {
		t.Fatal("2 should not trip")
	}
	if !CompactBreakerTripped(3) {
		t.Fatal("3 should trip")
	}
}

func TestNewProcessor_SeedsCompactionFailures(t *testing.T) {
	p := NewProcessor(ProcessorConfig{CompactionFailures: 2}, nil, nil, nil)
	if p.CompactionFailures() != 2 {
		t.Fatalf("got %d, want 2", p.CompactionFailures())
	}
}

func TestCheckDoomLoop_DefaultStopsAtThree(t *testing.T) {
	p := &Processor{}
	call := ToolCallPart("call", "read", `{"path":"a.go"}`)
	for i := 1; i <= 2; i++ {
		if res := p.checkDoomLoop([]Part{call}, TokenUsage{}); res != nil {
			t.Fatalf("call %d stopped early: %v", i, res.Error)
		}
	}
	res := p.checkDoomLoop([]Part{call}, TokenUsage{})
	if res == nil || res.Error == nil || !strings.Contains(res.Error.Error(), "last 3") {
		t.Fatalf("third identical call = %v, want doom loop at 3", res)
	}
}

func TestCheckDoomLoop_ConfiguredThreshold(t *testing.T) {
	p := &Processor{config: ProcessorConfig{DoomThreshold: 5}}
	call := ToolCallPart("call", "read", `{"path":"a.go"}`)
	for i := 1; i <= 4; i++ {
		if res := p.checkDoomLoop([]Part{call}, TokenUsage{}); res != nil {
			t.Fatalf("call %d stopped early: %v", i, res.Error)
		}
	}
	res := p.checkDoomLoop([]Part{call}, TokenUsage{})
	if res == nil || res.Error == nil || !strings.Contains(res.Error.Error(), "last 5") {
		t.Fatalf("fifth identical call = %v, want doom loop at 5", res)
	}
}

func TestCheckExternalDirectory_NilPerms_AllowsEverything(t *testing.T) {
	p := &Processor{
		config: ProcessorConfig{
			SessionID: "ses-test",
			Directory: "/project",
			Perms:     nil, // no permission service
		},
	}

	call := ToolCallPart("call-1", "read", `{"file_path":"/etc/passwd"}`)
	result := p.checkExternalDirectory(context.Background(), call)
	if result != "" {
		t.Errorf("expected empty string (allowed) when Perms is nil, got %q", result)
	}
}

func TestCheckExternalDirectory_WithPerms_DeniesExternalPath(t *testing.T) {
	b := bus.New()
	defer b.Close()

	perms := permission.NewService(b)

	p := &Processor{
		config: ProcessorConfig{
			SessionID: "ses-test",
			Directory: "/project",
			Perms:     perms,
			Ruleset: permission.Ruleset{
				{Permission: "external_directory", Pattern: "*", Action: permission.ActionDeny},
			},
		},
	}

	call := ToolCallPart("call-1", "read", `{"file_path":"/etc/passwd"}`)
	result := p.checkExternalDirectory(context.Background(), call)
	if result == "" {
		t.Error("expected denial message for external path when Perms is set, got empty string")
	}
}

func TestCheckExternalDirectory_WithPerms_AllowsInternalPath(t *testing.T) {
	b := bus.New()
	defer b.Close()

	perms := permission.NewService(b)

	p := &Processor{
		config: ProcessorConfig{
			SessionID: "ses-test",
			Directory: "/project",
			Perms:     perms,
			Ruleset: permission.Ruleset{
				{Permission: "external_directory", Pattern: "*", Action: permission.ActionDeny},
			},
		},
	}

	call := ToolCallPart("call-1", "read", `{"file_path":"/project/src/main.go"}`)
	result := p.checkExternalDirectory(context.Background(), call)
	if result != "" {
		t.Errorf("expected empty string for internal path, got %q", result)
	}
}

func TestCheckExternalDirectory_EmptyDirectory_AllowsEverything(t *testing.T) {
	b := bus.New()
	defer b.Close()

	perms := permission.NewService(b)

	p := &Processor{
		config: ProcessorConfig{
			SessionID: "ses-test",
			Directory: "", // no directory configured
			Perms:     perms,
		},
	}

	call := ToolCallPart("call-1", "read", `{"file_path":"/etc/passwd"}`)
	result := p.checkExternalDirectory(context.Background(), call)
	if result != "" {
		t.Errorf("expected empty string when Directory is empty, got %q", result)
	}
}
