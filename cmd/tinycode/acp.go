package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/bobbyjohnstx/tinycode/internal/acp"
	"github.com/bobbyjohnstx/tinycode/internal/agent"
	"github.com/bobbyjohnstx/tinycode/internal/mcp"
	"github.com/bobbyjohnstx/tinycode/internal/permission"
	"github.com/bobbyjohnstx/tinycode/internal/plugin"
	"github.com/bobbyjohnstx/tinycode/internal/provider"
	"github.com/bobbyjohnstx/tinycode/internal/safego"
	"github.com/bobbyjohnstx/tinycode/internal/server"
	"github.com/bobbyjohnstx/tinycode/internal/session"
)

func runACP(args []string) {
	setupLogger()
	slog.Info("starting tinycode ACP mode", "version", version)

	flags, cwd := parseACPFlags(args)
	if cwd != "" {
		absDir, err := filepath.Abs(cwd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cannot resolve --cwd %s: %v\n", cwd, err)
			os.Exit(1)
		}
		if err := os.Chdir(absDir); err != nil {
			fmt.Fprintf(os.Stderr, "cannot change to --cwd %s: %v\n", absDir, err)
			os.Exit(1)
		}
	}

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
		slog.Error("failed to start embedded server for ACP", "error", err)
		os.Exit(1)
	}
	slog.Debug("embedded server started for ACP", "url", listener.URL.String())

	store := session.NewStore(db.DB)
	adapter := acp.NewStoreAdapter(store)
	svc := acp.NewServiceWithConfig(acp.Config{
		Sessions:     adapter,
		Bus:          b,
		Runner:       srv.SessionManager(),
		Registry:     reg,
		DefaultModel: cfg.Model,
		DefaultCWD:   dir,
		AgentInfo: acp.AgentInfo{
			Name:    "tinycode",
			Version: version,
		},
	})

	transport := acp.NewStdioTransport(svc, os.Stdout)
	relay := acp.NewEventRelay(b, transport)
	relay.SetPermissionService(permSvc)
	safego.Go(func() { relay.Run(ctx) })

	if err := transport.HandleStdio(ctx, os.Stdin); err != nil && ctx.Err() == nil {
		slog.Error("ACP transport error", "error", err)
		os.Exit(1)
	}

	cancel()
	srv.WaitForShutdown()
}

func parseACPFlags(args []string) (commonFlags, string) {
	fs := flag.NewFlagSet("acp", flag.ExitOnError)
	var f commonFlags
	var cwd string
	fs.StringVar(&f.model, "m", "", "model to use (provider/model)")
	fs.StringVar(&f.model, "model", "", "model to use (provider/model)")
	fs.StringVar(&f.title, "title", "", "set session title")
	fs.StringVar(&f.appendSystemPrompt, "append-system-prompt", "", "append text to the system prompt")
	fs.StringVar(&f.appendSystemPromptFile, "append-system-prompt-file", "", "append file contents to the system prompt")
	fs.IntVar(&f.maxTokens, "max-tokens", 0, "cumulative token budget (input+output); abort when exceeded")
	fs.BoolVar(&f.safeMode, "safe-mode", false, "skip plugins, MCP, and user agents")
	fs.BoolVar(&f.continueSession, "c", false, "continue most recent session")
	fs.BoolVar(&f.continueSession, "continue", false, "continue most recent session")
	fs.StringVar(&f.resumeSession, "r", "", "resume session by ID or name")
	fs.StringVar(&f.resumeSession, "resume", "", "resume session by ID or name")
	fs.StringVar(&cwd, "cwd", "", "working directory for ACP sessions")
	_ = fs.Parse(args)

	if remaining := fs.Args(); len(remaining) > 0 {
		if info, err := os.Stat(remaining[0]); err == nil && info.IsDir() && cwd == "" {
			cwd = remaining[0]
		}
	}

	if f.appendSystemPromptFile != "" {
		data, err := os.ReadFile(f.appendSystemPromptFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading --append-system-prompt-file: %v\n", err)
			os.Exit(1)
		}
		if f.appendSystemPrompt != "" {
			f.appendSystemPrompt += "\n\n"
		}
		f.appendSystemPrompt += string(data)
	}

	return f, cwd
}
