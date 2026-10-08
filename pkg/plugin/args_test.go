package plugin

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUnmarshalToolArgs(t *testing.T) {
	t.Parallel()

	var input struct {
		Role string `json:"role"`
	}

	if err := UnmarshalToolArgs(nil, &input); err != nil {
		t.Fatalf("empty args: %v", err)
	}
	if err := UnmarshalToolArgs(json.RawMessage("null"), &input); err != nil {
		t.Fatalf("null args: %v", err)
	}
	if err := UnmarshalToolArgs(json.RawMessage(`{"role":"worker"}`), &input); err != nil {
		t.Fatalf("object: %v", err)
	}
	if input.Role != "worker" {
		t.Fatalf("role = %q", input.Role)
	}
	if err := UnmarshalToolArgs(json.RawMessage("{"), &input); err == nil || !strings.Contains(err.Error(), "parsing args") {
		t.Fatalf("expected parse error, got %v", err)
	}
}
