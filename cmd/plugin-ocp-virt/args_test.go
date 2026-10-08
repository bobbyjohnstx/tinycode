package main

import (
	"context"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

func TestVirtRejectsMalformedArgs(t *testing.T) {
	tool := newPlugin().Tools[0]
	_, err := tool.Execute(context.Background(), []byte("{"), plugin.ToolContext{})
	if err == nil || !strings.Contains(err.Error(), "parsing args") {
		t.Fatalf("expected parse error, got %v", err)
	}
}
