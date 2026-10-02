package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/bobbyjohnstx/tinycode/internal/acp"
	"github.com/bobbyjohnstx/tinycode/internal/session"
)

func runACP() {
	setupLogger()
	slog.Info("starting tinycode ACP mode", "version", version)

	b, db, _ := initDependencies()
	defer db.Close()
	defer b.Close()

	store := session.NewStore(db.DB)
	adapter := acp.NewStoreAdapter(store)
	svc := acp.NewService(adapter, b)

	transport := acp.NewStdioTransport(svc, os.Stdout)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := transport.HandleStdio(ctx, os.Stdin); err != nil {
		slog.Error("ACP transport error", "error", err)
		os.Exit(1)
	}
}
