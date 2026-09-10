package session

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	MinPreserveRecentTokens = 2000
	MaxPreserveRecentTokens = 15000

	compactionCircuitBreakerThreshold = 3

	// Truncation limits for compaction prompt serialization.
	maxTextChars       = 2000
	maxToolArgsChars   = 500
	maxToolOutputChars = 2000

	// Number of most recent tool outputs to preserve when masking observations.
	preserveRecentOutputs = 5

	summaryTemplate = `<task>
Summarize the conversation so far. The summary will be used in place of the original conversation to continue assisting the user.

Pay special attention to:
1. The user's original request and intent — so we can verify our work truly addresses it, not just technically "works"
2. Key technical concepts, architectural decisions, and design patterns discussed
3. Important file paths, function names, and code snippets that were central to the conversation
4. Any errors encountered and how they were resolved
5. The current state of the implementation and any pending tasks
6. Any user preferences, constraints, or requirements that were mentioned

Format the summary with these sections:
1. **Primary Request and Intent** - What the user ultimately wants to achieve
2. **Key Technical Concepts** - Important technical details that inform the work
3. **Files and Code Sections** - Specific files, functions, and code that were discussed or modified
4. **Errors and fixes** - Problems encountered and their solutions
5. **Problem Solving** - Key decisions made and their rationale
6. **All user messages** - Every distinct request or instruction the user gave, in order
7. **Pending Tasks** - Work that still needs to be done
8. **Current Work** - What was being worked on when summarization was triggered
9. **Optional Next Step** - The most logical next action to take

If there is a prior summary, incorporate it into the new summary while emphasizing recent developments.
</task>`
)

var (
	readFileRe  = regexp.MustCompile(`(?:read|cat|head|tail)\s+["']?([^\s"']+)["']?`)
	writeFileRe = regexp.MustCompile(`(?:write|edit|patch)\s+["']?([^\s"']+)["']?`)
)

type CompactionConfig struct {
	MaskObservations bool
	MinPreserve      int
	MaxPreserve      int
	MaxMessages      int
}

const defaultMaxMessages = 80

func DefaultCompactionConfig() CompactionConfig {
	return CompactionConfig{
		MaskObservations: true,
		MinPreserve:      MinPreserveRecentTokens,
		MaxPreserve:      MaxPreserveRecentTokens,
		MaxMessages:      defaultMaxMessages,
	}
}

type CompactionResult struct {
	Summary       string
	ReadFiles     []string
	ModifiedFiles []string
	PreTokens     int
	PostTokens    int
	CompactionNum int
}

func trackFiles(messages []Message) (readFiles, modifiedFiles []string) {
	readSet := make(map[string]bool)
	modifiedSet := make(map[string]bool)

	for _, msg := range messages {
		for _, part := range msg.Parts {
			switch part.Type {
			case PartToolCall:
				switch part.ToolName {
				case "read":
					if path := extractToolArgPath(part.ToolArgs); path != "" {
						readSet[path] = true
					}
				case "write", "edit", "patch", "apply_patch":
					if path := extractToolArgPath(part.ToolArgs); path != "" {
						modifiedSet[path] = true
					}
				case "shell", "bash":
					extractShellFiles(part.ToolArgs, readSet, modifiedSet)
				}
			}
		}
	}

	for path := range readSet {
		readFiles = append(readFiles, path)
	}
	for path := range modifiedSet {
		modifiedFiles = append(modifiedFiles, path)
	}
	return readFiles, modifiedFiles
}

func extractToolArgPath(argsJSON string) string {
	for _, key := range []string{`"file_path"`, `"path"`, `"file"`} {
		idx := strings.Index(argsJSON, key)
		if idx < 0 {
			continue
		}
		rest := argsJSON[idx+len(key):]
		colonIdx := strings.Index(rest, ":")
		if colonIdx < 0 {
			continue
		}
		rest = rest[colonIdx+1:]
		rest = strings.TrimSpace(rest)
		if len(rest) > 0 && rest[0] == '"' {
			rest = rest[1:]
			endQuote := strings.Index(rest, `"`)
			if endQuote > 0 {
				return rest[:endQuote]
			}
		}
	}
	return ""
}

func extractShellFiles(argsJSON string, readSet, modifiedSet map[string]bool) {
	cmdIdx := strings.Index(argsJSON, `"command"`)
	if cmdIdx < 0 {
		return
	}
	rest := argsJSON[cmdIdx:]
	colonIdx := strings.Index(rest, ":")
	if colonIdx < 0 {
		return
	}
	rest = strings.TrimSpace(rest[colonIdx+1:])
	if len(rest) > 0 && rest[0] == '"' {
		rest = rest[1:]
		endQuote := strings.Index(rest, `"`)
		if endQuote > 0 {
			cmd := rest[:endQuote]
			for _, m := range readFileRe.FindAllStringSubmatch(cmd, -1) {
				readSet[m[1]] = true
			}
			for _, m := range writeFileRe.FindAllStringSubmatch(cmd, -1) {
				modifiedSet[m[1]] = true
			}
		}
	}
}

func maskObservations(messages []Message) []Message {
	// Count total tool results to determine which to preserve.
	var totalResults int
	for _, msg := range messages {
		for _, part := range msg.Parts {
			if part.Type == PartToolResult {
				totalResults++
			}
		}
	}

	// Preserve the last preserveRecentOutputs tool results.
	preserveFrom := totalResults - preserveRecentOutputs
	if preserveFrom < 0 {
		preserveFrom = 0
	}

	resultIdx := 0
	result := make([]Message, len(messages))
	for i, msg := range messages {
		result[i] = Message{
			ID:        msg.ID,
			SessionID: msg.SessionID,
			Role:      msg.Role,
			Model:     msg.Model,
			Tokens:    msg.Tokens,
			CreatedAt: msg.CreatedAt,
		}
		parts := make([]Part, len(msg.Parts))
		copy(parts, msg.Parts)
		for j := range parts {
			if parts[j].Type == PartToolResult {
				if resultIdx < preserveFrom {
					parts[j] = Part{
						Type:       PartToolResult,
						ToolCallID: parts[j].ToolCallID,
						ToolName:   parts[j].ToolName,
						ToolResult: "[output masked for compaction]",
						ToolError:  parts[j].ToolError,
					}
				}
				resultIdx++
			}
		}
		result[i].Parts = parts
	}
	return result
}

// truncate shortens s to maxLen characters, appending a truncation marker if needed.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "... [truncated]"
}

func buildCompactionPrompt(messages []Message, priorSummary string, readFiles, modifiedFiles []string) string {
	var sb strings.Builder

	sb.WriteString(summaryTemplate)
	sb.WriteString("\n\n<conversation>\n")

	if priorSummary != "" {
		sb.WriteString("<prior-summary>\n")
		sb.WriteString(priorSummary)
		sb.WriteString("\n</prior-summary>\n\n")
	}

	for _, msg := range messages {
		sb.WriteString(fmt.Sprintf("<%s>\n", msg.Role))
		for _, part := range msg.Parts {
			switch part.Type {
			case PartText:
				sb.WriteString(truncate(part.Text, maxTextChars))
				sb.WriteString("\n")
			case PartToolCall:
				sb.WriteString(fmt.Sprintf("[tool_call: %s(%s)]\n", part.ToolName, truncate(part.ToolArgs, maxToolArgsChars)))
			case PartToolResult:
				sb.WriteString(fmt.Sprintf("[tool_result: %s = %s]\n", part.ToolName, truncate(part.ToolResult, maxToolOutputChars)))
			case PartReasoning:
				sb.WriteString(fmt.Sprintf("[reasoning: %s]\n", truncate(part.Text, maxTextChars)))
			}
		}
		sb.WriteString(fmt.Sprintf("</%s>\n", msg.Role))
	}

	sb.WriteString("</conversation>\n")

	if len(readFiles) > 0 {
		sb.WriteString("\n<read-files>\n")
		for _, f := range readFiles {
			sb.WriteString(f)
			sb.WriteString("\n")
		}
		sb.WriteString("</read-files>\n")
	}

	if len(modifiedFiles) > 0 {
		sb.WriteString("\n<modified-files>\n")
		for _, f := range modifiedFiles {
			sb.WriteString(f)
			sb.WriteString("\n")
		}
		sb.WriteString("</modified-files>\n")
	}

	return sb.String()
}

type LazyEstimator struct {
	tokensPerChar float64
}

func NewLazyEstimator(tokensPerChar float64) *LazyEstimator {
	if tokensPerChar <= 0 {
		tokensPerChar = 0.25 // ~4 chars per token default
	}
	return &LazyEstimator{tokensPerChar: tokensPerChar}
}

func (e *LazyEstimator) EstimateMessage(msg *Message) int {
	total := 0
	for _, part := range msg.Parts {
		switch part.Type {
		case PartText:
			total += int(float64(len(part.Text)) * e.tokensPerChar)
		case PartToolCall:
			total += int(float64(len(part.ToolName)+len(part.ToolArgs)) * e.tokensPerChar)
		case PartToolResult:
			total += int(float64(len(part.ToolResult)) * e.tokensPerChar)
		case PartReasoning:
			total += int(float64(len(part.Text)) * e.tokensPerChar)
		}
	}
	if total < 4 {
		total = 4
	}
	return total
}

func (e *LazyEstimator) FindPreserveBoundary(messages []Message, budgetTokens int) int {
	budget := budgetTokens
	if budget < MinPreserveRecentTokens {
		budget = MinPreserveRecentTokens
	}
	if budget > MaxPreserveRecentTokens {
		budget = MaxPreserveRecentTokens
	}

	spent := 0
	for i := len(messages) - 1; i >= 0; i-- {
		cost := e.EstimateMessage(&messages[i])
		if spent+cost > budget {
			return i + 1
		}
		spent += cost
	}
	return 0
}
