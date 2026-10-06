package main

import (
	"context"
	"encoding/base64"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/bobbyjohnstx/tinycode/internal/mcp"
	"github.com/bobbyjohnstx/tinycode/internal/plugin"
	"github.com/bobbyjohnstx/tinycode/internal/provider"
	"github.com/bobbyjohnstx/tinycode/internal/server"
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

	toolReg, permSvc, toolCtx := initTooling(b, dir, cfg)

	lspMgr := initLSP(dir, cfg, toolReg)
	defer lspMgr.Close()

	builtinMgr := initBuiltins(toolReg)

	pluginMgr := plugin.NewManager(slog.Default())
	defer pluginMgr.Shutdown()
	loadConfigPlugins(pluginMgr, toolReg, cfg, dir)
	shellRunner := plugin.NewShellHookRunner(cfg.Hooks, slog.Default())
	wireToolBeforeHook(toolCtx, pluginMgr, shellRunner)
	wireToolAfterHook(toolCtx, pluginMgr, builtinMgr, shellRunner)
	wirePermissionAskHook(permSvc, pluginMgr)

	if flags.model != "" {
		cfg.Model = flags.model
	}

	serveCfg := serverConfig(cfg, false)

	serveToken := resolveServeAuthToken(serveCfg.Hostname)
	serveCfg.Token = serveToken
	srv := server.New(serveCfg, server.Dependencies{
		Bus:             b,
		DB:              db.DB,
		Registry:        reg,
		AgentRegistry:   agentReg,
		PluginManager:   pluginMgr,
		BuiltinManager:  builtinMgr,
		ShellHookRunner: shellRunner,
		ToolRegistry:    toolReg,
		PermService:     permSvc,
		MCPService:      mcpSvc,
		Config:          cfg,
		JobManager:      toolCtx.JobManager,
		Discovery:       disc,
	})

	toolCtx.SubagentRunner = func(subCtx context.Context, parentSessionID string, parentDepth int, prompt, agent, directory string, autoApprove bool) (string, error) {
		return srv.RunSubagent(subCtx, parentSessionID, parentDepth, prompt, agent, directory, autoApprove)
	}

	listener, err := srv.Listen(ctx)
	if err != nil {
		slog.Error("failed to start server", "error", err)
		os.Exit(1)
	}

	slog.Info("server ready", "url", listener.URL.String())
	logServeAuthToken(serveToken, listener.URL.String())

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
	ensureProject(db.DB, dir)
	agentReg := initAgentRegistry(cfg, dir)

	toolReg, permSvc, toolCtx := initTooling(b, dir, cfg)

	lspMgr := initLSP(dir, cfg, toolReg)
	defer lspMgr.Close()

	builtinMgr := initBuiltins(toolReg)

	pluginMgr := plugin.NewManager(slog.Default())
	defer pluginMgr.Shutdown()
	loadConfigPlugins(pluginMgr, toolReg, cfg, dir)
	shellRunner := plugin.NewShellHookRunner(cfg.Hooks, slog.Default())
	wireToolBeforeHook(toolCtx, pluginMgr, shellRunner)
	wireToolAfterHook(toolCtx, pluginMgr, builtinMgr, shellRunner)
	wirePermissionAskHook(permSvc, pluginMgr)

	if flags.model != "" {
		cfg.Model = flags.model
	}

	webCfg := serverConfig(cfg, true)

	webToken := resolveWebAuthToken(webCfg.Hostname)
	webCfg.Token = webToken
	srv := server.New(webCfg, server.Dependencies{
		Bus:             b,
		DB:              db.DB,
		Registry:        reg,
		AgentRegistry:   agentReg,
		PluginManager:   pluginMgr,
		BuiltinManager:  builtinMgr,
		ShellHookRunner: shellRunner,
		ToolRegistry:    toolReg,
		PermService:     permSvc,
		MCPService:      mcpSvc,
		Config:          cfg,
		JobManager:      toolCtx.JobManager,
		Discovery:       disc,
	})

	toolCtx.SubagentRunner = func(subCtx context.Context, parentSessionID string, parentDepth int, prompt, agent, directory string, autoApprove bool) (string, error) {
		return srv.RunSubagent(subCtx, parentSessionID, parentDepth, prompt, agent, directory, autoApprove)
	}

	listener, err := srv.Listen(ctx)
	if err != nil {
		slog.Error("failed to start server", "error", err)
		os.Exit(1)
	}

	baseURL := listener.URL.String()
	authParam := base64.StdEncoding.EncodeToString([]byte("tinycode:" + webToken))
	browserURL := baseURL + "?auth_token=" + authParam
	slog.Info("web UI ready", "url", baseURL)
	logServeAuthToken(webToken, baseURL)

	openBrowser(browserURL)

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

// resolveServeAuthToken returns the bearer token for headless serve mode.
// TINYCODE_NO_AUTH disables auth. TINYCODE_AUTH_TOKEN is preferred;
// TINYCODE_SERVER_PASSWORD is accepted as a deprecated alias.
func resolveServeAuthToken(hostname string) string {
	if os.Getenv("TINYCODE_NO_AUTH") != "" {
		slog.Info("auth disabled via TINYCODE_NO_AUTH")
		checkNoAuthSafety(hostname)
		return ""
	}
	token := os.Getenv("TINYCODE_AUTH_TOKEN")
	if token == "" {
		if legacy := os.Getenv("TINYCODE_SERVER_PASSWORD"); legacy != "" {
			slog.Warn("TINYCODE_SERVER_PASSWORD is deprecated; use TINYCODE_AUTH_TOKEN")
			token = legacy
		}
	}
	if token == "" {
		token = generateToken()
	}
	return token
}

// resolveWebAuthToken returns the bearer token for web mode (persistent file fallback).
func resolveWebAuthToken(hostname string) string {
	if os.Getenv("TINYCODE_NO_AUTH") != "" {
		slog.Info("auth disabled via TINYCODE_NO_AUTH")
		checkNoAuthSafety(hostname)
		return ""
	}
	token := os.Getenv("TINYCODE_AUTH_TOKEN")
	if token == "" {
		if legacy := os.Getenv("TINYCODE_SERVER_PASSWORD"); legacy != "" {
			slog.Warn("TINYCODE_SERVER_PASSWORD is deprecated; use TINYCODE_AUTH_TOKEN")
			token = legacy
		}
	}
	if token == "" {
		token = loadOrCreateWebToken()
	}
	return token
}

func logServeAuthToken(token, baseURL string) {
	if token == "" {
		return
	}
	slog.Info("authentication required",
		"usage", "Authorization: Bearer <token>",
		"url", baseURL,
		"token", token,
	)
}
