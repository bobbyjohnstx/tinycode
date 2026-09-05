package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/bobbyjohnstx/tinycode-go/internal/acp"
	"github.com/bobbyjohnstx/tinycode-go/internal/agent"
	"github.com/bobbyjohnstx/tinycode-go/internal/bus"
	"github.com/bobbyjohnstx/tinycode-go/internal/config"
	"github.com/bobbyjohnstx/tinycode-go/internal/mcp"
	"github.com/bobbyjohnstx/tinycode-go/internal/permission"
	"github.com/bobbyjohnstx/tinycode-go/internal/plugin"
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
		runTUI()
		return
	}

	cmd := os.Args[1]
	switch cmd {
	case "tui":
		runTUI()
	case "serve":
		runServe()
	case "web":
		runWeb()
	case "acp":
		runACP()
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
	fmt.Println("Usage: tinycode [command] [flags]")
	fmt.Println()
	fmt.Println("Running with no command starts the terminal UI (same as 'tinycode tui').")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  tui        Start terminal UI (default)")
	fmt.Println("  serve      Start headless API server (port 4096)")
	fmt.Println("  web        Start server and open web interface")
	fmt.Println("  acp        Agent Client Protocol mode (stdio, for IDE integration)")
	fmt.Println("  version    Print version information")
	fmt.Println("  help       Show this help message")
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
	return reg
}

func initTooling(b *bus.Bus, directory string) (*tool.Registry, *permission.Service) {
	permSvc := permission.NewService(b)
	toolCtx := &tool.Context{
		Directory: directory,
		Perms:     permSvc,
	}
	toolReg := tool.NewRegistry(toolCtx)
	tool.RegisterBuiltins(toolReg)
	return toolReg, permSvc
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

	return disc
}

func runTUI() {
	setupLogger()

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

	toolReg, permSvc := initTooling(b, dir)

	pluginMgr := plugin.NewManager(slog.Default())
	defer pluginMgr.Shutdown()

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
}

func runServe() {
	setupLogger()
	slog.Info("starting tinycode server", "version", version)

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

	toolReg, permSvc := initTooling(b, dir)

	pluginMgr := plugin.NewManager(slog.Default())
	defer pluginMgr.Shutdown()

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
	slog.Info("server stopped")
}

func runWeb() {
	setupLogger()
	slog.Info("starting tinycode web", "version", version)

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

	toolReg, permSvc := initTooling(b, dir)

	pluginMgr := plugin.NewManager(slog.Default())
	defer pluginMgr.Shutdown()

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
