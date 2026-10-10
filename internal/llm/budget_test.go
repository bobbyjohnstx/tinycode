package llm

import (
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

func TestStreamBudgetHit(t *testing.T) {
	if StreamBudgetHit(10, 4096, time.Second, 5*time.Minute) {
		t.Error("a short stream should continue")
	}
	if !StreamBudgetHit(4096, 4096, time.Second, 5*time.Minute) {
		t.Error("token cap should stop the stream")
	}
	if !StreamBudgetHit(10, 4096, 5*time.Minute, 5*time.Minute) {
		t.Error("wall clock should stop a slow stream")
	}
}
