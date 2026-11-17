package session

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	// read is a read-only tool so it needs 2x threshold (6 calls for threshold 3).
	if !isDoomLoop([]ToolCallSignature{
		{Name: "read", Args: a},
		{Name: "read", Args: b},
		{Name: "read", Args: a},
		{Name: "read", Args: b},
		{Name: "read", Args: a},
		{Name: "read", Args: b},
	}, 3) {
		t.Fatal("expected doom after canonical compare with read-only double threshold")
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

func TestNewProcessor_SeedsCompactionFailures(t *testing.T) {
	p := NewProcessor(ProcessorConfig{CompactionFailures: 2}, nil, nil, nil)
	if p.CompactionFailures() != 2 {
		t.Fatalf("got %d, want 2", p.CompactionFailures())
	}
}

func TestCheckDoomLoop_DefaultStopsAtThree(t *testing.T) {
	p := &Processor{}
	call := ToolCallPart("call", "bash", `{"command":"ls"}`)
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
	call := ToolCallPart("call", "bash", `{"command":"ls"}`)
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

func respondToNextAsk(t *testing.T, perms *permission.Service, reply permission.Reply) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		pending := perms.List()
		if len(pending) > 0 {
			if err := perms.RespondToAsk(permission.ReplyInput{
				RequestID: pending[0].ID,
				Reply:     reply,
			}); err != nil {
				t.Fatal(err)
			}
			return
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for permission ask")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestCheckExternalDirectory_AlwaysCoversDirectory(t *testing.T) {
	b := bus.New()
	defer b.Close()
	perms := permission.NewService(b)
	perms.SetHasResponder(true)
	defer perms.Close()

	dir := t.TempDir()
	extDir := t.TempDir()
	fileA := filepath.Join(extDir, "a.txt")
	fileB := filepath.Join(extDir, "b.txt")
	os.WriteFile(fileA, []byte("a"), 0o644)
	os.WriteFile(fileB, []byte("b"), 0o644)

	p := &Processor{config: ProcessorConfig{SessionID: "ses-dir", Directory: dir, Perms: perms}}

	// First call asks — approve always
	callA := ToolCallPart("c1", "read", fmt.Sprintf(`{"file_path":%q}`, fileA))
	done := make(chan string)
	go func() { done <- p.checkExternalDirectory(context.Background(), callA) }()
	respondToNextAsk(t, perms, permission.ReplyAlways)
	if r := <-done; r != "" {
		t.Fatalf("expected allow, got %q", r)
	}

	// Second call in same directory should NOT ask (covered by always)
	callB := ToolCallPart("c2", "read", fmt.Sprintf(`{"file_path":%q}`, fileB))
	result := p.checkExternalDirectory(context.Background(), callB)
	if result != "" {
		t.Fatalf("expected auto-allow for sibling, got %q", result)
	}
}

func TestCheckExternalDirectory_AlwaysDoesNotCoverDifferentDirectory(t *testing.T) {
	b := bus.New()
	defer b.Close()
	perms := permission.NewService(b)
	perms.SetHasResponder(true)
	defer perms.Close()

	dir := t.TempDir()
	extDir1 := t.TempDir()
	extDir2 := t.TempDir()
	file1 := filepath.Join(extDir1, "a.txt")
	file2 := filepath.Join(extDir2, "c.txt")
	os.WriteFile(file1, []byte("a"), 0o644)
	os.WriteFile(file2, []byte("c"), 0o644)

	p := &Processor{config: ProcessorConfig{SessionID: "ses-dir2", Directory: dir, Perms: perms}}

	// Approve always for extDir1
	call1 := ToolCallPart("c1", "read", fmt.Sprintf(`{"file_path":%q}`, file1))
	done := make(chan string)
	go func() { done <- p.checkExternalDirectory(context.Background(), call1) }()
	respondToNextAsk(t, perms, permission.ReplyAlways)
	<-done

	// extDir2 should still ask
	call2 := ToolCallPart("c2", "read", fmt.Sprintf(`{"file_path":%q}`, file2))
	go func() { done <- p.checkExternalDirectory(context.Background(), call2) }()
	respondToNextAsk(t, perms, permission.ReplyReject)
	if r := <-done; r == "" {
		t.Fatal("expected denial for different directory")
	}
}
