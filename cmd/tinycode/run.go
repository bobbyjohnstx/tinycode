package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/agent"
	"github.com/bobbyjohnstx/tinycode/internal/llm"
	"github.com/bobbyjohnstx/tinycode/internal/mcp"
	"github.com/bobbyjohnstx/tinycode/internal/permission"
	"github.com/bobbyjohnstx/tinycode/internal/plugin"
	"github.com/bobbyjohnstx/tinycode/internal/provider"
	"github.com/bobbyjohnstx/tinycode/internal/safego"
	"github.com/bobbyjohnstx/tinycode/internal/session"
)

// runRun implements the headless CLI runner. Helper functions are in run_helpers.go.
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
	resumeFlag := fs.String("r", "", "resume session by ID or title/slug")
	fs.StringVar(resumeFlag, "resume", "", "resume session by ID or title/slug")
	titleFlag := fs.String("title", "", "session title")
	skipPermsFlag := fs.Bool("dangerously-skip-permissions", false, "auto-approve all tool permissions")
	interactiveFlag := fs.Bool("i", false, "show permission prompts (default: auto-deny asks)")
	fs.BoolVar(interactiveFlag, "interactive", false, "show permission prompts (default: auto-deny asks)")
	permsFlag := fs.String("permissions", "default", "permission handling: default, json")
	maxIterFlag := fs.Int("max-iterations", 0, "maximum processor iterations (0 = default 200)")
	multiTurnFlag := fs.Bool("multi-turn", false, "multi-turn mode: loop on stdin after initial prompt")
	failFastFlag := fs.Bool("fail-fast", false, "in multi-turn mode, exit immediately on first turn error")
	appendSPFlag := fs.String("append-system-prompt", "", "append text to the system prompt")
	appendSPFileFlag := fs.String("append-system-prompt-file", "", "append file contents to the system prompt")
	maxTokensFlag := fs.Int("max-tokens", 0, "cumulative token budget (input+output); abort when exceeded")
	safeModeFlag := fs.Bool("safe-mode", false, "skip plugins, MCP, and user agents")
	_ = fs.Parse(os.Args[2:])

	switch *formatFlag {
	case "default", "json", "":
	default:
		fmt.Fprintf(os.Stderr, "error: unknown --format %q (want default or json)\n", *formatFlag)
		os.Exit(1)
	}

	appendSP := *appendSPFlag
	if *appendSPFileFlag != "" {
		data, err := os.ReadFile(*appendSPFileFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading --append-system-prompt-file: %v\n", err)
			os.Exit(1)
		}
		if appendSP != "" {
			appendSP += "\n\n"
		}
		appendSP += string(data)
	}

	permsJSON := *permsFlag == "json"
	prompt := collectRunPrompt(fs.Args(), *multiTurnFlag, permsJSON)

	b, db, cfg := initDependencies()
	defer db.Close()
	defer b.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	var mcpSvc *mcp.Service
	if !*safeModeFlag && len(cfg.MCP) > 0 {
		mcpSvc = mcp.NewService(b)
		defer mcpSvc.Close()
		mcpSvc.Configure(ctx, cfg.MCP)
	}

	reg := provider.NewRegistry()
	disc := startDiscovery(ctx, reg, b, cfg)
	defer disc.Stop()

	dir, _ := os.Getwd()
	var agentReg *agent.Registry
	if *safeModeFlag {
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

	if cfg.Permission != nil {
		configRules := permission.FromConfig(cfg.Permission.Allow, cfg.Permission.Deny)
		permSvc.SetBaseRules(configRules)
		disabled := permission.Disabled(toolReg.List(), configRules)
		toolReg.SetDisabled(disabled)
	}

	permReplyCh := setupRunPermissions(b, permSvc, *skipPermsFlag, *interactiveFlag, *permsFlag)

	// When --permissions json is set, a single stdinMux goroutine owns stdin
	// for the process lifetime, dispatching prompts and permission replies to
	// separate channels. This prevents the deadlock where io.ReadAll consumed
	// stdin before the permission router could read replies.
	var mux *stdinMux
	if permsJSON && permReplyCh != nil {
		mux = newStdinMux(*formatFlag == "json", permReplyCh)
		safego.Go(mux.run)
	}

	builtinMgr := initBuiltins(toolReg)

	pluginMgr := plugin.NewManager(slog.Default())
	defer pluginMgr.Shutdown()
	if !*safeModeFlag {
		loadConfigPlugins(pluginMgr, toolReg, cfg, dir)
	}
	shellRunner := plugin.NewShellHookRunner(cfg.Hooks, slog.Default())
	wireToolBeforeHook(toolCtx, pluginMgr, shellRunner)
	wireToolAfterHook(toolCtx, pluginMgr, builtinMgr, shellRunner)
	wirePermissionAskHook(permSvc, pluginMgr)

	modelStr := *modelFlag
	if modelStr == "" {
		modelStr = cfg.Model
	}
	providerID, modelID, model := resolveRunModel(reg, modelStr)
	disc.Warmup(ctx, model)

	agentName, agentPerms, systemPrompt := buildRunAgentPrompt(*agentFlag, cfg, agentReg, model, dir, toolReg, appendSP)

	// Sync MCP tools — wait for async connections to settle first.
	if mcpSvc != nil {
		mcpSvc.WaitForConnections(ctx, 10*time.Second)
		mcpTools := mcpSvc.Tools(ctx)
		for _, def := range mcpTools {
			toolReg.Register(def)
		}
	}

	sessionID, existingMsgs, ms := resolveRunSession(db, *sessionFlag, *resumeFlag, *continueFlag, *titleFlag, dir, agentName, modelID, providerID)

	// Create LLM client and processor
	apiKey := ""
	if model.Options != nil {
		if key, ok := model.Options["api_key"].(string); ok {
			apiKey = key
		}
	}
	client := llm.NewClient(model.API.URL, apiKey, model.API.NPM, model.ProviderID)

	autoContinueMax := 0
	if cfg.Experimental != nil {
		autoContinueMax = cfg.Experimental.AutoContinue
	}

	compactionCfg := session.DefaultCompactionConfig()
	proc := session.NewProcessor(session.ProcessorConfig{
		SessionID:       sessionID,
		Agent:           agentName,
		Model:           model,
		SystemPrompt:    systemPrompt,
		Compaction:      compactionCfg,
		AgentPerms:      agentPerms,
		MaxIterations:   *maxIterFlag,
		TokenBudget:     *maxTokensFlag,
		Directory:       dir,
		Perms:           permSvc,
		AutoContinueMax: autoContinueMax,
	}, client, toolReg, b)
	proc.SetMessages(existingMsgs)

	isJSON := *formatFlag == "json"
	emitRunSession(sessionID, isJSON)
	streamRunOutput(b, isJSON)

	// When --permissions json with no positional prompt, read from the mux.
	if prompt == "" && !*multiTurnFlag && mux != nil {
		p, ok := <-mux.prompts
		if !ok || p == "" {
			fmt.Fprintf(os.Stderr, "error: no prompt provided\n")
			os.Exit(1)
		}
		prompt = p
	}

	// In multi-turn mode with no initial prompt from args, read the first line.
	if *multiTurnFlag && prompt == "" {
		var scanner *bufio.Scanner
		if mux == nil {
			scanner = bufio.NewScanner(os.Stdin)
		}
		p, ok := readNextPrompt(scanner, isJSON, permReplyCh, mux)
		if !ok || p == "" {
			return
		}
		prompt = p
	}

	result := proc.Process(ctx, prompt)

	if !*multiTurnFlag {
		finishRun(result, existingMsgs, ms, db, sessionID, isJSON)
		return
	}

	// Multi-turn: persist results without exiting, then loop.
	hadError := false
	existingMsgs = persistRunResult(result, existingMsgs, ms, db, sessionID)
	if !isJSON {
		fmt.Println()
	}
	if result != nil && result.Error != nil {
		hadError = true
		emitRunError(sessionID, result.Error, isJSON)
		if errors.Is(result.Error, context.Canceled) {
			os.Exit(130)
		}
		if *failFastFlag {
			os.Exit(1)
		}
	}

	var scanner *bufio.Scanner
	if mux == nil {
		scanner = bufio.NewScanner(os.Stdin)
	}
	for {
		if isJSON {
			line, _ := json.Marshal(map[string]string{"type": "ready"})
			fmt.Println(string(line))
		} else {
			fmt.Println()
		}

		prompt, ok := readNextPrompt(scanner, isJSON, permReplyCh, mux)
		if !ok {
			break
		}
		if prompt == "" {
			continue
		}

		result = proc.Process(ctx, prompt)
		existingMsgs = persistRunResult(result, existingMsgs, ms, db, sessionID)
		if !isJSON {
			fmt.Println()
		}
		if result != nil && result.Error != nil {
			hadError = true
			emitRunError(sessionID, result.Error, isJSON)
			if errors.Is(result.Error, context.Canceled) {
				os.Exit(130)
			}
			if *failFastFlag {
				os.Exit(1)
			}
		}
	}

	if hadError {
		os.Exit(1)
	}
	emitRunDone(sessionID, isJSON)
}
