package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/bobbyjohnstx/tinycode/internal/llm"
)

// ToolExecutor defines the interface for executing tool calls.
type ToolExecutor interface {
	Execute(ctx context.Context, name string, args json.RawMessage, sessionID string) (string, bool, error)
	ToolDefs(agentPerms []string) []llm.Tool
}

// AgentSwitcher is an optional interface implemented by ToolExecutors that
// support mid-session agent switches requested by the plan_enter/plan_exit
// tools. Checked via type assertion after each tool-call round so that
// ToolExecutor implementations without switch support (e.g. test mocks)
// don't need to change.
type AgentSwitcher interface {
	TakePendingAgentSwitch() (string, bool)
}

// applyPendingAgentSwitch checks for an approved plan_enter/plan_exit tool
// call and updates the processor's active agent bookkeeping. Returns the new
// agent name, or "" if no switch was pending. The new agent's permissions and
// system prompt take full effect starting with the session's next turn (the
// caller persists the switch and resolves a fresh agent on the next prompt).
func (p *Processor) applyPendingAgentSwitch() string {
	switcher, ok := p.tools.(AgentSwitcher)
	if !ok {
		return ""
	}
	target, has := switcher.TakePendingAgentSwitch()
	if !has || target == "" {
		return ""
	}

	p.mu.Lock()
	changed := p.config.Agent != target
	p.config.Agent = target
	p.mu.Unlock()

	if !changed {
		return ""
	}

	slog.Info("agent switched", "sessionID", p.config.SessionID, "agent", target)
	p.bus.Publish("session.agent.switched", map[string]any{
		"sessionID": p.config.SessionID,
		"agent":     target,
	})
	return target
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
			defer func() {
				if r := recover(); r != nil {
					slog.Error("tool panicked", "tool", call.ToolName, "panic", r)
					msg := fmt.Sprintf("tool panicked: %v", r)
					p.publishToolResult(call, msg, true)
					ch <- toolResult{
						index:  idx,
						result: ToolResultPart(call.ToolCallID, call.ToolName, msg, true),
						failed: true,
					}
				}
			}()

			// Permission check: if tool args reference paths outside the
			// configured directory, ask the permission service.
			if denied := p.checkExternalDirectory(ctx, call); denied != "" {
				p.publishToolResult(call, denied, true)
				ch <- toolResult{
					index:  idx,
					result: ToolResultPart(call.ToolCallID, call.ToolName, denied, true),
					failed: true,
				}
				return
			}

			output, isErr, err := p.tools.Execute(ctx, call.ToolName, json.RawMessage(call.ToolArgs), p.config.SessionID)
			if err != nil {
				p.publishToolResult(call, err.Error(), true)
				ch <- toolResult{
					index:  idx,
					result: ToolResultPart(call.ToolCallID, call.ToolName, err.Error(), true),
					failed: true,
				}
				return
			}

			p.publishToolResult(call, output, isErr)
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
	collected := 0
	for collected < len(toolCalls) {
		select {
		case tr := <-ch:
			results[tr.index] = tr.result
			if !tr.failed {
				allFailed = false
			}
			collected++
		case <-ctx.Done():
			// Context cancelled (abort). Fill uncollected slots with abort markers
			// so the message has valid tool results. The next iteration's
			// checkAbortAndContext will stop the processor.
			for i := range results {
				if results[i].Type == "" {
					results[i] = ToolResultPart(toolCalls[i].ToolCallID, toolCalls[i].ToolName, "aborted", true)
					p.publishToolResult(toolCalls[i], "aborted", true)
				}
			}
			return results, true
		}
	}

	return results, allFailed
}

// publishToolResult emits session.tool.result after a tool finishes executing
// so headless/JSON consumers can observe real output (distinct from LLM call-end).
func (p *Processor) publishToolResult(call Part, output string, isError bool) {
	if p.bus == nil {
		return
	}
	p.bus.Publish("session.tool.result", map[string]any{
		"sessionID":  p.config.SessionID,
		"toolCallID": call.ToolCallID,
		"toolName":   call.ToolName,
		"output":     output,
		"isError":    isError,
	})
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
