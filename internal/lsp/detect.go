package lsp

import (
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ServerSpec describes how to find and launch a language server.
type ServerSpec struct {
	Language    string
	Command     string
	Args        []string
	MarkerFiles []string
	ExtLangIDs  map[string]string // file extension -> LSP languageId
}

var defaultServers = []ServerSpec{
	{
		Language:    "go",
		Command:     "gopls",
		Args:        []string{"serve"},
		MarkerFiles: []string{"go.mod", "go.sum"},
		ExtLangIDs:  map[string]string{".go": "go"},
	},
	{
		Language:    "typescript",
		Command:     "typescript-language-server",
		Args:        []string{"--stdio"},
		MarkerFiles: []string{"tsconfig.json", "tsconfig.base.json"},
		ExtLangIDs: map[string]string{
			".ts": "typescript", ".tsx": "typescriptreact",
			".js": "javascript", ".jsx": "javascriptreact",
		},
	},
	{
		Language:    "python",
		Command:     "pyright-langserver",
		Args:        []string{"--stdio"},
		MarkerFiles: []string{"pyproject.toml", "setup.py", "requirements.txt", "Pipfile"},
		ExtLangIDs:  map[string]string{".py": "python"},
	},
	{
		Language:    "rust",
		Command:     "rust-analyzer",
		Args:        []string{},
		MarkerFiles: []string{"Cargo.toml"},
		ExtLangIDs:  map[string]string{".rs": "rust"},
	},
	{
		Language:    "shellscript",
		Command:     "bash-language-server",
		Args:        []string{"start"},
		MarkerFiles: nil, // always available
		ExtLangIDs:  map[string]string{".sh": "shellscript", ".bash": "shellscript", ".zsh": "shellscript"},
	},
	{
		Language:    "yaml",
		Command:     "yaml-language-server",
		Args:        []string{"--stdio"},
		MarkerFiles: nil, // always available
		ExtLangIDs:  map[string]string{".yaml": "yaml", ".yml": "yaml"},
	},
	{
		Language:    "json",
		Command:     "vscode-json-language-server",
		Args:        []string{"--stdio"},
		MarkerFiles: nil, // always available
		ExtLangIDs:  map[string]string{".json": "json", ".jsonc": "jsonc"},
	},
	{
		Language:    "dockerfile",
		Command:     "docker-langserver",
		Args:        []string{"--stdio"},
		MarkerFiles: []string{"Dockerfile", "docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"},
		ExtLangIDs:  map[string]string{".dockerfile": "dockerfile"},
	},
}

var installHints = map[string]string{
	"go":         "Install gopls: go install golang.org/x/tools/gopls@latest",
	"typescript": "Install typescript-language-server: npm install -g typescript-language-server typescript",
	"python":     "Install pyright: npm install -g pyright",
	"rust":        "Install rust-analyzer: see https://rust-analyzer.github.io/manual.html#installation",
	"shellscript": "Install bash-language-server: npm install -g bash-language-server",
	"yaml":        "Install yaml-language-server: npm install -g yaml-language-server",
	"json":        "Install vscode-json-language-server: npm install -g vscode-langservers-extracted",
	"dockerfile":  "Install docker-langserver: npm install -g dockerfile-language-server-nodejs",
}

// detectServers scans dir for marker files and checks if the corresponding
// server binary is in PATH. Servers with nil MarkerFiles are always detected
// if the binary is available.
func detectServers(dir string) []ServerSpec {
	var detected []ServerSpec
	for _, spec := range defaultServers {
		if spec.MarkerFiles == nil {
			if _, err := exec.LookPath(spec.Command); err == nil {
				slog.Info("lsp: detected server", "language", spec.Language, "command", spec.Command)
				detected = append(detected, spec)
			} else {
				slog.Debug("lsp: server not in PATH", "language", spec.Language, "command", spec.Command, "hint", installHints[spec.Language])
			}
			continue
		}
		markerFound := false
		for _, marker := range spec.MarkerFiles {
			if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
				markerFound = true
				if _, err := exec.LookPath(spec.Command); err == nil {
					slog.Info("lsp: detected server", "language", spec.Language, "command", spec.Command, "marker", marker)
					detected = append(detected, spec)
				} else {
					slog.Warn("lsp: project detected but server not in PATH", "language", spec.Language, "command", spec.Command, "marker", marker, "hint", installHints[spec.Language])
				}
				break
			}
		}
		if !markerFound {
			slog.Debug("lsp: no marker files found", "language", spec.Language, "markers", spec.MarkerFiles)
		}
	}
	if len(detected) == 0 {
		slog.Info("lsp: no language servers detected")
	} else {
		slog.Info("lsp: detection complete", "count", len(detected))
	}
	return detected
}

// languageForFile returns the language key for a file path based on extension,
// falling back to filename matching for files like Dockerfile.
func languageForFile(path string) string {
	ext := filepath.Ext(path)
	for _, spec := range defaultServers {
		if _, ok := spec.ExtLangIDs[ext]; ok {
			return spec.Language
		}
	}
	if isDockerfile(path) {
		return "dockerfile"
	}
	return ""
}

// languageIDForFile returns the LSP languageId for a file path.
func languageIDForFile(path string) string {
	ext := filepath.Ext(path)
	for _, spec := range defaultServers {
		if id, ok := spec.ExtLangIDs[ext]; ok {
			return id
		}
	}
	if isDockerfile(path) {
		return "dockerfile"
	}
	return ""
}

func isDockerfile(path string) bool {
	base := filepath.Base(path)
	return strings.HasPrefix(strings.ToLower(base), "dockerfile")
}

// specForLanguage returns the ServerSpec for a language, applying user overrides.
func specForLanguage(lang string, overrides map[string]ServerConfig) *ServerSpec {
	for _, spec := range defaultServers {
		if spec.Language != lang {
			continue
		}
		if cfg, ok := overrides[lang]; ok {
			if cfg.Disabled != nil && *cfg.Disabled {
				return nil
			}
			result := spec
			if cfg.Command != "" {
				result.Command = cfg.Command
			}
			if cfg.Args != nil {
				result.Args = cfg.Args
			}
			return &result
		}
		return &spec
	}
	return nil
}

func extForLanguage(lang string) string {
	exts := map[string]string{
		"go":          ".go",
		"typescript":  ".ts",
		"python":      ".py",
		"rust":        ".rs",
		"shellscript": ".sh",
		"yaml":        ".yaml",
		"json":        ".json",
		"dockerfile":  ".dockerfile",
	}
	if ext, ok := exts[lang]; ok {
		return ext
	}
	return ""
}
