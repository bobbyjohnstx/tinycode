package llm

type EventType string

const (
	EventTextDelta     EventType = "text-delta"
	EventToolCallBegin EventType = "tool-call-begin"
	EventToolCallDelta EventType = "tool-call-delta"
	EventToolCallEnd   EventType = "tool-call-end"
	EventFinish        EventType = "finish"
	EventError         EventType = "error"
)

type Event struct {
	Type EventType `json:"type"`

	// text-delta
	Text string `json:"text,omitempty"`

	// tool-call fields
	ToolCallID   string `json:"toolCallID,omitempty"`
	ToolName     string `json:"toolName,omitempty"`
	ToolCallArgs string `json:"toolCallArgs,omitempty"`

	// finish
	FinishReason string `json:"finishReason,omitempty"`
	Usage        *Usage `json:"usage,omitempty"`

	// error
	Error error `json:"-"`
}

type Usage struct {
	PromptTokens     int `json:"promptTokens"`
	CompletionTokens int `json:"completionTokens"`
	TotalTokens      int `json:"totalTokens"`
}

type Message struct {
	Role       string     `json:"role"`
	Content    any        `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}
