package main

import (
	"log/slog"

	daemonize "github.com/mackee/mcp-daemonize"
)

func main() {
	server := daemonize.New()
	if err := server.Start(); err != nil {
		slog.Error("failed to start server", slog.Any("error", err))
	}
}
