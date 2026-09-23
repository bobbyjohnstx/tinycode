package main

import (
	"flag"
	"fmt"
	"os"
)

type commonFlags struct {
	model                 string
	title                 string
	appendSystemPrompt    string
	appendSystemPromptFile string
	maxTokens             int
	safeMode              bool
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
	_ = fs.Parse(args)

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
