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

	authpkg "github.com/bobbyjohnstx/tinycode/internal/auth"
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

	args := os.Args[2:]
	if len(args) > 0 {
		switch args[0] {
		case "login":
			runProvidersLogin(args[1:])
			return
		case "logout":
			runProvidersLogout(args[1:])
			return
		case "list", "ls":
			args = args[1:]
		case "help", "--help", "-h":
			fmt.Println("Usage: tinycode providers [list|login|logout]")
			fmt.Println()
			fmt.Println("  list                         List discovered providers (default)")
			fmt.Println("  login <provider> [--api-key] Persist API key to auth.json")
			fmt.Println("  logout <provider>            Remove persisted credentials")
			return
		}
	}

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

func runProvidersLogin(args []string) {
	providerID, apiKey := parseProviderLoginArgs(args)
	if providerID == "" {
		fmt.Fprintf(os.Stderr, "Usage: tinycode providers login <providerID> [--api-key KEY]\n")
		os.Exit(1)
	}
	if apiKey == "" {
		fmt.Fprint(os.Stderr, "API key: ")
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading api key: %v\n", err)
			os.Exit(1)
		}
		apiKey = strings.TrimSpace(line)
	}
	if apiKey == "" {
		fmt.Fprintf(os.Stderr, "error: empty API key\n")
		os.Exit(1)
	}

	store, err := authpkg.Open(authpkg.DefaultPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
	}
	if err := store.Set(providerID, map[string]string{"apiKey": apiKey}); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Stored credentials for %s in %s\n", providerID, store.Path())
}

func runProvidersLogout(args []string) {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: tinycode providers logout <providerID>\n")
		os.Exit(1)
	}
	providerID := args[0]
	store, err := authpkg.Open(authpkg.DefaultPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
	}
	if err := store.Delete(providerID); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Removed credentials for %s from %s\n", providerID, store.Path())
}

func parseProviderLoginArgs(args []string) (providerID, apiKey string) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--api-key", "-k":
			if i+1 < len(args) {
				apiKey = args[i+1]
				i++
			}
		default:
			if !strings.HasPrefix(args[i], "-") && providerID == "" {
				providerID = args[i]
			}
		}
	}
	return providerID, apiKey
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
		sessions, err := store.List(projectID, 50, 0, false)
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
	sessions, err := store.List(projectID, 1000, 0, false)
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
	sanitize := false
	var sessionID string
	for _, a := range args {
		switch a {
		case "--sanitize":
			sanitize = true
		case "--help", "-h":
			fmt.Println("Usage: tinycode export <session-id> [--sanitize]")
			return
		default:
			if !strings.HasPrefix(a, "-") && sessionID == "" {
				sessionID = a
			}
		}
	}
	if sessionID == "" {
		fmt.Fprintf(os.Stderr, "Usage: tinycode export <session-id> [--sanitize]\n")
		os.Exit(1)
	}

	_, db, _ := initDependencies()
	defer db.Close()

	store := session.NewStore(db.DB)
	ms := session.NewMessageStore(store)

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

	payload := session.ExportPayload{Session: *info, Messages: messages}
	if sanitize {
		payload = session.SanitizeExport(payload)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func runImport() {
	setupLogger()

	args := os.Args[2:]
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Println("Usage: tinycode import <file.json>")
		fmt.Println("Always mints new session and message IDs. Share URLs are not supported.")
		if len(args) == 0 {
			os.Exit(1)
		}
		return
	}
	path := args[0]
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		fmt.Fprintf(os.Stderr, "error: share URL import is not supported\n")
		os.Exit(1)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading %s: %v\n", path, err)
		os.Exit(1)
	}

	var payload session.ExportPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		fmt.Fprintf(os.Stderr, "error parsing export JSON: %v\n", err)
		os.Exit(1)
	}
	if payload.Session.ID == "" && len(payload.Messages) == 0 {
		fmt.Fprintf(os.Stderr, "error: empty or unrecognized export (expected session + messages)\n")
		os.Exit(1)
	}

	_, db, _ := initDependencies()
	defer db.Close()

	dir, _ := os.Getwd()
	directory := payload.Session.Directory
	if directory == "" || strings.HasPrefix(directory, "[redacted:") {
		directory = dir
	}
	title := payload.Session.Title
	if strings.HasPrefix(title, "[redacted:") {
		title = "Imported session"
	}
	projectID := project.IDFromDirectory(directory)

	store := session.NewStore(db.DB)
	ms := session.NewMessageStore(store)
	created, err := store.Create(session.CreateInput{
		ProjectID: projectID,
		Directory: directory,
		Title:     title,
		Agent:     payload.Session.Agent,
		Model:     payload.Session.Model,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating session: %v\n", err)
		os.Exit(1)
	}

	msgs, err := session.RemapImportMessages(created.ID, payload.Messages)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if err := ms.AppendBatch(created.ID, msgs); err != nil {
		fmt.Fprintf(os.Stderr, "error importing messages: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Imported session %s (%d messages)\n", created.ID, len(msgs))
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
			printPluginEnableHint(name)
			return
		}

		fmt.Printf("Installing plugin %s ...\n", name)
		if err := attemptPluginInstall(name); err != nil {
			fmt.Fprintf(os.Stderr, "Plugin %s (%s): %v\n", name, entry.Description, err)
			fmt.Fprintf(os.Stderr, "Use --from <path> to install from a pre-built binary, or build from a source checkout.\n")
			os.Exit(1)
		}
		fmt.Printf("Installed plugin %s to %s\n", name, destPath)
		printPluginEnableHint(name)

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

func printPluginEnableHint(name string) {
	fmt.Printf("Enable by adding %q to \"plugins\" in %s\n", name, config.GlobalConfigFile())
}

// pluginInstallFunc installs a plugin binary by name. Injectable for tests.
type pluginInstallFunc func(name string) error

// pluginResolveFunc locates an installed plugin binary. Injectable for tests.
type pluginResolveFunc func(name string) (string, error)

// ensurePluginsInstalled returns names that are already resolved or newly
// installed. Failures are listed separately; callers should warn and continue (3A).
func ensurePluginsInstalled(names []string, resolve pluginResolveFunc, install pluginInstallFunc) (ok, failed []string) {
	for _, name := range names {
		if _, err := resolve(name); err == nil {
			ok = append(ok, name)
			continue
		}
		if err := install(name); err != nil {
			failed = append(failed, name)
			continue
		}
		ok = append(ok, name)
	}
	return ok, failed
}

// attemptPluginInstall builds from local source when available, otherwise
// downloads from the release plugins bundle.
func attemptPluginInstall(name string) error {
	pluginDir := filepath.Join(config.ConfigDir(), "plugins")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return fmt.Errorf("create plugins dir: %w", err)
	}
	destPath := filepath.Join(pluginDir, name)

	pluginSrc := filepath.Join("cmd", "plugin-"+name)
	if info, err := os.Stat(pluginSrc); err == nil && info.IsDir() {
		cmd := exec.Command("go", "build", "-ldflags", "-s -w", "-o", destPath, "./"+pluginSrc)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("build: %w", err)
		}
		return nil
	}

	return plugin.InstallFromRelease(name, plugin.InstallOptions{EmbeddedVersion: version})
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
		cleaned, err := config.ParseJSONC(string(data))
		if err == nil {
			_ = json.Unmarshal([]byte(cleaned), &result)
		}
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

	// --- Write config ---
	fmt.Printf("\nWrite to %s? [Y/n] ", configFile)
	line, _ = reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	if line != "" && line != "y" && line != "yes" {
		fmt.Println("Aborted.")
		return
	}

	installed, failed := ensurePluginsInstalled(selected, plugin.ResolveBinary, attemptPluginInstall)
	for _, name := range failed {
		fmt.Fprintf(os.Stderr, "warning: failed to install plugin %s\n", name)
	}
	if installed == nil {
		installed = []string{}
	}
	result["plugins"] = installed

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
	fmt.Printf("Plugins enabled: %d installed, %d skipped\n", len(installed), len(failed))
	if len(failed) > 0 {
		fmt.Println("Skipped plugins were not written to config. Fix install errors and re-run init or plugin install.")
	}
}

