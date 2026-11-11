package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/llm"
	"github.com/bobbyjohnstx/tinycode/internal/provider"

	_ "modernc.org/sqlite"
)

// ---------------------------------------------------------------------------
// trackFiles
// ---------------------------------------------------------------------------

func TestTrackFiles_ReadToolExtractsPath(t *testing.T) {
	messages := []Message{
		{Parts: []Part{ToolCallPart("c1", "read", `{"file_path": "/src/main.go"}`)}},
	}
	readFiles, _ := trackFiles(messages)
	if len(readFiles) != 1 || readFiles[0] != "/src/main.go" {
		t.Errorf("expected [/src/main.go], got %v", readFiles)
	}
}

func TestTrackFiles_WriteToolExtractsPath(t *testing.T) {
	messages := []Message{
		{Parts: []Part{ToolCallPart("c1", "write", `{"path": "/src/out.go"}`)}},
	}
	_, modifiedFiles := trackFiles(messages)
	if len(modifiedFiles) != 1 || modifiedFiles[0] != "/src/out.go" {
		t.Errorf("expected [/src/out.go], got %v", modifiedFiles)
	}
}

func TestTrackFiles_EditAndApplyPatchAreModified(t *testing.T) {
	messages := []Message{
		{Parts: []Part{ToolCallPart("c1", "edit", `{"file_path": "/a.go"}`)}},
		{Parts: []Part{ToolCallPart("c2", "apply_patch", `{"file": "/b.go"}`)}},
	}
	_, modifiedFiles := trackFiles(messages)
	modSet := make(map[string]bool)
	for _, f := range modifiedFiles {
		modSet[f] = true
	}
	if !modSet["/a.go"] {
		t.Error("expected /a.go in modified files")
	}
	if !modSet["/b.go"] {
		t.Error("expected /b.go in modified files")
	}
}

func TestTrackFiles_SkipsNonToolCallParts(t *testing.T) {
	messages := []Message{
		{Parts: []Part{TextPart("some text"), ReasoningPart("thinking")}},
	}
	readFiles, modifiedFiles := trackFiles(messages)
	if len(readFiles) != 0 {
		t.Errorf("expected no read files, got %v", readFiles)
	}
	if len(modifiedFiles) != 0 {
		t.Errorf("expected no modified files, got %v", modifiedFiles)
	}
}

func TestTrackFiles_DeduplicatesFiles(t *testing.T) {
	messages := []Message{
		{Parts: []Part{ToolCallPart("c1", "read", `{"file_path": "/x.go"}`)}},
		{Parts: []Part{ToolCallPart("c2", "read", `{"file_path": "/x.go"}`)}},
	}
	readFiles, _ := trackFiles(messages)
	if len(readFiles) != 1 {
		t.Errorf("expected 1 deduplicated read file, got %d: %v", len(readFiles), readFiles)
	}
}

func TestTrackFiles_EmptyMessages(t *testing.T) {
	readFiles, modifiedFiles := trackFiles(nil)
	if len(readFiles) != 0 || len(modifiedFiles) != 0 {
		t.Errorf("expected empty results for nil messages, got read=%v mod=%v", readFiles, modifiedFiles)
	}
}

// ---------------------------------------------------------------------------
// extractShellFiles
// ---------------------------------------------------------------------------

func TestExtractShellFiles_ReadCommands(t *testing.T) {
	tests := []struct {
		name     string
		argsJSON string
		wantRead string
	}{
		{"cat command", `{"command": "cat /tmp/data.txt"}`, "/tmp/data.txt"},
		{"head command", `{"command": "head /var/log/app.log"}`, "/var/log/app.log"},
		{"tail command", `{"command": "tail /etc/hosts"}`, "/etc/hosts"},
		{"read command", `{"command": "read myfile.txt"}`, "myfile.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			readSet := make(map[string]bool)
			modSet := make(map[string]bool)
			extractShellFiles(tt.argsJSON, readSet, modSet)
			if !readSet[tt.wantRead] {
				t.Errorf("expected %q in read set, got %v", tt.wantRead, readSet)
			}
		})
	}
}

func TestExtractShellFiles_WriteCommands(t *testing.T) {
	tests := []struct {
		name     string
		argsJSON string
		wantMod  string
	}{
		{"write command", `{"command": "write /tmp/out.txt"}`, "/tmp/out.txt"},
		{"edit command", `{"command": "edit /tmp/fix.go"}`, "/tmp/fix.go"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			readSet := make(map[string]bool)
			modSet := make(map[string]bool)
			extractShellFiles(tt.argsJSON, readSet, modSet)
			if !modSet[tt.wantMod] {
				t.Errorf("expected %q in modified set, got %v", tt.wantMod, modSet)
			}
		})
	}
}

func TestExtractShellFiles_NoCommandKey(t *testing.T) {
	readSet := make(map[string]bool)
	modSet := make(map[string]bool)
	extractShellFiles(`{"other": "value"}`, readSet, modSet)
	if len(readSet) != 0 || len(modSet) != 0 {
		t.Error("expected no files extracted when command key is absent")
	}
}

func TestExtractShellFiles_EmptyCommand(t *testing.T) {
	readSet := make(map[string]bool)
	modSet := make(map[string]bool)
	extractShellFiles(`{"command": ""}`, readSet, modSet)
	if len(readSet) != 0 || len(modSet) != 0 {
		t.Error("expected no files extracted from empty command")
	}
}

// ---------------------------------------------------------------------------
// maskObservations
// ---------------------------------------------------------------------------

func TestMaskObservations_MasksOldPreservesRecent(t *testing.T) {
	var messages []Message
	for i := 0; i < 8; i++ {
		messages = append(messages, Message{
			Parts: []Part{
				ToolResultPart("c"+string(rune('a'+i)), "read", strings.Repeat("x", 100), false),
			},
		})
	}
	masked := maskObservations(messages)

	// First 3 should be masked (8 - preserveRecentOutputs(5) = 3)
	for i := 0; i < 3; i++ {
		if masked[i].Parts[0].ToolResult != "[output masked for compaction]" {
			t.Errorf("message %d should be masked, got %q", i, masked[i].Parts[0].ToolResult)
		}
	}
	// Last 5 should be preserved
	for i := 3; i < 8; i++ {
		if masked[i].Parts[0].ToolResult == "[output masked for compaction]" {
			t.Errorf("message %d should be preserved, got masked", i)
		}
	}
}

func TestMaskObservations_PreservesToolErrorFlag(t *testing.T) {
	messages := []Message{
		{Parts: []Part{ToolResultPart("c1", "bash", "some error", true)}},
		// Add 5 more results so the first one is masked
		{Parts: []Part{ToolResultPart("c2", "read", "a", false)}},
		{Parts: []Part{ToolResultPart("c3", "read", "b", false)}},
		{Parts: []Part{ToolResultPart("c4", "read", "c", false)}},
		{Parts: []Part{ToolResultPart("c5", "read", "d", false)}},
		{Parts: []Part{ToolResultPart("c6", "read", "e", false)}},
	}
	masked := maskObservations(messages)
	// First result should be masked but ToolError preserved
	if !masked[0].Parts[0].ToolError {
		t.Error("expected ToolError to be preserved on masked result")
	}
	if masked[0].Parts[0].ToolResult != "[output masked for compaction]" {
		t.Errorf("expected masked output, got %q", masked[0].Parts[0].ToolResult)
	}
}

func TestMaskObservations_PreservesToolCallID(t *testing.T) {
	messages := []Message{
		{Parts: []Part{ToolResultPart("original-id", "read", "output", false)}},
		{Parts: []Part{ToolResultPart("c2", "read", "a", false)}},
		{Parts: []Part{ToolResultPart("c3", "read", "b", false)}},
		{Parts: []Part{ToolResultPart("c4", "read", "c", false)}},
		{Parts: []Part{ToolResultPart("c5", "read", "d", false)}},
		{Parts: []Part{ToolResultPart("c6", "read", "e", false)}},
	}
	masked := maskObservations(messages)
	if masked[0].Parts[0].ToolCallID != "original-id" {
		t.Errorf("expected ToolCallID preserved, got %q", masked[0].Parts[0].ToolCallID)
	}
}

func TestMaskObservations_DoesNotMutateOriginal(t *testing.T) {
	messages := []Message{
		{Parts: []Part{ToolResultPart("c1", "read", "original-output", false)}},
		{Parts: []Part{ToolResultPart("c2", "read", "a", false)}},
		{Parts: []Part{ToolResultPart("c3", "read", "b", false)}},
		{Parts: []Part{ToolResultPart("c4", "read", "c", false)}},
		{Parts: []Part{ToolResultPart("c5", "read", "d", false)}},
		{Parts: []Part{ToolResultPart("c6", "read", "e", false)}},
	}
	maskObservations(messages)
	if messages[0].Parts[0].ToolResult != "original-output" {
		t.Error("original message was mutated by maskObservations")
	}
}

func TestMaskObservations_FewResultsAllPreserved(t *testing.T) {
	messages := []Message{
		{Parts: []Part{ToolResultPart("c1", "read", "output1", false)}},
		{Parts: []Part{ToolResultPart("c2", "read", "output2", false)}},
	}
	masked := maskObservations(messages)
	for i, m := range masked {
		if m.Parts[0].ToolResult == "[output masked for compaction]" {
			t.Errorf("message %d should be preserved (within recent window), got masked", i)
		}
	}
}

func TestMaskObservations_PreservesNonToolResultParts(t *testing.T) {
	messages := []Message{
		{Parts: []Part{TextPart("some text")}},
		{Parts: []Part{ToolCallPart("c1", "read", `{"path":"a"}`)}},
	}
	masked := maskObservations(messages)
	if masked[0].Parts[0].Text != "some text" {
		t.Error("expected text part to be preserved")
	}
	if masked[1].Parts[0].ToolName != "read" {
		t.Error("expected tool call part to be preserved")
	}
}

// ---------------------------------------------------------------------------
// buildCompactionPrompt
// ---------------------------------------------------------------------------

func TestBuildCompactionPrompt_IncludesConversationTags(t *testing.T) {
	messages := []Message{
		{Role: RoleUser, Parts: []Part{TextPart("hello")}},
		{Role: RoleAssistant, Parts: []Part{TextPart("world")}},
	}
	prompt := buildCompactionPrompt(messages, "", nil, nil)
	if !strings.Contains(prompt, "<conversation>") {
		t.Error("missing <conversation> tag")
	}
	if !strings.Contains(prompt, "</conversation>") {
		t.Error("missing </conversation> tag")
	}
	if !strings.Contains(prompt, "<user>") {
		t.Error("missing <user> tag")
	}
	if !strings.Contains(prompt, "<assistant>") {
		t.Error("missing <assistant> tag")
	}
}

func TestBuildCompactionPrompt_IncludesToolCallFormatting(t *testing.T) {
	messages := []Message{
		{Role: RoleAssistant, Parts: []Part{
			ToolCallPart("c1", "read", `{"path":"a.go"}`),
		}},
	}
	prompt := buildCompactionPrompt(messages, "", nil, nil)
	if !strings.Contains(prompt, "[tool_call: read(") {
		t.Error("expected tool_call formatting in prompt")
	}
}

func TestBuildCompactionPrompt_IncludesToolResultFormatting(t *testing.T) {
	messages := []Message{
		{Role: RoleTool, Parts: []Part{
			ToolResultPart("c1", "read", "file contents", false),
		}},
	}
	prompt := buildCompactionPrompt(messages, "", nil, nil)
	if !strings.Contains(prompt, "[tool_result: read = ") {
		t.Error("expected tool_result formatting in prompt")
	}
}

func TestBuildCompactionPrompt_IncludesReasoningParts(t *testing.T) {
	messages := []Message{
		{Role: RoleAssistant, Parts: []Part{ReasoningPart("thinking about it")}},
	}
	prompt := buildCompactionPrompt(messages, "", nil, nil)
	if !strings.Contains(prompt, "[reasoning: thinking about it]") {
		t.Error("expected reasoning formatting in prompt")
	}
}

func TestBuildCompactionPrompt_NoPriorSummaryOmitsTag(t *testing.T) {
	messages := []Message{
		{Role: RoleUser, Parts: []Part{TextPart("test")}},
	}
	prompt := buildCompactionPrompt(messages, "", nil, nil)
	if strings.Contains(prompt, "<prior-summary>") {
		t.Error("expected no prior-summary tag when prior summary is empty")
	}
}

func TestBuildCompactionPrompt_NoFilesOmitsBlocks(t *testing.T) {
	messages := []Message{
		{Role: RoleUser, Parts: []Part{TextPart("test")}},
	}
	prompt := buildCompactionPrompt(messages, "", nil, nil)
	if strings.Contains(prompt, "<read-files>") {
		t.Error("expected no read-files block when no read files")
	}
	if strings.Contains(prompt, "<modified-files>") {
		t.Error("expected no modified-files block when no modified files")
	}
}

func TestBuildCompactionPrompt_TruncatesLongToolArgs(t *testing.T) {
	longArgs := strings.Repeat("a", 1000)
	messages := []Message{
		{Role: RoleAssistant, Parts: []Part{
			ToolCallPart("c1", "bash", longArgs),
		}},
	}
	prompt := buildCompactionPrompt(messages, "", nil, nil)
	if strings.Contains(prompt, longArgs) {
		t.Error("expected long tool args to be truncated")
	}
	if !strings.Contains(prompt, "... [truncated]") {
		t.Error("expected truncation marker for long tool args")
	}
}

func TestBuildCompactionPrompt_TruncatesLongToolOutput(t *testing.T) {
	longOutput := strings.Repeat("b", 3000)
	messages := []Message{
		{Role: RoleTool, Parts: []Part{
			ToolResultPart("c1", "read", longOutput, false),
		}},
	}
	prompt := buildCompactionPrompt(messages, "", nil, nil)
	if strings.Contains(prompt, longOutput) {
		t.Error("expected long tool output to be truncated")
	}
}

// ---------------------------------------------------------------------------
// truncate
// ---------------------------------------------------------------------------

func TestTruncate_ShortStringUnchanged(t *testing.T) {
	result := truncate("hello", 100)
	if result != "hello" {
		t.Errorf("expected %q, got %q", "hello", result)
	}
}

func TestTruncate_ExactLengthUnchanged(t *testing.T) {
	s := strings.Repeat("x", 50)
	result := truncate(s, 50)
	if result != s {
		t.Error("string at exact max length should not be truncated")
	}
}

func TestTruncate_LongStringTruncated(t *testing.T) {
	s := strings.Repeat("x", 200)
	result := truncate(s, 100)
	if !strings.HasSuffix(result, "... [truncated]") {
		t.Error("expected truncation marker")
	}
	// The prefix should be exactly maxLen chars
	prefix := result[:100]
	if prefix != strings.Repeat("x", 100) {
		t.Error("expected first 100 chars preserved")
	}
}

func TestTruncate_EmptyString(t *testing.T) {
	result := truncate("", 10)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

// ---------------------------------------------------------------------------
// LazyEstimator
// ---------------------------------------------------------------------------

func TestLazyEstimator_DefaultTokensPerChar(t *testing.T) {
	e := NewLazyEstimator(0) // should default to 0.25
	msg := &Message{Parts: []Part{TextPart(strings.Repeat("a", 400))}}
	tokens := e.EstimateMessage(msg)
	if tokens != 100 {
		t.Errorf("expected 100 tokens with default 0.25 ratio, got %d", tokens)
	}
}

func TestLazyEstimator_NegativeTokensPerChar(t *testing.T) {
	e := NewLazyEstimator(-1) // should default to 0.25
	msg := &Message{Parts: []Part{TextPart(strings.Repeat("a", 400))}}
	tokens := e.EstimateMessage(msg)
	if tokens != 100 {
		t.Errorf("expected 100 tokens with default 0.25 ratio, got %d", tokens)
	}
}

func TestLazyEstimator_EstimateMessage_ToolCallParts(t *testing.T) {
	e := NewLazyEstimator(0.25)
	msg := &Message{Parts: []Part{
		ToolCallPart("c1", "read", strings.Repeat("a", 396)), // name(4) + args(396) = 400 chars
	}}
	tokens := e.EstimateMessage(msg)
	if tokens != 100 {
		t.Errorf("expected 100 tokens for tool call, got %d", tokens)
	}
}

func TestLazyEstimator_EstimateMessage_ToolResultParts(t *testing.T) {
	e := NewLazyEstimator(0.25)
	msg := &Message{Parts: []Part{
		ToolResultPart("c1", "read", strings.Repeat("a", 400), false),
	}}
	tokens := e.EstimateMessage(msg)
	if tokens != 100 {
		t.Errorf("expected 100 tokens for tool result, got %d", tokens)
	}
}

func TestLazyEstimator_EstimateMessage_ReasoningParts(t *testing.T) {
	e := NewLazyEstimator(0.25)
	msg := &Message{Parts: []Part{ReasoningPart(strings.Repeat("a", 400))}}
	tokens := e.EstimateMessage(msg)
	if tokens != 100 {
		t.Errorf("expected 100 tokens for reasoning, got %d", tokens)
	}
}

func TestLazyEstimator_EstimateMessage_MultipleParts(t *testing.T) {
	e := NewLazyEstimator(0.25)
	msg := &Message{Parts: []Part{
		TextPart(strings.Repeat("a", 200)),  // 50 tokens
		TextPart(strings.Repeat("b", 200)),  // 50 tokens
	}}
	tokens := e.EstimateMessage(msg)
	if tokens != 100 {
		t.Errorf("expected 100 tokens for combined parts, got %d", tokens)
	}
}

func TestLazyEstimator_EstimateMessage_MinimumTokens(t *testing.T) {
	e := NewLazyEstimator(0.25)
	msg := &Message{Parts: []Part{TextPart("hi")}} // 2 chars * 0.25 = 0, but min is 4
	tokens := e.EstimateMessage(msg)
	if tokens != 4 {
		t.Errorf("expected minimum 4 tokens, got %d", tokens)
	}
}

func TestLazyEstimator_EstimateMessage_EmptyParts(t *testing.T) {
	e := NewLazyEstimator(0.25)
	msg := &Message{Parts: []Part{}}
	tokens := e.EstimateMessage(msg)
	if tokens != 4 {
		t.Errorf("expected minimum 4 tokens for empty parts, got %d", tokens)
	}
}

// ---------------------------------------------------------------------------
// FindPreserveBoundary
// ---------------------------------------------------------------------------

func TestFindPreserveBoundary_AllFitInBudget(t *testing.T) {
	e := NewLazyEstimator(0.25)
	// 2 messages of 100 tokens each = 200 tokens, well under MinPreserveRecentTokens (2000)
	messages := []Message{
		{Parts: []Part{TextPart(strings.Repeat("x", 400))}},
		{Parts: []Part{TextPart(strings.Repeat("x", 400))}},
	}
	boundary := e.FindPreserveBoundary(messages, 5000)
	if boundary != 0 {
		t.Errorf("expected boundary 0 (all fit), got %d", boundary)
	}
}

func TestFindPreserveBoundary_PartialPreservation(t *testing.T) {
	e := NewLazyEstimator(0.25)
	// 10 messages of 1000 tokens each = 10000 tokens total
	// Budget 5000, clamped to MinPreserve(2000)..MaxPreserve(15000) = 5000
	messages := make([]Message, 10)
	for i := range messages {
		messages[i] = Message{Parts: []Part{TextPart(strings.Repeat("x", 4000))}}
	}
	boundary := e.FindPreserveBoundary(messages, 5000)
	if boundary <= 0 {
		t.Error("expected boundary > 0 (not all messages preserved)")
	}
	if boundary >= len(messages) {
		t.Error("expected boundary within messages range")
	}
}

func TestFindPreserveBoundary_BudgetClampedToMin(t *testing.T) {
	e := NewLazyEstimator(0.25)
	// 3 messages of 400 tokens each = 1200. Even with budget=100,
	// it is clamped to MinPreserveRecentTokens=2000 so all 3 fit.
	messages := make([]Message, 3)
	for i := range messages {
		messages[i] = Message{Parts: []Part{TextPart(strings.Repeat("x", 1600))}}
	}
	boundary := e.FindPreserveBoundary(messages, 100)
	if boundary != 0 {
		t.Errorf("expected boundary 0 (budget clamped to 2000, all fit), got %d", boundary)
	}
}

func TestFindPreserveBoundary_BudgetClampedToMax(t *testing.T) {
	e := NewLazyEstimator(0.25)
	// Budget 50000 clamped to MaxPreserveRecentTokens=15000
	// 20 messages of 1000 tokens each = 20000 total > 15000
	messages := make([]Message, 20)
	for i := range messages {
		messages[i] = Message{Parts: []Part{TextPart(strings.Repeat("x", 4000))}}
	}
	boundary := e.FindPreserveBoundary(messages, 50000)
	if boundary <= 0 {
		t.Error("expected boundary > 0 (budget clamped, not all preserved)")
	}
}

func TestFindPreserveBoundary_EmptyMessages(t *testing.T) {
	e := NewLazyEstimator(0.25)
	boundary := e.FindPreserveBoundary(nil, 5000)
	if boundary != 0 {
		t.Errorf("expected boundary 0 for empty messages, got %d", boundary)
	}
}

// ---------------------------------------------------------------------------
// compactionOutputReserve
// ---------------------------------------------------------------------------

func TestCompactionOutputReserve_ZeroContext(t *testing.T) {
	result := compactionOutputReserve(0, 4096)
	if result != 1 {
		t.Errorf("expected 1 for zero context, got %d", result)
	}
}

func TestCompactionOutputReserve_OneContext(t *testing.T) {
	result := compactionOutputReserve(1, 4096)
	if result != 1 {
		t.Errorf("expected 1 for context=1, got %d", result)
	}
}

func TestCompactionOutputReserve_VerySmallContext(t *testing.T) {
	// contextLen=100: scaled=100/5=20, 20 < minCompactionReserve(2048),
	// so scaled=2048, but 2048 >= 100 so scaled=100/2=50.
	result := compactionOutputReserve(100, 50)
	if result != 50 {
		t.Errorf("expected 50 for context=100, got %d", result)
	}
}

func TestCompactionOutputReserve_OutputLimitLargerThanDefault(t *testing.T) {
	// contextLen=200000, outputLimit=30000: reserve = max(20000, 30000) = 30000
	// contextLen > reserve, so return reserve=30000
	result := compactionOutputReserve(200000, 30000)
	if result != 30000 {
		t.Errorf("expected 30000, got %d", result)
	}
}

// ---------------------------------------------------------------------------
// elideOldResults
// ---------------------------------------------------------------------------

func TestElideOldResults_SetsFlagOnce(t *testing.T) {
	p := &Processor{}
	p.messages = []Message{
		{Parts: []Part{ToolResultPart("c1", "read", "out", false)}},
	}
	p.elideOldResults()
	if !p.elisionDone {
		t.Error("expected elisionDone=true after elideOldResults")
	}
}

func TestElideOldResults_SecondCallNoop(t *testing.T) {
	p := &Processor{}
	p.messages = []Message{
		{Parts: []Part{ToolResultPart("c1", "read", "out", false)}},
	}
	p.elideOldResults()
	// Replace messages with new ones
	p.messages = []Message{
		{Parts: []Part{ToolResultPart("c2", "read", "new-out", false)}},
	}
	p.elideOldResults() // should be no-op
	if p.messages[0].Parts[0].ToolResult != "new-out" {
		t.Error("second elideOldResults call should be no-op")
	}
}

// ---------------------------------------------------------------------------
// Processor.Abort
// ---------------------------------------------------------------------------

func TestProcessor_Abort_SetsFlag(t *testing.T) {
	b := bus.New()
	defer b.Close()

	p := NewProcessor(ProcessorConfig{
		SessionID:  "ses_abort",
		Model:      &provider.Model{ID: "test-model"},
		Compaction: DefaultCompactionConfig(),
	}, &mockLLMClient{}, &stubToolExecutor{}, b)

	if p.isAborted() {
		t.Error("expected not aborted initially")
	}
	p.Abort()
	if !p.isAborted() {
		t.Error("expected aborted after Abort()")
	}
}

func TestProcessor_Abort_ResetOnProcess(t *testing.T) {
	client := &mockLLMClient{
		responses: []mockResponse{
			{events: []llm.Event{
				{Type: llm.EventTextDelta, Text: "hi"},
				{Type: llm.EventFinish, FinishReason: "stop", Usage: &llm.Usage{PromptTokens: 10, CompletionTokens: 2}},
			}},
		},
	}

	b := bus.New()
	defer b.Close()

	p := NewProcessor(ProcessorConfig{
		SessionID:  "ses_abort_reset",
		Model:      &provider.Model{ID: "test-model"},
		Compaction: DefaultCompactionConfig(),
	}, client, &stubToolExecutor{}, b)

	p.Abort()
	if !p.isAborted() {
		t.Fatal("expected aborted before Process")
	}

	result := p.Process(context.Background(), "hello")
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	// After Process starts, aborted should be reset to false
	if p.isAborted() {
		t.Error("expected aborted reset after Process start")
	}
}

// ---------------------------------------------------------------------------
// Processor.SetUserExtraParts
// ---------------------------------------------------------------------------

func TestSetUserExtraParts_IncludedInNextMessage(t *testing.T) {
	client := &mockLLMClient{
		responses: []mockResponse{
			{events: []llm.Event{
				{Type: llm.EventTextDelta, Text: "I see the image"},
				{Type: llm.EventFinish, FinishReason: "stop", Usage: &llm.Usage{PromptTokens: 10, CompletionTokens: 5}},
			}},
		},
	}

	b := bus.New()
	defer b.Close()

	p := NewProcessor(ProcessorConfig{
		SessionID:  "ses_extra",
		Model:      &provider.Model{ID: "test-model"},
		Compaction: DefaultCompactionConfig(),
	}, client, &stubToolExecutor{}, b)

	extraParts := []Part{ImagePart("base64data", "image/png")}
	p.SetUserExtraParts(extraParts)

	result := p.Process(context.Background(), "what is this?")
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}

	// The first message should be the user message with text + image parts
	userMsg := result.Messages[0]
	if userMsg.Role != RoleUser {
		t.Fatalf("expected first message to be user, got %s", userMsg.Role)
	}
	if len(userMsg.Parts) != 2 {
		t.Fatalf("expected 2 parts (text + image), got %d", len(userMsg.Parts))
	}
	if userMsg.Parts[0].Type != PartText {
		t.Errorf("expected first part to be text, got %s", userMsg.Parts[0].Type)
	}
	if userMsg.Parts[1].Type != PartImage {
		t.Errorf("expected second part to be image, got %s", userMsg.Parts[1].Type)
	}
	if userMsg.Parts[1].ImageData != "base64data" {
		t.Errorf("expected image data 'base64data', got %q", userMsg.Parts[1].ImageData)
	}
}

func TestSetUserExtraParts_ClearedAfterUse(t *testing.T) {
	client := &mockLLMClient{
		responses: []mockResponse{
			{events: []llm.Event{
				{Type: llm.EventTextDelta, Text: "first"},
				{Type: llm.EventFinish, FinishReason: "stop", Usage: &llm.Usage{PromptTokens: 10, CompletionTokens: 2}},
			}},
			{events: []llm.Event{
				{Type: llm.EventTextDelta, Text: "second"},
				{Type: llm.EventFinish, FinishReason: "stop", Usage: &llm.Usage{PromptTokens: 10, CompletionTokens: 2}},
			}},
		},
	}

	b := bus.New()
	defer b.Close()

	p := NewProcessor(ProcessorConfig{
		SessionID:  "ses_extra_clear",
		Model:      &provider.Model{ID: "test-model"},
		Compaction: DefaultCompactionConfig(),
	}, client, &stubToolExecutor{}, b)

	p.SetUserExtraParts([]Part{ImagePart("img", "image/png")})
	p.Process(context.Background(), "first")

	// Second process should NOT include the extra parts
	result := p.Process(context.Background(), "second")
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}

	// Find the second user message (messages 0,1 are from first turn; 2 is second user msg)
	secondUserMsg := result.Messages[2]
	if secondUserMsg.Role != RoleUser {
		t.Fatalf("expected message[2] to be user, got %s", secondUserMsg.Role)
	}
	if len(secondUserMsg.Parts) != 1 {
		t.Errorf("expected 1 part (text only) in second user message, got %d", len(secondUserMsg.Parts))
	}
}

func TestSetUserExtraParts_NilPartsNoEffect(t *testing.T) {
	client := &mockLLMClient{
		responses: []mockResponse{
			{events: []llm.Event{
				{Type: llm.EventTextDelta, Text: "ok"},
				{Type: llm.EventFinish, FinishReason: "stop", Usage: &llm.Usage{PromptTokens: 10, CompletionTokens: 2}},
			}},
		},
	}

	b := bus.New()
	defer b.Close()

	p := NewProcessor(ProcessorConfig{
		SessionID:  "ses_extra_nil",
		Model:      &provider.Model{ID: "test-model"},
		Compaction: DefaultCompactionConfig(),
	}, client, &stubToolExecutor{}, b)

	p.SetUserExtraParts(nil)

	result := p.Process(context.Background(), "hello")
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}

	userMsg := result.Messages[0]
	if len(userMsg.Parts) != 1 {
		t.Errorf("expected 1 part with nil extra parts, got %d", len(userMsg.Parts))
	}
}

// ---------------------------------------------------------------------------
// PartStore
// ---------------------------------------------------------------------------

func testPartDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	schema := `
		CREATE TABLE part (
			id TEXT PRIMARY KEY,
			message_id TEXT NOT NULL,
			session_id TEXT NOT NULL,
			time_created INTEGER NOT NULL,
			time_updated INTEGER NOT NULL,
			data TEXT NOT NULL
		);
		CREATE INDEX part_message_id_id_idx ON part(message_id, id);
		CREATE INDEX part_session_idx ON part(session_id);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("creating schema: %v", err)
	}
	return db
}

func TestNewPartStore_ReturnsNonNil(t *testing.T) {
	db := testPartDB(t)
	ps := NewPartStore(db)
	if ps == nil {
		t.Fatal("expected non-nil PartStore")
	}
}

func TestPartStore_SaveAndListByMessage(t *testing.T) {
	db := testPartDB(t)
	ps := NewPartStore(db)

	part := StoredPart{
		ID:        "part-1",
		MessageID: "msg-1",
		SessionID: "ses-1",
		Type:      "text",
		Text:      "hello world",
		Time:      PartTime{Start: 1000, End: 2000},
	}

	if err := ps.Save(part); err != nil {
		t.Fatalf("save: %v", err)
	}

	parts, err := ps.ListByMessage("msg-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(parts))
	}
	if parts[0].ID != "part-1" {
		t.Errorf("expected part ID 'part-1', got %q", parts[0].ID)
	}
	if parts[0].Text != "hello world" {
		t.Errorf("expected text 'hello world', got %q", parts[0].Text)
	}
	if parts[0].Type != "text" {
		t.Errorf("expected type 'text', got %q", parts[0].Type)
	}
	if parts[0].Time.Start != 1000 || parts[0].Time.End != 2000 {
		t.Errorf("expected time {1000, 2000}, got %+v", parts[0].Time)
	}
}

func TestPartStore_SaveMultiplePartsForMessage(t *testing.T) {
	db := testPartDB(t)
	ps := NewPartStore(db)

	for i := 0; i < 3; i++ {
		if err := ps.Save(StoredPart{
			ID:        "part-" + string(rune('a'+i)),
			MessageID: "msg-1",
			SessionID: "ses-1",
			Type:      "text",
			Text:      "text " + string(rune('a'+i)),
		}); err != nil {
			t.Fatalf("save part %d: %v", i, err)
		}
	}

	parts, err := ps.ListByMessage("msg-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts, got %d", len(parts))
	}
}

func TestPartStore_ListByMessage_ReturnsEmptyForNonexistent(t *testing.T) {
	db := testPartDB(t)
	ps := NewPartStore(db)

	parts, err := ps.ListByMessage("nonexistent-msg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parts != nil {
		t.Errorf("expected nil parts for nonexistent message, got %v", parts)
	}
}

func TestPartStore_ListByMessage_IsolatesByMessageID(t *testing.T) {
	db := testPartDB(t)
	ps := NewPartStore(db)

	ps.Save(StoredPart{ID: "p1", MessageID: "msg-1", SessionID: "ses-1", Type: "text", Text: "a"})
	ps.Save(StoredPart{ID: "p2", MessageID: "msg-2", SessionID: "ses-1", Type: "text", Text: "b"})

	parts, _ := ps.ListByMessage("msg-1")
	if len(parts) != 1 || parts[0].ID != "p1" {
		t.Errorf("expected only parts for msg-1, got %v", parts)
	}
}

func TestPartStore_DeleteByMessage_RemovesAllParts(t *testing.T) {
	db := testPartDB(t)
	ps := NewPartStore(db)

	ps.Save(StoredPart{ID: "p1", MessageID: "msg-1", SessionID: "ses-1", Type: "text", Text: "a"})
	ps.Save(StoredPart{ID: "p2", MessageID: "msg-1", SessionID: "ses-1", Type: "text", Text: "b"})
	ps.Save(StoredPart{ID: "p3", MessageID: "msg-2", SessionID: "ses-1", Type: "text", Text: "c"})

	if err := ps.DeleteByMessage("msg-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// Parts for msg-1 should be gone
	parts, _ := ps.ListByMessage("msg-1")
	if len(parts) != 0 {
		t.Errorf("expected 0 parts after delete, got %d", len(parts))
	}

	// Parts for msg-2 should remain
	parts2, _ := ps.ListByMessage("msg-2")
	if len(parts2) != 1 {
		t.Errorf("expected 1 part for msg-2 (unaffected), got %d", len(parts2))
	}
}

func TestPartStore_DeleteByMessage_NoErrorForNonexistent(t *testing.T) {
	db := testPartDB(t)
	ps := NewPartStore(db)

	err := ps.DeleteByMessage("nonexistent")
	if err != nil {
		t.Errorf("expected no error deleting nonexistent message parts, got %v", err)
	}
}

func TestPartStore_Save_UpsertOnDuplicateID(t *testing.T) {
	db := testPartDB(t)
	ps := NewPartStore(db)

	// Save initial
	ps.Save(StoredPart{ID: "p1", MessageID: "msg-1", SessionID: "ses-1", Type: "text", Text: "original"})

	// Save with same ID should upsert (INSERT OR REPLACE)
	ps.Save(StoredPart{ID: "p1", MessageID: "msg-1", SessionID: "ses-1", Type: "text", Text: "updated"})

	parts, _ := ps.ListByMessage("msg-1")
	if len(parts) != 1 {
		t.Fatalf("expected 1 part after upsert, got %d", len(parts))
	}
	if parts[0].Text != "updated" {
		t.Errorf("expected updated text, got %q", parts[0].Text)
	}
}

func TestPartStore_Save_PreservesPartTimeFields(t *testing.T) {
	db := testPartDB(t)
	ps := NewPartStore(db)

	ps.Save(StoredPart{
		ID:        "p1",
		MessageID: "msg-1",
		SessionID: "ses-1",
		Type:      "tool-call",
		Text:      "",
		Time:      PartTime{Start: 5000, End: 6000},
	})

	parts, _ := ps.ListByMessage("msg-1")
	if parts[0].Time.Start != 5000 {
		t.Errorf("expected Start=5000, got %d", parts[0].Time.Start)
	}
	if parts[0].Time.End != 6000 {
		t.Errorf("expected End=6000, got %d", parts[0].Time.End)
	}
}

// ---------------------------------------------------------------------------
// extractToolArgPath
// ---------------------------------------------------------------------------

func TestExtractToolArgPath_FilePathKey(t *testing.T) {
	got := extractToolArgPath(`{"file_path": "/src/main.go"}`)
	if got != "/src/main.go" {
		t.Errorf("expected /src/main.go, got %q", got)
	}
}

func TestExtractToolArgPath_PathKey(t *testing.T) {
	got := extractToolArgPath(`{"path": "/src/main.go"}`)
	if got != "/src/main.go" {
		t.Errorf("expected /src/main.go, got %q", got)
	}
}

func TestExtractToolArgPath_FileKey(t *testing.T) {
	got := extractToolArgPath(`{"file": "/src/main.go"}`)
	if got != "/src/main.go" {
		t.Errorf("expected /src/main.go, got %q", got)
	}
}

func TestExtractToolArgPath_NoMatchingKey(t *testing.T) {
	got := extractToolArgPath(`{"command": "ls"}`)
	if got != "" {
		t.Errorf("expected empty string for no matching key, got %q", got)
	}
}

func TestExtractToolArgPath_EmptyArgs(t *testing.T) {
	got := extractToolArgPath("")
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestExtractToolArgPath_PrefersFirstMatchingKey(t *testing.T) {
	// "file_path" is checked first
	got := extractToolArgPath(`{"file_path": "/first.go", "path": "/second.go"}`)
	if got != "/first.go" {
		t.Errorf("expected /first.go (file_path checked first), got %q", got)
	}
}

func TestAdjustPreserveForToolPairs_PullsBackToToolUse(t *testing.T) {
	messages := []Message{
		{Role: "user", Parts: []Part{{Type: "text", Text: "hello"}}},
		{Role: "assistant", Parts: []Part{
			{Type: PartToolCall, ToolCallID: "call_1", ToolName: "read"},
		}},
		{Role: "tool", Parts: []Part{
			{Type: PartToolResult, ToolCallID: "call_1", ToolName: "read", ToolResult: "file content"},
		}},
		{Role: "assistant", Parts: []Part{{Type: "text", Text: "done"}}},
	}
	// If preserveIdx=2 (tool_result message), it should pull back to 1 (tool_use)
	got := adjustPreserveForToolPairs(messages, 2)
	if got != 1 {
		t.Errorf("adjustPreserveForToolPairs(msgs, 2) = %d, want 1", got)
	}
}

func TestAdjustPreserveForToolPairs_NoAdjustWhenNoToolResult(t *testing.T) {
	messages := []Message{
		{Role: "user", Parts: []Part{{Type: "text", Text: "hello"}}},
		{Role: "assistant", Parts: []Part{{Type: "text", Text: "response"}}},
		{Role: "user", Parts: []Part{{Type: "text", Text: "follow up"}}},
	}
	got := adjustPreserveForToolPairs(messages, 2)
	if got != 2 {
		t.Errorf("adjustPreserveForToolPairs(msgs, 2) = %d, want 2 (no tool_result)", got)
	}
}

func TestAdjustPreserveForToolPairs_BoundsCheck(t *testing.T) {
	messages := []Message{
		{Role: "user", Parts: []Part{{Type: "text", Text: "hello"}}},
	}
	if got := adjustPreserveForToolPairs(messages, 0); got != 0 {
		t.Errorf("idx=0 should return 0, got %d", got)
	}
	if got := adjustPreserveForToolPairs(messages, 5); got != 5 {
		t.Errorf("idx out of range should return as-is, got %d", got)
	}
	if got := adjustPreserveForToolPairs(nil, 1); got != 1 {
		t.Errorf("nil messages should return idx, got %d", got)
	}
}

func TestAdjustPreserveForToolPairs_MultipleConsecutivePairs(t *testing.T) {
	// Two consecutive tool_use/tool_result pairs:
	//   0: user
	//   1: assistant (tool_call A, tool_call B)
	//   2: tool (tool_result A, tool_result B)
	//   3: assistant (tool_call C)
	//   4: tool (tool_result C)
	//   5: assistant (text "done")
	messages := []Message{
		{Role: RoleUser, Parts: []Part{TextPart("hello")}},
		{Role: RoleAssistant, Parts: []Part{
			ToolCallPart("call_a", "read", `{"path":"a.go"}`),
			ToolCallPart("call_b", "write", `{"path":"b.go"}`),
		}},
		{Role: RoleTool, Parts: []Part{
			ToolResultPart("call_a", "read", "content a", false),
			ToolResultPart("call_b", "write", "ok", false),
		}},
		{Role: RoleAssistant, Parts: []Part{
			ToolCallPart("call_c", "bash", `{"command":"ls"}`),
		}},
		{Role: RoleTool, Parts: []Part{
			ToolResultPart("call_c", "bash", "file list", false),
		}},
		{Role: RoleAssistant, Parts: []Part{TextPart("done")}},
	}

	// Split between pairs (idx=3, assistant with tool_call C) should stay.
	got := adjustPreserveForToolPairs(messages, 3)
	if got != 3 {
		t.Errorf("split at boundary between pairs: got %d, want 3", got)
	}

	// Split on second tool_result (idx=4) should pull back to idx=3 (its tool_use).
	got = adjustPreserveForToolPairs(messages, 4)
	if got != 3 {
		t.Errorf("split on second tool_result: got %d, want 3", got)
	}

	// Split on first tool_result (idx=2) should pull back to idx=1 (its tool_use).
	got = adjustPreserveForToolPairs(messages, 2)
	if got != 1 {
		t.Errorf("split on first tool_result: got %d, want 1", got)
	}
}

func TestAdjustPreserveForToolPairs_SplitOnToolResultInMiddleOfMultiplePairs(t *testing.T) {
	// Three tool pairs in sequence. Split lands on the middle tool_result.
	messages := []Message{
		{Role: RoleUser, Parts: []Part{TextPart("start")}},
		{Role: RoleAssistant, Parts: []Part{ToolCallPart("c1", "read", `{}`)}},
		{Role: RoleTool, Parts: []Part{ToolResultPart("c1", "read", "r1", false)}},
		{Role: RoleAssistant, Parts: []Part{ToolCallPart("c2", "edit", `{}`)}},
		{Role: RoleTool, Parts: []Part{ToolResultPart("c2", "edit", "r2", false)}},
		{Role: RoleAssistant, Parts: []Part{ToolCallPart("c3", "bash", `{}`)}},
		{Role: RoleTool, Parts: []Part{ToolResultPart("c3", "bash", "r3", false)}},
		{Role: RoleAssistant, Parts: []Part{TextPart("all done")}},
	}

	// Split at idx=4 (middle tool_result) should pull back to idx=3.
	got := adjustPreserveForToolPairs(messages, 4)
	if got != 3 {
		t.Errorf("split on middle tool_result: got %d, want 3", got)
	}
}

func TestAdjustPreserveForToolPairs_MessageWithTextAndToolResult(t *testing.T) {
	// A message that has both text and tool_result parts should trigger adjustment.
	messages := []Message{
		{Role: RoleUser, Parts: []Part{TextPart("hello")}},
		{Role: RoleAssistant, Parts: []Part{ToolCallPart("c1", "read", `{}`)}},
		{Role: RoleTool, Parts: []Part{
			TextPart("some context"),
			ToolResultPart("c1", "read", "file content", false),
		}},
		{Role: RoleAssistant, Parts: []Part{TextPart("done")}},
	}

	got := adjustPreserveForToolPairs(messages, 2)
	if got != 1 {
		t.Errorf("message with text+tool_result: got %d, want 1", got)
	}
}

func TestAdjustPreserveForToolPairs_NoAssistantBeforeToolResult(t *testing.T) {
	// Edge case: tool_result at idx but no assistant with tool_call before it.
	// Should return original idx.
	messages := []Message{
		{Role: RoleUser, Parts: []Part{TextPart("hello")}},
		{Role: RoleTool, Parts: []Part{
			ToolResultPart("c1", "read", "orphaned result", false),
		}},
		{Role: RoleAssistant, Parts: []Part{TextPart("ok")}},
	}

	got := adjustPreserveForToolPairs(messages, 1)
	if got != 1 {
		t.Errorf("no assistant with tool_call before tool_result: got %d, want 1", got)
	}
}

func TestAdjustPreserveForToolPairs_AssistantWithoutToolCall(t *testing.T) {
	// Assistant exists before tool_result but has no tool_call parts (only text).
	// Should return original idx since the walk-back finds an assistant without tool_call.
	messages := []Message{
		{Role: RoleUser, Parts: []Part{TextPart("hello")}},
		{Role: RoleAssistant, Parts: []Part{TextPart("thinking...")}},
		{Role: RoleTool, Parts: []Part{
			ToolResultPart("c1", "read", "orphaned result", false),
		}},
		{Role: RoleAssistant, Parts: []Part{TextPart("done")}},
	}

	got := adjustPreserveForToolPairs(messages, 2)
	if got != 2 {
		t.Errorf("assistant without tool_call before tool_result: got %d, want 2", got)
	}
}

// ---------------------------------------------------------------------------
// maskObservations edge cases
// ---------------------------------------------------------------------------

func TestMaskObservations_AllPartsAreToolResults(t *testing.T) {
	// Messages where every part in a message is a tool_result (no text parts).
	// All 7 messages have only tool_result parts. Last 5 preserved, first 2 masked.
	var messages []Message
	for i := 0; i < 7; i++ {
		messages = append(messages, Message{
			Parts: []Part{
				ToolResultPart("c"+string(rune('a'+i)), "read", "output-"+string(rune('a'+i)), false),
			},
		})
	}
	masked := maskObservations(messages)

	// First 2 should be masked (7 - 5 = 2)
	for i := 0; i < 2; i++ {
		if masked[i].Parts[0].ToolResult != "[output masked for compaction]" {
			t.Errorf("message %d should be masked, got %q", i, masked[i].Parts[0].ToolResult)
		}
	}
	// Last 5 should be preserved
	for i := 2; i < 7; i++ {
		if masked[i].Parts[0].ToolResult == "[output masked for compaction]" {
			t.Errorf("message %d should be preserved, got masked", i)
		}
	}
}

func TestMaskObservations_ToolResultWithEmptyOutput(t *testing.T) {
	// A tool_result with an empty ToolResult string should still be masked
	// when it falls outside the preserve window.
	messages := []Message{
		{Parts: []Part{ToolResultPart("c1", "bash", "", false)}},
	}
	// Add 5 more to push the first one out of the preserve window.
	for i := 2; i <= 6; i++ {
		messages = append(messages, Message{
			Parts: []Part{ToolResultPart("c"+string(rune('0'+i)), "read", "output", false)},
		})
	}
	masked := maskObservations(messages)

	if masked[0].Parts[0].ToolResult != "[output masked for compaction]" {
		t.Errorf("empty tool_result should be masked, got %q", masked[0].Parts[0].ToolResult)
	}
	// Verify the ToolCallID and ToolName are preserved on the masked result.
	if masked[0].Parts[0].ToolCallID != "c1" {
		t.Errorf("ToolCallID should be preserved, got %q", masked[0].Parts[0].ToolCallID)
	}
	if masked[0].Parts[0].ToolName != "bash" {
		t.Errorf("ToolName should be preserved, got %q", masked[0].Parts[0].ToolName)
	}
}

func TestMaskObservations_MixedToolCallAndToolResultInSameMessage(t *testing.T) {
	// Defensive test: a message with both tool_call and tool_result parts.
	// Only tool_result parts should be affected by masking.
	messages := []Message{
		{Parts: []Part{
			ToolCallPart("c1", "read", `{"path":"a.go"}`),
			ToolResultPart("c0", "write", "write ok", false),
		}},
	}
	// Add 5 more tool_result messages to push the first out of window.
	for i := 2; i <= 6; i++ {
		messages = append(messages, Message{
			Parts: []Part{ToolResultPart("c"+string(rune('0'+i)), "read", "output", false)},
		})
	}
	masked := maskObservations(messages)

	// The tool_call part should be unchanged.
	if masked[0].Parts[0].Type != PartToolCall {
		t.Errorf("tool_call part type should be preserved, got %s", masked[0].Parts[0].Type)
	}
	if masked[0].Parts[0].ToolName != "read" {
		t.Errorf("tool_call ToolName should be preserved, got %q", masked[0].Parts[0].ToolName)
	}
	if masked[0].Parts[0].ToolArgs != `{"path":"a.go"}` {
		t.Errorf("tool_call ToolArgs should be preserved, got %q", masked[0].Parts[0].ToolArgs)
	}
	// The tool_result part should be masked.
	if masked[0].Parts[1].ToolResult != "[output masked for compaction]" {
		t.Errorf("tool_result in mixed message should be masked, got %q", masked[0].Parts[1].ToolResult)
	}
}

func TestAdjustPreserveForToolPairs_ToolUseInLastCompacted(t *testing.T) {
	// Case 2: last compacted message has tool_use, next message has tool_result.
	// Split at idx=2 would orphan the tool_use from its results.
	messages := []Message{
		{Role: "user", Parts: []Part{{Type: "text", Text: "hello"}}},
		{Role: "assistant", Parts: []Part{
			{Type: PartToolCall, ToolCallID: "call_1", ToolName: "read"},
			{Type: PartToolCall, ToolCallID: "call_2", ToolName: "read"},
			{Type: PartToolCall, ToolCallID: "call_3", ToolName: "read"},
		}},
		{Role: "tool", Parts: []Part{
			{Type: PartToolResult, ToolCallID: "call_1", ToolName: "read", ToolResult: "file1"},
			{Type: PartToolResult, ToolCallID: "call_2", ToolName: "read", ToolResult: "file2"},
			{Type: PartToolResult, ToolCallID: "call_3", ToolName: "read", ToolResult: "file3"},
		}},
		{Role: "assistant", Parts: []Part{{Type: "text", Text: "done"}}},
	}
	// If preserveIdx=2 (tool_result message), case 1 pulls back to 1 (tool_use).
	got := adjustPreserveForToolPairs(messages, 2)
	if got != 1 {
		t.Errorf("adjustPreserveForToolPairs(msgs, 2) = %d, want 1", got)
	}
	// If preserveIdx=2 where idx-1 is the tool_use message, case 2 should pull back.
	// This is the same result as case 1 in this layout, confirming both paths work.
}

func TestAdjustPreserveForToolPairs_Case2_ToolUseAtBoundary(t *testing.T) {
	// idx-1 is an assistant with tool_use, idx is the tool_result.
	// Case 2 should pull back to include the tool_use.
	messages := []Message{
		{Role: "user", Parts: []Part{{Type: "text", Text: "setup"}}},
		{Role: "assistant", Parts: []Part{{Type: "text", Text: "analysis"}}},
		{Role: "user", Parts: []Part{{Type: "text", Text: "re-read"}}},
		{Role: "assistant", Parts: []Part{
			{Type: PartToolCall, ToolCallID: "call_x", ToolName: "read"},
		}},
		{Role: "tool", Parts: []Part{
			{Type: PartToolResult, ToolCallID: "call_x", ToolName: "read", ToolResult: "content"},
		}},
		{Role: "assistant", Parts: []Part{{Type: "text", Text: "here is what I found"}}},
	}
	// preserveIdx=4 means we compact 0-3 and preserve 4-5.
	// But message 3 has tool_use and message 4 has its result.
	// Case 2: idx-1 (message 3) has tool_call → pull back to 3.
	got := adjustPreserveForToolPairs(messages, 4)
	if got != 3 {
		t.Errorf("adjustPreserveForToolPairs(msgs, 4) = %d, want 3 (preserve tool_use with its result)", got)
	}
}

// ---------------------------------------------------------------------------
// vLLM compaction: thinking disabled, retry on loop/budget
// ---------------------------------------------------------------------------

// recordingClient captures the llm.Request for each Stream call and plays
// back the queued response.
type recordingClient struct {
	requests  []llm.Request
	responses []mockResponse
}

func (c *recordingClient) Stream(ctx context.Context, req llm.Request, opts ...llm.StreamOption) (<-chan llm.Event, error) {
	c.requests = append(c.requests, req)
	if len(c.responses) == 0 {
		return nil, errors.New("no more mock responses")
	}
	resp := c.responses[0]
	c.responses = c.responses[1:]
	if resp.err != nil {
		return nil, resp.err
	}
	ch := make(chan llm.Event, len(resp.events))
	for _, e := range resp.events {
		ch <- e
	}
	close(ch)
	return ch, nil
}

// compactableHistory returns a history that compact(force=true) will
// summarize (everything except the last message).
func compactableHistory() []Message {
	return []Message{
		{Role: RoleUser, Parts: []Part{TextPart("do something")}},
		{Role: RoleAssistant, Parts: []Part{ToolCallPart("c1", "read", `{"path":"x.go"}`)}},
		{Role: RoleTool, Parts: []Part{ToolResultPart("c1", "read", "content of x.go", false)}},
		{Role: RoleAssistant, Parts: []Part{TextPart("Here is the result.")}},
	}
}

func summaryEvents(text string) []llm.Event {
	return []llm.Event{
		{Type: llm.EventTextDelta, Text: text},
		{Type: llm.EventFinish, FinishReason: "stop", Usage: &llm.Usage{PromptTokens: 100, CompletionTokens: 50}},
	}
}

func TestCompact_vLLMDisablesThinkingOnSummaryRequest(t *testing.T) {
	vllmClient := &recordingClient{responses: []mockResponse{{events: summaryEvents("Summary with thinking off.")}}}
	otherClient := &recordingClient{responses: []mockResponse{{events: summaryEvents("Summary.")}}}

	p := NewProcessor(ProcessorConfig{
		SessionID:  "ses_vllm_kwargs",
		Model:      &provider.Model{ID: "qwen3.8-27b-w3", ProviderID: "vllm"},
		Compaction: DefaultCompactionConfig(),
	}, vllmClient, &stubToolExecutor{}, bus.New())
	p.SetMessages(compactableHistory())

	compacted, err := p.compact(context.Background(), true)
	if err != nil || !compacted {
		t.Fatalf("compact() = (%v, %v), want (true, nil)", compacted, err)
	}
	if len(vllmClient.requests) != 1 {
		t.Fatalf("expected 1 summary request, got %d", len(vllmClient.requests))
	}
	want := map[string]any{"enable_thinking": false}
	if !reflect.DeepEqual(vllmClient.requests[0].ChatTemplateKwargs, want) {
		t.Errorf("ChatTemplateKwargs = %v, want %v", vllmClient.requests[0].ChatTemplateKwargs, want)
	}
	if !strings.Contains(p.Messages()[0].Parts[0].Text, "Summary with thinking off") {
		t.Errorf("first message = %q, want the summary", p.Messages()[0].Parts[0].Text)
	}

	p2 := NewProcessor(ProcessorConfig{
		SessionID:  "ses_other_kwargs",
		Model:      &provider.Model{ID: "gpt-4o", ProviderID: "openrouter"},
		Compaction: DefaultCompactionConfig(),
	}, otherClient, &stubToolExecutor{}, bus.New())
	p2.SetMessages(compactableHistory())

	compacted, err = p2.compact(context.Background(), true)
	if err != nil || !compacted {
		t.Fatalf("compact() = (%v, %v), want (true, nil)", compacted, err)
	}
	if otherClient.requests[0].ChatTemplateKwargs != nil {
		t.Errorf("ChatTemplateKwargs = %v, want nil for non-vLLM models", otherClient.requests[0].ChatTemplateKwargs)
	}
}

func TestCompact_RetriesWithThinkingDisabledAfterReasoningLoop(t *testing.T) {
	client := &recordingClient{
		responses: []mockResponse{
			{events: []llm.Event{{Type: llm.EventError, Error: llm.ErrReasoningLoop}}},
			{events: summaryEvents("Second summary.")},
		},
	}
	p := NewProcessor(ProcessorConfig{
		SessionID:  "ses_retry_loop",
		Model:      &provider.Model{ID: "qwen3.8-27b-w3", ProviderID: "vllm"},
		Compaction: DefaultCompactionConfig(),
	}, client, &stubToolExecutor{}, bus.New())
	p.SetMessages(compactableHistory())

	compacted, err := p.compact(context.Background(), true)
	if err != nil || !compacted {
		t.Fatalf("compact() = (%v, %v), want (true, nil) after retry", compacted, err)
	}
	if len(client.requests) != 2 {
		t.Fatalf("expected 2 summary requests, got %d", len(client.requests))
	}
	want := map[string]any{"enable_thinking": false}
	if !reflect.DeepEqual(client.requests[1].ChatTemplateKwargs, want) {
		t.Errorf("retry ChatTemplateKwargs = %v, want %v", client.requests[1].ChatTemplateKwargs, want)
	}
	if !strings.Contains(p.Messages()[0].Parts[0].Text, "Second summary") {
		t.Errorf("first message = %q, want the retry summary", p.Messages()[0].Parts[0].Text)
	}
}

func TestCompact_RetriesWithThinkingDisabledAfterStreamBudget(t *testing.T) {
	client := &recordingClient{
		responses: []mockResponse{
			{events: []llm.Event{{Type: llm.EventError, Error: llm.ErrStreamBudget}}},
			{events: summaryEvents("Budget retry summary.")},
		},
	}
	p := NewProcessor(ProcessorConfig{
		SessionID:  "ses_retry_budget",
		Model:      &provider.Model{ID: "qwen3.8-27b-w3", ProviderID: "vllm"},
		Compaction: DefaultCompactionConfig(),
	}, client, &stubToolExecutor{}, bus.New())
	p.SetMessages(compactableHistory())

	compacted, err := p.compact(context.Background(), true)
	if err != nil || !compacted {
		t.Fatalf("compact() = (%v, %v), want (true, nil) after retry", compacted, err)
	}
	if len(client.requests) != 2 {
		t.Fatalf("expected 2 summary requests, got %d", len(client.requests))
	}
	if !strings.Contains(p.Messages()[0].Parts[0].Text, "Budget retry summary") {
		t.Errorf("first message = %q, want the retry summary", p.Messages()[0].Parts[0].Text)
	}
}

func TestCompact_NoRetryAfterReasoningLoopOnNonVLLM(t *testing.T) {
	client := &recordingClient{
		responses: []mockResponse{
			{events: []llm.Event{{Type: llm.EventError, Error: llm.ErrReasoningLoop}}},
			{events: summaryEvents("Should not be used.")},
		},
	}
	p := NewProcessor(ProcessorConfig{
		SessionID:  "ses_no_retry",
		Model:      &provider.Model{ID: "gpt-4o", ProviderID: "openrouter"},
		Compaction: DefaultCompactionConfig(),
	}, client, &stubToolExecutor{}, bus.New())
	p.SetMessages(compactableHistory())

	compacted, err := p.compact(context.Background(), true)
	if compacted || err == nil {
		t.Fatalf("compact() = (%v, %v), want (false, loop error)", compacted, err)
	}
	if !errors.Is(err, llm.ErrReasoningLoop) {
		t.Errorf("err = %v, want ErrReasoningLoop", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("expected exactly 1 summary request, got %d", len(client.requests))
	}
}

// ---------------------------------------------------------------------------
// end-to-end: real OpenAI client + readSSE + ReasoningLoop + compact()
// against a mock HTTP vLLM. Proves the fix through the actual production path:
// the summary request carries chat_template_kwargs on the wire, and a
// reasoning-loop stop on the first attempt is retried once and succeeds.
// ---------------------------------------------------------------------------

// mockVLLM returns an SSE chat-completions response. If kwargs is non-nil it
// requires the request to carry that exact chat_template_kwargs (else 400).
// If loopOnce is set, the first call streams reasoning that trips the loop
// detector; the second streams a clean summary.
func mockVLLM(t *testing.T, kwargs map[string]any, loopOnce bool) (*httptest.Server, *int) {
	t.Helper()
	calls := 0
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"data":[{"id":"test-model"}]}`)
	})
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ChatTemplateKwargs map[string]any `json:"chat_template_kwargs"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decoding request: %v", err)
		}
		if kwargs != nil && !reflect.DeepEqual(body.ChatTemplateKwargs, kwargs) {
			http.Error(w, `{"error":"chat_template_kwargs mismatch"}`, http.StatusBadRequest)
			return
		}
		calls++
		w.Header().Set("Content-Type", "text/event-stream")
		if loopOnce && calls == 1 {
			// 12 identical "but wait" sentences (≥10) trip the loop detector.
			for i := 0; i < 12; i++ {
				fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"But wait, let me reconsider the boundary condition on the loop index. \"}}]}\n\n")
			}
			return
		}
		fmt.Fprint(w, `data: {"choices":[{"delta":{"content":"End-to-end summary."}}]}` + "\n\n")
		fmt.Fprint(w, `data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestCompact_EndToEnd_ReasoningLoopRetryAgainstMockVLLM(t *testing.T) {
	want := map[string]any{"enable_thinking": false}
	srv, calls := mockVLLM(t, want, true)

	client := llm.NewOpenAIClient(srv.URL+"/v1", "key")
	p := NewProcessor(ProcessorConfig{
		SessionID:  "ses_e2e_vllm",
		Model:      &provider.Model{ID: "test-model", ProviderID: "vllm"},
		Compaction: DefaultCompactionConfig(),
	}, client, &stubToolExecutor{}, bus.New())
	p.SetMessages(compactableHistory())

	compacted, err := p.compact(context.Background(), true)
	if err != nil || !compacted {
		t.Fatalf("compact() = (%v, %v), want (true, nil) after one retry", compacted, err)
	}
	if *calls != 2 {
		t.Fatalf("expected 2 summary calls (loop + retry), got %d", *calls)
	}
	if !strings.Contains(p.Messages()[0].Parts[0].Text, "End-to-end summary") {
		t.Errorf("summary = %q, want the retry summary", p.Messages()[0].Parts[0].Text)
	}
}

func TestCompact_EndToEnd_NoLoopNoRetryAgainstMockVLLM(t *testing.T) {
	want := map[string]any{"enable_thinking": false}
	srv, calls := mockVLLM(t, want, false)

	client := llm.NewOpenAIClient(srv.URL+"/v1", "key")
	p := NewProcessor(ProcessorConfig{
		SessionID:  "ses_e2e_clean",
		Model:      &provider.Model{ID: "test-model", ProviderID: "vllm"},
		Compaction: DefaultCompactionConfig(),
	}, client, &stubToolExecutor{}, bus.New())
	p.SetMessages(compactableHistory())

	compacted, err := p.compact(context.Background(), true)
	if err != nil || !compacted {
		t.Fatalf("compact() = (%v, %v), want (true, nil)", compacted, err)
	}
	if *calls != 1 {
		t.Fatalf("expected exactly 1 summary call (no loop), got %d", *calls)
	}
}
