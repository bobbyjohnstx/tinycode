package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

func TestPluginID(t *testing.T) {
	p := newPlugin()
	if p.ID != "snippets" {
		t.Errorf("expected plugin ID 'snippets', got %q", p.ID)
	}
}

func TestPluginHasTwoTools(t *testing.T) {
	p := newPlugin()
	if len(p.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(p.Tools))
	}
	expected := []string{"snippet_list", "snippet_expand"}
	for i, name := range expected {
		if p.Tools[i].Name != name {
			t.Errorf("tool[%d]: expected %q, got %q", i, name, p.Tools[i].Name)
		}
	}
}

func TestBuiltinTemplates(t *testing.T) {
	templates := builtinTemplates()
	expected := []string{"deployment", "service", "route", "configmap", "pvc"}
	for _, name := range expected {
		if _, ok := templates[name]; !ok {
			t.Errorf("missing built-in template %q", name)
		}
	}
}

func TestExpandTemplateFullSubstitution(t *testing.T) {
	content := "name: {{name}}, ns: {{namespace}}"
	vars := map[string]string{"name": "myapp", "namespace": "prod"}
	result, unresolved := expandTemplate(content, vars)
	if result != "name: myapp, ns: prod" {
		t.Errorf("unexpected result: %q", result)
	}
	if len(unresolved) != 0 {
		t.Errorf("expected no unresolved, got %v", unresolved)
	}
}

func TestExpandTemplatePartialSubstitution(t *testing.T) {
	content := "name: {{name}}, ns: {{namespace}}, img: {{image}}"
	vars := map[string]string{"name": "myapp"}
	result, unresolved := expandTemplate(content, vars)
	if result != "name: myapp, ns: {{namespace}}, img: {{image}}" {
		t.Errorf("unexpected result: %q", result)
	}
	if len(unresolved) != 2 {
		t.Errorf("expected 2 unresolved, got %d: %v", len(unresolved), unresolved)
	}
}

func TestExpandTemplateDeduplicatesUnresolved(t *testing.T) {
	content := "a: {{name}}, b: {{name}}"
	result, unresolved := expandTemplate(content, map[string]string{})
	if result != content {
		t.Errorf("unexpected result: %q", result)
	}
	if len(unresolved) != 1 {
		t.Errorf("expected 1 unresolved (deduplicated), got %d: %v", len(unresolved), unresolved)
	}
}

func TestExpandTemplateNoVariables(t *testing.T) {
	content := "static content"
	result, unresolved := expandTemplate(content, map[string]string{})
	if result != "static content" {
		t.Errorf("unexpected result: %q", result)
	}
	if len(unresolved) != 0 {
		t.Errorf("expected no unresolved, got %v", unresolved)
	}
}

func TestSnippetListBuiltins(t *testing.T) {
	p := newPlugin()
	tool := p.Tools[0] // snippet_list
	result, err := tool.Execute(context.Background(), nil, plugin_toolContext())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, name := range []string{"deployment", "service", "route", "configmap", "pvc"} {
		if !contains(result, name) {
			t.Errorf("snippet_list output missing %q", name)
		}
	}
}

func TestSnippetExpandKnownTemplate(t *testing.T) {
	p := newPlugin()
	tool := p.Tools[1] // snippet_expand
	args := json.RawMessage(`{"name":"service","variables":{"name":"web","namespace":"default","port":"8080"}}`)
	result, err := tool.Execute(context.Background(), args, plugin_toolContext())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(result, "name: web") {
		t.Errorf("expected expanded name, got: %s", result)
	}
	if !contains(result, "namespace: default") {
		t.Errorf("expected expanded namespace, got: %s", result)
	}
	if contains(result, "Unresolved") {
		t.Errorf("expected no unresolved variables, got: %s", result)
	}
}

func TestSnippetExpandUnknownTemplate(t *testing.T) {
	p := newPlugin()
	tool := p.Tools[1]
	args := json.RawMessage(`{"name":"nonexistent"}`)
	result, err := tool.Execute(context.Background(), args, plugin_toolContext())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(result, "Unknown template") {
		t.Errorf("expected unknown template message, got: %s", result)
	}
	if !contains(result, "Available templates:") {
		t.Errorf("expected available templates list, got: %s", result)
	}
}

func TestSnippetExpandWithUnresolvedVariables(t *testing.T) {
	p := newPlugin()
	tool := p.Tools[1]
	args := json.RawMessage(`{"name":"deployment","variables":{"name":"myapp"}}`)
	result, err := tool.Execute(context.Background(), args, plugin_toolContext())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(result, "Unresolved variables:") {
		t.Errorf("expected unresolved variables note, got: %s", result)
	}
}

func TestSnippetExpandEmptyName(t *testing.T) {
	p := newPlugin()
	tool := p.Tools[1]
	args := json.RawMessage(`{"name":""}`)
	_, err := tool.Execute(context.Background(), args, plugin_toolContext())
	if err == nil {
		t.Error("expected error for empty name")
	}
}

func TestSnippetExpandInvalidJSON(t *testing.T) {
	p := newPlugin()
	tool := p.Tools[1]
	args := json.RawMessage(`{invalid}`)
	_, err := tool.Execute(context.Background(), args, plugin_toolContext())
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLoadCustomTemplates(t *testing.T) {
	dir := t.TempDir()
	content := "apiVersion: v1\nkind: MyCustom\nmetadata:\n  name: {{name}}"
	if err := os.WriteFile(filepath.Join(dir, "custom.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "another.yml"), []byte("data: {{val}}"), 0644); err != nil {
		t.Fatal(err)
	}
	// Non-yaml file should be ignored.
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("ignore"), 0644); err != nil {
		t.Fatal(err)
	}

	templates := loadCustomTemplates(dir)
	if len(templates) != 2 {
		t.Fatalf("expected 2 custom templates, got %d", len(templates))
	}
	if templates["custom"].Content != content {
		t.Errorf("unexpected custom content: %q", templates["custom"].Content)
	}
	if templates["custom"].Description != "Custom template: custom" {
		t.Errorf("unexpected description: %q", templates["custom"].Description)
	}
	if _, ok := templates["another"]; !ok {
		t.Error("missing 'another' template from .yml file")
	}
}

func TestLoadCustomTemplatesEmptyDir(t *testing.T) {
	templates := loadCustomTemplates(t.TempDir())
	if len(templates) != 0 {
		t.Errorf("expected 0 templates from empty dir, got %d", len(templates))
	}
}

func TestLoadCustomTemplatesNonexistentDir(t *testing.T) {
	templates := loadCustomTemplates("/nonexistent/path/to/snippets")
	if len(templates) != 0 {
		t.Errorf("expected 0 templates from nonexistent dir, got %d", len(templates))
	}
}

func TestGetAllTemplatesCustomOverridesBuiltin(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "service.yaml"), []byte("custom service"), 0644); err != nil {
		t.Fatal(err)
	}
	templates := getAllTemplates(dir)
	if templates["service"].Content != "custom service" {
		t.Errorf("expected custom override, got: %q", templates["service"].Content)
	}
	// Other builtins should still exist.
	if _, ok := templates["deployment"]; !ok {
		t.Error("missing deployment template after custom override")
	}
}

func TestToolSchema(t *testing.T) {
	p := newPlugin()
	expandTool := p.Tools[1]
	params := expandTool.Parameters

	typ, ok := params["type"]
	if !ok || typ != "object" {
		t.Errorf("expected type 'object', got %v", typ)
	}

	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties to be map[string]any")
	}
	if _, ok := props["name"]; !ok {
		t.Error("missing 'name' property in schema")
	}
	if _, ok := props["variables"]; !ok {
		t.Error("missing 'variables' property in schema")
	}

	required, ok := params["required"].([]string)
	if !ok {
		t.Fatal("expected required to be []string")
	}
	if len(required) != 1 || required[0] != "name" {
		t.Errorf("expected required [name], got %v", required)
	}
}

// helpers

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func plugin_toolContext() plugin.ToolContext {
	return plugin.ToolContext{}
}
