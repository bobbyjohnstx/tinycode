package lsp

import (
	"context"
	"log/slog"
	"os/exec"
	"sort"
	"sync"
	"time"
)

// Manager provides lazy-initialized LSP clients per language.
type Manager struct {
	mu         sync.Mutex
	clients    map[string]*Client
	connecting map[string]*sync.Mutex // per-language connect serialization
	disabled   bool
	dir        string
	detected   []ServerSpec
	overrides  map[string]ServerConfig
	timeout    time.Duration
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewManager creates a manager that lazily connects to LSP servers.
// Pass nil for cfg to use defaults (auto-detect, no overrides).
// LSP is enabled by default; set lsp:false (or enabled:false) to disable.
func NewManager(dir string, cfg *Config) *Manager {
	overrides := make(map[string]ServerConfig)
	timeout := defaultRequestTimeout

	if cfg != nil {
		if cfg.Enabled != nil && !*cfg.Enabled {
			return &Manager{
				clients:    make(map[string]*Client),
				connecting: make(map[string]*sync.Mutex),
				disabled:   true,
				dir:        dir,
			}
		}
		if cfg.Servers != nil {
			overrides = cfg.Servers
		}
		if cfg.Timeout != nil && *cfg.Timeout > 0 {
			timeout = time.Duration(*cfg.Timeout) * time.Second
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		clients:    make(map[string]*Client),
		connecting: make(map[string]*sync.Mutex),
		dir:        dir,
		detected:   detectServers(dir),
		overrides:  overrides,
		timeout:    timeout,
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Disabled reports whether LSP was explicitly turned off in config.
func (m *Manager) Disabled() bool {
	return m.disabled
}

// DiagnosticCounts returns aggregate error and warning counts across all
// connected language servers. Returns zeros when disabled or no clients.
func (m *Manager) DiagnosticCounts() (errors, warnings int) {
	if m.disabled {
		return 0, 0
	}

	m.mu.Lock()
	clients := make([]*Client, 0, len(m.clients))
	for _, c := range m.clients {
		clients = append(clients, c)
	}
	m.mu.Unlock()

	for _, c := range clients {
		e, w := c.diagnosticCounts()
		errors += e
		warnings += w
	}
	return errors, warnings
}

// ClientForFile returns an LSP client for the given file path.
// Returns (nil, lang, nil) if no server is available for the file's language.
func (m *Manager) ClientForFile(ctx context.Context, file string) (*Client, string, error) {
	lang := languageForFile(file)
	if lang == "" {
		return nil, "", nil
	}
	return m.getOrConnect(lang), lang, nil
}

// ClientForLanguage returns an LSP client for the given language key.
func (m *Manager) ClientForLanguage(ctx context.Context, lang string) *Client {
	return m.getOrConnect(lang)
}

// AvailableLanguages returns languages that have detected or connected servers,
// in sorted order for deterministic selection.
func (m *Manager) AvailableLanguages() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.availableLanguagesLocked()
}

// PreferredLanguage returns the first sorted project-detected language,
// falling back to the first sorted AvailableLanguages entry.
func (m *Manager) PreferredLanguage() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.detected) > 0 {
		langs := make([]string, 0, len(m.detected))
		seen := make(map[string]bool, len(m.detected))
		for _, d := range m.detected {
			if seen[d.Language] {
				continue
			}
			seen[d.Language] = true
			langs = append(langs, d.Language)
		}
		sort.Strings(langs)
		return langs[0]
	}

	langs := m.availableLanguagesLocked()
	if len(langs) == 0 {
		return ""
	}
	return langs[0]
}

func (m *Manager) availableLanguagesLocked() []string {
	seen := make(map[string]bool)
	var langs []string
	for l := range m.clients {
		if !seen[l] {
			seen[l] = true
			langs = append(langs, l)
		}
	}
	for _, d := range m.detected {
		if !seen[d.Language] {
			seen[d.Language] = true
			langs = append(langs, d.Language)
		}
	}
	sort.Strings(langs)
	return langs
}

// Close shuts down all LSP server processes.
func (m *Manager) Close() {
	if m.cancel != nil {
		m.cancel()
	}

	m.mu.Lock()
	clients := make([]*Client, 0, len(m.clients))
	for _, c := range m.clients {
		clients = append(clients, c)
	}
	m.clients = make(map[string]*Client)
	m.mu.Unlock()

	for _, c := range clients {
		if err := c.close(); err != nil {
			slog.Warn("error closing lsp client", "language", c.spec.Language, "error", err)
		}
	}
}

// getOrConnect returns a connected client for lang, or nil if unavailable.
// The manager map lock is held only for map lookups/updates; connect runs
// outside that lock under a per-language mutex so other languages can
// initialize concurrently. Failed LookPath/connect does not permanently
// block retries — the next call may try again.
func (m *Manager) getOrConnect(lang string) *Client {
	if m.disabled {
		return nil
	}

	m.mu.Lock()
	if client, ok := m.clients[lang]; ok {
		m.mu.Unlock()
		return client
	}
	langMu, ok := m.connecting[lang]
	if !ok {
		langMu = &sync.Mutex{}
		m.connecting[lang] = langMu
	}
	m.mu.Unlock()

	langMu.Lock()
	defer langMu.Unlock()

	// Double-check: another goroutine may have connected while we waited.
	m.mu.Lock()
	if client, ok := m.clients[lang]; ok {
		m.mu.Unlock()
		return client
	}
	m.mu.Unlock()

	spec := specForLanguage(lang, m.overrides)
	if spec == nil {
		return nil
	}

	if _, err := exec.LookPath(spec.Command); err != nil {
		slog.Warn("lsp: server not in PATH on connect", "language", lang, "command", spec.Command, "hint", installHints[lang])
		return nil
	}

	var env map[string]string
	if cfg, ok := m.overrides[lang]; ok {
		env = cfg.Env
	}

	slog.Info("lsp: connecting to server", "language", lang, "command", spec.Command)
	client := newClient(*spec, m.dir, env, m.timeout)
	if err := client.connect(m.ctx); err != nil {
		slog.Warn("lsp: server failed to start", "language", lang, "command", spec.Command, "error", err)
		return nil
	}

	m.mu.Lock()
	if existing, ok := m.clients[lang]; ok {
		m.mu.Unlock()
		_ = client.close()
		return existing
	}
	m.clients[lang] = client
	m.mu.Unlock()

	slog.Info("lsp: connected", "language", lang, "command", spec.Command)
	return client
}
