package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"time"

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

	// Plugins
	pluginCount := countConfigPlugins(cfg)
	fmt.Printf("%s Plugins: %d configured\n", checkPass, pluginCount)
	checkConfiguredPlugins(cfg)

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
	checkDevTools(dir)

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

	if failed {
		os.Exit(1)
	}
}

func checkProviders(cfg *config.Info, failed *bool) {
	type providerProbe struct {
		name string
		url  string
	}

	probes := []providerProbe{
		{name: "ollama", url: ollamaURL()},
		{name: "lm-studio", url: lmStudioURL()},
	}

	// Check vLLM if configured
	if v := os.Getenv("TINYCODE_VLLM_HOST"); v != "" {
		probes = append(probes, providerProbe{name: "vllm", url: v})
	}

	anyLocal := false
	for _, p := range probes {
		if probeHTTP(p.url) {
			fmt.Printf("%s Provider: %s -- connected (%s)\n", checkPass, p.name, p.url)
			anyLocal = true
			continue
		}
		// Local refusal is a warning — fail only if no usable provider path remains.
		fmt.Printf("%s Provider: %s -- connection refused (%s)\n", checkWarn, p.name, p.url)
	}

	hasCloudKey := false
	if apiKey := os.Getenv("OPENROUTER_API_KEY"); apiKey != "" {
		fmt.Printf("%s Provider: openrouter -- API key set\n", checkPass)
		hasCloudKey = true
	}

	// Config-defined providers
	anyConfig := false
	knownProbe := map[string]bool{"ollama": true, "vllm": true, "lm-studio": true, "openrouter": true}
	for id, pc := range cfg.Provider {
		if knownProbe[id] {
			continue
		}
		if pc.Options != nil {
			if baseURL, ok := pc.Options["baseURL"].(string); ok && baseURL != "" {
				if probeHTTP(baseURL) {
					fmt.Printf("%s Provider: %s -- connected (%s)\n", checkPass, id, baseURL)
					anyConfig = true
				} else {
					fmt.Printf("%s Provider: %s -- connection refused (%s)\n", checkWarn, id, baseURL)
				}
				continue
			}
		}
		fmt.Printf("%s Provider: %s -- configured (no URL to probe)\n", checkWarn, id)
	}

	if providerPathUsable(anyLocal, hasCloudKey, anyConfig) {
		return
	}

	fmt.Printf("%s Provider: no usable provider path (no local provider connected and no cloud API key)\n", checkWarn)
	printProviderNextSteps()
	*failed = true
}

// providerPathUsable reports whether doctor found at least one way to reach a model.
func providerPathUsable(anyLocalConnected, hasCloudAPIKey, anyConfigConnected bool) bool {
	return anyLocalConnected || hasCloudAPIKey || anyConfigConnected
}

func printProviderNextSteps() {
	fmt.Println("    next steps:")
	fmt.Println("    - start Ollama: ollama serve")
	fmt.Println("    - or set OPENROUTER_API_KEY for cloud models")
	fmt.Println("    - then run: tinycode  (connect dialog opens if no model)")
	fmt.Println("    - or in the TUI: /connect")
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

func checkDevTools(dir string) {
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

		if _, err := exec.LookPath(t.command); err == nil {
			fmt.Printf("%s Dev tool: %s\n", checkPass, t.name)
		} else {
			fmt.Printf("%s Dev tool: %s -- not found\n", checkWarn, t.name)
			fmt.Printf("    install: %s\n", t.install)
		}
	}

	// Java / jdtls — warn-only; never sets failed. Marker Glob is project dir only
	// (shallower than LSP markerExists walk) — intentional.
	checkJavaDevTools(dir)
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

// checkConfiguredPlugins warns when a configured non-builtin plugin binary
// cannot be resolved. Never marks the doctor run as failed (5A/A15).
func checkConfiguredPlugins(cfg *config.Info) {
	if cfg == nil || len(cfg.Plugins) == 0 {
		return
	}
	specs, err := plugin.ParsePluginConfig(cfg.Plugins)
	if err != nil {
		return
	}
	for _, s := range specs {
		if plugin.IsBuiltin(s.Name) {
			continue
		}
		if _, err := plugin.ResolveBinary(s.Name); err != nil {
			fmt.Printf("%s Plugin %s configured but not installed\n", checkWarn, s.Name)
		}
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
