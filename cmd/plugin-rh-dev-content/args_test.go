package main

import (
	"context"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

func TestRhDevRecentRejectsMalformedArgs(t *testing.T) {
	var tool plugin.ToolDef
	for _, candidate := range newPlugin().Tools {
		if candidate.Name == "rh_dev_recent" {
			tool = candidate
			break
		}
	}
	if tool.Execute == nil {
		t.Fatal("rh_dev_recent missing")
	}
	_, err := tool.Execute(context.Background(), []byte("{"), plugin.ToolContext{})
	if err == nil || !strings.Contains(err.Error(), "parsing args") {
		t.Fatalf("expected parse error, got %v", err)
	}
}
