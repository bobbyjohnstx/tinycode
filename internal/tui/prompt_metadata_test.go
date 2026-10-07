package tui

import (
	"strings"
	"testing"
)

func TestPromptMetadata_DedupesModelLine(t *testing.T) {
	p := NewPromptInput(80)
	p.SetMetadata("general", "ornith-1.0-9b-mlx", "lm-studio")
	meta := p.renderMetadata()
	if meta != "" {
		t.Fatalf("expected empty metadata without thinking/effort/images, got %q", meta)
	}
	view := p.View()
	if strings.Contains(view, "ornith-1.0-9b-mlx") {
		t.Error("prompt view should not repeat model (status bar owns it)")
	}
	if strings.Contains(view, "lm-studio") {
		t.Error("prompt view should not repeat provider (status bar owns it)")
	}
}

func TestPromptMetadata_KeepsExtras(t *testing.T) {
	p := NewPromptInput(80)
	p.SetMetadata("build", "m", "p")
	p.SetThinkingLevel("high")
	p.SetEffortLevel("high")
	p.AddImage(2048)
	meta := p.renderMetadata()
	if !strings.Contains(meta, "thinking:high") {
		t.Fatalf("missing thinking: %q", meta)
	}
	if !strings.Contains(meta, "effort:high") {
		t.Fatalf("missing effort: %q", meta)
	}
	if !strings.Contains(meta, "image") {
		t.Fatalf("missing image: %q", meta)
	}
}
