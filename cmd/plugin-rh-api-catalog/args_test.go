package main

import (
	"context"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

func TestRhAPIListRejectsMalformedArgs(t *testing.T) {
	var tool plugin.ToolDef
	for _, candidate := range newPlugin(options{}).Tools {
		if candidate.Name == "rh_api_list" {
			tool = candidate
			break
		}
	}
	if tool.Execute == nil {
		t.Fatal("rh_api_list missing")
	}
	_, err := tool.Execute(context.Background(), []byte("{"), plugin.ToolContext{})
	if err == nil || !strings.Contains(err.Error(), "parsing args") {
		t.Fatalf("expected parse error, got %v", err)
	}
}
