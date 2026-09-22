package session

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/bobbyjohnstx/tinycode-go/internal/bus"
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
	MaxIterations    int
	ThinkingBudget   *int
	Perms            *permission.Service
	Ruleset          permission.Ruleset
	UserDisplayText  string
}

type Processor struct {
	config            ProcessorConfig
	client            llm.Client
	tools             ToolExecutor
	bus               *bus.Bus
	messages          []Message
	priorSummary      string
	compactionCount   int
	aborted           bool
	recentToolCalls   []toolCallSignature
	autoContinueCount int
	userExtraParts    []Part
	mu                sync.Mutex
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

// SetUserExtraParts sets additional parts (e.g. images) to include in the
// next user message alongside the text.
func (p *Processor) SetUserExtraParts(parts []Part) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.userExtraParts = parts
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

func (p *Processor) maxIter() int {
	if p.config.MaxIterations > 0 {
		return p.config.MaxIterations
	}
	return maxIterations
}

type ProcessResult struct {
	Messages []Message
	Usage    TokenUsage
	Error    error
	Aborted  bool
}

// Process runs the main processor loop. Helper methods are in processor_loop.go.
func (p *Processor) Process(ctx context.Context, userMessage string) *ProcessResult {
	return p.ProcessWithID(ctx, userMessage, "")
}

func (p *Processor) ProcessWithID(ctx context.Context, userMessage, messageID string) *ProcessResult {
	p.mu.Lock()
	p.aborted = false
	p.autoContinueCount = 0
	p.mu.Unlock()

	p.addUserMessage(userMessage, messageID)

	totalUsage := TokenUsage{}
	var consecutiveToolFailures int
	var hadPriorToolCalls bool
	iteration := 0

	for {
		iteration++
		if iteration > p.maxIter() {
			return &ProcessResult{
				Messages: p.Messages(),
				Usage:    totalUsage,
				Error:    fmt.Errorf("processor exceeded %d iterations", p.maxIter()),
			}
		}
		slog.Info("processor loop iteration", "sessionID", p.config.SessionID, "iteration", iteration, "messageCount", len(p.Messages()))

		if result := p.checkAbortAndContext(ctx, iteration, totalUsage); result != nil {
			return result
		}

		assistantMsg, usage, stepErr := p.runLLMStep(ctx, iteration)

		if stepErr != nil {
			if result := p.handleLLMError(ctx, stepErr, totalUsage, iteration); result != nil {
				return result
			}
			continue
		}

		totalUsage.Input += usage.Input
		totalUsage.Output += usage.Output
		totalUsage.Reasoning += usage.Reasoning
		totalUsage.Cache.Read += usage.Cache.Read
		totalUsage.Cache.Write += usage.Cache.Write

		assistantMsg.Tokens = &MsgUsage{
			Input:     usage.Input,
			Output:    usage.Output,
			Reasoning: usage.Reasoning,
			Cache: CacheUsage{
				Read:  usage.Cache.Read,
				Write: usage.Cache.Write,
			},
		}

		p.checkCompaction(ctx, totalUsage)

		p.mu.Lock()
		p.messages = append(p.messages, *assistantMsg)
		p.mu.Unlock()

		p.bus.Publish("session.message", map[string]any{
			"sessionID": p.config.SessionID,
			"message":   *assistantMsg,
		})

		toolCalls := extractToolCalls(assistantMsg)
		hadToolCalls := len(toolCalls) > 0
		p.logLLMResponse(assistantMsg, toolCalls, usage, iteration)

		if len(toolCalls) == 0 {
			if p.tryAutoContinue(hadPriorToolCalls) {
				continue
			}
			slog.Info("processor done (no tool calls)", "sessionID", p.config.SessionID, "iteration", iteration)
			return &ProcessResult{
				Messages: p.Messages(),
				Usage:    totalUsage,
			}
		}

		if result := p.checkDoomLoop(toolCalls, totalUsage); result != nil {
			return result
		}

		stop, result, failures := p.runToolCalls(ctx, toolCalls, totalUsage, consecutiveToolFailures, iteration)
		consecutiveToolFailures = failures
		if stop {
			return result
		}

		hadPriorToolCalls = hadToolCalls
	}
}
