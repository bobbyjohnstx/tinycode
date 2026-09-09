package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

// template holds a snippet template's metadata and content.
type template struct {
	Description string
	Content     string
}

// builtinTemplates returns the built-in K8s/OCP YAML templates.
func builtinTemplates() map[string]template {
	return map[string]template{
		"deployment": {
			Description: "Kubernetes Deployment",
			Content: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{name}}
  namespace: {{namespace}}
spec:
  replicas: {{replicas}}
  selector:
    matchLabels:
      app: {{name}}
  template:
    metadata:
      labels:
        app: {{name}}
    spec:
      containers:
        - name: {{name}}
          image: {{image}}
          ports:
            - containerPort: {{port}}`,
		},
		"service": {
			Description: "Kubernetes Service",
			Content: `apiVersion: v1
kind: Service
metadata:
  name: {{name}}
  namespace: {{namespace}}
spec:
  selector:
    app: {{name}}
  ports:
    - port: {{port}}
      targetPort: {{port}}
  type: ClusterIP`,
		},
		"route": {
			Description: "OpenShift Route",
			Content: `apiVersion: route.openshift.io/v1
kind: Route
metadata:
  name: {{name}}
  namespace: {{namespace}}
spec:
  to:
    kind: Service
    name: {{name}}
  port:
    targetPort: {{port}}
  tls:
    termination: edge`,
		},
		"configmap": {
			Description: "Kubernetes ConfigMap",
			Content: `apiVersion: v1
kind: ConfigMap
metadata:
  name: {{name}}
  namespace: {{namespace}}
data:
  {{key}}: {{value}}`,
		},
		"pvc": {
			Description: "Kubernetes PersistentVolumeClaim",
			Content: `apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: {{name}}
  namespace: {{namespace}}
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: {{size}}`,
		},
	}
}

// snippetsDir returns the directory to scan for custom templates.
func snippetsDir() string {
	if dir := os.Getenv("SNIPPETS_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".tinycode", "snippets")
}

// loadCustomTemplates reads .yaml/.yml files from the snippets directory.
func loadCustomTemplates(dir string) map[string]template {
	templates := map[string]template{}
	if dir == "" {
		return templates
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return templates
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ext)
		content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		templates[name] = template{
			Description: "Custom template: " + name,
			Content:     string(content),
		}
	}
	return templates
}

// getAllTemplates merges built-in and custom templates. Custom templates
// override built-in ones with the same name.
func getAllTemplates(dir string) map[string]template {
	all := builtinTemplates()
	for k, v := range loadCustomTemplates(dir) {
		all[k] = v
	}
	return all
}

// unresolvedRe matches {{variable}} placeholders.
var unresolvedRe = regexp.MustCompile(`\{\{[^}]+\}\}`)

// expandTemplate replaces {{key}} placeholders and returns unresolved ones.
func expandTemplate(content string, variables map[string]string) (string, []string) {
	result := content
	for key, value := range variables {
		result = strings.ReplaceAll(result, "{{"+key+"}}", value)
	}
	matches := unresolvedRe.FindAllString(result, -1)
	seen := map[string]bool{}
	var unresolved []string
	for _, m := range matches {
		if !seen[m] {
			seen[m] = true
			unresolved = append(unresolved, m)
		}
	}
	return result, unresolved
}

// listArgs is the input schema for snippet_list (no required fields).
type listArgs struct{}

// expandArgs is the input schema for snippet_expand.
type expandArgs struct {
	Name      string            `json:"name"`
	Variables map[string]string `json:"variables"`
}

// newPlugin returns the snippets plugin definition.
func newPlugin() plugin.Plugin {
	return plugin.Plugin{
		ID:    "snippets",
		Tools: buildTools(),
	}
}

func buildTools() []plugin.ToolDef {
	return []plugin.ToolDef{
		{
			Name:        "snippet_list",
			Description: "List available snippet templates with their names and descriptions",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Execute: func(_ context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				templates := getAllTemplates(snippetsDir())
				names := make([]string, 0, len(templates))
				for name := range templates {
					names = append(names, name)
				}
				sort.Strings(names)
				lines := make([]string, 0, len(names))
				for _, name := range names {
					lines = append(lines, name+" — "+templates[name].Description)
				}
				return strings.Join(lines, "\n"), nil
			},
		},
		{
			Name:        "snippet_expand",
			Description: "Expand a snippet template by name, replacing {{variable}} placeholders with provided values",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{
						"type":        "string",
						"description": "Template name (e.g., 'deployment', 'service')",
					},
					"variables": map[string]any{
						"type":        "object",
						"description": "Variable substitutions as key-value pairs",
						"additionalProperties": map[string]any{"type": "string"},
					},
				},
				"required": []string{"name"},
			},
			Execute: func(_ context.Context, raw json.RawMessage, _ plugin.ToolContext) (string, error) {
				var args expandArgs
				if err := json.Unmarshal(raw, &args); err != nil {
					return "", fmt.Errorf("invalid arguments: %w", err)
				}
				if args.Name == "" {
					return "", fmt.Errorf("name is required")
				}

				templates := getAllTemplates(snippetsDir())
				tmpl, ok := templates[args.Name]
				if !ok {
					names := make([]string, 0, len(templates))
					for name := range templates {
						names = append(names, name)
					}
					sort.Strings(names)
					return fmt.Sprintf("Unknown template %q. Available templates: %s",
						args.Name, strings.Join(names, ", ")), nil
				}

				vars := args.Variables
				if vars == nil {
					vars = map[string]string{}
				}
				result, unresolved := expandTemplate(tmpl.Content, vars)

				if len(unresolved) > 0 {
					return result + "\n\nNote: Unresolved variables: " + strings.Join(unresolved, ", "), nil
				}
				return result, nil
			},
		},
	}
}

func main() {
	plugin.Run(newPlugin())
}
