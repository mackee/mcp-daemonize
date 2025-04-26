package main

import (
	"context"
	"log/slog"

	daemonize "github.com/mackee/mcp-daemonize"
)

func main() {
	ctx := context.Background()
	server := daemonize.New()
	if err := server.Start(ctx); err != nil {
		slog.ErrorContext(ctx, "failed to start server", slog.Any("error", err))
	}
}
