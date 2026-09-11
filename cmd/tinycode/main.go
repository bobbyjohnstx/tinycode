package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	_ "github.com/bobbyjohnstx/tinycode-go/internal/earlyinit"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	defer func() {
		if logFile != nil {
			logFile.Close()
		}
	}()

	if len(os.Args) < 2 {
		runTUI(nil)
		return
	}

	cmd := os.Args[1]

	// If the first arg is a flag, treat it as a TUI invocation with flags.
	if strings.HasPrefix(cmd, "-") {
		runTUI(os.Args[1:])
		return
	}

	// If the first arg is an existing directory, use it as the working directory.
	if info, err := os.Stat(cmd); err == nil && info.IsDir() {
		absDir, err := filepath.Abs(cmd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "resolving directory: %v\n", err)
			os.Exit(1)
		}
		if err := os.Chdir(absDir); err != nil {
			fmt.Fprintf(os.Stderr, "changing directory: %v\n", err)
			os.Exit(1)
		}
		runTUI(os.Args[2:])
		return
	}

	switch cmd {
	case "tui":
		runTUI(os.Args[2:])
	case "serve":
		runServe()
	case "web":
		runWeb()
	case "acp":
		runACP()
	case "run":
		runRun()
	case "models":
		runModels()
	case "providers":
		runProviders()
	case "session":
		runSession()
	case "status":
		runStatus()
	case "export":
		runExport()
	case "plugin":
		runPlugin()
	case "agent":
		runAgent()
	case "debug":
		runDebug()
	case "version", "--version", "-v":
		printVersion()
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printVersion() {
	fmt.Printf("tinycode %s (%s) built %s\n", version, commit, date)
	fmt.Printf("go %s %s/%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
}

func printUsage() {
	fmt.Println("tinycode - Local-LLM-first AI coding assistant")
	fmt.Println()
	fmt.Println("Usage: tinycode [command|directory] [flags]")
	fmt.Println()
	fmt.Println("Running with no command starts the terminal UI (same as 'tinycode tui').")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  tui        Start terminal UI (default)")
	fmt.Println("  run        Run a prompt non-interactively and exit")
	fmt.Println("  serve      Start headless API server (port 4096)")
	fmt.Println("  web        Start server and open web interface")
	fmt.Println("  acp        Agent Client Protocol mode (stdio, for IDE integration)")
	fmt.Println("  models     List available models")
	fmt.Println("  providers  List discovered providers")
	fmt.Println("  session    Manage sessions (list, delete)")
	fmt.Println("  status     Show server health and status")
	fmt.Println("  export     Export session messages as JSON")
	fmt.Println("  plugin     Manage plugins (list, install, uninstall)")
	fmt.Println("  agent      List available agents")
	fmt.Println("  debug      Debug info (config, paths)")
	fmt.Println("  version    Print version information")
	fmt.Println("  help       Show this help message")
	fmt.Println()
	fmt.Println("Global flags (tui, run, serve, web):")
	fmt.Println("  -m, --model       Model to use (provider/model)")
	fmt.Println()
	fmt.Println("Run flags:")
	fmt.Println("  --agent           Agent to use (default: build)")
	fmt.Println("  --format          Output format: default, json")
	fmt.Println("  -c, --continue    Continue an existing session")
	fmt.Println("  -s, --session     Session ID to continue")
	fmt.Println("  --title           Session title")
	fmt.Println("  --dangerously-skip-permissions  Auto-approve all tool permissions")
	fmt.Println("  -i, --interactive  Show permission prompts (default: auto-deny)")
	fmt.Println("  --permissions     Permission handling: default, json")
	fmt.Println("  --max-iterations  Maximum processor iterations (0 = default 200)")
	fmt.Println("  --multi-turn      Multi-turn mode: loop on stdin after initial prompt")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  tinycode                              Start TUI in current directory")
	fmt.Println("  tinycode ~/projects/myapp              Start TUI in specified directory")
	fmt.Println("  tinycode -m ollama/qwen3:8b            Start TUI with specific model")
	fmt.Println("  tinycode run -m ollama/qwen3:8b \"fix the bug\"")
	fmt.Println("  tinycode run ~/projects/myapp \"fix the bug\"")
	fmt.Println("  echo \"explain main.go\" | tinycode run -m ollama/qwen3:8b")
	fmt.Println("  tinycode run --format json --multi-turn -m ollama/qwen3:8b")
	fmt.Println("  tinycode run --max-iterations 50 -m ollama/qwen3:8b \"summarize\"")
	fmt.Println("  tinycode serve -m ollama/qwen3:8b      Start server with specific model")
	fmt.Println()
	fmt.Println("Environment:")
	fmt.Println("  TINYCODE_PORT       Override default server port (4096)")
	fmt.Println("  TINYCODE_HOST       Override default bind address (127.0.0.1)")
	fmt.Println("  TINYCODE_DB         Override database path")
	fmt.Println("  TINYCODE_LOG_LEVEL  Set log level (debug, info, warn, error)")
	fmt.Println("  TINYCODE_WEB_DIR    Serve web UI from directory (dev mode)")
	fmt.Println()
	fmt.Println("Logs: ~/.local/share/tinycode/tinycode.log")
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max < 4 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

// copyFile copies src to dst, preserving executable permissions.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("create destination: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	return nil
}
