package lsp

import (
	"context"
	"log/slog"
	"os/exec"
	"sync"
	"time"
)

// Manager provides lazy-initialized LSP clients per language.
type Manager struct {
	mu        sync.Mutex
	clients   map[string]*Client
	unavail   map[string]bool
	dir       string
	detected  []ServerSpec
	overrides map[string]ServerConfig
	timeout   time.Duration
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewManager creates a manager that lazily connects to LSP servers.
// Pass nil for cfg to use defaults (auto-detect, no overrides).
func NewManager(dir string, cfg *Config) *Manager {
	overrides := make(map[string]ServerConfig)
	timeout := defaultRequestTimeout

	if cfg != nil {
		if cfg.Enabled != nil && !*cfg.Enabled {
			return &Manager{
				clients: make(map[string]*Client),
				unavail: map[string]bool{"*": true},
				dir:     dir,
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
		clients:   make(map[string]*Client),
		unavail:   make(map[string]bool),
		dir:       dir,
		detected:  detectServers(dir),
		overrides: overrides,
		timeout:   timeout,
		ctx:       ctx,
		cancel:    cancel,
	}
}

// ClientForFile returns an LSP client for the given file path.
// Returns (nil, lang, nil) if no server is available for the file's language.
func (m *Manager) ClientForFile(ctx context.Context, file string) (*Client, string, error) {
	lang := languageForFile(file)
	if lang == "" {
		return nil, "", nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getOrConnect(lang), lang, nil
}

// ClientForLanguage returns an LSP client for the given language key.
func (m *Manager) ClientForLanguage(ctx context.Context, lang string) *Client {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getOrConnect(lang)
}

// AvailableLanguages returns languages that have detected or connected servers.
func (m *Manager) AvailableLanguages() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	var langs []string
	for l := range m.clients {
		langs = append(langs, l)
	}
	if len(langs) > 0 {
		return langs
	}
	for _, d := range m.detected {
		if !m.unavail[d.Language] {
			langs = append(langs, d.Language)
		}
	}
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

func (m *Manager) getOrConnect(lang string) *Client {
	if m.unavail[lang] || m.unavail["*"] {
		return nil
	}
	if client, ok := m.clients[lang]; ok {
		return client
	}

	spec := specForLanguage(lang, m.overrides)
	if spec == nil {
		m.unavail[lang] = true
		return nil
	}

	if _, err := exec.LookPath(spec.Command); err != nil {
		slog.Warn("lsp: server not in PATH on connect", "language", lang, "command", spec.Command, "hint", installHints[lang])
		m.unavail[lang] = true
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
		m.unavail[lang] = true
		return nil
	}

	slog.Info("lsp: connected", "language", lang, "command", spec.Command)
	m.clients[lang] = client
	return client
}
