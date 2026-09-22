package session

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/bobbyjohnstx/tinycode-go/internal/id"
	"github.com/bobbyjohnstx/tinycode-go/internal/provider"
)

// runLLMStep publishes step events, calls the LLM, and returns the result.
func (p *Processor) runLLMStep(ctx context.Context, iteration int) (*Message, *TokenUsage, error) {
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

	return assistantMsg, usage, err
}

// logLLMResponse logs summary info about the LLM response.
func (p *Processor) logLLMResponse(assistantMsg *Message, toolCalls []Part, usage *TokenUsage, iteration int) {
	textLen := 0
	for _, part := range assistantMsg.Parts {
		if part.Type == PartText {
			textLen += len(part.Text)
		}
	}
	slog.Info("LLM response", "sessionID", p.config.SessionID, "iteration", iteration, "toolCalls", len(toolCalls), "textLen", textLen, "parts", len(assistantMsg.Parts), "inputTokens", usage.Input, "outputTokens", usage.Output)
}

// addUserMessage creates a user message, appends it to the conversation, and
// publishes it on the bus. If messageID is non-empty it is used as the message
// ID so that the client's optimistic message merges correctly.
func (p *Processor) addUserMessage(userMessage string, messageID string) {
	userMsgID := messageID
	if userMsgID == "" {
		userMsgID, _ = id.Ascending("message")
	}
	parts := []Part{TextPart(userMessage)}
	if len(p.userExtraParts) > 0 {
		parts = append(parts, p.userExtraParts...)
		p.userExtraParts = nil
	}
	userMsg := Message{
		ID:        userMsgID,
		SessionID: p.config.SessionID,
		Role:      RoleUser,
		Parts:     parts,
		CreatedAt: time.Now(),
	}

	p.mu.Lock()
	p.messages = append(p.messages, userMsg)
	p.mu.Unlock()

	// Publish with display text if set (e.g., short "/swarm ..." instead
	// of the full instruction prefix). The full text stays in the
	// conversation for LLM context.
	eventMsg := userMsg
	if p.config.UserDisplayText != "" {
		eventMsg.Parts = []Part{TextPart(p.config.UserDisplayText)}
	}

	p.bus.Publish("session.message", map[string]any{
		"sessionID": p.config.SessionID,
		"message":   eventMsg,
	})
}

// checkAbortAndContext returns a ProcessResult if the processor should stop due
// to abort or context cancellation.
func (p *Processor) checkAbortAndContext(ctx context.Context, iteration int, totalUsage TokenUsage) *ProcessResult {
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

	return nil
}

// handleLLMError handles an error from callLLM, attempting compaction on overflow.
// Returns a ProcessResult if processing should stop, or nil to retry.
func (p *Processor) handleLLMError(ctx context.Context, err error, totalUsage TokenUsage, iteration int) *ProcessResult {
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
			return nil
		}
	}
	return &ProcessResult{
		Messages: p.Messages(),
		Usage:    totalUsage,
		Error:    err,
	}
}

// checkCompaction runs proactive overflow detection and message-count compaction.
func (p *Processor) checkCompaction(ctx context.Context, totalUsage TokenUsage) {
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

	if maxMsg := p.config.Compaction.MaxMessages; maxMsg > 0 && len(p.Messages()) >= maxMsg {
		slog.Info("message-count compaction triggered", "sessionID", p.config.SessionID, "messages", len(p.Messages()), "maxMessages", maxMsg)
		if _, compactErr := p.compact(ctx); compactErr != nil {
			slog.Warn("message-count compaction failed", "sessionID", p.config.SessionID, "error", compactErr)
		}
	}
}

// tryAutoContinue checks if the processor should auto-continue after the LLM
// returned no tool calls. Returns true if a nudge was injected and the loop
// should continue.
func (p *Processor) tryAutoContinue(hadPriorToolCalls bool) bool {
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
		return true
	}
	return false
}

// checkDoomLoop records tool call signatures and returns a ProcessResult if a
// doom loop is detected.
func (p *Processor) checkDoomLoop(toolCalls []Part, totalUsage TokenUsage) *ProcessResult {
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
	return nil
}

// runToolCalls executes tool calls, stores the result message, and checks for
// consecutive failure thresholds. Returns (shouldStop bool, result *ProcessResult,
// newConsecutiveFailures int).
func (p *Processor) runToolCalls(ctx context.Context, toolCalls []Part, totalUsage TokenUsage, consecutiveToolFailures, iteration int) (bool, *ProcessResult, int) {
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
			return true, &ProcessResult{
				Messages: p.Messages(),
				Usage:    totalUsage,
				Error:    fmt.Errorf("%d consecutive tool call failures", consecutiveToolFailures),
			}, consecutiveToolFailures
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

	return false, nil, consecutiveToolFailures
}
