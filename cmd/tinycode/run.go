package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/bobbyjohnstx/tinycode-go/internal/llm"
	"github.com/bobbyjohnstx/tinycode-go/internal/mcp"
	"github.com/bobbyjohnstx/tinycode-go/internal/permission"
	"github.com/bobbyjohnstx/tinycode-go/internal/plugin"
	"github.com/bobbyjohnstx/tinycode-go/internal/provider"
	"github.com/bobbyjohnstx/tinycode-go/internal/session"
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
	titleFlag := fs.String("title", "", "session title")
	skipPermsFlag := fs.Bool("dangerously-skip-permissions", false, "auto-approve all tool permissions")
	interactiveFlag := fs.Bool("i", false, "show permission prompts (default: auto-deny)")
	fs.BoolVar(interactiveFlag, "interactive", false, "show permission prompts (default: auto-deny)")
	permsFlag := fs.String("permissions", "default", "permission handling: default, json")
	maxIterFlag := fs.Int("max-iterations", 0, "maximum processor iterations (0 = default 200)")
	multiTurnFlag := fs.Bool("multi-turn", false, "multi-turn mode: loop on stdin after initial prompt")
	_ = fs.Parse(os.Args[2:])

	prompt := collectRunPrompt(fs.Args(), *multiTurnFlag)

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

	if cfg.Permission != nil {
		configRules := permission.FromConfig(cfg.Permission.Allow, cfg.Permission.Deny)
		permSvc.SetBaseRules(configRules)
		disabled := permission.Disabled(toolReg.List(), configRules)
		toolReg.SetDisabled(disabled)
	}

	permReplyCh := setupRunPermissions(b, permSvc, *skipPermsFlag, *interactiveFlag, *permsFlag)

	builtinMgr := initBuiltins(toolReg)

	pluginMgr := plugin.NewManager(slog.Default())
	defer pluginMgr.Shutdown()
	loadConfigPlugins(pluginMgr, cfg, dir)
	wireToolAfterHook(toolCtx, pluginMgr, builtinMgr)

	modelStr := *modelFlag
	if modelStr == "" {
		modelStr = cfg.Model
	}
	providerID, modelID, model := resolveRunModel(reg, modelStr)

	agentName, agentPerms, systemPrompt := buildRunAgentPrompt(*agentFlag, cfg, agentReg, model, dir, toolReg)

	// Sync MCP tools
	if mcpSvc != nil {
		mcpTools := mcpSvc.Tools(ctx)
		for _, def := range mcpTools {
			toolReg.Register(def)
		}
	}

	sessionID, existingMsgs, ms := resolveRunSession(db, *sessionFlag, *continueFlag, *titleFlag, dir, agentName, modelID, providerID)

	// Create LLM client and processor
	apiKey := ""
	if model.Options != nil {
		if key, ok := model.Options["api_key"].(string); ok {
			apiKey = key
		}
	}
	client := llm.NewOpenAIClient(model.API.URL+"/v1", apiKey)

	compactionCfg := session.DefaultCompactionConfig()
	proc := session.NewProcessor(session.ProcessorConfig{
		SessionID:     sessionID,
		Agent:         agentName,
		Model:         model,
		SystemPrompt:  systemPrompt,
		Compaction:    compactionCfg,
		AgentPerms:    agentPerms,
		MaxIterations: *maxIterFlag,
	}, client, toolReg, b)
	proc.SetMessages(existingMsgs)

	isJSON := *formatFlag == "json"
	streamRunOutput(b, isJSON)

	// In multi-turn mode with no initial prompt from args, read the first line.
	if *multiTurnFlag && prompt == "" {
		scanner := bufio.NewScanner(os.Stdin)
		p, ok := readNextPrompt(scanner, isJSON, permReplyCh)
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
	existingMsgs = persistRunResult(result, existingMsgs, ms, db, sessionID)
	if !isJSON {
		fmt.Println()
	}
	if result != nil && result.Error != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", result.Error)
	}

	scanner := bufio.NewScanner(os.Stdin)
	for {
		if isJSON {
			line, _ := json.Marshal(map[string]string{"type": "ready"})
			fmt.Println(string(line))
		} else {
			fmt.Println()
		}

		prompt, ok := readNextPrompt(scanner, isJSON, permReplyCh)
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
			fmt.Fprintf(os.Stderr, "error: %v\n", result.Error)
		}
	}
}
