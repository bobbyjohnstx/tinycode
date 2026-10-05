package session

import (
	"context"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/llm"
	"github.com/bobbyjohnstx/tinycode/internal/provider"
)

func TestCompactionOutputReserve(t *testing.T) {
	tests := []struct {
		name        string
		contextLen  int
		outputLimit int
		want        int
	}{
		{name: "large window keeps 20k", contextLen: 100000, outputLimit: 4096, want: 20000},
		{name: "32k window keeps 20k", contextLen: 32768, outputLimit: 4096, want: 20000},
		{name: "output limit above 20k is kept when it fits", contextLen: 128000, outputLimit: 64000, want: 64000},
		{name: "8k window uses a fifth with a 2048 floor", contextLen: 8192, outputLimit: 4096, want: 2048},
		{name: "4k window uses the 2048 floor", contextLen: 4096, outputLimit: 2048, want: 2048},
		{name: "tiny window stays under the context", contextLen: 1500, outputLimit: 512, want: 750},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compactionOutputReserve(tt.contextLen, tt.outputLimit)
			if got != tt.want {
				t.Errorf("compactionOutputReserve(%d, %d) = %d, want %d", tt.contextLen, tt.outputLimit, got, tt.want)
			}
		})
	}
}

func TestCheckCompaction_SmallContextElides(t *testing.T) {
	// context 8192, reserve 2048, threshold 6144, soft threshold 4915.
	// 5000 input tokens is above the soft line and below compaction.
	client := &mockLLMClient{
		responses: []mockResponse{
			{events: []llm.Event{
				{Type: llm.EventTextDelta, Text: "response"},
				{Type: llm.EventFinish, FinishReason: "stop", Usage: &llm.Usage{PromptTokens: 5000, CompletionTokens: 100}},
			}},
		},
	}

	b := bus.New()
	defer b.Close()

	p := NewProcessor(ProcessorConfig{
		SessionID:       "ses_small",
		Model:           &provider.Model{ID: "local", Limit: provider.ModelLimit{Context: 8192, Output: 4096}},
		Compaction:      DefaultCompactionConfig(),
		AutoContinueMax: -1,
	}, client, &stubToolExecutor{}, b)

	var messages []Message
	for i := 0; i < 10; i++ {
		messages = append(messages, Message{
			Parts: []Part{
				ToolResultPart("c", "read", "output", false),
			},
		})
	}
	p.SetMessages(messages)

	result := p.Process(context.Background(), "test query")
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if !p.elisionDone {
		t.Error("expected elision on an 8192-token window")
	}
	if p.compactionCount != 0 {
		t.Errorf("expected compactionCount=0, got %d", p.compactionCount)
	}
}
