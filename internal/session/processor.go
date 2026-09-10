package session

import (
	"context"
	"fmt"
	"log/slog"
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
)

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

		// Message-count compaction: compact when conversation exceeds MaxMessages.
		// This catches cases where the model doesn't report token usage (local
		// models) or the context limit isn't known, preventing unbounded growth.
		if maxMsg := p.config.Compaction.MaxMessages; maxMsg > 0 && len(p.Messages()) >= maxMsg {
			slog.Info("message-count compaction triggered", "sessionID", p.config.SessionID, "messages", len(p.Messages()), "maxMessages", maxMsg)
			if _, compactErr := p.compact(ctx); compactErr != nil {
				slog.Warn("message-count compaction failed", "sessionID", p.config.SessionID, "error", compactErr)
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
