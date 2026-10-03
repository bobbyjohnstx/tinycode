package tui

// ContextBreakdown holds per-category token estimates.
type ContextBreakdown struct {
	SystemTokens     int
	UserTokens       int
	AssistantTokens  int
	ToolCallTokens   int
	ToolResultTokens int
	TotalTokens      int
	ContextLimit     int
	Percent          int
	LargestItem      string
	LargestSize      int
	Suggestion       string
}

// AnalyzeContext walks conversation messages and estimates tokens per category.
// It uses 0.25 tokens per character as the estimation heuristic (same as LazyEstimator).
// If apiTokens > 0, the total is taken from the API; otherwise the estimate is used.
func AnalyzeContext(messages []MessageView, contextLimit int, apiTokens int) ContextBreakdown {
	var b ContextBreakdown
	b.ContextLimit = contextLimit

	type bucket struct {
		name   string
		tokens int
	}
	var buckets []bucket

	for _, msg := range messages {
		size := estimateMessageTokens(msg)
		switch msg.Info.Role {
		case "system":
			b.SystemTokens += size
		case "user":
			b.UserTokens += size
		case "assistant":
			b.AssistantTokens += size
		default:
			// Categorize tool_call and tool_result parts.
			for _, p := range msg.Parts {
				switch p.Type {
				case "tool_call":
					b.ToolCallTokens += estimateChars(p.ToolName + p.ToolArgs)
				case "tool_result":
					b.ToolResultTokens += estimateChars(p.Text)
				}
			}
		}

		// Separate tool parts from assistant/user messages too.
		for _, p := range msg.Parts {
			switch p.Type {
			case "tool_call":
				tc := estimateChars(p.ToolName + p.ToolArgs)
				buckets = append(buckets, bucket{name: "tool_call:" + p.ToolName, tokens: tc})
				// Shift from role bucket to tool bucket.
				if msg.Info.Role == "assistant" {
					b.AssistantTokens -= tc
					b.ToolCallTokens += tc
				}
			case "tool_result":
				tr := estimateChars(p.Text)
				buckets = append(buckets, bucket{name: "tool_result:" + p.ToolName, tokens: tr})
				if msg.Info.Role == "assistant" {
					b.AssistantTokens -= tr
					b.ToolResultTokens += tr
				}
			}
		}
	}

	estimated := b.SystemTokens + b.UserTokens + b.AssistantTokens + b.ToolCallTokens + b.ToolResultTokens
	if apiTokens > 0 {
		b.TotalTokens = apiTokens
	} else {
		b.TotalTokens = estimated
	}

	if b.ContextLimit > 0 && b.TotalTokens > 0 {
		b.Percent = b.TotalTokens * 100 / b.ContextLimit
	}

	// Track largest single item across role buckets and tool parts.
	roleBuckets := []bucket{
		{name: "System messages", tokens: b.SystemTokens},
		{name: "User messages", tokens: b.UserTokens},
		{name: "Assistant messages", tokens: b.AssistantTokens},
		{name: "Tool calls", tokens: b.ToolCallTokens},
		{name: "Tool results", tokens: b.ToolResultTokens},
	}
	all := append(roleBuckets, buckets...)
	for _, bk := range all {
		if bk.tokens > b.LargestSize {
			b.LargestSize = bk.tokens
			b.LargestItem = bk.name
		}
	}

	if b.Percent > 70 {
		b.Suggestion = "Context usage is high. Consider running /compact to summarize and free space."
	}

	return b
}

// estimateMessageTokens estimates tokens for a full message using 0.25 tok/char.
func estimateMessageTokens(msg MessageView) int {
	total := 0
	for _, p := range msg.Parts {
		switch p.Type {
		case "tool_call":
			total += estimateChars(p.ToolName + p.ToolArgs)
		case "tool_result":
			total += estimateChars(p.Text)
		default:
			total += estimateChars(p.Text)
		}
	}
	return total
}

// estimateChars converts character count to estimated token count (0.25 tok/char).
func estimateChars(s string) int {
	n := len(s)
	return (n + 3) / 4 // ceiling division by 4
}
