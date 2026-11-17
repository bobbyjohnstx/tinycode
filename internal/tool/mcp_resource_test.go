package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

type mockMCPResourceService struct {
	resources []MCPResourceInfo
	templates []MCPResourceTemplateInfo
	content   map[string]string // uri -> content
	err       error
}

func (m *mockMCPResourceService) ListResources(_ context.Context) ([]MCPResourceInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.resources, nil
}

func (m *mockMCPResourceService) ListResourceTemplates(_ context.Context) ([]MCPResourceTemplateInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.templates, nil
}

func (m *mockMCPResourceService) ReadResource(_ context.Context, uri string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	if m.content != nil {
		if c, ok := m.content[uri]; ok {
			return c, nil
		}
	}
	return "", fmt.Errorf("resource not found: %s", uri)
}

func TestMCPResourceTool_ListResources(t *testing.T) {
	svc := &mockMCPResourceService{
		resources: []MCPResourceInfo{
			{URI: "file:///a.txt", Name: "A", Description: "File A"},
			{URI: "db://users", Name: "Users", MimeType: "application/json"},
		},
	}
	def := MCPResourceTool(svc)

	result, err := def.Execute(context.Background(), nil, json.RawMessage(`{"action":"list"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "2 resources") {
		t.Errorf("expected '2 resources' in output, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "file:///a.txt") {
		t.Errorf("expected URI in output, got: %s", result.Output)
	}
}

func TestMCPResourceTool_ListResources_Empty(t *testing.T) {
	svc := &mockMCPResourceService{}
	def := MCPResourceTool(svc)

	result, err := def.Execute(context.Background(), nil, json.RawMessage(`{"action":"list"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Error("empty list should not be an error")
	}
	if !strings.Contains(result.Output, "No resources") {
		t.Errorf("expected 'No resources' message, got: %s", result.Output)
	}
}

func TestMCPResourceTool_ListResources_TransportError(t *testing.T) {
	svc := &mockMCPResourceService{err: fmt.Errorf("connection refused")}
	def := MCPResourceTool(svc)

	result, err := def.Execute(context.Background(), nil, json.RawMessage(`{"action":"list"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for transport error")
	}
}

func TestMCPResourceTool_ListTemplates(t *testing.T) {
	svc := &mockMCPResourceService{
		templates: []MCPResourceTemplateInfo{
			{URITemplate: "file:///{path}", Name: "File", Description: "A file by path"},
		},
	}
	def := MCPResourceTool(svc)

	result, err := def.Execute(context.Background(), nil, json.RawMessage(`{"action":"templates"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "1 resource templates") {
		t.Errorf("expected '1 resource templates' in output, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "file:///{path}") {
		t.Errorf("expected template URI in output, got: %s", result.Output)
	}
}

func TestMCPResourceTool_ListTemplates_Empty(t *testing.T) {
	svc := &mockMCPResourceService{}
	def := MCPResourceTool(svc)

	result, err := def.Execute(context.Background(), nil, json.RawMessage(`{"action":"templates"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Error("empty templates should not be an error")
	}
}

func TestMCPResourceTool_ReadResource(t *testing.T) {
	svc := &mockMCPResourceService{
		content: map[string]string{
			"file:///hello.txt": "Hello, world!",
		},
	}
	def := MCPResourceTool(svc)

	result, err := def.Execute(context.Background(), nil, json.RawMessage(`{"action":"read","uri":"file:///hello.txt"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if result.Output != "Hello, world!" {
		t.Errorf("expected 'Hello, world!', got %q", result.Output)
	}
}

func TestMCPResourceTool_ReadResource_MissingURI(t *testing.T) {
	svc := &mockMCPResourceService{}
	def := MCPResourceTool(svc)

	result, err := def.Execute(context.Background(), nil, json.RawMessage(`{"action":"read"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for missing URI")
	}
}

func TestMCPResourceTool_ReadResource_NotFound(t *testing.T) {
	svc := &mockMCPResourceService{
		content: map[string]string{},
	}
	def := MCPResourceTool(svc)

	result, err := def.Execute(context.Background(), nil, json.RawMessage(`{"action":"read","uri":"file:///missing.txt"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for missing resource")
	}
}

func TestMCPResourceTool_InvalidAction(t *testing.T) {
	svc := &mockMCPResourceService{}
	def := MCPResourceTool(svc)

	result, err := def.Execute(context.Background(), nil, json.RawMessage(`{"action":"delete"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid action")
	}
}

func TestMCPResourceTool_Properties(t *testing.T) {
	svc := &mockMCPResourceService{}
	def := MCPResourceTool(svc)

	if def.ID != "mcp_resource" {
		t.Errorf("expected ID 'mcp_resource', got %q", def.ID)
	}
	if def.Permission != "read" {
		t.Errorf("expected permission 'read', got %q", def.Permission)
	}
}
