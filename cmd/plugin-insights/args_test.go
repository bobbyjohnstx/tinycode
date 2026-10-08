package main

import (
	"context"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

func TestInsightsMemoryRejectsMalformedArgs(t *testing.T) {
	st := &state{}
	st.set(t.TempDir())
	_, err := buildInsightsMemory(st).Execute(context.Background(), []byte("{"), plugin.ToolContext{})
	if err == nil || !strings.Contains(err.Error(), "parsing args") {
		t.Fatalf("expected parse error, got %v", err)
	}
}
