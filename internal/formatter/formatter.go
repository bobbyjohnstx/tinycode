package formatter

import (
	"bytes"
	"context"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/config"
	"github.com/bobbyjohnstx/tinycode/internal/procenv"
)

const formatTimeout = 30 * time.Second

// Status is one entry from GET /formatter.
type Status struct {
	Name       string   `json:"name"`
	Extensions []string `json:"extensions"`
	Enabled    bool     `json:"enabled"`
}

type item struct {
	name        string
	extensions  []string
	enabled     bool
	command     []string
	environment map[string]string
	builtin     bool
}

// Runner formats files after a write. A nil runner formats nothing.
type Runner struct {
	items []item
}

// Resolve turns config into the formatters that should run.
// Omitted config and false disable formatting. True enables the built-in
// gofmt formatter. An object enables the built-ins and applies per-name overrides.
func Resolve(cfg *config.FormatterConfig) *Runner {
	if cfg == nil || cfg.Enabled == nil || !*cfg.Enabled {
		return &Runner{}
	}

	byName := map[string]item{
		"gofmt": {
			name:       "gofmt",
			extensions: []string{".go"},
			enabled:    true,
			builtin:    true,
		},
	}
	for name, over := range cfg.Items {
		cur := byName[name]
		cur.name = name
		if over.Disabled != nil {
			cur.enabled = !*over.Disabled
		} else if !cur.builtin {
			cur.enabled = true
		}
		if len(over.Command) > 0 {
			cur.command = append([]string(nil), over.Command...)
		}
		if len(over.Extensions) > 0 {
			cur.extensions = normalizeExts(over.Extensions)
		}
		if len(over.Environment) > 0 {
			cur.environment = over.Environment
		}
		if !cur.builtin && len(cur.command) == 0 {
			cur.enabled = false
		}
		byName[name] = cur
	}

	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	items := make([]item, 0, len(names))
	for _, name := range names {
		it := byName[name]
		if len(it.extensions) == 0 {
			it.extensions = []string{}
		}
		items = append(items, it)
	}
	return &Runner{items: items}
}

// Status lists every configured formatter. A disabled config returns an empty list.
func (r *Runner) Status() []Status {
	if r == nil || len(r.items) == 0 {
		return []Status{}
	}
	out := make([]Status, 0, len(r.items))
	for _, it := range r.items {
		exts := it.extensions
		if exts == nil {
			exts = []string{}
		}
		out = append(out, Status{
			Name:       it.name,
			Extensions: exts,
			Enabled:    it.enabled,
		})
	}
	return out
}

// Format rewrites path with the first enabled formatter for its extension.
// changed is true when the file bytes differ after the formatter runs.
func (r *Runner) Format(ctx context.Context, path string) (name string, changed bool, err error) {
	if r == nil {
		return "", false, nil
	}
	ext := strings.ToLower(filepath.Ext(path))
	var match *item
	for i := range r.items {
		it := &r.items[i]
		if !it.enabled {
			continue
		}
		for _, want := range it.extensions {
			if want == ext {
				match = it
				break
			}
		}
		if match != nil {
			break
		}
	}
	if match == nil {
		return "", false, nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return match.name, false, err
	}
	before, err := os.ReadFile(path)
	if err != nil {
		return match.name, false, err
	}

	if match.builtin && len(match.command) == 0 {
		out, ferr := format.Source(before)
		if ferr != nil {
			return match.name, false, ferr
		}
		if bytes.Equal(before, out) {
			return match.name, false, nil
		}
		if werr := os.WriteFile(path, out, info.Mode().Perm()); werr != nil {
			return match.name, false, werr
		}
		return match.name, true, nil
	}

	if err := runCommand(ctx, *match, path); err != nil {
		return match.name, false, err
	}
	after, err := os.ReadFile(path)
	if err != nil {
		return match.name, false, err
	}
	return match.name, !bytes.Equal(before, after), nil
}

func runCommand(ctx context.Context, it item, path string) error {
	if len(it.command) == 0 {
		return fmt.Errorf("formatter %s has no command", it.name)
	}
	ctx, cancel := context.WithTimeout(ctx, formatTimeout)
	defer cancel()
	args := append(append([]string{}, it.command[1:]...), path)
	cmd := exec.CommandContext(ctx, it.command[0], args...)
	cmd.Dir = filepath.Dir(path)
	cmd.Env = procenv.Child(it.environment)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return fmt.Errorf("%s: %w", it.name, err)
		}
		return fmt.Errorf("%s: %w: %s", it.name, err, msg)
	}
	return nil
}

func normalizeExts(exts []string) []string {
	out := make([]string, 0, len(exts))
	for _, ext := range exts {
		ext = strings.ToLower(strings.TrimSpace(ext))
		if ext == "" {
			continue
		}
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		out = append(out, ext)
	}
	return out
}
