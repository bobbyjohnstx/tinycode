package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

func TestSilenceUnconfiguredReturnsError(t *testing.T) {
	p := newPlugin(options{})
	var tool plugin.ToolDef
	for _, candidate := range p.Tools {
		if candidate.Name == "obs_alert_silence" {
			tool = candidate
			break
		}
	}
	_, err := tool.Execute(context.Background(), []byte(`{"alertName":"X","duration":"1h","comment":"c"}`), plugin.ToolContext{})
	if err == nil {
		t.Fatal("expected configuration error")
	}
}

func TestSilenceReturnsErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := newPlugin(options{PrometheusURL: srv.URL, AlertManagerURL: srv.URL})
	var tool plugin.ToolDef
	for _, candidate := range p.Tools {
		if candidate.Name == "obs_alert_silence" {
			tool = candidate
			break
		}
	}
	if tool.Execute == nil {
		t.Fatal("obs_alert_silence missing")
	}
	_, err := tool.Execute(context.Background(), []byte(`{"alertName":"X","duration":"nope","comment":"c"}`), plugin.ToolContext{})
	if err == nil || !strings.Contains(err.Error(), "invalid duration") {
		t.Fatalf("expected duration error, got %v", err)
	}
	_, err = tool.Execute(context.Background(), []byte(`{"alertName":"X","duration":"1h","comment":"c"}`), plugin.ToolContext{})
	if err == nil || !strings.Contains(err.Error(), "silence alert") {
		t.Fatalf("expected silence error, got %v", err)
	}
}
