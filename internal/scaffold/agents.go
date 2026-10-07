// Package scaffold generates project bootstrap files for tinycode.
package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// InitPrompt is sent to the LLM after /init creates or finds AGENTS.md.
const InitPrompt = `I've created a root AGENTS.md for this project. Review it, suggest improvements based on the actual codebase structure, and help me configure tinycode for this project (model selection, agent preferences, skills).`

// InitPromptExisting is used when AGENTS.md already exists.
const InitPromptExisting = `This project already has a root AGENTS.md. Review it, suggest improvements based on the actual codebase structure, and help me configure tinycode for this project (model selection, agent preferences, skills).`

// EnsureRootAgentsMD writes a compact root AGENTS.md when missing.
// Returns (created, path, error). created is false when the file already exists.
func EnsureRootAgentsMD(dir string) (created bool, path string, err error) {
	path = filepath.Join(dir, "AGENTS.md")
	if _, err := os.Stat(path); err == nil {
		return false, path, nil
	} else if !os.IsNotExist(err) {
		return false, path, err
	}
	content := GenerateAgentsMD(dir)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return false, path, err
	}
	return true, path, nil
}

// GenerateAgentsMD builds a compact AGENTS.md from repo indicator files.
func GenerateAgentsMD(dir string) string {
	lang, keyFiles := detectLanguage(dir)
	build, test, lint := detectCommands(dir, lang)
	instructions := detectInstructionFiles(dir)
	dirs := detectKeyDirs(dir)

	var b strings.Builder
	b.WriteString("# AGENTS.md\n\n")
	b.WriteString("Compact project instructions for AI agents. Prefer executable sources of truth over this file when they conflict.\n\n")

	b.WriteString("## Language / ecosystem\n\n")
	if lang != "" {
		b.WriteString("- Primary: " + lang + "\n")
	} else {
		b.WriteString("- Primary: unknown (no go.mod / package.json / Cargo.toml / pyproject.toml / Makefile detected)\n")
	}
	if len(keyFiles) > 0 {
		b.WriteString("- Indicators: " + strings.Join(keyFiles, ", ") + "\n")
	}
	b.WriteString("\n")

	b.WriteString("## Commands\n\n")
	b.WriteString(fmt.Sprintf("- Build: `%s`\n", orDash(build)))
	b.WriteString(fmt.Sprintf("- Test: `%s`\n", orDash(test)))
	b.WriteString(fmt.Sprintf("- Lint: `%s`\n", orDash(lint)))
	b.WriteString("\n")

	if len(dirs) > 0 {
		b.WriteString("## Key directories\n\n")
		for _, d := range dirs {
			b.WriteString("- `" + d + "/`\n")
		}
		b.WriteString("\n")
	}

	if len(instructions) > 0 {
		b.WriteString("## Existing instruction files\n\n")
		for _, f := range instructions {
			b.WriteString("- `" + f + "`\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("## Agent delegation\n\n")
	b.WriteString("- Use `build` for implementation and edits.\n")
	b.WriteString("- Use `plan` for design before large changes.\n")
	b.WriteString("- Use `architect` for read-only analysis.\n")
	b.WriteString("- Use `code-reviewer` for review of diffs and PRs.\n")
	b.WriteString("- Prefer `/ask <agent>` for specialists; keep Tab cycle for primary personas.\n")

	return b.String()
}

func orDash(s string) string {
	if s == "" {
		return "_(not detected — fill in)_"
	}
	return s
}

func detectLanguage(dir string) (lang string, indicators []string) {
	checks := []struct {
		file string
		lang string
	}{
		{"go.mod", "Go"},
		{"package.json", "Node.js / JavaScript"},
		{"Cargo.toml", "Rust"},
		{"pyproject.toml", "Python"},
		{"requirements.txt", "Python"},
		{"Makefile", "Make"},
		{"Gemfile", "Ruby"},
		{"pom.xml", "Java (Maven)"},
		{"build.gradle", "Java (Gradle)"},
		{"build.gradle.kts", "Kotlin/Java (Gradle)"},
	}
	for _, c := range checks {
		if fileExists(filepath.Join(dir, c.file)) {
			indicators = append(indicators, c.file)
			if lang == "" {
				lang = c.lang
			}
		}
	}
	return lang, indicators
}

func detectCommands(dir, lang string) (build, test, lint string) {
	if fileExists(filepath.Join(dir, "Makefile")) {
		targets := makefileTargets(filepath.Join(dir, "Makefile"))
		build = firstTarget(targets, "build", "all")
		test = firstTarget(targets, "test", "check")
		lint = firstTarget(targets, "lint", "fmt", "vet")
		if build != "" {
			build = "make " + build
		}
		if test != "" {
			test = "make " + test
		}
		if lint != "" {
			lint = "make " + lint
		}
	}

	pkgPath := filepath.Join(dir, "package.json")
	if fileExists(pkgPath) {
		scripts := packageJSONScripts(pkgPath)
		if build == "" {
			build = npmScript(scripts, "build")
		}
		if test == "" {
			test = npmScript(scripts, "test")
		}
		if lint == "" {
			lint = npmScript(scripts, "lint")
		}
	}

	switch {
	case lang == "Go" || fileExists(filepath.Join(dir, "go.mod")):
		if build == "" {
			build = "go build ./..."
		}
		if test == "" {
			test = "go test ./..."
		}
		if lint == "" {
			lint = "go vet ./..."
		}
	case lang == "Rust" || fileExists(filepath.Join(dir, "Cargo.toml")):
		if build == "" {
			build = "cargo build"
		}
		if test == "" {
			test = "cargo test"
		}
		if lint == "" {
			lint = "cargo clippy"
		}
	case strings.HasPrefix(lang, "Python") || fileExists(filepath.Join(dir, "pyproject.toml")):
		if test == "" {
			test = "pytest"
		}
		if lint == "" {
			lint = "ruff check ."
		}
	}
	return build, test, lint
}

func detectInstructionFiles(dir string) []string {
	var out []string
	candidates := []string{
		"AGENTS.md",
		"CLAUDE.md",
		"GEMINI.md",
		".cursorrules",
		".cursor/rules",
		".claude",
		".tinycode",
	}
	for _, c := range candidates {
		p := filepath.Join(dir, c)
		if fileExists(p) || dirExists(p) {
			out = append(out, c)
		}
	}
	return out
}

func detectKeyDirs(dir string) []string {
	candidates := []string{
		"cmd", "internal", "pkg", "src", "lib", "app", "apps", "packages",
		"test", "tests", "docs", "scripts",
	}
	var out []string
	for _, c := range candidates {
		if dirExists(filepath.Join(dir, c)) {
			out = append(out, c)
		}
	}
	return out
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func dirExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

func makefileTargets(path string) map[string]bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	out := make(map[string]bool)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ".") {
			continue
		}
		if idx := strings.IndexByte(line, ':'); idx > 0 {
			name := strings.TrimSpace(line[:idx])
			if name != "" && !strings.ContainsAny(name, " \t$=") {
				out[name] = true
			}
		}
	}
	return out
}

func firstTarget(targets map[string]bool, names ...string) string {
	for _, n := range names {
		if targets[n] {
			return n
		}
	}
	return ""
}

func packageJSONScripts(path string) map[string]bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	// Minimal parse: look for "scripts" block keys without full JSON dependency.
	out := make(map[string]bool)
	inScripts := false
	for _, line := range strings.Split(string(data), "\n") {
		trim := strings.TrimSpace(line)
		if strings.Contains(trim, `"scripts"`) {
			inScripts = true
			continue
		}
		if inScripts {
			if strings.HasPrefix(trim, "}") {
				break
			}
			if strings.HasPrefix(trim, `"`) {
				parts := strings.SplitN(trim, `"`, 3)
				if len(parts) >= 2 && parts[1] != "" {
					out[parts[1]] = true
				}
			}
		}
	}
	return out
}

func npmScript(scripts map[string]bool, name string) string {
	if scripts[name] {
		return "npm run " + name
	}
	return ""
}
