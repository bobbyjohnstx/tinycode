package session

import (
	"context"
	"database/sql"
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
