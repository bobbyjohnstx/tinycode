package tool

import (
	"encoding/json"
	"testing"
	"time"
)

func TestBodyTimeout_QuestionUsesParentOnly(t *testing.T) {
	d, ok := bodyTimeout("question", json.RawMessage(`{}`))
	if ok {
		t.Fatalf("question must not set body timeout, got %v", d)
	}
}

func TestBodyTimeout_DefaultIs60s(t *testing.T) {
	d, ok := bodyTimeout("read", json.RawMessage(`{}`))
	if !ok || d != 60*time.Second {
		t.Fatalf("got %v ok=%v, want 60s", d, ok)
	}
}

func TestBodyTimeout_BashDefaultIsMax60And120(t *testing.T) {
	d, ok := bodyTimeout("bash", json.RawMessage(`{"command":"true"}`))
	if !ok || d != 120*time.Second {
		t.Fatalf("got %v ok=%v, want 120s", d, ok)
	}
}

func TestBodyTimeout_BashExplicitBelow60Becomes60(t *testing.T) {
	d, ok := bodyTimeout("bash", json.RawMessage(`{"command":"true","timeout":10000}`))
	if !ok || d != 60*time.Second {
		t.Fatalf("got %v, want 60s", d)
	}
}

func TestBodyTimeout_BashExplicitAbove60Honored(t *testing.T) {
	d, ok := bodyTimeout("bash", json.RawMessage(`{"command":"true","timeout":180000}`))
	if !ok || d != 180*time.Second {
		t.Fatalf("got %v, want 180s", d)
	}
}
