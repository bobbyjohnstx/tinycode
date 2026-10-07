// Package console serves a thin CLI-shaped ops UI for tinycode serve (#637).
// tinycode web keeps the Solid SPA; this console is registered only when ServeWebUI is false.
package console

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/agent"
	"github.com/bobbyjohnstx/tinycode/internal/config"
	"github.com/bobbyjohnstx/tinycode/internal/lsp"
	"github.com/bobbyjohnstx/tinycode/internal/mcp"
	"github.com/bobbyjohnstx/tinycode/internal/plugin"
	"github.com/bobbyjohnstx/tinycode/internal/project"
	"github.com/bobbyjohnstx/tinycode/internal/provider"
	"github.com/bobbyjohnstx/tinycode/internal/session"
)

//go:embed templates/*.html
var templateFS embed.FS

// Deps are the live server dependencies the console reads.
type Deps struct {
	Version      string
	Directory    string
	DB           *sql.DB
	Registry     *provider.Registry
	Agents       *agent.Registry
	Plugins      *plugin.Manager
	MCP          *mcp.Service
	LSP          *lsp.Manager
	Config       *config.Info
	SessionStore func() *session.Store
}

// Handlers serves ops console pages.
type Handlers struct {
	deps Deps
}

// New creates console handlers.
func New(deps Deps) *Handlers {
	return &Handlers{deps: deps}
}

// Register mounts console routes on mux (Go 1.22+ patterns).
func (h *Handlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", h.Home)
	mux.HandleFunc("GET /status", h.Status)
	mux.HandleFunc("GET /doctor", h.Doctor)
	mux.HandleFunc("GET /providers", h.Providers)
	mux.HandleFunc("GET /models", h.Models)
	mux.HandleFunc("GET /agents", h.Agents)
	mux.HandleFunc("GET /plugins", h.Plugins)
	mux.HandleFunc("GET /sessions", h.Sessions)
	mux.HandleFunc("POST /sessions/{id}/delete", h.SessionDelete)
}

type pageData struct {
	Title         string
	Version       string
	Directory     string
	Healthy       bool
	ProviderCount int
	SessionCount  int
	Database      string
	Checks        []Check
	Heading       string
	Subtitle      string
	Empty         string
	Headers       []string
	Rows          [][]string
	Sessions      []SessionRow
}

// Check is one doctor row.
type Check struct {
	Name   string
	Level  string // ok, warn, bad
	Detail string
}

// SessionRow is one sessions table row.
type SessionRow struct {
	ID      string
	Title   string
	Agent   string
	Updated string
}

func (h *Handlers) render(w http.ResponseWriter, page string, data pageData) {
	if data.Title == "" {
		data.Title = "Ops"
	}
	t, err := template.New("").ParseFS(templateFS, "templates/layout.html", "templates/"+page+".html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handlers) Home(w http.ResponseWriter, r *http.Request) {
	healthy, _ := h.dbStatus(r.Context())
	h.render(w, "home", pageData{
		Title:         "Home",
		Version:       h.version(),
		Directory:     h.deps.Directory,
		Healthy:       healthy,
		ProviderCount: len(h.providers()),
		SessionCount:  h.sessionCount(),
	})
}

func (h *Handlers) Status(w http.ResponseWriter, r *http.Request) {
	healthy, dbStatus := h.dbStatus(r.Context())
	h.render(w, "status", pageData{
		Title:     "Status",
		Version:   h.version(),
		Directory: h.deps.Directory,
		Healthy:   healthy,
		Database:  dbStatus,
	})
}

func (h *Handlers) Doctor(w http.ResponseWriter, r *http.Request) {
	h.render(w, "doctor", pageData{
		Title:  "Doctor",
		Checks: h.doctorChecks(r.Context()),
	})
}

func (h *Handlers) Providers(w http.ResponseWriter, _ *http.Request) {
	rows := make([][]string, 0)
	for _, p := range h.providers() {
		rows = append(rows, []string{p.ID, p.Name, fmt.Sprintf("%d", len(p.Models))})
	}
	h.render(w, "table", pageData{
		Title:    "Providers",
		Heading:  "Providers",
		Subtitle: "Analogous to tinycode providers",
		Empty:    "No providers discovered",
		Headers:  []string{"ID", "Name", "Models"},
		Rows:     rows,
	})
}

func (h *Handlers) Models(w http.ResponseWriter, _ *http.Request) {
	rows := make([][]string, 0)
	for _, p := range h.providers() {
		for id, m := range p.Models {
			name := id
			if m != nil && m.Name != "" {
				name = m.Name
			}
			modelID := id
			if m != nil && m.ID != "" {
				modelID = m.ID
			}
			rows = append(rows, []string{p.ID, modelID, name})
		}
	}
	h.render(w, "table", pageData{
		Title:    "Models",
		Heading:  "Models",
		Subtitle: "Analogous to tinycode models",
		Empty:    "No models discovered",
		Headers:  []string{"Provider", "ID", "Name"},
		Rows:     rows,
	})
}

func (h *Handlers) Agents(w http.ResponseWriter, _ *http.Request) {
	rows := make([][]string, 0)
	if h.deps.Agents != nil {
		defaultAgent := ""
		if h.deps.Config != nil {
			defaultAgent = h.deps.Config.DefaultAgent
		}
		for _, a := range h.deps.Agents.ListAll(defaultAgent) {
			mode := string(a.Mode)
			if mode == "" {
				mode = "—"
			}
			status := "enabled"
			if a.Disabled {
				status = "disabled"
			}
			rows = append(rows, []string{a.Name, mode, status})
		}
	}
	h.render(w, "table", pageData{
		Title:    "Agents",
		Heading:  "Agents",
		Subtitle: "Analogous to tinycode agent",
		Empty:    "No agents loaded",
		Headers:  []string{"Name", "Mode", "Status"},
		Rows:     rows,
	})
}

func (h *Handlers) Plugins(w http.ResponseWriter, _ *http.Request) {
	rows := make([][]string, 0)
	if h.deps.Plugins != nil {
		for _, p := range h.deps.Plugins.List() {
			rows = append(rows, []string{p.ID, p.Name})
		}
	}
	h.render(w, "table", pageData{
		Title:    "Plugins",
		Heading:  "Plugins",
		Subtitle: "Analogous to tinycode plugin list",
		Empty:    "No plugins loaded",
		Headers:  []string{"ID", "Name"},
		Rows:     rows,
	})
}

func (h *Handlers) Sessions(w http.ResponseWriter, _ *http.Request) {
	h.render(w, "sessions", pageData{
		Title:    "Sessions",
		Sessions: h.sessionRows(),
	})
}

func (h *Handlers) SessionDelete(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if r.FormValue("confirm") != "delete" {
		http.Error(w, "confirm=delete required", http.StatusBadRequest)
		return
	}
	id := r.PathValue("id")
	if id == "" || h.deps.SessionStore == nil {
		http.Error(w, "session store unavailable", http.StatusBadRequest)
		return
	}
	store := h.deps.SessionStore()
	if store == nil {
		http.Error(w, "session store unavailable", http.StatusBadRequest)
		return
	}
	if err := store.Delete(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/sessions", http.StatusSeeOther)
}

func (h *Handlers) version() string {
	if h.deps.Version == "" {
		return "dev"
	}
	return h.deps.Version
}

func (h *Handlers) dbStatus(ctx context.Context) (healthy bool, detail string) {
	if h.deps.DB == nil {
		return false, "no database"
	}
	if err := h.deps.DB.PingContext(ctx); err != nil {
		return false, "error: " + err.Error()
	}
	return true, "ok"
}

func (h *Handlers) providers() []*provider.Info {
	if h.deps.Registry == nil {
		return nil
	}
	return h.deps.Registry.ListProviders()
}

func (h *Handlers) sessionCount() int {
	return len(h.sessionRows())
}

func (h *Handlers) sessionRows() []SessionRow {
	if h.deps.SessionStore == nil {
		return nil
	}
	store := h.deps.SessionStore()
	if store == nil {
		return nil
	}
	projectID := project.IDFromDirectory(h.deps.Directory)
	list, err := store.List(projectID, 100, 0, false)
	if err != nil {
		return nil
	}
	rows := make([]SessionRow, 0, len(list))
	for _, s := range list {
		updated := ""
		if !s.UpdatedAt.IsZero() {
			updated = s.UpdatedAt.Format(time.RFC3339)
		}
		title := s.Title
		if title == "" {
			title = "(untitled)"
		}
		rows = append(rows, SessionRow{
			ID:      s.ID,
			Title:   title,
			Agent:   s.Agent,
			Updated: updated,
		})
	}
	return rows
}

func (h *Handlers) doctorChecks(ctx context.Context) []Check {
	var checks []Check
	healthy, detail := h.dbStatus(ctx)
	level := "ok"
	if !healthy {
		level = "bad"
	}
	checks = append(checks, Check{Name: "Database", Level: level, Detail: detail})
	checks = append(checks, Check{Name: "Version", Level: "ok", Detail: h.version()})

	dir := h.deps.Directory
	if dir == "" {
		checks = append(checks, Check{Name: "Directory", Level: "warn", Detail: "not set"})
	} else {
		checks = append(checks, Check{Name: "Directory", Level: "ok", Detail: dir})
	}

	provs := h.providers()
	if len(provs) == 0 {
		checks = append(checks, Check{Name: "Providers", Level: "warn", Detail: "none discovered — use TUI /connect"})
	} else {
		checks = append(checks, Check{Name: "Providers", Level: "ok", Detail: fmt.Sprintf("%d available", len(provs))})
	}

	if h.deps.Agents == nil {
		checks = append(checks, Check{Name: "Agents", Level: "warn", Detail: "registry not attached"})
	} else {
		defaultAgent := ""
		if h.deps.Config != nil {
			defaultAgent = h.deps.Config.DefaultAgent
		}
		n := len(h.deps.Agents.ListAll(defaultAgent))
		checks = append(checks, Check{Name: "Agents", Level: "ok", Detail: fmt.Sprintf("%d loaded", n)})
	}

	if h.deps.Plugins == nil {
		checks = append(checks, Check{Name: "Plugins", Level: "warn", Detail: "manager not attached"})
	} else {
		n := len(h.deps.Plugins.List())
		checks = append(checks, Check{Name: "Plugins", Level: "ok", Detail: fmt.Sprintf("%d loaded", n)})
	}

	if h.deps.MCP == nil {
		checks = append(checks, Check{Name: "MCP", Level: "ok", Detail: "none configured"})
	} else {
		status := h.deps.MCP.Status(ctx)
		parts := make([]string, 0, len(status))
		warn := false
		for name, st := range status {
			parts = append(parts, name+":"+string(st.Status))
			if st.Status != mcp.StatusConnected {
				warn = true
			}
		}
		lvl := "ok"
		detail := "none configured"
		if len(parts) > 0 {
			detail = strings.Join(parts, ", ")
			if warn {
				lvl = "warn"
			}
		}
		checks = append(checks, Check{Name: "MCP", Level: lvl, Detail: detail})
	}

	if h.deps.LSP == nil {
		checks = append(checks, Check{Name: "LSP", Level: "ok", Detail: "not attached"})
	} else if h.deps.LSP.Disabled() {
		checks = append(checks, Check{Name: "LSP", Level: "ok", Detail: "disabled"})
	} else {
		langs := h.deps.LSP.AvailableLanguages()
		checks = append(checks, Check{Name: "LSP", Level: "ok", Detail: fmt.Sprintf("%d languages", len(langs))})
	}

	return checks
}
