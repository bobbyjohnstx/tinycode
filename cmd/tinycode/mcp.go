package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/config"
	"github.com/bobbyjohnstx/tinycode/internal/mcp"
)

func runMCP() {
	setupLogger()

	args := os.Args[2:]
	if len(args) == 0 {
		printMCPUsage()
		os.Exit(1)
	}

	switch args[0] {
	case "list":
		runMCPList(args[1:])
	case "add":
		runMCPAdd(args[1:])
	case "auth":
		runMCPAuth(args[1:])
	case "logout":
		runMCPLogout(args[1:])
	case "debug":
		runMCPDebug(args[1:])
	case "help", "-h", "--help":
		printMCPUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown mcp subcommand: %s\n\n", args[0])
		printMCPUsage()
		os.Exit(1)
	}
}

func printMCPUsage() {
	fmt.Println("Usage: tinycode mcp <list|add|auth|logout|debug> [args]")
	fmt.Println()
	fmt.Println("Subcommands:")
	fmt.Println("  list                         List configured MCP servers and connection status")
	fmt.Println("  add [flags] NAME [-- CMD...] Add a stdio MCP server")
	fmt.Println("  add [flags] --transport sse|http|streamable-http NAME URL")
	fmt.Println("  auth NAME --token TOKEN      Set Authorization: Bearer <token>")
	fmt.Println("  auth NAME --env VAR          Set Authorization: Bearer {env:VAR}")
	fmt.Println("  logout NAME                  Clear Authorization header")
	fmt.Println("  debug NAME                   Handshake/ping diagnostics for one server")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  --project                    Write to project config (.tinycode/tinycode.json)")
	fmt.Println("  --transport TYPE             stdio (default), sse, streamable-http, or http")
	fmt.Println("  -e KEY=VALUE                 Env var for stdio servers (repeatable)")
	fmt.Println("  --header \"Key: Value\"        HTTP header for remote servers (repeatable)")
	fmt.Println()
	fmt.Println("Interactive OAuth is not supported. Use Bearer tokens or {env:VAR} refs.")
}

func runMCPList(args []string) {
	fs := flag.NewFlagSet("mcp list", flag.ExitOnError)
	_ = fs.Parse(args)

	dir, _ := os.Getwd()
	cfg, err := config.Load(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		os.Exit(1)
	}
	if len(cfg.MCP) == 0 {
		fmt.Println("No MCP servers configured.")
		return
	}

	b := bus.New()
	defer b.Close()
	svc := mcp.NewService(b)
	defer svc.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	svc.Configure(ctx, cfg.MCP)
	svc.WaitForConnections(ctx, 10*time.Second)
	status := svc.Status(ctx)

	names := make([]string, 0, len(cfg.MCP))
	for name := range cfg.MCP {
		names = append(names, name)
	}
	sort.Strings(names)

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tTRANSPORT\tSTATUS\tTOOLS\tDETAIL")
	for _, name := range names {
		mc := cfg.MCP[name]
		transport := mcpTransportLabel(mc)
		st := status[name]
		statusStr := string(st.Status)
		if statusStr == "" {
			statusStr = "unknown"
		}
		detail := st.Error
		if detail == "" {
			detail = mcpEndpointLabel(mc)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n", name, transport, statusStr, st.ToolCount, truncateStr(detail, 60))
	}
	w.Flush()
}

func runMCPAdd(args []string) {
	opts, rest, err := parseMCPAddArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		fmt.Fprintf(os.Stderr, "Usage: tinycode mcp add [flags] NAME [-- command args...]\n")
		fmt.Fprintf(os.Stderr, "       tinycode mcp add --transport sse|http NAME URL\n")
		os.Exit(1)
	}
	if len(rest) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: tinycode mcp add [flags] NAME [-- command args...]\n")
		fmt.Fprintf(os.Stderr, "       tinycode mcp add --transport sse|http NAME URL\n")
		os.Exit(1)
	}

	name := rest[0]
	rest = rest[1:]

	transportType := normalizeMCPTransport(opts.transport)
	cfg := config.MCPConfig{}

	switch transportType {
	case "", "stdio":
		cmd, cmdArgs, err := parseMCPStdioArgs(rest)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		cfg.Command = cmd
		cfg.Args = cmdArgs
		if len(opts.envs) > 0 {
			cfg.Env = map[string]string(opts.envs)
		}
		if opts.transport != "" {
			cfg.Transport = "stdio"
		}
	case "sse", "streamable-http":
		if len(rest) < 1 {
			fmt.Fprintf(os.Stderr, "error: URL required for %s transport\n", transportType)
			os.Exit(1)
		}
		cfg.URL = rest[0]
		cfg.Transport = transportType
		if len(opts.headers) > 0 {
			cfg.Headers = map[string]string(opts.headers)
		}
		if len(opts.envs) > 0 {
			fmt.Fprintf(os.Stderr, "warning: -e is ignored for remote transports; use --header or mcp auth\n")
		}
	default:
		fmt.Fprintf(os.Stderr, "error: unknown transport %q (use stdio, sse, streamable-http, or http)\n", opts.transport)
		os.Exit(1)
	}

	dir, _ := os.Getwd()
	path := config.ResolveMCPConfigPath(opts.project, dir)
	if err := config.WriteMCPServer(path, name, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Added MCP server %q to %s\n", name, path)
}

type mcpAddOpts struct {
	project   bool
	transport string
	envs      envFlag
	headers   headerFlag
}

// parseMCPAddArgs accepts flags before or after NAME, stopping at `--`.
func parseMCPAddArgs(args []string) (mcpAddOpts, []string, error) {
	var opts mcpAddOpts
	var positionals []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			positionals = append(positionals, args[i:]...)
			break
		}
		switch {
		case a == "--project":
			opts.project = true
		case a == "--transport":
			if i+1 >= len(args) {
				return opts, nil, fmt.Errorf("--transport requires a value")
			}
			i++
			opts.transport = args[i]
		case strings.HasPrefix(a, "--transport="):
			opts.transport = strings.TrimPrefix(a, "--transport=")
		case a == "-e":
			if i+1 >= len(args) {
				return opts, nil, fmt.Errorf("-e requires KEY=VALUE")
			}
			i++
			if err := opts.envs.Set(args[i]); err != nil {
				return opts, nil, err
			}
		case strings.HasPrefix(a, "-e="):
			if err := opts.envs.Set(strings.TrimPrefix(a, "-e=")); err != nil {
				return opts, nil, err
			}
		case a == "--header":
			if i+1 >= len(args) {
				return opts, nil, fmt.Errorf("--header requires \"Key: Value\"")
			}
			i++
			if err := opts.headers.Set(args[i]); err != nil {
				return opts, nil, err
			}
		case strings.HasPrefix(a, "--header="):
			if err := opts.headers.Set(strings.TrimPrefix(a, "--header=")); err != nil {
				return opts, nil, err
			}
		case a == "-h" || a == "--help":
			return opts, nil, fmt.Errorf("help")
		case strings.HasPrefix(a, "-"):
			return opts, nil, fmt.Errorf("unknown flag %s", a)
		default:
			positionals = append(positionals, a)
		}
	}
	return opts, positionals, nil
}

func runMCPAuth(args []string) {
	project, token, envVar, name, err := parseMCPAuthArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		fmt.Fprintf(os.Stderr, "Usage: tinycode mcp auth [--project] NAME (--token TOKEN | --env VAR)\n")
		os.Exit(1)
	}

	authValue := ""
	if token != "" {
		authValue = "Bearer " + token
	} else {
		authValue = "Bearer {env:" + envVar + "}"
	}

	dir, _ := os.Getwd()
	path := config.ResolveMCPConfigPath(project, dir)
	if err := config.UpdateMCPServerHeaders(path, name, nil, authValue, false); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if envVar != "" {
		fmt.Printf("Set Authorization for %q to Bearer {env:%s} in %s\n", name, envVar, path)
	} else {
		fmt.Printf("Set Authorization Bearer token for %q in %s\n", name, path)
	}
	fmt.Println("Note: interactive OAuth is not supported; restart tinycode to apply.")
}

func runMCPLogout(args []string) {
	project, name, err := parseMCPLogoutArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		fmt.Fprintf(os.Stderr, "Usage: tinycode mcp logout [--project] NAME\n")
		os.Exit(1)
	}

	dir, _ := os.Getwd()
	path := config.ResolveMCPConfigPath(project, dir)
	if err := config.UpdateMCPServerHeaders(path, name, nil, "", true); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Cleared Authorization for %q in %s\n", name, path)
}

func runMCPDebug(args []string) {
	fs := flag.NewFlagSet("mcp debug", flag.ExitOnError)
	_ = fs.Parse(args)
	rest := fs.Args()
	if len(rest) != 1 {
		fmt.Fprintf(os.Stderr, "Usage: tinycode mcp debug NAME\n")
		os.Exit(1)
	}
	name := rest[0]

	dir, _ := os.Getwd()
	cfg, err := config.Load(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		os.Exit(1)
	}
	mc, ok := cfg.MCP[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "error: MCP server %q not configured\n", name)
		os.Exit(1)
	}

	fmt.Printf("MCP debug: %s\n", name)
	fmt.Printf("Transport: %s\n", mcpTransportLabel(mc))
	fmt.Printf("Endpoint:  %s\n", mcpEndpointLabel(mc))
	if len(mc.Headers) > 0 {
		if _, hasAuth := mc.Headers["Authorization"]; hasAuth {
			fmt.Println("Auth:      Authorization header present")
		} else {
			fmt.Printf("Headers:   %d configured\n", len(mc.Headers))
		}
	}

	b := bus.New()
	defer b.Close()
	svc := mcp.NewService(b)
	defer svc.Close()

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	fmt.Println("Connecting (initialize + tools/list)...")
	svc.Configure(ctx, map[string]config.MCPConfig{name: mc})
	svc.WaitForConnections(ctx, 12*time.Second)
	elapsed := time.Since(start)

	st := svc.Status(ctx)[name]
	fmt.Printf("Status:    %s\n", st.Status)
	fmt.Printf("Duration:  %s\n", elapsed.Round(time.Millisecond))
	if st.Error != "" {
		fmt.Printf("Error:     %s\n", st.Error)
	}
	fmt.Printf("Tools:     %d\n", st.ToolCount)

	tools := svc.Tools(ctx)
	if len(tools) == 0 {
		if st.Status != mcp.StatusConnected {
			os.Exit(1)
		}
		return
	}
	names := make([]string, 0, len(tools))
	for id := range tools {
		names = append(names, id)
	}
	sort.Strings(names)
	for _, id := range names {
		fmt.Printf("  - %s\n", id)
	}
	if st.Status != mcp.StatusConnected {
		os.Exit(1)
	}
}

func parseMCPAuthArgs(args []string) (project bool, token, envVar, name string, err error) {
	var positionals []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--project":
			project = true
		case a == "--token":
			if i+1 >= len(args) {
				return false, "", "", "", fmt.Errorf("--token requires a value")
			}
			i++
			token = args[i]
		case strings.HasPrefix(a, "--token="):
			token = strings.TrimPrefix(a, "--token=")
		case a == "--env":
			if i+1 >= len(args) {
				return false, "", "", "", fmt.Errorf("--env requires a value")
			}
			i++
			envVar = args[i]
		case strings.HasPrefix(a, "--env="):
			envVar = strings.TrimPrefix(a, "--env=")
		case a == "-h" || a == "--help":
			return false, "", "", "", fmt.Errorf("help")
		case strings.HasPrefix(a, "-"):
			return false, "", "", "", fmt.Errorf("unknown flag %s", a)
		default:
			positionals = append(positionals, a)
		}
	}
	if len(positionals) != 1 {
		return false, "", "", "", fmt.Errorf("exactly one server name required")
	}
	if (token == "" && envVar == "") || (token != "" && envVar != "") {
		return false, "", "", "", fmt.Errorf("provide exactly one of --token or --env")
	}
	return project, token, envVar, positionals[0], nil
}

func parseMCPLogoutArgs(args []string) (project bool, name string, err error) {
	var positionals []string
	for _, a := range args {
		switch {
		case a == "--project":
			project = true
		case a == "-h" || a == "--help":
			return false, "", fmt.Errorf("help")
		case strings.HasPrefix(a, "-"):
			return false, "", fmt.Errorf("unknown flag %s", a)
		default:
			positionals = append(positionals, a)
		}
	}
	if len(positionals) != 1 {
		return false, "", fmt.Errorf("exactly one server name required")
	}
	return project, positionals[0], nil
}

func parseMCPStdioArgs(rest []string) (command string, args []string, err error) {
	if len(rest) == 0 {
		return "", nil, fmt.Errorf("stdio add requires `-- <command> [args...]`")
	}
	if rest[0] == "--" {
		rest = rest[1:]
	}
	if len(rest) == 0 {
		return "", nil, fmt.Errorf("stdio add requires a command after `--`")
	}
	return rest[0], rest[1:], nil
}

func normalizeMCPTransport(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "":
		return ""
	case "stdio":
		return "stdio"
	case "sse":
		return "sse"
	case "http", "streamable-http", "streamable_http", "streamablehttp":
		return "streamable-http"
	default:
		return strings.ToLower(t)
	}
}

func mcpTransportLabel(mc config.MCPConfig) string {
	if mc.Transport != "" {
		return mc.Transport
	}
	if mc.Command != "" {
		return "stdio"
	}
	if mc.URL != "" {
		return "sse"
	}
	return "stdio"
}

func mcpEndpointLabel(mc config.MCPConfig) string {
	if mc.URL != "" {
		return mc.URL
	}
	if mc.Command == "" {
		return "-"
	}
	parts := append([]string{mc.Command}, mc.Args...)
	return strings.Join(parts, " ")
}

type envFlag map[string]string

func (e *envFlag) String() string {
	if e == nil || len(*e) == 0 {
		return ""
	}
	parts := make([]string, 0, len(*e))
	for k, v := range *e {
		parts = append(parts, k+"="+v)
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func (e *envFlag) Set(value string) error {
	if *e == nil {
		*e = map[string]string{}
	}
	key, val, ok := strings.Cut(value, "=")
	if !ok {
		key = value
		val = "{env:" + value + "}"
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("invalid -e value %q (expected KEY=VALUE)", value)
	}
	(*e)[key] = val
	return nil
}

type headerFlag map[string]string

func (h *headerFlag) String() string {
	if h == nil || len(*h) == 0 {
		return ""
	}
	parts := make([]string, 0, len(*h))
	for k, v := range *h {
		parts = append(parts, k+": "+v)
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func (h *headerFlag) Set(value string) error {
	if *h == nil {
		*h = map[string]string{}
	}
	key, val, ok := strings.Cut(value, ":")
	if !ok {
		return fmt.Errorf("invalid --header %q (expected \"Key: Value\")", value)
	}
	key = strings.TrimSpace(key)
	val = strings.TrimSpace(val)
	if key == "" {
		return fmt.Errorf("invalid --header %q (empty key)", value)
	}
	(*h)[key] = val
	return nil
}
