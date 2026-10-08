package main

import (
	"context"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

func TestRhacsRejectsMalformedArgs(t *testing.T) {
	p := newPlugin(options{CentralURL: "https://central.example.com", APIToken: "t"})
	var tool plugin.ToolDef
	for _, candidate := range p.Tools {
		if candidate.Name == "rhacs_violations" {
			tool = candidate
			break
		}
	}
	if tool.Execute == nil {
		t.Fatal("rhacs_violations missing")
	}
	_, err := tool.Execute(context.Background(), []byte("{"), plugin.ToolContext{})
	if err == nil || !strings.Contains(err.Error(), "parsing args") {
		t.Fatalf("expected parse error, got %v", err)
	}
}
