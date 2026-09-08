package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/bobbyjohnstx/tinycode-go/internal/bus"
	"github.com/bobbyjohnstx/tinycode-go/internal/id"
	"github.com/bobbyjohnstx/tinycode-go/internal/llm"
	"github.com/bobbyjohnstx/tinycode-go/internal/provider"
)

const (
	maxConsecutiveToolFailures = 3
)

type ToolExecutor interface {
	Execute(ctx context.Context, name string, args json.RawMessage, sessionID string) (string, bool, error)
	ToolDefs(agentPerms []string) []llm.Tool
}

type ProcessorConfig struct {
	SessionID      string
	Agent          string
	Model          *provider.Model
	SubagentDepth  int
	MaxSubagents   int
	SystemPrompt   string
	Compaction     CompactionConfig
	AgentPerms     []string
}

type Processor struct {
	config      ProcessorConfig
	client      llm.Client
	tools       ToolExecutor
	bus         *bus.Bus
	messages    []Message
	priorSummary string
	compactionCount int
	aborted     bool
	mu          sync.Mutex
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

type ProcessResult struct {
	Messages []Message
	Usage    TokenUsage
	Error    error
	Aborted  bool
}

func (p *Processor) Process(ctx context.Context, userMessage string) *ProcessResult {
	p.mu.Lock()
	p.aborted = false
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
	iteration := 0

	for {
		iteration++
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

		assistantMsg, usage, err := p.callLLM(ctx)
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

		if len(toolCalls) == 0 {
			slog.Info("processor done (no tool calls)", "sessionID", p.config.SessionID, "iteration", iteration)
			return &ProcessResult{
				Messages: p.Messages(),
				Usage:    totalUsage,
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
				p.bus.Publish("session.warning", map[string]any{
					"sessionID": p.config.SessionID,
					"message":   "Multiple consecutive tool call failures. Consider switching to a larger model.",
				})
				consecutiveToolFailures = 0
			}
		} else {
			consecutiveToolFailures = 0
		}
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

		for _, part := range msg.Parts {
			switch part.Type {
			case PartText:
				llmMsg.Content = part.Text
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

		llmMessages = append(llmMessages, llmMsg)
	}

	tools := p.tools.ToolDefs(p.config.AgentPerms)

	req := llm.Request{
		Model:    p.config.Model.ID,
		Messages: llmMessages,
		Tools:    tools,
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

	summaryReq := llm.Request{
		Model: p.config.Model.ID,
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

func extractToolCalls(msg *Message) []Part {
	var calls []Part
	for _, part := range msg.Parts {
		if part.Type == PartToolCall {
			calls = append(calls, part)
		}
	}
	return calls
}

func statusFromError(err error) int {
	msg := err.Error()
	if strings.Contains(msg, "HTTP 429") || strings.Contains(msg, "429") {
		return 429
	}
	if strings.Contains(msg, "HTTP 500") || strings.Contains(msg, "500") {
		return 500
	}
	if strings.Contains(msg, "HTTP 502") || strings.Contains(msg, "502") {
		return 502
	}
	if strings.Contains(msg, "HTTP 503") || strings.Contains(msg, "503") {
		return 503
	}
	return 0
}
