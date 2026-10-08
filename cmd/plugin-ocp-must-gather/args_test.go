package main

import (
	"context"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode/pkg/mustgather"
	"github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

func TestMgNodesRejectsMalformedArgs(t *testing.T) {
	st := &state{}
	st.set(&mustgather.Root{Path: t.TempDir()})
	_, err := buildMgNodes(st).Execute(context.Background(), []byte("{"), plugin.ToolContext{})
	if err == nil || !strings.Contains(err.Error(), "parsing args") {
		t.Fatalf("expected parse error, got %v", err)
	}
	_, err = buildMgNodes(st).Execute(context.Background(), nil, plugin.ToolContext{})
	if err != nil && strings.Contains(err.Error(), "parsing args") {
		t.Fatalf("empty args should be accepted, got %v", err)
	}
}
