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
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/term"

	"github.com/bobbyjohnstx/tinycode-go/internal/llm"
	"github.com/bobbyjohnstx/tinycode-go/internal/mcp"
	"github.com/bobbyjohnstx/tinycode-go/internal/permission"
	"github.com/bobbyjohnstx/tinycode-go/internal/plugin"
	"github.com/bobbyjohnstx/tinycode-go/internal/project"
	"github.com/bobbyjohnstx/tinycode-go/internal/provider"
	"github.com/bobbyjohnstx/tinycode-go/internal/session"
)

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
			if err := scanner.Err(); err != nil {
				slog.Warn("stdin scanner error", "error", err)
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
