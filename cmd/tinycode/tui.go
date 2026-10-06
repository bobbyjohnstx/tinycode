package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/bobbyjohnstx/tinycode/internal/agent"
	"github.com/bobbyjohnstx/tinycode/internal/mcp"
	"github.com/bobbyjohnstx/tinycode/internal/permission"
	"github.com/bobbyjohnstx/tinycode/internal/plugin"
	"github.com/bobbyjohnstx/tinycode/internal/project"
	"github.com/bobbyjohnstx/tinycode/internal/provider"
	"github.com/bobbyjohnstx/tinycode/internal/server"
	"github.com/bobbyjohnstx/tinycode/internal/session"
	"github.com/bobbyjohnstx/tinycode/internal/tui"
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
	if !flags.safeMode && len(cfg.MCP) > 0 {
		mcpSvc = mcp.NewService(b)
		defer mcpSvc.Close()
		mcpSvc.Configure(ctx, cfg.MCP)
	}

	reg := provider.NewRegistry()
	disc := startDiscovery(ctx, reg, b, cfg)
	defer disc.Stop()

	dir, _ := os.Getwd()
	var agentReg *agent.Registry
	if flags.safeMode {
		agentReg = agent.NewRegistry()
		defaultPerms := permission.Ruleset{
			{Permission: "*", Pattern: "*", Action: permission.ActionAllow},
		}
		if err := agentReg.LoadDefaults(defaultPerms, nil); err != nil {
			slog.Warn("failed to load default agents", "error", err)
		}
	} else {
		agentReg = initAgentRegistry(cfg, dir)
	}

	toolReg, permSvc, toolCtx := initTooling(b, dir, cfg)

	lspMgr := initLSP(dir, cfg, toolReg)
	defer lspMgr.Close()

	builtinMgr := initBuiltins(toolReg)

	pluginMgr := plugin.NewManager(slog.Default())
	defer pluginMgr.Shutdown()
	if !flags.safeMode {
		loadConfigPlugins(pluginMgr, toolReg, cfg, dir)
	}
	shellRunner := plugin.NewShellHookRunner(cfg.Hooks, slog.Default())
	wireToolBeforeHook(toolCtx, pluginMgr, shellRunner)
	wireToolAfterHook(toolCtx, pluginMgr, builtinMgr, shellRunner)
	wirePermissionAskHook(permSvc, pluginMgr)

	if flags.model != "" {
		cfg.Model = flags.model
	}

	token := generateToken()
	slog.Debug("generated auth token", "token", token)

	srvCfg := serverConfig(cfg, false)
	srvCfg.Port = 0
	srvCfg.Token = token
	srvCfg.AppendSystemPrompt = flags.appendSystemPrompt
	srvCfg.TokenBudget = flags.maxTokens
	srv := server.New(srvCfg, server.Dependencies{
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
		slog.Error("failed to start embedded server", "error", err)
		os.Exit(1)
	}

	serverURL := listener.URL.String()
	slog.Debug("embedded server started", "url", serverURL)

	resumeSessionID := resolveResumeSession(db.DB, flags, dir)

	if err := tui.Run(ctx, tui.RunConfig{
		ServerURL:       serverURL,
		Directory:       dir,
		Theme:           cfg.Theme,
		Token:           token,
		Version:         version,
		ScopedModels:    cfg.ScopedModels,
		ShellHooks:      cfg.Hooks,
		InitialTitle:    flags.title,
		SafeMode:        flags.safeMode,
		ResumeSessionID: resumeSessionID,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "tui: %v\n", err)
		os.Exit(1)
	}

	cancel()
	srv.WaitForShutdown()
}

// resolveResumeSession resolves a session ID from --continue or --resume flags.
// Returns empty string if no resume was requested or the session was not found.
func resolveResumeSession(sqlDB *sql.DB, flags commonFlags, dir string) string {
	if !flags.continueSession && flags.resumeSession == "" {
		return ""
	}

	store := session.NewStore(sqlDB)
	projectID := project.IDFromDirectory(dir)

	if flags.continueSession {
		sessions, err := store.List(projectID, 1, 0, false)
		if err != nil || len(sessions) == 0 {
			slog.Warn("no sessions to continue")
			return ""
		}
		return sessions[0].ID
	}

	// --resume: try as session ID first, then search by title/slug
	if info, err := store.Get(flags.resumeSession); err == nil {
		return info.ID
	}
	sessions, err := store.List(projectID, 50, 0, false)
	if err != nil {
		return ""
	}
	for _, s := range sessions {
		if s.Title == flags.resumeSession || s.Slug == flags.resumeSession {
			return s.ID
		}
	}
	slog.Warn("session not found", "resume", flags.resumeSession)
	return ""
}
