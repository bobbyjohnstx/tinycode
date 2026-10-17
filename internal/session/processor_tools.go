package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/bobbyjohnstx/tinycode/internal/llm"
	"github.com/bobbyjohnstx/tinycode/internal/safego"
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

const maxConcurrentTools = 8

func (p *Processor) executeTools(ctx context.Context, toolCalls []Part) ([]Part, bool) {
	type toolResult struct {
		index  int
		result Part
		failed bool
	}

	type questionGate struct {
		wait <-chan struct{}
		done chan struct{}
	}

	// Serialize "question" tools by call-index order; other tools stay parallel.
	gates := make(map[int]questionGate)
	first := make(chan struct{})
	close(first)
	prevQuestionDone := (<-chan struct{})(first)
	for i, tc := range toolCalls {
		if tc.ToolName != "question" {
			continue
		}
		done := make(chan struct{})
		gates[i] = questionGate{wait: prevQuestionDone, done: done}
		prevQuestionDone = done
	}

	results := make([]Part, len(toolCalls))
	ch := make(chan toolResult, len(toolCalls))
	sem := make(chan struct{}, maxConcurrentTools)

	var wg sync.WaitGroup
launch:
	for i, tc := range toolCalls {
		select {
		case <-ctx.Done():
			break launch
		default:
		}
		wg.Add(1)
		idx, call := i, tc
		safego.Go(func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					// Domain recover must stay: safego.Go alone would swallow
					// the panic without sending on ch and hang the collector.
					safego.LogPanic(nil, "tool panicked", r, "tool", call.ToolName)
					msg := fmt.Sprintf("tool panicked: %v", r)
					p.publishToolResult(call, msg, true)
					ch <- toolResult{
						index:  idx,
						result: ToolResultPart(call.ToolCallID, call.ToolName, msg, true),
						failed: true,
					}
				}
			}()

			// Wait on prior questions before taking a concurrency slot so
			// idle question waiters do not starve non-question tools.
			if g, ok := gates[idx]; ok {
				select {
				case <-g.wait:
				case <-ctx.Done():
					p.publishToolResult(call, "aborted", true)
					ch <- toolResult{
						index:  idx,
						result: ToolResultPart(call.ToolCallID, call.ToolName, "aborted", true),
						failed: true,
					}
					close(g.done)
					return
				}
				defer close(g.done)
			}

			select {
			case <-ctx.Done():
				p.publishToolResult(call, "aborted", true)
				ch <- toolResult{
					index:  idx,
					result: ToolResultPart(call.ToolCallID, call.ToolName, "aborted", true),
					failed: true,
				}
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()

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
		})
	}

	safego.Go(func() {
		wg.Wait()
		close(ch)
	})

	allFailed := true
	collected := 0
	for collected < len(toolCalls) {
		select {
		case tr, ok := <-ch:
			if !ok {
				goto fillAborted
			}
			results[tr.index] = tr.result
			if !tr.failed {
				allFailed = false
			}
			collected++
		case <-ctx.Done():
			// Abort: wait for in-flight workers (bounded by Registry body policy /
			// parent cancel) so we do not leak goroutines or double-publish results.
			for tr := range ch {
				if results[tr.index].Type == "" {
					results[tr.index] = tr.result
					if !tr.failed {
						allFailed = false
					}
				}
			}
			goto fillAborted
		}
	}
	return results, allFailed

fillAborted:
	allFailed = true
	for i := range results {
		if results[i].Type == "" {
			results[i] = ToolResultPart(toolCalls[i].ToolCallID, toolCalls[i].ToolName, "aborted", true)
			p.publishToolResult(toolCalls[i], "aborted", true)
		}
		if !results[i].ToolError {
			allFailed = false
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
