package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/bobbyjohnstx/tinycode-go/internal/agent"
	"github.com/bobbyjohnstx/tinycode-go/internal/bus"
	"github.com/bobbyjohnstx/tinycode-go/internal/config"
	"github.com/bobbyjohnstx/tinycode-go/internal/lsp"
	"github.com/bobbyjohnstx/tinycode-go/internal/permission"
	"github.com/bobbyjohnstx/tinycode-go/internal/plugin"
	"github.com/bobbyjohnstx/tinycode-go/internal/session"
	"github.com/bobbyjohnstx/tinycode-go/internal/provider"
	"github.com/bobbyjohnstx/tinycode-go/internal/server"
	"github.com/bobbyjohnstx/tinycode-go/internal/storage"
	"github.com/bobbyjohnstx/tinycode-go/internal/tool"
)

var logFile *os.File

func setupLogger() {
	level := slog.LevelInfo
	switch os.Getenv("TINYCODE_LOG_LEVEL") {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	dataDir := config.DataDir()
	os.MkdirAll(dataDir, 0o755)

	logPath := filepath.Join(dataDir, "tinycode.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
		slog.Warn("failed to open log file, falling back to stderr", "path", logPath, "error", err)
		return
	}

	logFile = f
	slog.SetDefault(slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: level})))
}

func initDependencies() (*bus.Bus, *storage.DB, *config.Info) {
	b := bus.New()

	db, err := storage.Open(storage.DefaultPath())
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}

	dir, _ := os.Getwd()
	cfg, err := config.Load(dir)
	if err != nil {
		slog.Warn("failed to load config", "error", err)
		cfg = &config.Info{}
	}

	return b, db, cfg
}

// generateToken produces a cryptographically random 64-character hex token
// for authenticating HTTP requests between the TUI client and the embedded server.
func generateToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		slog.Error("failed to generate auth token", "error", err)
		os.Exit(1)
	}
	return hex.EncodeToString(b)
}

// loadOrCreateWebToken returns a stable auth token for web mode by reading
// from a file in the data directory. If the file does not exist or is invalid,
// a new token is generated and persisted. This ensures browser tabs from
// previous server sessions remain authenticated after a server restart.
func loadOrCreateWebToken() string {
	tokenPath := filepath.Join(config.DataDir(), "web_token")
	if data, err := os.ReadFile(tokenPath); err == nil {
		token := strings.TrimSpace(string(data))
		if len(token) == 64 {
			return token
		}
	}
	token := generateToken()
	os.MkdirAll(config.DataDir(), 0o755)
	if err := os.WriteFile(tokenPath, []byte(token+"\n"), 0o600); err != nil {
		slog.Warn("failed to persist web token", "error", err)
	}
	return token
}

func serverConfig(cfg *config.Info, serveWebUI bool) server.Config {
	port := 4096
	host := "127.0.0.1"

	if cfg.Server != nil {
		if cfg.Server.Port != nil {
			port = *cfg.Server.Port
		}
		if cfg.Server.Host != "" {
			host = cfg.Server.Host
		}
	}

	if v := os.Getenv("TINYCODE_PORT"); v != "" {
		fmt.Sscanf(v, "%d", &port)
	}
	if v := os.Getenv("TINYCODE_HOST"); v != "" {
		host = v
	}

	dir, _ := os.Getwd()

	return server.Config{
		Port:         port,
		Hostname:     host,
		ServeWebUI:   serveWebUI,
		WebUIDir:     os.Getenv("TINYCODE_WEB_DIR"),
		Directory:    dir,
		DefaultModel: cfg.Model,
		DefaultAgent: cfg.DefaultAgent,
	}
}

func initAgentRegistry(cfg *config.Info, directory string) *agent.Registry {
	defaultPerms := permission.Ruleset{
		{Permission: "*", Pattern: "*", Action: permission.ActionAllow},
	}
	var userPerms permission.Ruleset
	if cfg.Permission != nil {
		for _, a := range cfg.Permission.Allow {
			userPerms = append(userPerms, permission.Rule{Permission: a, Pattern: "*", Action: permission.ActionAllow})
		}
		for _, d := range cfg.Permission.Deny {
			userPerms = append(userPerms, permission.Rule{Permission: d, Pattern: "*", Action: permission.ActionDeny})
		}
	}

	reg := agent.NewRegistry()
	if err := reg.LoadDefaults(defaultPerms, userPerms); err != nil {
		slog.Warn("failed to load default agents", "error", err)
	}
	reg.LoadUserAgents(config.ConfigDir(), directory, defaultPerms, userPerms)

	if len(cfg.Agents) > 0 {
		overrides := make(map[string]agent.ConfigOverride, len(cfg.Agents))
		for name, raw := range cfg.Agents {
			var co agent.ConfigOverride
			if err := json.Unmarshal(raw, &co); err != nil {
				slog.Warn("failed to parse agent config override", "agent", name, "error", err)
				continue
			}
			overrides[name] = co
		}
		reg.ApplyConfigOverrides(overrides, defaultPerms, userPerms)
	}

	return reg
}

func initTooling(b *bus.Bus, directory string, cfg ...*config.Info) (*tool.Registry, *permission.Service, *tool.Context) {
	permSvc := permission.NewService(b)
	subagentBudget := &atomic.Int32{}
	subagentBudget.Store(20)
	toolCtx := &tool.Context{
		Directory:      directory,
		Perms:          permSvc,
		Bus:            b,
		JobManager:     session.NewJobManager(b),
		SubagentCount:  &atomic.Int32{},
		SubagentBudget: subagentBudget,
		TaskRoundDone:  &atomic.Bool{},
	}
	if len(cfg) > 0 && cfg[0] != nil && cfg[0].AutoApprove != nil && *cfg[0].AutoApprove {
		toolCtx.AutoApprove = true
	}
	toolReg := tool.NewRegistry(toolCtx)
	tool.RegisterBuiltins(toolReg)
	return toolReg, permSvc, toolCtx
}

func initLSP(dir string, cfg *config.Info, toolReg *tool.Registry) *lsp.Manager {
	var lspCfg *lsp.Config
	if cfg.LSP != nil {
		lspCfg = &lsp.Config{
			Enabled: cfg.LSP.Enabled,
			Timeout: cfg.LSP.Timeout,
		}
		if cfg.LSP.Servers != nil {
			lspCfg.Servers = make(map[string]lsp.ServerConfig, len(cfg.LSP.Servers))
			for k, v := range cfg.LSP.Servers {
				lspCfg.Servers[k] = lsp.ServerConfig{
					Command:  v.Command,
					Args:     v.Args,
					Disabled: v.Disabled,
					Env:      v.Env,
				}
			}
		}
	}
	mgr := lsp.NewManager(dir, lspCfg)
	lsp.RegisterTools(toolReg, mgr)
	return mgr
}

func initBuiltins(toolReg *tool.Registry) *plugin.BuiltinManager {
	bm := plugin.NewBuiltinManager()
	bm.Register(plugin.NewContextPruningPlugin())
	bm.Register(plugin.NewNotifyPlugin())
	bm.Register(plugin.NewCodeReviewPlugin())
	bm.Register(plugin.NewHandoffPlugin())

	// Register builtin tools with the tool registry so the LLM can call them.
	for _, bt := range bm.AllTools() {
		bt := bt // capture loop variable
		toolReg.Register(&tool.Def{
			ID:          bt.Name,
			Description: bt.Description,
			Parameters:  bt.Parameters,
			Execute: func(ctx context.Context, tc *tool.Context, args json.RawMessage) (*tool.ExecuteResult, error) {
				result, err := bm.CallTool(ctx, bt.Name, args)
				if err != nil {
					return &tool.ExecuteResult{Output: err.Error(), IsError: true}, nil
				}
				return &tool.ExecuteResult{Output: result}, nil
			},
		})
	}

	return bm
}

func wireToolAfterHook(toolCtx *tool.Context, mgr *plugin.Manager, bm *plugin.BuiltinManager) {
	toolCtx.AfterHook = func(sessionID, toolName, output string, isError bool) (string, bool, bool) {
		modified := false

		// Run builtin hooks first (e.g., context-pruning).
		if bm != nil {
			if modOut, modErr, changed := bm.DispatchToolExecAfter(context.Background(), toolName, output, isError); changed {
				output = modOut
				isError = modErr
				modified = true
			}
		}

		// Then run external plugin hooks.
		result, err := plugin.DispatchToolExecAfter(mgr, plugin.ToolExecAfterEvent{
			SessionID: sessionID,
			ToolName:  toolName,
			Output:    output,
			IsError:   isError,
		})
		if err == nil && result != nil {
			output = result.Output
			isError = result.IsError
			modified = true
		}

		if modified {
			return output, isError, true
		}
		return "", false, false
	}
}

func startDiscovery(ctx context.Context, reg *provider.Registry, b *bus.Bus, cfg *config.Info) *provider.Discovery {
	disc := provider.NewDiscovery(reg, b)

	ollamaURL := "http://127.0.0.1:11434"
	lmStudioURL := "http://127.0.0.1:1234"
	vllmURL := ""

	if v := os.Getenv("OLLAMA_HOST"); v != "" {
		ollamaURL = v
	}
	if v := os.Getenv("TINYCODE_LMSTUDIO_HOST"); v != "" {
		lmStudioURL = v
	}
	if v := os.Getenv("TINYCODE_VLLM_HOST"); v != "" {
		vllmURL = v
	}

	if cfg.EnabledProviders != nil || cfg.DisabledProviders != nil {
		reg.SetFilters(cfg.EnabledProviders, cfg.DisabledProviders)
	}

	// Wire auto-profiling config from provider.ollama.options.auto_profile
	if ollamaCfg, ok := cfg.Provider["ollama"]; ok && ollamaCfg.Options != nil {
		if apRaw, ok := ollamaCfg.Options["auto_profile"]; ok {
			data, _ := json.Marshal(apRaw)
			var apCfg provider.AutoProfileConfig
			if err := json.Unmarshal(data, &apCfg); err == nil {
				disc.SetAutoProfile(&apCfg)
			} else {
				slog.Warn("failed to parse auto_profile config", "error", err)
			}
		}
	}

	disc.Start(ctx, ollamaURL, vllmURL, lmStudioURL)

	if apiKey := os.Getenv("OPENROUTER_API_KEY"); apiKey != "" {
		go func() {
			if err := disc.DiscoverOpenRouter(ctx, apiKey); err != nil {
				slog.Warn("openrouter discovery failed", "error", err)
			}
		}()
	}

	registerConfigProviders(reg, cfg)

	return disc
}

// registerConfigProviders registers custom API providers from config that
// aren't handled by the discovery loop (Ollama, vLLM, LM Studio, OpenRouter).
func registerConfigProviders(reg *provider.Registry, cfg *config.Info) {
	knownDiscovery := map[string]bool{
		"ollama":     true,
		"vllm":       true,
		"lm-studio":  true,
		"openrouter": true,
	}

	for id, pc := range cfg.Provider {
		if knownDiscovery[id] {
			continue
		}
		if len(pc.Models) == 0 {
			continue
		}

		baseURL := ""
		apiKey := ""
		name := id
		if pc.Options != nil {
			if u, ok := pc.Options["baseURL"].(string); ok {
				baseURL = u
			}
			if k, ok := pc.Options["apiKey"].(string); ok {
				apiKey = k
			}
			if n, ok := pc.Options["name"].(string); ok {
				name = n
			}
		}
		if baseURL == "" {
			slog.Warn("custom provider has no baseURL, skipping", "provider", id)
			continue
		}

		// Strip trailing /v1 — the LLM client factory adds it.
		baseURL = strings.TrimSuffix(strings.TrimSuffix(baseURL, "/"), "/v1")

		models := make(map[string]*provider.Model, len(pc.Models))
		for modelID, mc := range pc.Models {
			contextLen := 8192
			outputLen := 4096
			if mc.Limit != nil {
				if mc.Limit.Context > 0 {
					contextLen = mc.Limit.Context
				}
				if mc.Limit.Output > 0 {
					outputLen = mc.Limit.Output
				}
			}

			models[modelID] = &provider.Model{
				ID:         modelID,
				ProviderID: id,
				Name:       modelID,
				API: provider.ModelAPI{
					ID:  modelID,
					URL: baseURL,
				},
				Status:  "active",
				Headers: make(map[string]string),
				Options: map[string]any{"api_key": apiKey},
				Limit:   provider.ModelLimit{Context: contextLen, Output: outputLen},
				Capabilities: provider.ModelCaps{
					Temperature: true,
					ToolCall:    true,
					Input:       provider.ModalityCaps{Text: true},
					Output:      provider.ModalityCaps{Text: true},
				},
			}
		}

		reg.Register(&provider.Info{
			ID:      id,
			Name:    name,
			Source:  "config",
			Env:     pc.Env,
			Options: pc.Options,
			Models:  models,
		})

		slog.Info("registered config provider", "provider", id, "models", len(models))
	}
}

func loadConfigPlugins(mgr *plugin.Manager, cfg *config.Info, dir string) {
	mgr.SetDirectory(dir)
	if len(cfg.Plugins) == 0 {
		return
	}
	specs, err := plugin.ParsePluginConfig(cfg.Plugins)
	if err != nil {
		slog.Warn("failed to parse plugin config", "error", err)
		return
	}
	for _, spec := range specs {
		if _, err := mgr.Load(spec.Name, spec.Options); err != nil {
			slog.Warn("failed to load plugin", "name", spec.Name, "error", err)
		}
	}
}
