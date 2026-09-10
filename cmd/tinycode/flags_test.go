package main

import "testing"

func TestParseCommonFlags_ShortModel(t *testing.T) {
	flags := parseCommonFlags("test", []string{"-m", "ollama/test"})
	if flags.model != "ollama/test" {
		t.Errorf("expected model %q, got %q", "ollama/test", flags.model)
	}
}

func TestParseCommonFlags_LongModel(t *testing.T) {
	flags := parseCommonFlags("test", []string{"--model", "ollama/test"})
	if flags.model != "ollama/test" {
		t.Errorf("expected model %q, got %q", "ollama/test", flags.model)
	}
}

func TestParseCommonFlags_NoArgs(t *testing.T) {
	flags := parseCommonFlags("test", nil)
	if flags.model != "" {
		t.Errorf("expected empty model, got %q", flags.model)
	}
}

func TestParseCommonFlags_EmptySlice(t *testing.T) {
	flags := parseCommonFlags("test", []string{})
	if flags.model != "" {
		t.Errorf("expected empty model, got %q", flags.model)
	}
}
