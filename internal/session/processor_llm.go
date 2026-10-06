package session

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/id"
	"github.com/bobbyjohnstx/tinycode/internal/llm"
	"github.com/bobbyjohnstx/tinycode/internal/provider"
)

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
		Model:          p.config.Model.ID,
		Messages:       llmMessages,
		Tools:          tools,
		Temperature:    p.config.Temperature,
		TopP:           p.config.TopP,
		MaxTokens:      p.config.MaxTokens,
		ThinkingBudget: p.config.ThinkingBudget,
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
	p.compacted = true
	p.mu.Unlock()

	p.bus.Publish("session.compacted", map[string]any{
		"sessionID":     p.config.SessionID,
		"compactionNum": compNum,
		"preMessages":   len(messages),
		"postMessages":  len(preserved) + 1,
	})

	return true, nil
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
