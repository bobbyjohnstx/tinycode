package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/auth"
	"github.com/bobbyjohnstx/tinycode/internal/config"
	"github.com/bobbyjohnstx/tinycode/internal/plugin"
	"github.com/bobbyjohnstx/tinycode/internal/skill"
	"github.com/bobbyjohnstx/tinycode/internal/storage"
)

const (
	checkPass = "\033[32m✓\033[0m" // green check
	checkFail = "\033[31m✗\033[0m" // red X
	checkWarn = "\033[33m!\033[0m" // yellow !
)

func runDoctor() {
	var failed bool

	// Version info
	fmt.Printf("%s Version: %s\n", checkPass, version)
	if _, err := plugin.PluginReleaseTag(version, os.Getenv("TINYCODE_PLUGIN_RELEASE")); err != nil {
		fmt.Printf("%s Plugin release tag: non-release build — set TINYCODE_PLUGIN_RELEASE=vX.Y.Z for plugin install, or use --from\n", checkWarn)
	}
	fmt.Printf("%s Go: %s\n", checkPass, runtime.Version())
	fmt.Printf("%s OS/Arch: %s/%s\n", checkPass, runtime.GOOS, runtime.GOARCH)

	// Config
	dir, _ := os.Getwd()
	cfg, err := config.Load(dir)
	if err != nil {
		fmt.Printf("%s Config: %v\n", checkFail, err)
		failed = true
		cfg = &config.Info{}
	} else {
		configFile := config.GlobalConfigFile()
		fmt.Printf("%s Config: %s (valid)\n", checkPass, configFile)
	}

	// Data dir
	dataDir := config.DataDir()
	if checkWritable(dataDir) {
		fmt.Printf("%s Data dir: %s (writable)\n", checkPass, dataDir)
	} else {
		fmt.Printf("%s Data dir: %s (not writable)\n", checkFail, dataDir)
		failed = true
	}

	// Database
	dbPath := storage.DefaultPath()
	if _, err := os.Stat(dbPath); err == nil {
		fmt.Printf("%s Database: %s (accessible)\n", checkPass, dbPath)
	} else if os.IsNotExist(err) {
		// DB doesn't exist yet — that's ok if data dir is writable
		if checkWritable(filepath.Dir(dbPath)) {
			fmt.Printf("%s Database: %s (will be created)\n", checkWarn, dbPath)
		} else {
			fmt.Printf("%s Database: %s (directory not writable)\n", checkFail, dbPath)
			failed = true
		}
	} else {
		fmt.Printf("%s Database: %s (%v)\n", checkFail, dbPath, err)
		failed = true
	}

	// Agents — use the agent registry for accurate counts
	configDir := config.ConfigDir()
	agentReg := initAgentRegistry(cfg, dir)
	allAgents := agentReg.ListAll(cfg.DefaultAgent)
	enabledCount := 0
	disabledCount := 0
	for _, a := range allAgents {
		if a.Disabled {
			disabledCount++
		} else {
			enabledCount++
		}
	}
	if disabledCount > 0 {
		fmt.Printf("%s Agents: %d loaded (%d disabled)\n", checkPass, enabledCount, disabledCount)
	} else {
		fmt.Printf("%s Agents: %d loaded\n", checkPass, enabledCount)
	}

	// Providers
	checkProviders(cfg, &failed)

	// MCP
	if len(cfg.MCP) > 0 {
		for name, mcp := range cfg.MCP {
			if mcp.URL != "" {
				if probeHTTP(mcp.URL) {
					fmt.Printf("%s MCP: %s -- connected\n", checkPass, name)
				} else {
					fmt.Printf("%s MCP: %s -- unreachable (%s)\n", checkFail, name, mcp.URL)
					failed = true
				}
			} else if mcp.Command != "" {
				fmt.Printf("%s MCP: %s -- stdio (%s)\n", checkPass, name, mcp.Command)
			} else {
				fmt.Printf("%s MCP: %s -- no command or URL configured\n", checkWarn, name)
			}
		}
	} else {
		fmt.Printf("%s MCP: none configured\n", checkPass)
	}

	// Plugins — configured vs resolvable; new-user tips when empty/missing
	pluginHints := reportPlugins(cfg, dataDir)

	// Skills
	var skillPaths []string
	if cfg.Skills != nil {
		skillPaths = cfg.Skills.Paths
	}
	skills := skill.DiscoverWithPaths(configDir, dir, skillPaths)
	bundledSkills := 0
	userSkills := 0
	for _, s := range skills {
		switch s.Source {
		case "builtin":
			bundledSkills++
		default:
			userSkills++
		}
	}
	fmt.Printf("%s Skills: %d bundled + %d user\n", checkPass, bundledSkills, userSkills)

	// LSP and diagnostics tools
	lspHints := checkDevTools(dir)

	// IDEs (ACP setup hints — detection only, no install)
	checkIDEs()

	// Clipboard helpers (warn only — never fail the doctor run)
	checkClipboard()

	// Log file
	logPath := filepath.Join(dataDir, "tinycode.log")
	if checkWritable(filepath.Dir(logPath)) {
		fmt.Printf("%s Log file: %s (writable)\n", checkPass, logPath)
	} else {
		fmt.Printf("%s Log file: %s (not writable)\n", checkFail, logPath)
		failed = true
	}

	printDoctorNextSteps(append(pluginHints, lspHints...))

	if failed {
		os.Exit(1)
	}
}

func checkProviders(cfg *config.Info, failed *bool) {
	type providerProbe struct {
		name     string
		url      string
		endpoint string // "ollama" (/api/tags) or "openai" (/v1/models)
	}

	probes := []providerProbe{
		{name: "ollama", url: ollamaURL(), endpoint: "ollama"},
		{name: "lm-studio", url: lmStudioURL(), endpoint: "openai"},
	}

	if v := os.Getenv("TINYCODE_VLLM_HOST"); v != "" {
		probes = append(probes, providerProbe{name: "vllm", url: v, endpoint: "openai"})
	}

	anyLocal := false
	for _, p := range probes {
		n, err := probeModelCount(p.url, p.endpoint)
		switch {
		case err != nil:
			// Local refusal is a warning — fail only if no usable provider path remains.
			fmt.Printf("%s Provider: %s -- connection refused (%s)\n", checkWarn, p.name, p.url)
		case n == 0:
			fmt.Printf("%s Provider: %s -- reachable but 0 models (%s)\n", checkWarn, p.name, p.url)
			printPullHint(p.name)
		default:
			fmt.Printf("%s Provider: %s -- %d models (%s)\n", checkPass, p.name, n, p.url)
			anyLocal = true
		}
	}

	hasCloudKey := openRouterCloudUsable()
	if hasCloudKey {
		fmt.Printf("%s Provider: openrouter -- API key set\n", checkPass)
	}

	// Config-defined providers: require usable models via /v1/models (not bare base URL).
	anyConfig := false
	knownProbe := map[string]bool{"ollama": true, "vllm": true, "lm-studio": true, "openrouter": true}
	for id, pc := range cfg.Provider {
		if knownProbe[id] {
			continue
		}
		if pc.Options != nil {
			if baseURL, ok := pc.Options["baseURL"].(string); ok && baseURL != "" {
				n, err := probeModelCount(baseURL, "openai")
				switch {
				case err != nil:
					fmt.Printf("%s Provider: %s -- connection refused (%s)\n", checkWarn, id, baseURL)
				case n == 0:
					fmt.Printf("%s Provider: %s -- reachable but 0 models (%s)\n", checkWarn, id, baseURL)
				default:
					fmt.Printf("%s Provider: %s -- %d models (%s)\n", checkPass, id, n, baseURL)
					anyConfig = true
				}
				continue
			}
		}
		fmt.Printf("%s Provider: %s -- configured (no URL to probe)\n", checkWarn, id)
	}

	if providerPathUsable(anyLocal, hasCloudKey, anyConfig) {
		return
	}

	fmt.Printf("%s Provider: no usable provider path (no local provider with models and no cloud API key)\n", checkWarn)
	printProviderNextSteps()
	*failed = true
}

// providerPathUsable reports whether doctor found at least one way to reach a model.
func providerPathUsable(anyLocalUsable, hasCloudAPIKey, anyConfigUsable bool) bool {
	return anyLocalUsable || hasCloudAPIKey || anyConfigUsable
}

// openRouterCloudUsable is true when OPENROUTER_API_KEY is set or auth.json
// has a non-empty openrouter API key.
func openRouterCloudUsable() bool {
	if os.Getenv("OPENROUTER_API_KEY") != "" {
		return true
	}
	store, err := auth.Open(auth.DefaultPath())
	if err != nil || store == nil {
		return false
	}
	return store.APIKey("openrouter") != ""
}

func printPullHint(name string) {
	switch name {
	case "ollama":
		fmt.Println("    hint: pull a model — ollama pull llama3.2")
	default:
		fmt.Println("    hint: load or download a model in the provider UI, then re-run doctor")
	}
}

func printProviderNextSteps() {
	fmt.Println("    next steps:")
	fmt.Println("    - start Ollama: ollama serve && ollama pull <model>")
	fmt.Println("    - or set OPENROUTER_API_KEY / run: tinycode providers login openrouter")
	fmt.Println("    - then run: tinycode  (connect dialog opens if no model)")
	fmt.Println("    - or in the TUI: /connect")
}

// probeModelCount hits the same discovery endpoints as provider.Discovery.
// endpoint is "ollama" (/api/tags) or "openai" (/v1/models).
func probeModelCount(baseURL, endpoint string) (int, error) {
	base := strings.TrimRight(baseURL, "/")
	var url string
	switch endpoint {
	case "ollama":
		url = base + "/api/tags"
	default:
		url = base + "/v1/models"
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("status %d", resp.StatusCode)
	}

	switch endpoint {
	case "ollama":
		var body struct {
			Models []json.RawMessage `json:"models"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			return 0, err
		}
		return len(body.Models), nil
	default:
		var body struct {
			Data []json.RawMessage `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			return 0, err
		}
		return len(body.Data), nil
	}
}

// ollamaURL returns the Ollama base URL for doctor probes.
// Keep in sync with startDiscovery in config.go.
func ollamaURL() string {
	if v := os.Getenv("TINYCODE_OLLAMA_HOST"); v != "" {
		return v
	}
	if v := os.Getenv("OLLAMA_HOST"); v != "" {
		return v
	}
	return "http://127.0.0.1:11434"
}

func lmStudioURL() string {
	if v := os.Getenv("TINYCODE_LMSTUDIO_HOST"); v != "" {
		return v
	}
	return "http://127.0.0.1:1234"
}

func probeHTTP(url string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return true
}

func checkWritable(dir string) bool {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false
	}
	f, err := os.CreateTemp(dir, ".tinycode-doctor-*")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return true
}

// checkDevTools reports language-server binaries for the current project.
// Returns short next-step hints for printDoctorNextSteps (warn-only).
func checkDevTools(dir string) []string {
	type devTool struct {
		name    string
		command string
		install string
		markers []string // only check if these files exist in project dir
	}

	tools := []devTool{
		{name: "gopls (Go LSP)", command: "gopls", install: "go install golang.org/x/tools/gopls@latest", markers: []string{"go.mod"}},
		{name: "typescript-language-server", command: "typescript-language-server", install: "npm install -g typescript-language-server typescript", markers: []string{"tsconfig.json", "package.json"}},
		{name: "pyright (Python LSP)", command: "pyright-langserver", install: "npm install -g pyright", markers: []string{"pyproject.toml", "setup.py", "requirements.txt"}},
		{name: "ruff (Python lint)", command: "ruff", install: "pip install ruff", markers: []string{"pyproject.toml", "setup.py", "requirements.txt", "*.py"}},
		{name: "bash-language-server", command: "bash-language-server", install: "npm install -g bash-language-server", markers: nil},
	}

	var hints []string
	relevantCount := 0
	missingCount := 0
	for _, t := range tools {
		relevant := t.markers == nil
		if !relevant {
			for _, m := range t.markers {
				matches, _ := filepath.Glob(filepath.Join(dir, m))
				if len(matches) > 0 {
					relevant = true
					break
				}
			}
		}
		if !relevant {
			continue
		}
		relevantCount++

		if _, err := exec.LookPath(t.command); err == nil {
			fmt.Printf("%s Dev tool: %s\n", checkPass, t.name)
		} else {
			missingCount++
			fmt.Printf("%s Dev tool: %s -- not found\n", checkWarn, t.name)
			fmt.Printf("    install: %s\n", t.install)
			hints = append(hints, t.install)
		}
	}

	if relevantCount == 0 {
		fmt.Printf("%s LSP: no project markers in this directory (go.mod, package.json, …)\n", checkPass)
		fmt.Printf("    tip: open a project root, or LSP tools will report no server for that file type\n")
	} else if missingCount == 0 {
		fmt.Printf("%s LSP: %d language tool(s) ready for this project\n", checkPass, relevantCount)
	} else {
		fmt.Printf("%s LSP: %d of %d language tool(s) missing — lsp_* tools will fail until installed\n",
			checkWarn, missingCount, relevantCount)
		if tip := goBinPathTip(); tip != "" {
			fmt.Printf("    tip: %s\n", tip)
			hints = append(hints, tip)
		}
	}

	// Java / jdtls — warn-only; never sets failed. Marker Glob is project dir only
	// (shallower than LSP markerExists walk) — intentional.
	checkJavaDevTools(dir)
	return hints
}

// goBinPathTip returns a PATH fix hint when `go` is available but its bin dir
// is not on PATH — the usual reason gopls is "installed" yet invisible to lsp_*.
func goBinPathTip() string {
	if _, err := exec.LookPath("go"); err != nil {
		return ""
	}
	out, err := exec.Command("go", "env", "GOPATH").Output()
	if err != nil {
		return ""
	}
	gopath := strings.TrimSpace(string(out))
	if gopath == "" {
		return ""
	}
	goBin := filepath.Join(gopath, "bin")
	pathEnv := os.Getenv("PATH")
	for _, p := range filepath.SplitList(pathEnv) {
		if p == goBin {
			return ""
		}
	}
	return fmt.Sprintf("go install puts binaries in %s — add it to PATH, then re-run doctor", goBin)
}

var (
	javaVersionModern = regexp.MustCompile(`version "(\d+)`)
	javaVersionLegacy = regexp.MustCompile(`version "1\.(\d+)`)
)

// parseJavaMajorVersion extracts the Java major version from `java -version` stderr.
func parseJavaMajorVersion(stderr string) (int, error) {
	if m := javaVersionLegacy.FindStringSubmatch(stderr); len(m) == 2 {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			return 0, fmt.Errorf("parse legacy java version: %w", err)
		}
		return n, nil
	}
	if m := javaVersionModern.FindStringSubmatch(stderr); len(m) == 2 {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			return 0, fmt.Errorf("parse java version: %w", err)
		}
		return n, nil
	}
	return 0, fmt.Errorf("unrecognized java version output")
}

func javaProjectMarkersPresent(dir string) bool {
	for _, m := range []string{"pom.xml", "build.gradle", "build.gradle.kts"} {
		matches, _ := filepath.Glob(filepath.Join(dir, m))
		if len(matches) > 0 {
			return true
		}
	}
	return false
}

// checkJavaDevTools warns when jdtls is missing or Java is older than 21.
// Never marks the doctor run as failed.
func checkJavaDevTools(dir string) {
	if !javaProjectMarkersPresent(dir) {
		return
	}

	if _, err := exec.LookPath("jdtls"); err == nil {
		fmt.Printf("%s Dev tool: jdtls (Java LSP)\n", checkPass)
	} else {
		fmt.Printf("%s Dev tool: jdtls (Java LSP) -- not found\n", checkWarn)
		fmt.Printf("    install: Install jdtls (Eclipse JDT LS, e.g. brew install jdtls) and JDK 21+\n")
	}

	javaPath, err := exec.LookPath("java")
	if err != nil {
		fmt.Printf("%s Dev tool: java -- not found (JDT LS needs Java 21+)\n", checkWarn)
		return
	}

	cmd := exec.Command(javaPath, "-version")
	out, err := cmd.CombinedOutput()
	// java -version writes to stderr; CombinedOutput captures both.
	major, parseErr := parseJavaMajorVersion(string(out))
	if err != nil && len(out) == 0 {
		fmt.Printf("%s Dev tool: java -- could not run java -version\n", checkWarn)
		return
	}
	if parseErr != nil {
		fmt.Printf("%s Dev tool: java -- could not parse version (JDT LS needs Java 21+)\n", checkWarn)
		return
	}
	if major >= 21 {
		fmt.Printf("%s Dev tool: java %d\n", checkPass, major)
		return
	}
	fmt.Printf("%s Dev tool: java %d -- JDT LS needs Java 21+\n", checkWarn, major)
}

func countConfigPlugins(cfg *config.Info) int {
	if len(cfg.Plugins) == 0 {
		return 0
	}
	specs, err := plugin.ParsePluginConfig(cfg.Plugins)
	if err != nil {
		return 0
	}
	return len(specs)
}

// reportPlugins prints configured vs resolvable plugin status and returns
// next-step hints for new users. Never marks the doctor run as failed.
func reportPlugins(cfg *config.Info, dataDir string) []string {
	var hints []string
	inConfig := map[string]bool{}
	if cfg != nil && len(cfg.Plugins) > 0 {
		if specs, err := plugin.ParsePluginConfig(cfg.Plugins); err == nil {
			for _, s := range specs {
				inConfig[s.Name] = true
			}
		}
	}

	if cfg == nil || len(cfg.Plugins) == 0 {
		fmt.Printf("%s Plugins: none configured (builtins still work)\n", checkPass)
		fmt.Printf("    tip: tinycode plugin install safety-net\n")
		fmt.Printf("    tip: then tinycode plugin enable safety-net\n")
		hints = append(hints, `tinycode plugin install safety-net`)
		hints = append(hints, `tinycode plugin enable safety-net`)
		hints = append(hints, reportOrphanPluginBinaries(inConfig)...)
		return hints
	}

	specs, err := plugin.ParsePluginConfig(cfg.Plugins)
	if err != nil {
		fmt.Printf("%s Plugins: config present but unreadable (%v)\n", checkWarn, err)
		hints = append(hints, "fix plugins array in config (string or [name, options] entries)")
		hints = append(hints, reportOrphanPluginBinaries(inConfig)...)
		return hints
	}

	configured := 0
	resolvable := 0
	var missing []string
	for _, s := range specs {
		if plugin.IsBuiltin(s.Name) {
			continue
		}
		configured++
		if _, err := plugin.ResolveBinary(s.Name); err != nil {
			missing = append(missing, s.Name)
			fmt.Printf("%s Plugin %s configured but not installed\n", checkWarn, s.Name)
			fmt.Printf("    install: tinycode plugin install %s\n", s.Name)
		} else {
			resolvable++
		}
	}

	if configured == 0 {
		fmt.Printf("%s Plugins: only builtins listed in config\n", checkPass)
		hints = append(hints, reportOrphanPluginBinaries(inConfig)...)
		return hints
	}

	if len(missing) == 0 {
		fmt.Printf("%s Plugins: %d configured, %d installed\n", checkPass, configured, resolvable)
		fmt.Printf("    logs: %s/plugin-<name>.log (stderr from plugin processes)\n", dataDir)
	} else {
		fmt.Printf("%s Plugins: %d configured, %d installed, %d missing\n",
			checkWarn, configured, resolvable, len(missing))
		for _, name := range missing {
			hints = append(hints, "tinycode plugin install "+name)
		}
	}
	hints = append(hints, reportOrphanPluginBinaries(inConfig)...)
	return hints
}

// reportOrphanPluginBinaries tips/WARNs for binaries under ConfigDir()/plugins
// that are not listed in config. Never fails doctor.
func reportOrphanPluginBinaries(inConfig map[string]bool) []string {
	pluginDir := filepath.Join(config.ConfigDir(), "plugins")
	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		return nil
	}
	var orphans []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if inConfig[name] {
			continue
		}
		info, err := e.Info()
		if err != nil || info.Mode()&0o111 == 0 {
			continue
		}
		orphans = append(orphans, name)
	}
	if len(orphans) == 0 {
		return nil
	}
	fmt.Printf("%s Plugins: %d installed binary(ies) not in config\n", checkWarn, len(orphans))
	var hints []string
	for _, name := range orphans {
		fmt.Printf("    tip: tinycode plugin enable %s\n", name)
		hints = append(hints, "tinycode plugin enable "+name)
	}
	return hints
}

// checkConfiguredPlugins is kept for unit tests; prefer reportPlugins in runDoctor.
func checkConfiguredPlugins(cfg *config.Info) {
	_ = reportPlugins(cfg, config.DataDir())
}

func printDoctorNextSteps(hints []string) {
	if len(hints) == 0 {
		return
	}
	seen := map[string]bool{}
	fmt.Println("    next steps:")
	for _, h := range hints {
		if h == "" || seen[h] {
			continue
		}
		seen[h] = true
		fmt.Printf("    - %s\n", h)
	}
}

// checkClipboard reports Linux clipboard helpers. Warns when none are found;
// never marks the doctor run as failed — the rest of the app works without them.
func checkClipboard() {
	if runtime.GOOS != "linux" {
		// darwin pbcopy is standard — skip
		return
	}
	for _, bin := range []string{"wl-copy", "xclip", "xsel"} {
		if _, err := exec.LookPath(bin); err == nil {
			fmt.Printf("%s Clipboard: %s found\n", checkPass, bin)
			return
		}
	}
	fmt.Printf("%s Clipboard: no wl-copy/xclip/xsel in PATH (copy — mouse or /copy — will show an error toast)\n", checkWarn)
}

// checkIDEs reports IDEs found on PATH and points at ACP setup docs.
// Information only — no install or modify actions.
func checkIDEs() {
	ides := []struct {
		command string
		name    string
	}{
		{command: "code", name: "VS Code"},
		{command: "cursor", name: "Cursor"},
		{command: "nvim", name: "Neovim"},
		{command: "idea", name: "JetBrains"},
	}

	found := false
	for _, ide := range ides {
		if _, err := exec.LookPath(ide.command); err == nil {
			fmt.Printf("%s %s detected -- ACP setup: see docs/acp-integration.md\n", checkPass, ide.name)
			found = true
		}
	}
	if !found {
		fmt.Printf("%s No IDE detected -- tinycode works standalone via TUI and web\n", checkPass)
	}
}
