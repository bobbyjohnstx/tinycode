package session

import (
	"strings"
)

// toolIntentPhrases are patterns that indicate the model described a tool
// action in text instead of emitting a tool_call.
var toolIntentPhrases = []string{
	"i will write",
	"i'll write",
	"let me write",
	"let me create the file",
	"let me create a file",
	"i will create the file",
	"i will create a file",
	"i'll create the file",
	"i'll create a file",
	"i'll save the",
	"i will save the",
	"i'll save it to",
	"i will save it to",
	"writing the file",
	"writing the findings",
	"writing the results",
	"saving the file",
	"saving the findings",
	"i will edit the",
	"i'll edit the",
	"let me edit the",
	"i will update the file",
	"i'll update the file",
}

const toolIntentHint = "You described a file operation in text but did not call a tool. " +
	"To create or modify files, use the write, edit, or apply_patch tool — do not just describe the action."

// detectToolIntent returns true if the assistant text contains phrases
// suggesting the model intended to use a tool but emitted text instead.
func detectToolIntent(text string) bool {
	lower := strings.ToLower(text)
	for _, phrase := range toolIntentPhrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

// extractAssistantText returns the concatenated text parts from a message.
func extractAssistantText(msg *Message) string {
	if msg == nil {
		return ""
	}
	var sb strings.Builder
	for _, p := range msg.Parts {
		if p.Type == "text" && p.Text != "" {
			sb.WriteString(p.Text)
			sb.WriteString(" ")
		}
	}
	return sb.String()
}

// injectToolIntentHint appends a user-role hint message so the model
// retries the action with an explicit nudge to use tools.
func (p *Processor) injectToolIntentHint() {
	hint := Message{
		Role: "user",
		Parts: []Part{
			{Type: "text", Text: toolIntentHint},
		},
	}
	p.mu.Lock()
	p.messages = append(p.messages, hint)
	p.mu.Unlock()
}
