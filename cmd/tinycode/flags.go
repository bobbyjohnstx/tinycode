package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

type commonFlags struct {
	model                 string
	title                 string
	appendSystemPrompt    string
	appendSystemPromptFile string
	maxTokens             int
	safeMode              bool
	continueSession       bool
	resumeSession         string
}

func parseCommonFlags(name string, args []string) commonFlags {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	var f commonFlags
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
	_ = fs.Parse(args)

	// First positional argument is a directory.
	if remaining := fs.Args(); len(remaining) > 0 {
		if info, err := os.Stat(remaining[0]); err == nil && info.IsDir() {
			absDir, err := filepath.Abs(remaining[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "cannot resolve directory %s: %v\n", remaining[0], err)
				os.Exit(1)
			}
			if err := os.Chdir(absDir); err != nil {
				fmt.Fprintf(os.Stderr, "cannot change to directory %s: %v\n", absDir, err)
				os.Exit(1)
			}
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

	return f
}
