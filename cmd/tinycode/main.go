package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"text/tabwriter"

	"golang.org/x/term"

	_ "github.com/bobbyjohnstx/tinycode-go/internal/earlyinit"

	"github.com/bobbyjohnstx/tinycode-go/internal/acp"
	"github.com/bobbyjohnstx/tinycode-go/internal/lsp"
	"github.com/bobbyjohnstx/tinycode-go/internal/agent"
	"github.com/bobbyjohnstx/tinycode-go/internal/bus"
	"github.com/bobbyjohnstx/tinycode-go/internal/config"
	"github.com/bobbyjohnstx/tinycode-go/internal/llm"
	"github.com/bobbyjohnstx/tinycode-go/internal/mcp"
	"github.com/bobbyjohnstx/tinycode-go/internal/permission"
	"github.com/bobbyjohnstx/tinycode-go/internal/plugin"
	"github.com/bobbyjohnstx/tinycode-go/internal/project"
	"github.com/bobbyjohnstx/tinycode-go/internal/provider"
	"github.com/bobbyjohnstx/tinycode-go/internal/server"
	"github.com/bobbyjohnstx/tinycode-go/internal/session"
	"github.com/bobbyjohnstx/tinycode-go/internal/storage"
	"github.com/bobbyjohnstx/tinycode-go/internal/tool"
	"github.com/bobbyjohnstx/tinycode-go/internal/tui"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	defer func() {
		if logFile != nil {
			logFile.Close()
		}
	}()

	if len(os.Args) < 2 {
		runTUI(nil)
		return
	}

	cmd := os.Args[1]

	// If the first arg is a flag, treat it as a TUI invocation with flags.
	if strings.HasPrefix(cmd, "-") {
		runTUI(os.Args[1:])
		return
	}

	// If the first arg is an existing directory, use it as the working directory.
	if info, err := os.Stat(cmd); err == nil && info.IsDir() {
		absDir, err := filepath.Abs(cmd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "resolving directory: %v\n", err)
			os.Exit(1)
		}
		if err := os.Chdir(absDir); err != nil {
			fmt.Fprintf(os.Stderr, "changing directory: %v\n", err)
			os.Exit(1)
		}
		runTUI(os.Args[2:])
		return
	}

	switch cmd {
	case "tui":
		runTUI(os.Args[2:])
	case "serve":
		runServe()
	case "web":
		runWeb()
	case "acp":
		runACP()
	case "run":
		runRun()
	case "models":
		runModels()
	case "providers":
		runProviders()
	case "session":
		runSession()
	case "status":
		runStatus()
	case "export":
		runExport()
	case "agent":
		runAgent()
	case "debug":
		runDebug()
	case "version", "--version", "-v":
		printVersion()
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printVersion() {
	fmt.Printf("tinycode %s (%s) built %s\n", version, commit, date)
	fmt.Printf("go %s %s/%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
}

func printUsage() {
	fmt.Println("tinycode - Local-LLM-first AI coding assistant")
	fmt.Println()
	fmt.Println("Usage: tinycode [command|directory] [flags]")
	fmt.Println()
	fmt.Println("Running with no command starts the terminal UI (same as 'tinycode tui').")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  tui        Start terminal UI (default)")
	fmt.Println("  run        Run a prompt non-interactively and exit")
	fmt.Println("  serve      Start headless API server (port 4096)")
	fmt.Println("  web        Start server and open web interface")
	fmt.Println("  acp        Agent Client Protocol mode (stdio, for IDE integration)")
	fmt.Println("  models     List available models")
	fmt.Println("  providers  List discovered providers")
	fmt.Println("  session    Manage sessions (list, delete)")
	fmt.Println("  status     Show server health and status")
	fmt.Println("  export     Export session messages as JSON")
	fmt.Println("  agent      List available agents")
	fmt.Println("  debug      Debug info (config, paths)")
	fmt.Println("  version    Print version information")
	fmt.Println("  help       Show this help message")
	fmt.Println()
	fmt.Println("Global flags (tui, run, serve, web):")
	fmt.Println("  -m, --model       Model to use (provider/model)")
	fmt.Println()
	fmt.Println("Run flags:")
	fmt.Println("  --agent           Agent to use (default: build)")
	fmt.Println("  --format          Output format: default, json")
	fmt.Println("  -c, --continue    Continue an existing session")
	fmt.Println("  -s, --session     Session ID to continue")
	fmt.Println("  --title           Session title")
	fmt.Println("  --dangerously-skip-permissions  Auto-approve all tool permissions")
	fmt.Println("  -i, --interactive  Show permission prompts (default: auto-deny)")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  tinycode                              Start TUI in current directory")
	fmt.Println("  tinycode ~/projects/myapp              Start TUI in specified directory")
	fmt.Println("  tinycode -m ollama/qwen3:8b            Start TUI with specific model")
	fmt.Println("  tinycode run -m ollama/qwen3:8b \"fix the bug\"")
	fmt.Println("  tinycode run ~/projects/myapp \"fix the bug\"")
	fmt.Println("  tinycode serve -m ollama/qwen3:8b      Start server with specific model")
	fmt.Println()
	fmt.Println("Environment:")
	fmt.Println("  TINYCODE_PORT       Override default server port (4096)")
	fmt.Println("  TINYCODE_HOST       Override default bind address (127.0.0.1)")
	fmt.Println("  TINYCODE_DB         Override database path")
	fmt.Println("  TINYCODE_LOG_LEVEL  Set log level (debug, info, warn, error)")
	fmt.Println("  TINYCODE_WEB_DIR    Serve web UI from directory (dev mode)")
	fmt.Println()
	fmt.Println("Logs: ~/.local/share/tinycode/tinycode.log")
}

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
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
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

func initTooling(b *bus.Bus, directory string) (*tool.Registry, *permission.Service, *tool.Context) {
	permSvc := permission.NewService(b)
	toolCtx := &tool.Context{
		Directory: directory,
		Perms:     permSvc,
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

func wireToolAfterHook(toolCtx *tool.Context, mgr *plugin.Manager) {
	toolCtx.AfterHook = func(sessionID, toolName, output string, isError bool) (string, bool, bool) {
		result, err := plugin.DispatchToolExecAfter(mgr, plugin.ToolExecAfterEvent{
			SessionID: sessionID,
			ToolName:  toolName,
			Output:    output,
			IsError:   isError,
		})
		if err != nil || result == nil {
			return "", false, false
		}
		return result.Output, result.IsError, true
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
			data, err := json.Marshal(apRaw)
			if err == nil {
				var apCfg provider.AutoProfileConfig
				if err := json.Unmarshal(data, &apCfg); err == nil {
					disc.SetAutoProfile(&apCfg)
				} else {
					slog.Warn("failed to parse auto_profile config", "error", err)
				}
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
		if _, err := mgr.Load(spec.Name); err != nil {
			slog.Warn("failed to load plugin", "name", spec.Name, "error", err)
		}
	}
}

type commonFlags struct {
	model string
}

func parseCommonFlags(name string, args []string) commonFlags {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	modelFlag := fs.String("m", "", "model to use (provider/model)")
	fs.StringVar(modelFlag, "model", "", "model to use (provider/model)")
	_ = fs.Parse(args)
	return commonFlags{model: *modelFlag}
}

func runTUI(args []string) {
	setupLogger()

	flags := parseCommonFlags("tui", args)

	b, db, cfg := initDependencies()
	defer db.Close()
	defer b.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	var mcpSvc *mcp.Service
	if len(cfg.MCP) > 0 {
		mcpSvc = mcp.NewService(b)
		defer mcpSvc.Close()
		mcpSvc.Configure(ctx, cfg.MCP)
	}

	reg := provider.NewRegistry()
	disc := startDiscovery(ctx, reg, b, cfg)
	defer disc.Stop()

	dir, _ := os.Getwd()
	agentReg := initAgentRegistry(cfg, dir)

	toolReg, permSvc, toolCtx := initTooling(b, dir)

	lspMgr := initLSP(dir, cfg, toolReg)
	defer lspMgr.Close()

	pluginMgr := plugin.NewManager(slog.Default())
	defer pluginMgr.Shutdown()
	loadConfigPlugins(pluginMgr, cfg, dir)
	wireToolAfterHook(toolCtx, pluginMgr)

	if flags.model != "" {
		cfg.Model = flags.model
	}

	srvCfg := serverConfig(cfg, false)
	srvCfg.Port = 0
	srv := server.New(srvCfg, server.Dependencies{
		Bus:           b,
		DB:            db.DB,
		Registry:      reg,
		AgentRegistry: agentReg,
		PluginManager: pluginMgr,
		ToolRegistry:  toolReg,
		PermService:   permSvc,
		MCPService:    mcpSvc,
		Config:        cfg,
	})

	listener, err := srv.Listen(ctx)
	if err != nil {
		slog.Error("failed to start embedded server", "error", err)
		os.Exit(1)
	}

	serverURL := listener.URL.String()
	slog.Debug("embedded server started", "url", serverURL)

	if err := tui.Run(ctx, tui.RunConfig{
		ServerURL: serverURL,
		Directory: dir,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "tui: %v\n", err)
		os.Exit(1)
	}

	cancel()
	srv.WaitForShutdown()
}

func runServe() {
	setupLogger()
	slog.Info("starting tinycode server", "version", version)

	flags := parseCommonFlags("serve", os.Args[2:])

	b, db, cfg := initDependencies()
	defer db.Close()
	defer b.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	var mcpSvc *mcp.Service
	if len(cfg.MCP) > 0 {
		mcpSvc = mcp.NewService(b)
		defer mcpSvc.Close()
		mcpSvc.Configure(ctx, cfg.MCP)
	}

	reg := provider.NewRegistry()
	disc := startDiscovery(ctx, reg, b, cfg)
	defer disc.Stop()

	dir, _ := os.Getwd()
	agentReg := initAgentRegistry(cfg, dir)

	toolReg, permSvc, toolCtx := initTooling(b, dir)

	lspMgr := initLSP(dir, cfg, toolReg)
	defer lspMgr.Close()

	pluginMgr := plugin.NewManager(slog.Default())
	defer pluginMgr.Shutdown()
	loadConfigPlugins(pluginMgr, cfg, dir)
	wireToolAfterHook(toolCtx, pluginMgr)

	if flags.model != "" {
		cfg.Model = flags.model
	}

	srv := server.New(serverConfig(cfg, false), server.Dependencies{
		Bus:           b,
		DB:            db.DB,
		Registry:      reg,
		AgentRegistry: agentReg,
		PluginManager: pluginMgr,
		ToolRegistry:  toolReg,
		PermService:   permSvc,
		MCPService:    mcpSvc,
		Config:        cfg,
	})

	listener, err := srv.Listen(ctx)
	if err != nil {
		slog.Error("failed to start server", "error", err)
		os.Exit(1)
	}

	slog.Info("server ready", "url", listener.URL.String())

	<-ctx.Done()
	srv.WaitForShutdown()
	slog.Info("server stopped")
}

func runWeb() {
	setupLogger()
	slog.Info("starting tinycode web", "version", version)

	flags := parseCommonFlags("web", os.Args[2:])

	b, db, cfg := initDependencies()
	defer db.Close()
	defer b.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	var mcpSvc *mcp.Service
	if len(cfg.MCP) > 0 {
		mcpSvc = mcp.NewService(b)
		defer mcpSvc.Close()
		mcpSvc.Configure(ctx, cfg.MCP)
	}

	reg := provider.NewRegistry()
	disc := startDiscovery(ctx, reg, b, cfg)
	defer disc.Stop()

	dir, _ := os.Getwd()
	agentReg := initAgentRegistry(cfg, dir)

	toolReg, permSvc, toolCtx := initTooling(b, dir)

	lspMgr := initLSP(dir, cfg, toolReg)
	defer lspMgr.Close()

	pluginMgr := plugin.NewManager(slog.Default())
	defer pluginMgr.Shutdown()
	loadConfigPlugins(pluginMgr, cfg, dir)
	wireToolAfterHook(toolCtx, pluginMgr)

	if flags.model != "" {
		cfg.Model = flags.model
	}

	srv := server.New(serverConfig(cfg, true), server.Dependencies{
		Bus:           b,
		DB:            db.DB,
		Registry:      reg,
		AgentRegistry: agentReg,
		PluginManager: pluginMgr,
		ToolRegistry:  toolReg,
		PermService:   permSvc,
		MCPService:    mcpSvc,
		Config:        cfg,
	})

	listener, err := srv.Listen(ctx)
	if err != nil {
		slog.Error("failed to start server", "error", err)
		os.Exit(1)
	}

	url := listener.URL.String()
	slog.Info("web UI ready", "url", url)

	openBrowser(url)

	<-ctx.Done()
	srv.WaitForShutdown()
	slog.Info("server stopped")
}

func runACP() {
	setupLogger()
	slog.Info("starting tinycode ACP mode", "version", version)

	b, db, _ := initDependencies()
	defer db.Close()
	defer b.Close()

	store := session.NewStore(db.DB)
	adapter := acp.NewStoreAdapter(store)
	svc := acp.NewService(adapter, b)

	transport := acp.NewStdioTransport(svc, os.Stdout)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := transport.HandleStdio(ctx, os.Stdin); err != nil {
		slog.Error("ACP transport error", "error", err)
		os.Exit(1)
	}
}

func runRun() {
	setupLogger()

	fs := flag.NewFlagSet("run", flag.ExitOnError)
	modelFlag := fs.String("m", "", "model to use (provider/model)")
	fs.StringVar(modelFlag, "model", "", "model to use (provider/model)")
	agentFlag := fs.String("agent", "", "agent to use")
	formatFlag := fs.String("format", "default", "output format: default, json")
	continueFlag := fs.Bool("c", false, "continue most recent session")
	fs.BoolVar(continueFlag, "continue", false, "continue most recent session")
	sessionFlag := fs.String("s", "", "session ID to continue")
	fs.StringVar(sessionFlag, "session", "", "session ID to continue")
	titleFlag := fs.String("title", "", "session title")
	skipPermsFlag := fs.Bool("dangerously-skip-permissions", false, "auto-approve all tool permissions")
	interactiveFlag := fs.Bool("i", false, "show permission prompts (default: auto-deny)")
	fs.BoolVar(interactiveFlag, "interactive", false, "show permission prompts (default: auto-deny)")
	_ = fs.Parse(os.Args[2:])

	// If the first positional arg is a directory, chdir to it.
	positional := fs.Args()
	if len(positional) > 0 {
		if info, err := os.Stat(positional[0]); err == nil && info.IsDir() {
			absDir, err := filepath.Abs(positional[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "resolving directory: %v\n", err)
				os.Exit(1)
			}
			if err := os.Chdir(absDir); err != nil {
				fmt.Fprintf(os.Stderr, "changing directory: %v\n", err)
				os.Exit(1)
			}
			positional = positional[1:]
		}
	}

	// Collect prompt from remaining args + stdin
	prompt := strings.Join(positional, " ")
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		stdinData, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "reading stdin: %v\n", err)
			os.Exit(1)
		}
		if s := strings.TrimSpace(string(stdinData)); s != "" {
			if prompt != "" {
				prompt += "\n\n"
			}
			prompt += s
		}
	}
	if prompt == "" {
		fmt.Fprintf(os.Stderr, "error: no prompt provided\n")
		os.Exit(1)
	}

	b, db, cfg := initDependencies()
	defer db.Close()
	defer b.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	var mcpSvc *mcp.Service
	if len(cfg.MCP) > 0 {
		mcpSvc = mcp.NewService(b)
		defer mcpSvc.Close()
		mcpSvc.Configure(ctx, cfg.MCP)
	}

	reg := provider.NewRegistry()
	disc := startDiscovery(ctx, reg, b, cfg)
	defer disc.Stop()

	dir, _ := os.Getwd()
	agentReg := initAgentRegistry(cfg, dir)

	toolReg, permSvc, toolCtx := initTooling(b, dir)

	lspMgr := initLSP(dir, cfg, toolReg)
	defer lspMgr.Close()

	// Configure permission behavior via bus subscriber
	if *skipPermsFlag || !*interactiveFlag {
		permSub := b.Subscribe("permission.asked")
		go func() {
			for evt := range permSub.C {
				req, ok := evt.Properties.(permission.Request)
				if !ok {
					continue
				}
				reply := permission.ReplyReject
				if *skipPermsFlag {
					reply = permission.ReplyOnce
				}
				permSvc.RespondToAsk(permission.ReplyInput{
					RequestID: req.ID,
					Reply:     reply,
				})
			}
		}()
	} else {
		// Interactive mode: prompt on stderr
		permSub := b.Subscribe("permission.asked")
		go func() {
			scanner := bufio.NewScanner(os.Stdin)
			for evt := range permSub.C {
				req, ok := evt.Properties.(permission.Request)
				if !ok {
					continue
				}
				fmt.Fprintf(os.Stderr, "Permission requested: %s %v\nAllow? [y/N]: ", req.Permission, req.Patterns)
				reply := permission.ReplyReject
				if scanner.Scan() && strings.TrimSpace(strings.ToLower(scanner.Text())) == "y" {
					reply = permission.ReplyOnce
				}
				permSvc.RespondToAsk(permission.ReplyInput{
					RequestID: req.ID,
					Reply:     reply,
				})
			}
		}()
	}

	pluginMgr := plugin.NewManager(slog.Default())
	defer pluginMgr.Shutdown()
	loadConfigPlugins(pluginMgr, cfg, dir)
	wireToolAfterHook(toolCtx, pluginMgr)

	// Resolve model
	modelStr := *modelFlag
	if modelStr == "" {
		modelStr = cfg.Model
	}
	if modelStr == "" {
		fmt.Fprintf(os.Stderr, "error: no model specified (use --model or set model in config)\n")
		os.Exit(1)
	}

	providerID, modelID := provider.ParseModel(modelStr)
	if providerID == "" {
		// Try to find the model across all providers
		models := reg.ListModels()
		for _, m := range models {
			if m.ID == modelID || m.Name == modelID {
				providerID = m.ProviderID
				modelID = m.ID
				break
			}
		}
	}
	if providerID == "" {
		fmt.Fprintf(os.Stderr, "error: could not find model %q — specify as provider/model\n", modelStr)
		os.Exit(1)
	}

	model, err := reg.GetModel(providerID, modelID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Resolve agent
	agentName := *agentFlag
	if agentName == "" {
		agentName = cfg.DefaultAgent
	}
	if agentName == "" {
		agentName = "build"
	}

	agentInfo := agentReg.Get(agentName, model.SizeB())
	var agentPrompt string
	var agentPerms []string
	if agentInfo != nil {
		agentPrompt = agentInfo.Prompt
		for _, rule := range agentInfo.Permission {
			if rule.Action == permission.ActionAllow {
				agentPerms = append(agentPerms, rule.Permission)
			}
		}
	}

	// Build system prompt
	var instructions string
	if len(cfg.Instructions) > 0 {
		instructions = strings.Join(cfg.Instructions, "\n\n")
	}
	systemPrompt := session.BuildSystemPrompt(session.SystemPromptInput{
		AgentPrompt:  agentPrompt,
		Instructions: instructions,
		Directory:    dir,
		ToolDefs:     toolReg.ToolDefs(agentPerms),
	})

	// Sync MCP tools
	if mcpSvc != nil {
		mcpTools := mcpSvc.Tools(ctx)
		for _, def := range mcpTools {
			toolReg.Register(def)
		}
	}

	// Create or continue session
	store := session.NewStore(db.DB)
	ms := session.NewMessageStore(store)
	projectID := project.IDFromDirectory(dir)

	var sessionID string
	var existingMsgs []session.Message

	if *sessionFlag != "" {
		sessionID = *sessionFlag
		info, err := store.Get(sessionID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: session %q not found: %v\n", sessionID, err)
			os.Exit(1)
		}
		_ = info
		existingMsgs, _ = ms.List(sessionID)
	} else if *continueFlag {
		sessions, err := store.List(projectID, 1, 0)
		if err != nil || len(sessions) == 0 {
			fmt.Fprintf(os.Stderr, "error: no sessions to continue\n")
			os.Exit(1)
		}
		sessionID = sessions[0].ID
		existingMsgs, _ = ms.List(sessionID)
	} else {
		title := *titleFlag
		info, err := store.Create(session.CreateInput{
			ProjectID: projectID,
			Directory: dir,
			Title:     title,
			Agent:     agentName,
			Model:     &session.ModelRef{ID: modelID, ProviderID: providerID},
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "error creating session: %v\n", err)
			os.Exit(1)
		}
		sessionID = info.ID
	}

	// Create LLM client
	apiKey := ""
	if model.Options != nil {
		if key, ok := model.Options["api_key"].(string); ok {
			apiKey = key
		}
	}
	client := llm.NewOpenAIClient(model.API.URL+"/v1", apiKey)

	// Create and run processor
	compactionCfg := session.DefaultCompactionConfig()
	proc := session.NewProcessor(session.ProcessorConfig{
		SessionID:    sessionID,
		Agent:        agentName,
		Model:        model,
		SystemPrompt: systemPrompt,
		Compaction:   compactionCfg,
		AgentPerms:   agentPerms,
	}, client, toolReg, b)
	proc.SetMessages(existingMsgs)

	// Subscribe to text deltas for streaming output
	deltaSub := b.Subscribe("session.text.delta")
	toolBeginSub := b.Subscribe("session.tool.begin")
	toolEndSub := b.Subscribe("session.tool.end")

	isJSON := *formatFlag == "json"

	go func() {
		for evt := range deltaSub.C {
			props, ok := evt.Properties.(map[string]any)
			if !ok {
				continue
			}
			text, _ := props["text"].(string)
			if isJSON {
				line, _ := json.Marshal(map[string]any{"type": "text", "text": text})
				fmt.Println(string(line))
			} else {
				fmt.Print(text)
			}
		}
	}()
	go func() {
		for evt := range toolBeginSub.C {
			props, ok := evt.Properties.(map[string]any)
			if !ok {
				continue
			}
			if isJSON {
				line, _ := json.Marshal(map[string]any{
					"type":       "tool_begin",
					"toolName":   props["toolName"],
					"toolCallID": props["toolCallID"],
				})
				fmt.Println(string(line))
			}
		}
	}()
	go func() {
		for evt := range toolEndSub.C {
			props, ok := evt.Properties.(map[string]any)
			if !ok {
				continue
			}
			if isJSON {
				line, _ := json.Marshal(map[string]any{
					"type":       "tool_end",
					"toolName":   props["toolName"],
					"toolCallID": props["toolCallID"],
					"toolArgs":   props["toolArgs"],
				})
				fmt.Println(string(line))
			}
		}
	}()

	result := proc.Process(ctx, prompt)

	// Persist messages
	if result != nil && len(result.Messages) > len(existingMsgs) {
		newMsgs := result.Messages[len(existingMsgs):]
		for i := range newMsgs {
			_ = ms.Append(&newMsgs[i])
		}
		_ = store.UpdateCost(sessionID, 0, session.TokenUsage{
			Input:  result.Usage.Input,
			Output: result.Usage.Output,
		})
	}

	if !isJSON {
		fmt.Println() // ensure trailing newline
	}

	if result != nil && result.Error != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", result.Error)
		os.Exit(1)
	}
}

func runModels() {
	setupLogger()

	b, _, cfg := initDependencies()
	defer b.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reg := provider.NewRegistry()
	disc := startDiscovery(ctx, reg, b, cfg)
	defer disc.Stop()

	models := reg.ListModels()
	if len(models) == 0 {
		fmt.Println("No models discovered.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "PROVIDER\tMODEL\tCONTEXT\tSTATUS")
	for _, m := range models {
		ctxStr := "-"
		if m.Limit.Context > 0 {
			ctxStr = fmt.Sprintf("%dk", m.Limit.Context/1000)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", m.ProviderID, m.ID, ctxStr, m.Status)
	}
	w.Flush()
}

func runProviders() {
	setupLogger()

	b, _, cfg := initDependencies()
	defer b.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reg := provider.NewRegistry()
	disc := startDiscovery(ctx, reg, b, cfg)
	defer disc.Stop()

	providers := reg.ListProviders()
	if len(providers) == 0 {
		fmt.Println("No providers discovered.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tSOURCE\tMODELS")
	for _, p := range providers {
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\n", p.ID, p.Name, p.Source, len(p.Models))
	}
	w.Flush()
}

func runSession() {
	setupLogger()

	args := os.Args[2:]
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: tinycode session <list|delete> [args]\n")
		os.Exit(1)
	}

	_, db, _ := initDependencies()
	defer db.Close()

	dir, _ := os.Getwd()
	store := session.NewStore(db.DB)
	projectID := project.IDFromDirectory(dir)

	switch args[0] {
	case "list":
		sessions, err := store.List(projectID, 50, 0)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if len(sessions) == 0 {
			fmt.Println("No sessions.")
			return
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tTITLE\tAGENT\tCREATED")
		for _, s := range sessions {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s.ID, truncateStr(s.Title, 40), s.Agent, s.CreatedAt.Format("2006-01-02 15:04"))
		}
		w.Flush()

	case "delete":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: tinycode session delete <session-id>\n")
			os.Exit(1)
		}
		if err := store.Delete(args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Deleted session %s\n", args[1])

	default:
		fmt.Fprintf(os.Stderr, "Unknown session subcommand: %s\nUsage: tinycode session <list|delete> [args]\n", args[0])
		os.Exit(1)
	}
}

func runStatus() {
	setupLogger()

	b, db, cfg := initDependencies()
	defer db.Close()
	defer b.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reg := provider.NewRegistry()
	disc := startDiscovery(ctx, reg, b, cfg)
	defer disc.Stop()

	providers := reg.ListProviders()
	models := reg.ListModels()

	fmt.Printf("tinycode %s (%s)\n", version, commit)
	fmt.Printf("Providers: %d\n", len(providers))
	fmt.Printf("Models:    %d\n", len(models))
	fmt.Printf("Database:  %s\n", storage.DefaultPath())
	fmt.Printf("Config:    %s\n", config.GlobalConfigFile())
	fmt.Printf("Data dir:  %s\n", config.DataDir())

	dir, _ := os.Getwd()
	store := session.NewStore(db.DB)
	projectID := project.IDFromDirectory(dir)
	sessions, err := store.List(projectID, 1000, 0)
	if err == nil {
		fmt.Printf("Sessions:  %d (this project)\n", len(sessions))
	}

	if cfg.Model != "" {
		fmt.Printf("Default model: %s\n", cfg.Model)
	}
}

func runExport() {
	setupLogger()

	args := os.Args[2:]
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: tinycode export <session-id>\n")
		os.Exit(1)
	}

	_, db, _ := initDependencies()
	defer db.Close()

	store := session.NewStore(db.DB)
	ms := session.NewMessageStore(store)

	sessionID := args[0]
	info, err := store.Get(sessionID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: session %q not found: %v\n", sessionID, err)
		os.Exit(1)
	}

	messages, err := ms.List(sessionID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	export := map[string]any{
		"session":  info,
		"messages": messages,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(export); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func runAgent() {
	setupLogger()

	_, _, cfg := initDependencies()

	dir, _ := os.Getwd()
	agentReg := initAgentRegistry(cfg, dir)

	agents := agentReg.List(cfg.DefaultAgent)
	if len(agents) == 0 {
		fmt.Println("No agents available.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tMODE\tDESCRIPTION")
	for _, a := range agents {
		desc := truncateStr(a.Description, 50)
		fmt.Fprintf(w, "%s\t%s\t%s\n", a.Name, a.Mode, desc)
	}
	w.Flush()
}

func runDebug() {
	setupLogger()

	args := os.Args[2:]
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: tinycode debug <config|paths>\n")
		os.Exit(1)
	}

	switch args[0] {
	case "config":
		dir, _ := os.Getwd()
		cfg, err := config.Load(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
			os.Exit(1)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(cfg)

	case "paths":
		dir, _ := os.Getwd()
		fmt.Printf("Config dir:  %s\n", config.ConfigDir())
		fmt.Printf("Data dir:    %s\n", config.DataDir())
		fmt.Printf("Config file: %s\n", config.GlobalConfigFile())
		fmt.Printf("Database:    %s\n", storage.DefaultPath())
		fmt.Printf("Working dir: %s\n", dir)

		projectFiles := config.ProjectConfigFiles("tinycode", dir)
		if len(projectFiles) > 0 {
			fmt.Println("Project config files:")
			for _, f := range projectFiles {
				fmt.Printf("  %s\n", f)
			}
		}
		dotDirs := config.ProjectDotDirs(dir)
		if len(dotDirs) > 0 {
			fmt.Println("Project .tinycode dirs:")
			for _, d := range dotDirs {
				fmt.Printf("  %s\n", d)
			}
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown debug subcommand: %s\nUsage: tinycode debug <config|paths>\n", args[0])
		os.Exit(1)
	}
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max < 4 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return
	}
	_ = cmd.Start()
}
