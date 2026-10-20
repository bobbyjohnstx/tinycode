package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/bobbyjohnstx/tinycode/internal/config"
	"github.com/bobbyjohnstx/tinycode/internal/tui"
	"github.com/bobbyjohnstx/tinycode/internal/tui/api"
)

type attachFlags struct {
	url             string
	dir             string
	continueSession bool
	sessionID       string
	fork            bool
	password        string
	username        string
	title           string
}

func runAttach(args []string) {
	setupLogger()

	fs := flag.NewFlagSet("attach", flag.ExitOnError)
	var f attachFlags
	fs.StringVar(&f.dir, "dir", "", "directory to run in (chdir if local; else remote path hint)")
	fs.BoolVar(&f.continueSession, "c", false, "continue the last session")
	fs.BoolVar(&f.continueSession, "continue", false, "continue the last session")
	fs.StringVar(&f.sessionID, "s", "", "session id to continue")
	fs.StringVar(&f.sessionID, "session", "", "session id to continue")
	fs.BoolVar(&f.fork, "fork", false, "fork the session when continuing (requires -c or -s)")
	fs.StringVar(&f.password, "p", "", "auth password / bearer token (defaults to TINYCODE_AUTH_TOKEN or TINYCODE_SERVER_PASSWORD)")
	fs.StringVar(&f.password, "password", "", "auth password / bearer token")
	fs.StringVar(&f.username, "u", "", "basic auth username (unused; password maps to Bearer token)")
	fs.StringVar(&f.username, "username", "", "basic auth username (unused; password maps to Bearer token)")
	fs.StringVar(&f.title, "title", "", "set session title")
	_ = fs.Parse(args)

	pos := fs.Args()
	if len(pos) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: tinycode attach <url> [flags]\n")
		fmt.Fprintf(os.Stderr, "\nAttach the TUI to a remote tinycode serve (no local embedded server).\n")
		fmt.Fprintf(os.Stderr, "Note: attach connects to the tinycode HTTP API — it is not a remote inference URL.\n")
		os.Exit(1)
	}
	f.url = strings.TrimRight(pos[0], "/")

	if f.fork && !f.continueSession && f.sessionID == "" {
		fmt.Fprintf(os.Stderr, "error: --fork requires --continue or --session\n")
		os.Exit(1)
	}

	directory := resolveAttachDirectory(f.dir)
	token := resolveAttachToken(f.password)

	client := api.New(f.url, directory, token)
	resumeID, err := resolveRemoteResume(client, f.continueSession, f.sessionID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if f.fork && resumeID != "" {
		branched, err := client.BranchSession(resumeID, f.title)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error forking session: %v\n", err)
			os.Exit(1)
		}
		resumeID = branched.ID
		slog.Info("forked remote session", "from", f.sessionID, "to", resumeID)
	}

	dir, _ := os.Getwd()
	cfg, err := config.Load(dir)
	if err != nil {
		slog.Warn("failed to load config", "error", err)
		cfg = &config.Info{}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := tui.Run(ctx, tui.RunConfig{
		ServerURL:       f.url,
		Directory:       directory,
		Theme:           cfg.Theme,
		Token:           token,
		Version:         version,
		ScopedModels:    cfg.ScopedModels,
		CycleAgents:     cfg.CycleAgents,
		ShellHooks:      cfg.Hooks,
		InitialTitle:    f.title,
		ResumeSessionID: resumeID,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "tui: %v\n", err)
		os.Exit(1)
	}
}

func resolveAttachDirectory(dirFlag string) string {
	if dirFlag == "" {
		cwd, _ := os.Getwd()
		return cwd
	}
	if err := os.Chdir(dirFlag); err == nil {
		cwd, _ := os.Getwd()
		return cwd
	}
	// Remote path that does not exist locally — pass through as directory hint.
	return dirFlag
}

func resolveAttachToken(passwordFlag string) string {
	if passwordFlag != "" {
		return passwordFlag
	}
	if v := os.Getenv("TINYCODE_AUTH_TOKEN"); v != "" {
		return v
	}
	if v := os.Getenv("TINYCODE_SERVER_PASSWORD"); v != "" {
		return v
	}
	return ""
}

// resolveRemoteResume finds a session on the remote server.
// Never uses local SQLite resolveResumeSession.
func resolveRemoteResume(client *api.Client, continueLast bool, sessionArg string) (string, error) {
	if !continueLast && sessionArg == "" {
		return "", nil
	}
	if sessionArg != "" {
		if info, err := client.GetSession(sessionArg); err == nil && info != nil {
			return info.ID, nil
		}
		sessions, err := client.ListSessions(50, 0)
		if err != nil {
			return "", fmt.Errorf("listing sessions: %w", err)
		}
		for _, s := range sessions {
			if s.ID == sessionArg || s.Title == sessionArg || s.Slug == sessionArg {
				return s.ID, nil
			}
		}
		return "", fmt.Errorf("session not found: %s", sessionArg)
	}
	sessions, err := client.ListSessions(1, 0)
	if err != nil {
		return "", fmt.Errorf("listing sessions: %w", err)
	}
	if len(sessions) == 0 {
		return "", fmt.Errorf("no sessions to continue")
	}
	return sessions[0].ID, nil
}
