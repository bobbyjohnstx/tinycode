package llm

import (
	"strings"
	"testing"
	"time"
)

func TestApproxTokens(t *testing.T) {
	if got := ApproxTokens(""); got != 0 {
		t.Errorf("empty = %d, want 0", got)
	}
	if got := ApproxTokens("abcd"); got != 1 {
		t.Errorf("4 bytes = %d, want 1", got)
	}
	if got := ApproxTokens("abcde"); got != 2 {
		t.Errorf("5 bytes = %d, want 2", got)
	}
}

func TestReasoningLoop(t *testing.T) {
	if ReasoningLoop("But wait, the footer tag is fine. The handler returns the error.") {
		t.Error("one second guess is not a loop")
	}
	var stalled strings.Builder
	for i := 0; i < 4; i++ {
		stalled.WriteString("But wait, index.html still has the same unclosed footer. ")
	}
	if !ReasoningLoop(stalled.String()) {
		t.Error("repeated but wait should stop the trace")
	}

	sentence := "Looking at templates/index.html, the footer div is never closed and the form posts to the wrong path."
	var repeated strings.Builder
	for i := 0; i < 3; i++ {
		repeated.WriteString(sentence)
		repeated.WriteString(" ")
	}
	if !ReasoningLoop(repeated.String()) {
		t.Error("the same long sentence three times should stop the trace")
	}
}

func TestStreamBudgetHit(t *testing.T) {
	if StreamBudgetHit(10, DefaultStreamTokens, time.Second, StreamWallClock) {
		t.Error("a short stream should continue")
	}
	if !StreamBudgetHit(DefaultStreamTokens, DefaultStreamTokens, time.Second, StreamWallClock) {
		t.Error("token cap should stop the stream")
	}
	if !StreamBudgetHit(10, DefaultStreamTokens, StreamWallClock, StreamWallClock) {
		t.Error("wall clock should stop a slow stream")
	}
}
