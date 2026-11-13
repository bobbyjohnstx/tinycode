package session

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/id"
	"github.com/bobbyjohnstx/tinycode/internal/llm"
	"github.com/bobbyjohnstx/tinycode/internal/provider"
)

const answerFollowUp = "Stop all reasoning and thinking. You MUST provide a direct answer NOW using only the information already available in the conversation. Do not use any tools."

const (
	// vllmDefaultThinkingTokenBudget is the thinking budget sent to vLLM when
	// neither a /thinking level nor provider.vllm.options.thinking_token_budget
	// is set.
	vllmDefaultThinkingTokenBudget = 4096
	// vllmMinThinkingTokenBudget: smaller budgets starve the reasoning model
	// and hurt answer quality, so requests are clamped to at least this.
	vllmMinThinkingTokenBudget = 1024
	// vllmAnswerHeadroom is kept after a thinking budget when sizing the
	// stream's max_tokens, for the answer and tool calls.
	vllmAnswerHeadroom = 8192
	// compactionSummaryMaxTokens caps the compaction summary's output. The
	// default stream cap (DefaultStreamTokens=16384) truncates long sessions'
	// summaries ("stream budget exceeded"); a dedicated, larger cap keeps full
	// summaries while staying far below context. The model's output limit wins
	// when known and smaller. Applies to all providers.
	compactionSummaryMaxTokens = 32768
)

func (p *Processor) callLLM(ctx context.Context) (*Message, *TokenUsage, error) {
	req := p.buildRequest()

	slog.Info("callLLM", "sessionID", p.config.SessionID, "model", req.Model, "llmMessages", len(req.Messages), "tools", len(req.Tools))

	var opts []llm.StreamOption
	if p.config.Model != nil && len(p.config.Model.Headers) > 0 {
		opts = append(opts, llm.WithHeaders(p.config.Model.Headers))
	}

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

		ch, err := p.client.Stream(ctx, req, opts...)
		if err != nil {
			if p.isVLLM() && req.ThinkingTokenBudget != nil && statusFromError(err) == 400 &&
				strings.Contains(strings.ToLower(err.Error()), "thinking_token_budget") {
				errLower := strings.ToLower(err.Error())
				permanent := strings.Contains(errLower, "not configured") ||
					strings.Contains(errLower, "not supported") ||
					strings.Contains(errLower, "unexpected keyword")
				if permanent {
					p.mu.Lock()
					p.thinkingBudgetDisabled = true
					p.mu.Unlock()
					slog.Warn("vLLM rejected thinking_token_budget (unsupported); disabling for session",
						"sessionID", p.config.SessionID, "error", err)
				} else {
					slog.Warn("vLLM rejected thinking_token_budget (value error); retrying without",
						"sessionID", p.config.SessionID, "error", err)
				}
				req.ThinkingTokenBudget = nil
				// The raised max_tokens stays: with the budget gone the model
				// may reason uncapped and still needs the room to finish.
				lastErr = err
				continue
			}
			if provider.IsRetryable(err.Error()) || provider.IsRetryableStatus(statusFromError(err)) {
				lastErr = err
				continue
			}
			return nil, nil, err
		}

		msg, usage, err := p.consumeStream(ch)
		if err != nil {
			if ctx.Err() != nil || err.Error() == "aborted" || errors.Is(err, llm.ErrStreamBudget) {
				return msg, usage, err
			}
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

// effectiveModelID returns Model.API.ID when set, otherwise Model.ID.
func effectiveModelID(m *provider.Model) string {
	if m == nil {
		return ""
	}
	if m.API.ID != "" {
		return m.API.ID
	}
	return m.ID
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
		var imageParts []llm.ContentPart
		for _, part := range msg.Parts {
			switch part.Type {
			case PartText:
				textParts = append(textParts, part.Text)
			case PartImage:
				imageParts = append(imageParts, llm.ContentPart{
					Type:      "image",
					ImageData: part.ImageData,
					MediaType: part.MediaType,
				})
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
					ThoughtSignature: part.ThoughtSig,
					ExtraContent:     extraContentFromThoughtSig(part.ThoughtSig),
				})
			}
		}

		if len(imageParts) > 0 {
			// Build multipart content: text + images.
			var contentParts []llm.ContentPart
			if len(textParts) > 0 {
				contentParts = append(contentParts, llm.ContentPart{
					Type: "text",
					Text: strings.Join(textParts, "\n"),
				})
			}
			contentParts = append(contentParts, imageParts...)
			llmMsg.ContentParts = contentParts
		} else if len(textParts) > 0 {
			llmMsg.Content = strings.Join(textParts, "\n")
		} else {
			llmMsg.Content = ""
		}

		if llmMsg.Content == "" && len(llmMsg.ContentParts) == 0 && len(llmMsg.ToolCalls) == 0 {
			continue
		}

		llmMessages = append(llmMessages, llmMsg)
	}

	tools := p.tools.ToolDefs(p.config.AgentPerms)

	req := llm.Request{
		Model:          effectiveModelID(p.config.Model),
		Messages:       llmMessages,
		Tools:          tools,
		Temperature:    p.config.Temperature,
		TopP:           p.config.TopP,
		MaxTokens:      p.config.MaxTokens,
		ThinkingBudget: p.config.ThinkingBudget,
	}
	if p.config.Model != nil {
		req.ModelOutputLimit = p.config.Model.Limit.Output
	}

	if p.isVLLM() {
		if budget := p.vllmThinkingBudget(); budget != 0 {
			req.ThinkingTokenBudget = &budget
			// vLLM counts thinking tokens against max_tokens, and the stream
			// enforcer counts reasoning toward the same cap. When no explicit
			// max_tokens is configured, size it around the budget so higher
			// /thinking levels can finish (never shrinking the default cap).
			if p.config.MaxTokens == nil {
				req.MaxTokens = p.vllmStreamCap(budget)
			}
		} else if p.config.ThinkingBudget != nil && *p.config.ThinkingBudget == 0 {
			// /thinking off — explicitly disable reasoning on the server
			req.ChatTemplateKwargs = map[string]any{"enable_thinking": false}
		}
	} else if p.isLMStudio() || p.isOpenRouter() {
		req.ReasoningEffort = p.reasoningEffort()
	} else if p.isOllama() {
		req.Think = p.ollamaThink()
	}

	clampMaxTokens(&req, p.config.Model)

	// Clamp vLLM thinking budget AFTER clampMaxTokens, so the budget
	// stays under the (possibly lowered) max_tokens.
	if p.isVLLM() && req.ThinkingTokenBudget != nil && req.MaxTokens != nil {
		if *req.ThinkingTokenBudget >= *req.MaxTokens {
			clamped := *req.MaxTokens - vllmAnswerHeadroom
			if clamped < vllmMinThinkingTokenBudget {
				clamped = vllmMinThinkingTokenBudget
			}
			if clamped >= *req.MaxTokens {
				clamped = *req.MaxTokens - 1
			}
			slog.Warn("clamped vLLM thinking budget to fit max_tokens",
				"original", *req.ThinkingTokenBudget,
				"clamped", clamped,
				"maxTokens", *req.MaxTokens)
			*req.ThinkingTokenBudget = clamped
		}
	}

	return req
}

// vllmThinkingBudget resolves the thinking_token_budget for vLLM models: a
// configured /thinking level wins, then the provider option, then the
// built-in default. Negative values mean "no budget" (returns 0, field
// omitted); positive values are clamped to at least vllmMinThinkingTokenBudget.
// Returns 0 when the budget is disabled for the session.
func (p *Processor) vllmThinkingBudget() int {
	p.mu.Lock()
	disabled := p.thinkingBudgetDisabled
	p.mu.Unlock()
	if disabled {
		return 0
	}

	var budget int
	switch {
	case p.config.ThinkingBudget != nil && *p.config.ThinkingBudget == 0:
		return 0 // /thinking off — explicitly disabled
	case p.config.ThinkingBudget != nil && *p.config.ThinkingBudget > 0:
		budget = *p.config.ThinkingBudget
	case p.config.VLLMThinkingTokenBudget != nil:
		budget = *p.config.VLLMThinkingTokenBudget
	default:
		budget = vllmDefaultThinkingTokenBudget
	}

	if budget < 0 {
		return 0
	}
	if budget < vllmMinThinkingTokenBudget {
		return vllmMinThinkingTokenBudget
	}
	return budget
}

// vllmStreamCap sizes max_tokens for a vLLM request that carries a thinking
// budget: the budget plus answer headroom, floored at the default stream cap
// so the effective cap never shrinks, and capped by the model's output limit
// when known and smaller. (After the min-budget clamp, budget+headroom is
// always above the floor; the floor documents that invariant.)
func (p *Processor) vllmStreamCap(budget int) *int {
	limit := budget + vllmAnswerHeadroom
	if limit < llm.DefaultStreamTokens {
		limit = llm.DefaultStreamTokens
	}
	if p.config.Model != nil && p.config.Model.Limit.Output > 0 && p.config.Model.Limit.Output < limit {
		limit = p.config.Model.Limit.Output
	}
	return &limit
}

func (p *Processor) consumeStream(ch <-chan llm.Event) (*Message, *TokenUsage, error) {
	msgID, _ := id.Ascending("message")

	var textParts []string
	var toolCallParts []Part
	var reasoningParts []string
	var streamErr error
	usage := &TokenUsage{}

	activeToolCalls := make(map[string]*Part)

	aborted := false
	for event := range ch {
		if p.isAborted() {
			aborted = true
			break
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
			p.bus.Publish("session.reasoning.delta", map[string]any{
				"sessionID": p.config.SessionID,
				"text":      event.Text,
			})

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
				tc.ThoughtSig = event.ThoughtSignature
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
				if event.Usage.PromptTokens > 0 {
					usage.Input = event.Usage.PromptTokens
				}
				if event.Usage.CompletionTokens > 0 {
					usage.Output = event.Usage.CompletionTokens
				}
				if event.Usage.CacheCreationTokens > 0 {
					usage.Cache.Write = event.Usage.CacheCreationTokens
				}
				if event.Usage.CacheReadTokens > 0 {
					usage.Cache.Read = event.Usage.CacheReadTokens
				}
			}

		case llm.EventError:
			streamErr = event.Error
		}
	}

	// Flush incomplete tool calls (stream ended / budget / cancel without End).
	for id, tc := range activeToolCalls {
		toolCallParts = append(toolCallParts, *tc)
		p.bus.Publish("session.tool.end", map[string]any{
			"sessionID":  p.config.SessionID,
			"toolCallID": id,
			"toolName":   tc.ToolName,
			"toolArgs":   tc.ToolArgs,
			"incomplete": true,
		})
		delete(activeToolCalls, id)
	}

	budget := errors.Is(streamErr, llm.ErrStreamBudget)
	if streamErr != nil && !budget && !aborted {
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

	if aborted {
		return msg, usage, fmt.Errorf("aborted")
	}
	if budget {
		return msg, usage, streamErr
	}
	return msg, usage, nil
}

// recoverStreamBudget keeps a capped completion that already has an answer.
// When the cap hits before any answer and tool results are already in the
// session, it asks once for the report and does not retry that call.
func (p *Processor) recoverStreamBudget(ctx context.Context, iteration int, msg *Message, usage *TokenUsage, err error) (*Message, *TokenUsage, error) {
	if !errors.Is(err, llm.ErrStreamBudget) {
		return msg, usage, err
	}
	if messageHasVisibleAnswer(msg) {
		slog.Info("stream budget kept partial answer", "sessionID", p.config.SessionID, "iteration", iteration)
		return msg, usage, nil
	}
	if p.answerFollowUpUsed || !p.hasToolResult() {
		return msg, usage, err
	}
	p.answerFollowUpUsed = true
	slog.Info("stream budget produced no answer", "sessionID", p.config.SessionID, "iteration", iteration)
	p.addUserMessage(answerFollowUp, "")
	msg2, usage2, err2 := p.runLLMStep(ctx, iteration)
	if usage == nil {
		usage = &TokenUsage{}
	}
	if usage2 != nil {
		usage.Input += usage2.Input
		usage.Output += usage2.Output
		usage.Reasoning += usage2.Reasoning
		usage.Cache.Read += usage2.Cache.Read
		usage.Cache.Write += usage2.Cache.Write
	}
	if errors.Is(err2, llm.ErrStreamBudget) && messageHasVisibleAnswer(msg2) {
		return msg2, usage, nil
	}
	// Before returning the error, try to surface reasoning content
	if errors.Is(err2, llm.ErrStreamBudget) && !messageHasVisibleAnswer(msg2) {
		var reasoning strings.Builder
		for i := range msg2.Parts {
			if msg2.Parts[i].Type == PartReasoning && msg2.Parts[i].Text != "" {
				reasoning.WriteString(msg2.Parts[i].Text)
			}
		}
		if reasoning.Len() > 0 {
			msg2.Parts = append(msg2.Parts, Part{
				Type: PartText,
				Text: "[From model reasoning] " + reasoning.String(),
			})
			slog.Info("stream budget surfaced reasoning as answer",
				"sessionID", p.config.SessionID)
			return msg2, usage, nil
		}
	}
	return msg2, usage, err2
}

func messageHasVisibleAnswer(msg *Message) bool {
	if msg == nil {
		return false
	}
	// Text only — tool-call-only / mid-reasoning must not suppress budget follow-up.
	for _, part := range msg.Parts {
		if part.Type == PartText && strings.TrimSpace(part.Text) != "" {
			return true
		}
	}
	return false
}

func (p *Processor) hasToolResult() bool {
	for _, msg := range p.Messages() {
		for _, part := range msg.Parts {
			if part.Type == PartToolResult {
				return true
			}
		}
	}
	return false
}

// Compact runs a manual context compaction pass (same logic as proactive overflow compaction).
// Returns (true, nil) when history was compacted, (false, nil) when there was nothing to compact.
func (p *Processor) Compact(ctx context.Context) (bool, error) {
	return p.compact(ctx, false)
}

func (p *Processor) compact(ctx context.Context, force bool) (bool, error) {
	p.mu.Lock()
	compNum := p.compactionCount
	messages := append([]Message(nil), p.messages...)
	p.mu.Unlock()

	if compNum >= compactionCircuitBreakerThreshold {
		return false, fmt.Errorf("compaction circuit breaker: max %d attempts reached", compactionCircuitBreakerThreshold)
	}

	estimator := NewLazyEstimator(0.25)
	preserveIdx := estimator.FindPreserveBoundary(messages, p.config.Compaction.MaxPreserve)

	if preserveIdx <= 1 {
		if force && len(messages) > 2 {
			// The local estimate says the history fits, but the caller is
			// already over the provider limit or the message cap. Summarize
			// everything except the last turn.
			preserveIdx = len(messages) - 2
			if preserveIdx < 1 {
				preserveIdx = 1
			}
			slog.Info("compaction forced; estimator kept the whole history",
				"sessionID", p.config.SessionID,
				"messages", len(messages),
				"preserveFrom", preserveIdx)
		} else {
			slog.Info("compaction skipped; nothing older than the last turn",
				"sessionID", p.config.SessionID,
				"messages", len(messages))
			return false, nil
		}
	}

	p.mu.Lock()
	p.compactionCount++
	compNum = p.compactionCount
	p.mu.Unlock()

	// Ensure preserveIdx doesn't split a tool_use/tool_result pair.
	// If the first preserved message contains tool_results, pull back to
	// include the prior assistant message with the matching tool_use blocks.
	preserveIdx = adjustPreserveForToolPairs(messages, preserveIdx)

	toCompact := messages[:preserveIdx]
	preserved := messages[preserveIdx:]

	if p.config.Compaction.MaskObservations {
		toCompact = maskObservations(toCompact)
	}

	readFiles, modifiedFiles := trackFiles(messages)
	prompt := buildCompactionPrompt(toCompact, p.priorSummary, readFiles, modifiedFiles)

	compactionModel := effectiveModelID(p.config.Model)
	if p.config.CompactionModel != "" {
		compactionModel = p.config.CompactionModel
	} else if p.config.SmallModel != "" {
		compactionModel = p.config.SmallModel
	}

	summaryReq := llm.Request{
		Model:    compactionModel,
		Messages: []llm.Message{{Role: "user", Content: prompt}},
	}
	if p.isVLLM() {
		// vLLM passes chat_template_kwargs to the chat template. Disabling
		// thinking keeps the summary short and stops reasoning models from
		// stalling on the loop detector. Templates without the key ignore it.
		summaryReq.ChatTemplateKwargs = map[string]any{"enable_thinking": false}
	} else if p.isLMStudio() || p.isOpenRouter() {
		summaryReq.ReasoningEffort = "none"
	} else if p.isOllama() {
		summaryReq.Think = false
	}
	// Long sessions' summaries can exceed the default stream cap ("stream
	// budget exceeded"); give the summary a dedicated, larger cap for all
	// providers, clamped by the model's output limit when known and smaller.
	limit := compactionSummaryMaxTokens
	if p.config.Model != nil && p.config.Model.Limit.Output > 0 && p.config.Model.Limit.Output < limit {
		limit = p.config.Model.Limit.Output
	}
	summaryReq.MaxTokens = &limit

	var opts []llm.StreamOption
	if p.config.Model != nil && len(p.config.Model.Headers) > 0 {
		opts = append(opts, llm.WithHeaders(p.config.Model.Headers))
	}

	summary, err := p.runSummaryStream(ctx, summaryReq, opts...)
	if err != nil {
		return false, fmt.Errorf("compaction summary: %w", err)
	}
	if strings.TrimSpace(summary) == "" {
		return false, fmt.Errorf("compaction summary is empty")
	}

	summaryMsgID, _ := id.Ascending("message")
	summaryMsg := Message{
		ID:        summaryMsgID,
		SessionID: p.config.SessionID,
		Role:      RoleUser,
		Parts:     []Part{TextPart(summary)},
		CreatedAt: time.Now(),
	}

	p.mu.Lock()
	p.messages = append([]Message{summaryMsg}, preserved...)
	p.priorSummary = summary
	p.compacted = true
	p.compactionCount = 0
	p.mu.Unlock()

	p.bus.Publish("session.compacted", map[string]any{
		"sessionID":     p.config.SessionID,
		"compactionNum": compNum,
		"preMessages":   len(messages),
		"postMessages":  len(preserved) + 1,
	})

	if len(readFiles) > 0 || len(modifiedFiles) > 0 {
		seen := make(map[string]bool)
		var paths []string
		for _, f := range readFiles {
			if !seen[f] {
				seen[f] = true
				paths = append(paths, f)
			}
		}
		for _, f := range modifiedFiles {
			if !seen[f] {
				seen[f] = true
				paths = append(paths, f)
			}
		}
		sort.Strings(paths)
		hint := "Files already read before compaction: " + strings.Join(paths, ", ") + ". Do not re-read unless their content has changed."
		p.addUserMessage(hint, "")
	}

	return true, nil
}

func extraContentFromThoughtSig(sig string) *llm.ExtraContent {
	if sig == "" {
		return nil
	}
	return &llm.ExtraContent{
		Google: &llm.GoogleExtra{
			ThoughtSignature: sig,
		},
	}
}

func clampMaxTokens(req *llm.Request, model *provider.Model) {
	if model == nil || req.MaxTokens == nil {
		return
	}
	cap := model.Limit.Output
	if cap <= 0 {
		cap = model.Limit.Context
	}
	if cap <= 0 {
		return // unknown model limits, no-op
	}
	if *req.MaxTokens > cap {
		slog.Warn("clamped max_tokens to model limit",
			"requested", *req.MaxTokens,
			"limit", cap,
			"model", model.ID)
		*req.MaxTokens = cap
	}
}

// isVLLM reports whether the session's active model is served by vLLM.
func (p *Processor) isVLLM() bool {
	return p.config.Model != nil && strings.EqualFold(p.config.Model.ProviderID, "vllm")
}

// isLMStudio reports whether the session's active model is served by LM Studio.
func (p *Processor) isLMStudio() bool {
	return p.config.Model != nil && strings.EqualFold(p.config.Model.ProviderID, "lm-studio")
}

// isOpenRouter reports whether the session's active model is served by OpenRouter.
func (p *Processor) isOpenRouter() bool {
	return p.config.Model != nil && strings.EqualFold(p.config.Model.ProviderID, "openrouter")
}

// isOllama reports whether the session's active model is served by Ollama.
func (p *Processor) isOllama() bool {
	return p.config.Model != nil && strings.EqualFold(p.config.Model.ProviderID, "ollama")
}

// reasoningEffort maps the /thinking level to reasoning_effort for LM Studio
// and OpenRouter. Returns "" (omitted from wire) when no thinking level is set.
func (p *Processor) reasoningEffort() string {
	if p.config.ThinkingBudget == nil {
		return ""
	}
	budget := *p.config.ThinkingBudget
	switch {
	case budget == 0:
		return "none"
	case budget <= 1024:
		return "low"
	case budget <= 4096:
		return "medium"
	case budget <= 16384:
		return "high"
	default:
		return "xhigh"
	}
}

// ollamaThink maps the /thinking level to Ollama's think parameter.
// Returns nil (omitted from wire) when no thinking level is set.
func (p *Processor) ollamaThink() any {
	if p.config.ThinkingBudget == nil {
		return nil
	}
	budget := *p.config.ThinkingBudget
	switch {
	case budget == 0:
		return false
	case budget <= 1024:
		return "low"
	case budget <= 4096:
		return "medium"
	case budget <= 16384:
		return "high"
	default:
		return "max"
	}
}

// runSummaryStream runs one compaction summary request and returns the
// joined summary text. ErrStreamBudget keeps the partial summary;
// other stream errors end the call.
func (p *Processor) runSummaryStream(ctx context.Context, req llm.Request, opts ...llm.StreamOption) (string, error) {
	ch, err := p.client.Stream(ctx, req, opts...)
	if err != nil {
		return "", fmt.Errorf("requesting compaction summary: %w", err)
	}
	var parts []string
	for event := range ch {
		if event.Type == llm.EventTextDelta {
			parts = append(parts, event.Text)
		}
		if event.Type == llm.EventError {
			if errors.Is(event.Error, llm.ErrStreamBudget) {
				break // keep partial summary
			}
			return "", event.Error
		}
	}
	return strings.Join(parts, ""), nil
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
