package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	daemonize "github.com/mackee/mcp-daemonize"
)

func main() {
	ctx := context.Background()
	nctx, cancel := signal.NotifyContext(ctx, os.Interrupt, os.Kill, syscall.SIGTERM)
	defer cancel()

	server := daemonize.New(nctx)
	if err := server.Start(ctx); err != nil {
		slog.ErrorContext(ctx, "failed to start server", slog.Any("error", err))
	}
}
