package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/bobbyjohnstx/tinycode-go/internal/bus"
	"github.com/bobbyjohnstx/tinycode-go/internal/id"
	"github.com/bobbyjohnstx/tinycode-go/internal/llm"
	"github.com/bobbyjohnstx/tinycode-go/internal/permission"
	"github.com/bobbyjohnstx/tinycode-go/internal/provider"
)

const (
	maxIterations                   = 200
	maxConsecutiveToolFailures      = 10
	consecutiveToolFailureWarnEvery = 3
	defaultDoomThreshold            = 3
	defaultAutoContinueMax          = 0
)

type ToolExecutor interface {
	Execute(ctx context.Context, name string, args json.RawMessage, sessionID string) (string, bool, error)
	ToolDefs(agentPerms []string) []llm.Tool
}

type ProcessorConfig struct {
	SessionID        string
	Agent            string
	Model            *provider.Model
	SubagentDepth    int
	MaxSubagents     int
	SystemPrompt     string
	Compaction       CompactionConfig
	AgentPerms       []string
	DoomThreshold    int
	AutoContinueMax  int
	Directory        string
	CompactionModel  string
	SmallModel       string
	Temperature      *float64
	TopP             *float64
	MaxTokens        *int
	Perms            *permission.Service
	Ruleset          permission.Ruleset
}

// toolCallSignature captures the identity of a tool call for doom-loop detection.
type toolCallSignature struct {
	Name string
	Args string
}

type Processor struct {
	config           ProcessorConfig
	client           llm.Client
	tools            ToolExecutor
	bus              *bus.Bus
	messages         []Message
	priorSummary     string
	compactionCount  int
	aborted          bool
	recentToolCalls  []toolCallSignature
	autoContinueCount int
	mu               sync.Mutex
}

func NewProcessor(config ProcessorConfig, client llm.Client, tools ToolExecutor, eventBus *bus.Bus) *Processor {
	return &Processor{
		config: config,
		client: client,
		tools:  tools,
		bus:    eventBus,
	}
}

func (p *Processor) SetMessages(messages []Message) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.messages = messages
}

func (p *Processor) Messages() []Message {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]Message(nil), p.messages...)
}

func (p *Processor) Abort() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.aborted = true
}

func (p *Processor) isAborted() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.aborted
}

func (p *Processor) doomThreshold() int {
	if p.config.DoomThreshold > 0 {
		return p.config.DoomThreshold
	}
	return defaultDoomThreshold
}

func (p *Processor) autoContinueLimit() int {
	if p.config.AutoContinueMax < 0 {
		return 0 // explicitly disabled
	}
	if p.config.AutoContinueMax > 0 {
		return p.config.AutoContinueMax
	}
	return defaultAutoContinueMax
}

// isDoomLoop checks if the last N tool call signatures are all identical.
func isDoomLoop(recent []toolCallSignature, threshold int) bool {
	if len(recent) < threshold {
		return false
	}
	tail := recent[len(recent)-threshold:]
	first := tail[0]
	for _, sig := range tail[1:] {
		if sig.Name != first.Name || sig.Args != first.Args {
			return false
		}
	}
	return true
}

type ProcessResult struct {
	Messages []Message
	Usage    TokenUsage
	Error    error
	Aborted  bool
}

func (p *Processor) Process(ctx context.Context, userMessage string) *ProcessResult {
	p.mu.Lock()
	p.aborted = false
	p.autoContinueCount = 0
	p.mu.Unlock()

	userMsgID, _ := id.Ascending("message")
	userMsg := Message{
		ID:        userMsgID,
		SessionID: p.config.SessionID,
		Role:      RoleUser,
		Parts:     []Part{TextPart(userMessage)},
		CreatedAt: time.Now(),
	}

	p.mu.Lock()
	p.messages = append(p.messages, userMsg)
	p.mu.Unlock()

	p.bus.Publish("session.message", map[string]any{
		"sessionID": p.config.SessionID,
		"message":   userMsg,
	})

	totalUsage := TokenUsage{}
	var consecutiveToolFailures int
	var hadPriorToolCalls bool
	iteration := 0

	for {
		iteration++
		if iteration > maxIterations {
			return &ProcessResult{
				Messages: p.Messages(),
				Usage:    totalUsage,
				Error:    fmt.Errorf("processor exceeded %d iterations", maxIterations),
			}
		}
		slog.Info("processor loop iteration", "sessionID", p.config.SessionID, "iteration", iteration, "messageCount", len(p.Messages()))

		if p.isAborted() {
			slog.Info("processor aborted", "sessionID", p.config.SessionID, "iteration", iteration)
			return &ProcessResult{
				Messages: p.Messages(),
				Usage:    totalUsage,
				Aborted:  true,
			}
		}

		select {
		case <-ctx.Done():
			slog.Info("processor context cancelled", "sessionID", p.config.SessionID, "iteration", iteration)
			return &ProcessResult{
				Messages: p.Messages(),
				Usage:    totalUsage,
				Error:    ctx.Err(),
			}
		default:
		}

		stepID, _ := id.Ascending("step")
		p.bus.Publish("session.step.start", map[string]any{
			"sessionID": p.config.SessionID,
			"stepID":    stepID,
			"iteration": iteration,
			"model":     p.config.Model.ID,
		})

		assistantMsg, usage, err := p.callLLM(ctx)

		stepUsage := map[string]any{}
		if usage != nil {
			stepUsage = map[string]any{"input": usage.Input, "output": usage.Output}
		}
		var stepErr string
		if err != nil {
			stepErr = err.Error()
		}
		p.bus.Publish("session.step.finish", map[string]any{
			"sessionID": p.config.SessionID,
			"stepID":    stepID,
			"iteration": iteration,
			"usage":     stepUsage,
			"error":     stepErr,
		})

		if err != nil {
			slog.Error("callLLM failed", "sessionID", p.config.SessionID, "iteration", iteration, "error", err)
			if provider.IsOverflow(err.Error()) {
				compacted, compactErr := p.compact(ctx)
				if compactErr != nil {
					return &ProcessResult{
						Messages: p.Messages(),
						Usage:    totalUsage,
						Error:    fmt.Errorf("compaction failed after overflow: %w", compactErr),
					}
				}
				if compacted {
					continue
				}
			}
			return &ProcessResult{
				Messages: p.Messages(),
				Usage:    totalUsage,
				Error:    err,
			}
		}

		totalUsage.Input += usage.Input
		totalUsage.Output += usage.Output
		totalUsage.Reasoning += usage.Reasoning
		totalUsage.Cache.Read += usage.Cache.Read
		totalUsage.Cache.Write += usage.Cache.Write

		// Proactive overflow detection: compact before hitting the hard limit.
		// Skip when the model doesn't report token usage (totalUsage.Input == 0)
		// to avoid spurious compaction with local models.
		if p.config.Model != nil && p.config.Model.Limit.Context > 0 && totalUsage.Input > 0 {
			outputReserve := max(p.config.Model.Limit.Output, 20000)
			threshold := p.config.Model.Limit.Context - outputReserve
			if threshold > 0 && totalUsage.Input >= threshold {
				slog.Info("proactive compaction triggered", "sessionID", p.config.SessionID, "inputTokens", totalUsage.Input, "threshold", threshold)
				if _, compactErr := p.compact(ctx); compactErr != nil {
					slog.Warn("proactive compaction failed", "sessionID", p.config.SessionID, "error", compactErr)
				}
			}
		}

		p.mu.Lock()
		p.messages = append(p.messages, *assistantMsg)
		p.mu.Unlock()

		p.bus.Publish("session.message", map[string]any{
			"sessionID": p.config.SessionID,
			"message":   *assistantMsg,
		})

		toolCalls := extractToolCalls(assistantMsg)
		textLen := 0
		for _, part := range assistantMsg.Parts {
			if part.Type == PartText {
				textLen += len(part.Text)
			}
		}
		slog.Info("LLM response", "sessionID", p.config.SessionID, "iteration", iteration, "toolCalls", len(toolCalls), "textLen", textLen, "parts", len(assistantMsg.Parts), "inputTokens", usage.Input, "outputTokens", usage.Output)

		hadToolCalls := len(toolCalls) > 0

		if len(toolCalls) == 0 {
			// Auto-continue: if previous iteration had tool calls and we haven't
			// exceeded the limit, nudge the LLM to keep going.
			if hadPriorToolCalls && p.autoContinueCount < p.autoContinueLimit() {
				p.autoContinueCount++
				nudgeID, _ := id.Ascending("message")
				nudgeMsg := Message{
					ID:        nudgeID,
					SessionID: p.config.SessionID,
					Role:      RoleUser,
					Parts:     []Part{TextPart("Continue with your next step.")},
					CreatedAt: time.Now(),
				}
				p.mu.Lock()
				p.messages = append(p.messages, nudgeMsg)
				p.mu.Unlock()
				p.bus.Publish("session.message", map[string]any{
					"sessionID": p.config.SessionID,
					"message":   nudgeMsg,
				})
				slog.Info("auto-continue nudge", "sessionID", p.config.SessionID, "count", p.autoContinueCount)
				continue
			}

			slog.Info("processor done (no tool calls)", "sessionID", p.config.SessionID, "iteration", iteration)
			return &ProcessResult{
				Messages: p.Messages(),
				Usage:    totalUsage,
			}
		}

		// Doom-loop detection: check for repeated identical tool calls.
		for _, tc := range toolCalls {
			p.recentToolCalls = append(p.recentToolCalls, toolCallSignature{
				Name: tc.ToolName,
				Args: tc.ToolArgs,
			})
		}
		threshold := p.doomThreshold()
		if len(p.recentToolCalls) > threshold {
			p.recentToolCalls = p.recentToolCalls[len(p.recentToolCalls)-threshold:]
		}
		if isDoomLoop(p.recentToolCalls, threshold) {
			return &ProcessResult{
				Messages: p.Messages(),
				Usage:    totalUsage,
				Error:    fmt.Errorf("doom loop detected: last %d tool calls were identical (%s)", threshold, p.recentToolCalls[0].Name),
			}
		}

		results, allFailed := p.executeTools(ctx, toolCalls)
		slog.Info("tools executed", "sessionID", p.config.SessionID, "iteration", iteration, "toolCount", len(toolCalls), "allFailed", allFailed)

		toolMsgID, _ := id.Ascending("message")
		toolMsg := Message{
			ID:        toolMsgID,
			SessionID: p.config.SessionID,
			Role:      RoleTool,
			Parts:     results,
			CreatedAt: time.Now(),
		}

		p.mu.Lock()
		p.messages = append(p.messages, toolMsg)
		p.mu.Unlock()

		p.bus.Publish("session.message", map[string]any{
			"sessionID": p.config.SessionID,
			"message":   toolMsg,
		})

		if allFailed {
			consecutiveToolFailures++
			if consecutiveToolFailures >= maxConsecutiveToolFailures {
				return &ProcessResult{
					Messages: p.Messages(),
					Usage:    totalUsage,
					Error:    fmt.Errorf("%d consecutive tool call failures", consecutiveToolFailures),
				}
			}
			if consecutiveToolFailures%consecutiveToolFailureWarnEvery == 0 {
				p.bus.Publish("session.warning", map[string]any{
					"sessionID": p.config.SessionID,
					"message":   "Multiple consecutive tool call failures. Consider switching to a larger model.",
				})
			}
		} else {
			consecutiveToolFailures = 0
		}

		hadPriorToolCalls = hadToolCalls
	}
}

func (p *Processor) callLLM(ctx context.Context) (*Message, *TokenUsage, error) {
	req := p.buildRequest()

	slog.Info("callLLM", "sessionID", p.config.SessionID, "model", p.config.Model.ID, "llmMessages", len(req.Messages), "tools", len(req.Tools))

	var lastErr error
	for attempt := 0; attempt <= provider.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := provider.RetryDelay(attempt)
			slog.Warn("callLLM retry", "sessionID", p.config.SessionID, "attempt", attempt, "delay", delay, "lastErr", lastErr)
			select {
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		ch, err := p.client.Stream(ctx, req)
		if err != nil {
			if provider.IsRetryable(err.Error()) || provider.IsRetryableStatus(statusFromError(err)) {
				lastErr = err
				continue
			}
			return nil, nil, err
		}

		msg, usage, err := p.consumeStream(ch)
		if err != nil {
			if provider.IsRetryable(err.Error()) {
				lastErr = err
				continue
			}
			return nil, nil, err
		}

		return msg, usage, nil
	}

	return nil, nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

func (p *Processor) buildRequest() llm.Request {
	messages := p.Messages()
	llmMessages := make([]llm.Message, 0, len(messages)+1)

	if p.config.SystemPrompt != "" {
		llmMessages = append(llmMessages, llm.Message{
			Role:    "system",
			Content: p.config.SystemPrompt,
		})
	}

	for _, msg := range messages {
		if msg.Role == RoleTool {
			// Each tool result must be a separate LLM message per the OpenAI API.
			for _, part := range msg.Parts {
				if part.Type == PartToolResult {
					llmMessages = append(llmMessages, llm.Message{
						Role:       "tool",
						Content:    part.ToolResult,
						ToolCallID: part.ToolCallID,
					})
				}
			}
			continue
		}

		llmMsg := llm.Message{Role: string(msg.Role)}

		var textParts []string
		for _, part := range msg.Parts {
			switch part.Type {
			case PartText:
				textParts = append(textParts, part.Text)
			case PartToolCall:
				if llmMsg.ToolCalls == nil {
					llmMsg.ToolCalls = []llm.ToolCall{}
				}
				llmMsg.ToolCalls = append(llmMsg.ToolCalls, llm.ToolCall{
					ID:   part.ToolCallID,
					Type: "function",
					Function: llm.FunctionCall{
						Name:      part.ToolName,
						Arguments: part.ToolArgs,
					},
				})
			}
		}

		if len(textParts) > 0 {
			llmMsg.Content = strings.Join(textParts, "\n")
		}

		llmMessages = append(llmMessages, llmMsg)
	}

	tools := p.tools.ToolDefs(p.config.AgentPerms)

	req := llm.Request{
		Model:       p.config.Model.ID,
		Messages:    llmMessages,
		Tools:       tools,
		Temperature: p.config.Temperature,
		TopP:        p.config.TopP,
		MaxTokens:   p.config.MaxTokens,
	}

	return req
}

func (p *Processor) consumeStream(ch <-chan llm.Event) (*Message, *TokenUsage, error) {
	msgID, _ := id.Ascending("message")

	var textParts []string
	var toolCallParts []Part
	var reasoningParts []string
	var streamErr error
	usage := &TokenUsage{}

	activeToolCalls := make(map[string]*Part)

	for event := range ch {
		if p.isAborted() {
			return nil, nil, fmt.Errorf("aborted")
		}

		switch event.Type {
		case llm.EventTextDelta:
			textParts = append(textParts, event.Text)
			p.bus.Publish("session.text.delta", map[string]any{
				"sessionID": p.config.SessionID,
				"text":      event.Text,
			})

		case llm.EventReasoningDelta:
			reasoningParts = append(reasoningParts, event.Text)

		case llm.EventToolCallBegin:
			part := ToolCallPart(event.ToolCallID, event.ToolName, "")
			activeToolCalls[event.ToolCallID] = &part
			p.bus.Publish("session.tool.begin", map[string]any{
				"sessionID":  p.config.SessionID,
				"toolCallID": event.ToolCallID,
				"toolName":   event.ToolName,
			})

		case llm.EventToolCallDelta:
			if tc, ok := activeToolCalls[event.ToolCallID]; ok {
				tc.ToolArgs += event.ToolCallArgs
			}

		case llm.EventToolCallEnd:
			if tc, ok := activeToolCalls[event.ToolCallID]; ok {
				tc.ToolArgs = event.ToolCallArgs
				toolCallParts = append(toolCallParts, *tc)
				delete(activeToolCalls, event.ToolCallID)
				p.bus.Publish("session.tool.end", map[string]any{
					"sessionID":  p.config.SessionID,
					"toolCallID": event.ToolCallID,
					"toolName":   event.ToolName,
					"toolArgs":   event.ToolCallArgs,
				})
			}

		case llm.EventFinish:
			if event.Usage != nil {
				usage.Input = event.Usage.PromptTokens
				usage.Output = event.Usage.CompletionTokens
			}

		case llm.EventError:
			streamErr = event.Error
		}
	}

	if streamErr != nil {
		return nil, nil, streamErr
	}

	var parts []Part
	if len(reasoningParts) > 0 {
		parts = append(parts, ReasoningPart(strings.Join(reasoningParts, "")))
	}
	if len(textParts) > 0 {
		parts = append(parts, TextPart(strings.Join(textParts, "")))
	}
	parts = append(parts, toolCallParts...)

	msg := &Message{
		ID:        msgID,
		SessionID: p.config.SessionID,
		Role:      RoleAssistant,
		Parts:     parts,
		Model:     p.config.Model.ID,
		CreatedAt: time.Now(),
	}

	return msg, usage, nil
}

func (p *Processor) executeTools(ctx context.Context, toolCalls []Part) ([]Part, bool) {
	type toolResult struct {
		index  int
		result Part
		failed bool
	}

	results := make([]Part, len(toolCalls))
	ch := make(chan toolResult, len(toolCalls))

	var wg sync.WaitGroup
	for i, tc := range toolCalls {
		wg.Add(1)
		go func(idx int, call Part) {
			defer wg.Done()

			// Permission check: if tool args reference paths outside the
			// configured directory, ask the permission service.
			if denied := p.checkExternalDirectory(ctx, call); denied != "" {
				ch <- toolResult{
					index:  idx,
					result: ToolResultPart(call.ToolCallID, call.ToolName, denied, true),
					failed: true,
				}
				return
			}

			output, isErr, err := p.tools.Execute(ctx, call.ToolName, json.RawMessage(call.ToolArgs), p.config.SessionID)
			if err != nil {
				ch <- toolResult{
					index: idx,
					result: ToolResultPart(call.ToolCallID, call.ToolName, err.Error(), true),
					failed: true,
				}
				return
			}

			ch <- toolResult{
				index:  idx,
				result: ToolResultPart(call.ToolCallID, call.ToolName, output, isErr),
				failed: isErr,
			}
		}(i, tc)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	allFailed := true
	for tr := range ch {
		results[tr.index] = tr.result
		if !tr.failed {
			allFailed = false
		}
	}

	return results, allFailed
}

func (p *Processor) compact(ctx context.Context) (bool, error) {
	p.mu.Lock()
	p.compactionCount++
	compNum := p.compactionCount
	messages := append([]Message(nil), p.messages...)
	p.mu.Unlock()

	if compNum > compactionCircuitBreakerThreshold {
		p.bus.Publish("session.warning", map[string]any{
			"sessionID": p.config.SessionID,
			"message":   fmt.Sprintf("Compaction #%d: consider starting a new session or using subagents.", compNum),
		})
	}

	estimator := NewLazyEstimator(0.25)
	preserveIdx := estimator.FindPreserveBoundary(messages, p.config.Compaction.MaxPreserve)

	if preserveIdx <= 1 {
		return false, nil
	}

	toCompact := messages[:preserveIdx]
	preserved := messages[preserveIdx:]

	if p.config.Compaction.MaskObservations {
		toCompact = maskObservations(toCompact)
	}

	readFiles, modifiedFiles := trackFiles(messages)
	prompt := buildCompactionPrompt(toCompact, p.priorSummary, readFiles, modifiedFiles)

	compactionModel := p.config.Model.ID
	if p.config.CompactionModel != "" {
		compactionModel = p.config.CompactionModel
	} else if p.config.SmallModel != "" {
		compactionModel = p.config.SmallModel
	}

	summaryReq := llm.Request{
		Model: compactionModel,
		Messages: []llm.Message{
			{Role: "user", Content: prompt},
		},
	}

	ch, err := p.client.Stream(ctx, summaryReq)
	if err != nil {
		return false, fmt.Errorf("requesting compaction summary: %w", err)
	}

	var summaryParts []string
	for event := range ch {
		if event.Type == llm.EventTextDelta {
			summaryParts = append(summaryParts, event.Text)
		}
		if event.Type == llm.EventError {
			return false, event.Error
		}
	}

	summary := strings.Join(summaryParts, "")

	summaryMsgID, _ := id.Ascending("message")
	summaryMsg := Message{
		ID:        summaryMsgID,
		SessionID: p.config.SessionID,
		Role:      RoleSystem,
		Parts:     []Part{TextPart(summary)},
		CreatedAt: time.Now(),
	}

	p.mu.Lock()
	p.messages = append([]Message{summaryMsg}, preserved...)
	p.priorSummary = summary
	p.mu.Unlock()

	p.bus.Publish("session.compacted", map[string]any{
		"sessionID":     p.config.SessionID,
		"compactionNum": compNum,
		"preMessages":   len(messages),
		"postMessages":  len(preserved) + 1,
	})

	return true, nil
}

// checkExternalDirectory returns a denial message if the tool call targets a
// path outside the configured directory and the permission service denies it.
// Returns "" if allowed.
func (p *Processor) checkExternalDirectory(ctx context.Context, call Part) string {
	if p.config.Directory == "" || p.config.Perms == nil {
		return ""
	}

	paths := extractPathsFromArgs(call.ToolArgs)
	for _, path := range paths {
		if !isInsideDirectory(path, p.config.Directory) {
			askID, _ := id.Ascending("perm")
			err := p.config.Perms.Ask(ctx, permission.AskInput{
				ID:         askID,
				SessionID:  p.config.SessionID,
				Permission: "external_directory",
				Patterns:   []string{path},
				Metadata: map[string]any{
					"tool": call.ToolName,
					"path": path,
				},
				Ruleset: p.config.Ruleset,
			})
			if err != nil {
				return fmt.Sprintf("permission denied: tool %q targets path %q outside project directory %q", call.ToolName, path, p.config.Directory)
			}
		}
	}
	return ""
}

// extractPathsFromArgs extracts file path values from a tool call's JSON args.
func extractPathsFromArgs(argsJSON string) []string {
	var paths []string
	var args map[string]json.RawMessage
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return nil
	}
	for _, key := range []string{"file_path", "path", "file", "directory"} {
		raw, ok := args[key]
		if !ok {
			continue
		}
		var val string
		if json.Unmarshal(raw, &val) == nil && val != "" {
			paths = append(paths, val)
		}
	}
	return paths
}

// isInsideDirectory reports whether path is inside the directory (or is the
// directory itself). Both paths are cleaned before comparison.
func isInsideDirectory(path, directory string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absDir, err := filepath.Abs(directory)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absDir, absPath)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, "..")
}

func extractToolCalls(msg *Message) []Part {
	var calls []Part
	for _, part := range msg.Parts {
		if part.Type == PartToolCall {
			calls = append(calls, part)
		}
	}
	return calls
}

var httpStatusRe = regexp.MustCompile(`\bHTTP (\d{3})\b`)

func statusFromError(err error) int {
	m := httpStatusRe.FindStringSubmatch(err.Error())
	if m == nil {
		return 0
	}
	code := 0
	for _, c := range m[1] {
		code = code*10 + int(c-'0')
	}
	return code
}
