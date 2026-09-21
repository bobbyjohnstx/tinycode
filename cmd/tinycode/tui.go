package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/bobbyjohnstx/tinycode-go/internal/mcp"
	"github.com/bobbyjohnstx/tinycode-go/internal/plugin"
	"github.com/bobbyjohnstx/tinycode-go/internal/provider"
	"github.com/bobbyjohnstx/tinycode-go/internal/server"
	"github.com/bobbyjohnstx/tinycode-go/internal/tui"
)

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

	builtinMgr := initBuiltins(toolReg)

	pluginMgr := plugin.NewManager(slog.Default())
	defer pluginMgr.Shutdown()
	loadConfigPlugins(pluginMgr, cfg, dir)
	wireToolAfterHook(toolCtx, pluginMgr, builtinMgr)

	if flags.model != "" {
		cfg.Model = flags.model
	}

	token := generateToken()
	slog.Debug("generated auth token", "token", token)

	srvCfg := serverConfig(cfg, false)
	srvCfg.Port = 0
	srvCfg.Token = token
	srv := server.New(srvCfg, server.Dependencies{
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

	toolCtx.SubagentRunner = func(subCtx context.Context, parentSessionID, prompt, agent, directory string) (string, error) {
		return srv.RunSubagent(subCtx, parentSessionID, prompt, agent, directory)
	}

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
		Theme:     cfg.Theme,
		Token:     token,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "tui: %v\n", err)
		os.Exit(1)
	}

	cancel()
	srv.WaitForShutdown()
}
