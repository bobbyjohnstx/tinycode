package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/bobbyjohnstx/tinycode/internal/config"
	"github.com/bobbyjohnstx/tinycode/internal/plugin"
	"github.com/bobbyjohnstx/tinycode/internal/project"
	"github.com/bobbyjohnstx/tinycode/internal/provider"
	"github.com/bobbyjohnstx/tinycode/internal/session"
	"github.com/bobbyjohnstx/tinycode/internal/storage"
)

func runModels() {
	setupLogger()

	b, db, cfg := initDependencies()
	defer b.Close()
	defer db.Close()

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

	b, db, cfg := initDependencies()
	defer b.Close()
	defer db.Close()

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

func runPlugin() {
	setupLogger()

	args := os.Args[2:]
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: tinycode plugin <list|install|uninstall> [args]\n")
		os.Exit(1)
	}

	switch args[0] {
	case "list":
		var catFilter string
		for i := 1; i < len(args); i++ {
			if args[i] == "--category" && i+1 < len(args) {
				catFilter = args[i+1]
				break
			}
		}

		var entries []plugin.RegistryEntry
		if catFilter != "" {
			entries = plugin.RegistryByCategory(catFilter)
			if len(entries) == 0 {
				cats := plugin.Categories()
				slugs := make([]string, len(cats))
				for i, c := range cats {
					slugs[i] = c.Slug
				}
				fmt.Fprintf(os.Stderr, "No plugins in category %q.\nValid categories: %s\n", catFilter, strings.Join(slugs, ", "))
				os.Exit(1)
			}
		} else {
			entries = plugin.Registry()
		}

		inConfig := map[string]bool{}
		if dir, err := os.Getwd(); err == nil {
			if cfg, err := config.Load(dir); err == nil {
				if specs, err := plugin.ParsePluginConfig(cfg.Plugins); err == nil {
					for _, s := range specs {
						inConfig[s.Name] = true
					}
				}
			}
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tCATEGORY\tDESCRIPTION\tINSTALLED\tIN_CONFIG")
		pluginDir := filepath.Join(config.ConfigDir(), "plugins")
		for _, e := range entries {
			installed := "no"
			binPath := filepath.Join(pluginDir, e.Name)
			if info, err := os.Stat(binPath); err == nil && info.Mode()&0o111 != 0 {
				installed = "yes"
			} else if _, err := exec.LookPath("tinycode-plugin-" + e.Name); err == nil {
				installed = "yes (PATH)"
			}
			cfgStatus := "no"
			if inConfig[e.Name] {
				cfgStatus = "yes"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", e.Name, e.Category, e.Description, installed, cfgStatus)
		}
		w.Flush()

	case "install":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: tinycode plugin install <name> [--from <path>]\n")
			os.Exit(1)
		}
		name := args[1]

		entry, ok := plugin.LookupRegistry(name)
		if !ok {
			fmt.Fprintf(os.Stderr, "Unknown plugin: %s\nRun 'tinycode plugin list' to see available plugins.\n", name)
			os.Exit(1)
		}

		pluginDir := filepath.Join(config.ConfigDir(), "plugins")
		if err := os.MkdirAll(pluginDir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "error creating plugin directory: %v\n", err)
			os.Exit(1)
		}
		destPath := filepath.Join(pluginDir, name)

		// --from flag: copy from a local path (e.g. dist/plugins/plugin-notify)
		if len(args) >= 4 && args[2] == "--from" {
			srcPath := args[3]
			if err := copyFile(srcPath, destPath); err != nil {
				fmt.Fprintf(os.Stderr, "error installing plugin: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Installed plugin %s from %s\n", name, srcPath)
			return
		}

		// Default: build from source if cmd/plugin-<name> exists
		pluginSrc := filepath.Join("cmd", "plugin-"+name)
		if info, err := os.Stat(pluginSrc); err == nil && info.IsDir() {
			fmt.Printf("Building plugin %s from source ...\n", name)
			cmd := exec.Command("go", "build", "-ldflags", "-s -w", "-o", destPath, "./"+pluginSrc)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "build failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Installed plugin %s to %s\n", name, destPath)
			return
		}

		fmt.Fprintf(os.Stderr, "Plugin %s (%s) source not found locally.\n", name, entry.Description)
		fmt.Fprintf(os.Stderr, "Use --from <path> to install from a pre-built binary.\n")
		os.Exit(1)

	case "uninstall":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: tinycode plugin uninstall <name>\n")
			os.Exit(1)
		}
		name := args[1]
		pluginDir := filepath.Join(config.ConfigDir(), "plugins")
		binPath := filepath.Join(pluginDir, name)
		if err := os.Remove(binPath); err != nil {
			if os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "Plugin %s is not installed in %s\n", name, pluginDir)
			} else {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
			}
			os.Exit(1)
		}
		fmt.Printf("Uninstalled plugin %s\n", name)

	default:
		fmt.Fprintf(os.Stderr, "Unknown plugin subcommand: %s\nUsage: tinycode plugin <list|install|uninstall> [args]\n", args[0])
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

// initRoles defines the role-to-plugin mapping for `tinycode init`.
var initRoles = []struct {
	Label   string
	Plugins []string
}{
	{
		Label:   "OpenShift SRE / Platform Admin",
		Plugins: []string{"ocp-context-injection", "ocp-must-gather", "etcd-diag", "ingress-inspect", "audit-logs", "ocp-obs-metrics", "ocp-obs-logging", "ocp-virt", "insights", "ocp-odf", "safety-net"},
	},
	{
		Label:   "Security / Compliance",
		Plugins: []string{"rhacs", "lightwell", "container-linter", "log-sanitizer", "safety-net", "audit-logs"},
	},
	{
		Label:   "AI/ML / Data Science",
		Plugins: []string{"rhoai-mlflow", "rhoai-pipelines", "rhoai-serving", "ocp-context-injection"},
	},
	{
		Label:   "Platform / Infrastructure",
		Plugins: []string{"rhacm", "tekton", "aap-bridge", "satellite", "rhdp-provisioner", "ocp-context-injection", "safety-net"},
	},
	{
		Label:   "Developer",
		Plugins: []string{"quay", "rhdh", "rh-api-catalog", "rh-dev-content", "rh-ecosystem-catalog", "tekton", "container-linter"},
	},
}

func runInit() {
	setupLogger()

	reader := bufio.NewReader(os.Stdin)
	result := make(map[string]any)

	configFile := config.GlobalConfigFile()
	if data, err := os.ReadFile(configFile); err == nil {
		_ = json.Unmarshal(data, &result)
	}

	fmt.Println("tinycode init — Red Hat plugin/role setup")
	fmt.Println("(Not required for first run. For models: /connect in the TUI, OPENROUTER_API_KEY, or Ollama.)")
	fmt.Println()

	// --- Step 1: Model selection ---
	fmt.Println("Discovering providers...")
	b, _, cfg := initDependencies()
	defer b.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reg := provider.NewRegistry()
	disc := startDiscovery(ctx, reg, b, cfg)
	defer disc.Stop()

	models := reg.ListModels()
	if len(models) == 0 {
		fmt.Println("  No providers found. You can configure providers later in the config file.")
		fmt.Println("  Skipping model selection.")
	} else {
		providers := reg.ListProviders()
		fmt.Printf("  Found %d provider(s), %d model(s)\n\n", len(providers), len(models))

		fmt.Println("Select default model:")
		for i, m := range models {
			ctxStr := ""
			if m.Limit.Context > 0 {
				ctxStr = fmt.Sprintf(" (%dk context)", m.Limit.Context/1000)
			}
			fmt.Printf("  %2d. %s/%s%s\n", i+1, m.ProviderID, m.ID, ctxStr)
		}
		fmt.Println()

		defaultModel := ""
		if len(models) > 0 {
			defaultModel = models[0].ProviderID + "/" + models[0].ID
		}
		if cfg.Model != "" {
			defaultModel = cfg.Model
		}

		if defaultModel != "" {
			fmt.Printf("Model [%s]: ", defaultModel)
		} else {
			fmt.Print("Model: ")
		}
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)

		if line == "" {
			result["model"] = defaultModel
			fmt.Printf("  Using %s\n", defaultModel)
		} else if idx, err := strconv.Atoi(line); err == nil && idx >= 1 && idx <= len(models) {
			m := models[idx-1]
			chosen := m.ProviderID + "/" + m.ID
			result["model"] = chosen
			fmt.Printf("  Using %s\n", chosen)
		} else {
			result["model"] = line
			fmt.Printf("  Using %s\n", line)
		}
	}
	fmt.Println()

	// --- Step 2: Username ---
	defaultUser := os.Getenv("USER")
	if defaultUser == "" {
		defaultUser = os.Getenv("USERNAME")
	}
	if existing, ok := result["username"].(string); ok && existing != "" {
		defaultUser = existing
	}

	if defaultUser != "" {
		fmt.Printf("Username [%s]: ", defaultUser)
	} else {
		fmt.Print("Username: ")
	}
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		line = defaultUser
	}
	if line != "" {
		result["username"] = line
	}
	fmt.Println()

	// --- Step 3: Plugin selection ---
	fmt.Println("What's your primary role?")
	for i, role := range initRoles {
		fmt.Printf("  %d. %s\n", i+1, role.Label)
	}
	fmt.Printf("  %d. Custom (pick individual plugins)\n", len(initRoles)+1)
	fmt.Println()

	fmt.Print("> ")
	line, _ = reader.ReadString('\n')
	line = strings.TrimSpace(line)

	choice, err := strconv.Atoi(line)
	if err != nil || choice < 1 || choice > len(initRoles)+1 {
		fmt.Fprintf(os.Stderr, "Invalid choice: %s\n", line)
		os.Exit(1)
	}

	var selected []string

	if choice <= len(initRoles) {
		role := initRoles[choice-1]
		selected = role.Plugins
		fmt.Printf("\nSelected plugins for %s:\n", role.Label)
		for _, name := range selected {
			entry, ok := plugin.LookupRegistry(name)
			desc := name
			if ok {
				desc = entry.Description
			}
			fmt.Printf("  - %s: %s\n", name, desc)
		}
	} else {
		fmt.Println("\nAvailable plugins:")
		entries := plugin.Registry()
		for i, e := range entries {
			fmt.Printf("  %2d. [%s] %s — %s\n", i+1, e.Category, e.Name, e.Description)
		}
		fmt.Println()
		fmt.Println("Enter plugin numbers separated by spaces (e.g. 1 3 5 12):")
		fmt.Print("> ")
		line, _ = reader.ReadString('\n')
		line = strings.TrimSpace(line)

		parts := strings.Fields(line)
		if len(parts) == 0 {
			fmt.Fprintln(os.Stderr, "No plugins selected.")
			os.Exit(1)
		}
		for _, p := range parts {
			idx, err := strconv.Atoi(p)
			if err != nil || idx < 1 || idx > len(entries) {
				fmt.Fprintf(os.Stderr, "Invalid selection: %s\n", p)
				os.Exit(1)
			}
			selected = append(selected, entries[idx-1].Name)
		}
		fmt.Println("\nSelected plugins:")
		for _, name := range selected {
			fmt.Printf("  - %s\n", name)
		}
	}
	result["plugins"] = selected

	// --- Write config ---
	fmt.Printf("\nWrite to %s? [Y/n] ", configFile)
	line, _ = reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	if line != "" && line != "y" && line != "yes" {
		fmt.Println("Aborted.")
		return
	}

	configDir := filepath.Dir(configFile)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating config directory: %v\n", err)
		os.Exit(1)
	}

	out, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error marshaling config: %v\n", err)
		os.Exit(1)
	}
	out = append(out, '\n')

	if err := os.WriteFile(configFile, out, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nWrote config to %s\n", configFile)
	fmt.Println("Run 'tinycode plugin list' to see plugin installation status.")
}

