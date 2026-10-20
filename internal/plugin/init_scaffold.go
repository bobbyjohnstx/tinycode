package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var pluginNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// InitScaffold writes a hello-world plugin under dir using pkg/plugin.
// Critically documents that ToolCallParams use json:"args", not "arguments".
func InitScaffold(dir, name string) error {
	name = strings.TrimSpace(name)
	if !pluginNameRe.MatchString(name) {
		return fmt.Errorf("invalid plugin name %q (use lowercase letters, digits, hyphens)", name)
	}
	if dir == "" {
		dir = "plugin-" + name
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	mainPath := filepath.Join(dir, "main.go")
	if _, err := os.Stat(mainPath); err == nil {
		return fmt.Errorf("%s already exists", mainPath)
	}

	modPath := filepath.Join(dir, "go.mod")
	mainSrc := scaffoldMain(name)
	modSrc := fmt.Sprintf("module tinycode-plugin-%s\n\ngo 1.24.0\n\nrequire github.com/bobbyjohnstx/tinycode v0.0.0\n\nreplace github.com/bobbyjohnstx/tinycode => ../\n", name)

	readme := fmt.Sprintf(`# tinycode-plugin-%s

Hello-world plugin scaffolded by `+"`tinycode plugin init`"+`.

## Important: JSON-RPC field is `+"`args`"+`, not `+"`arguments`"+`

The plugin protocol (`+"`pkg/plugin`"+`) uses `+"`json:\"args\"`"+` on tool call params.
Sending `+"`arguments`"+` silently zero-values the struct and the tool runs with empty params.

## Build

`+"```bash"+`
go build -o tinycode-plugin-%s .
tinycode plugin install %s --from ./tinycode-plugin-%s
`+"```"+`
`, name, name, name, name)

	if err := os.WriteFile(mainPath, []byte(mainSrc), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(modPath, []byte(modSrc), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0o644); err != nil {
		return err
	}
	return nil
}

func scaffoldMain(name string) string {
	return fmt.Sprintf(`package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

func main() {
	plugin.Run(plugin.Plugin{
		ID: %q,
		Tools: []plugin.ToolDef{
			{
				Name:        "hello",
				Description: "Say hello (scaffold example)",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name": map[string]any{
							"type":        "string",
							"description": "Name to greet",
						},
					},
				},
				Execute: func(_ context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
					// Tool args arrive as JSON in the protocol field "args" (not "arguments").
					var input struct {
						Name string `+"`json:\"name\"`"+`
					}
					if len(args) > 0 {
						if err := json.Unmarshal(args, &input); err != nil {
							return "", fmt.Errorf("parsing args: %%w", err)
						}
					}
					if input.Name == "" {
						input.Name = "world"
					}
					return fmt.Sprintf("hello, %%s", input.Name), nil
				},
			},
		},
	})
}
`, name)
}
