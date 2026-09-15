package main

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/bobbyjohnstx/tinycode-go/internal/mcp"
	"github.com/bobbyjohnstx/tinycode-go/internal/plugin"
	"github.com/bobbyjohnstx/tinycode-go/internal/provider"
	"github.com/bobbyjohnstx/tinycode-go/internal/server"
)

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

	builtinMgr := initBuiltins(toolReg)

	pluginMgr := plugin.NewManager(slog.Default())
	defer pluginMgr.Shutdown()
	loadConfigPlugins(pluginMgr, cfg, dir)
	wireToolAfterHook(toolCtx, pluginMgr, builtinMgr)

	if flags.model != "" {
		cfg.Model = flags.model
	}

	serveToken := generateToken()
	slog.Debug("generated auth token for serve mode", "token", serveToken)

	serveCfg := serverConfig(cfg, false)
	serveCfg.Token = serveToken
	srv := server.New(serveCfg, server.Dependencies{
		Bus:            b,
		DB:             db.DB,
		Registry:       reg,
		AgentRegistry:  agentReg,
		PluginManager:  pluginMgr,
		BuiltinManager: builtinMgr,
		ToolRegistry:   toolReg,
		PermService:    permSvc,
		MCPService:     mcpSvc,
		Config:         cfg,
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

	builtinMgr := initBuiltins(toolReg)

	pluginMgr := plugin.NewManager(slog.Default())
	defer pluginMgr.Shutdown()
	loadConfigPlugins(pluginMgr, cfg, dir)
	wireToolAfterHook(toolCtx, pluginMgr, builtinMgr)

	if flags.model != "" {
		cfg.Model = flags.model
	}

	webToken := generateToken()
	slog.Debug("generated auth token for web mode", "token", webToken)

	webCfg := serverConfig(cfg, true)
	webCfg.Token = webToken
	srv := server.New(webCfg, server.Dependencies{
		Bus:            b,
		DB:             db.DB,
		Registry:       reg,
		AgentRegistry:  agentReg,
		PluginManager:  pluginMgr,
		BuiltinManager: builtinMgr,
		ToolRegistry:   toolReg,
		PermService:    permSvc,
		MCPService:     mcpSvc,
		Config:         cfg,
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
