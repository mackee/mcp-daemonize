package daemonize

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type Server struct {
	Daemons map[string]*Daemon
}

func New() *Server {
	return &Server{
		Daemons: make(map[string]*Daemon),
	}
}

func (s *Server) Start() error {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))
	signal.Ignore(syscall.SIGPIPE)

	ms := server.NewMCPServer(
		"Daemonize",
		"1.0.0",
		server.WithResourceCapabilities(true, true),
		server.WithLogging(),
		server.WithRecovery(),
	)

	startTool := mcp.NewTool("daemonize_start",
		mcp.WithDescription("Start a daemon"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the daemon"),
		),
		mcp.WithArray("command",
			mcp.Description("Command to run"),
			mcp.Items(map[string]any{
				"type": "string",
			}),
		),
		mcp.WithString("workdir",
			mcp.Required(),
			mcp.Description("Working directory of the daemon in absolute path"),
		),
	)
	stopTool := mcp.NewTool("daemonize_stop",
		mcp.WithDescription("Stop a daemon"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the daemon"),
		),
	)
	listTool := mcp.NewTool("daemonize_list",
		mcp.WithDescription("List running daemons"),
	)
	logsTool := mcp.NewTool("daemonize_logs",
		mcp.WithDescription("Get logs of a daemon"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the daemon"),
		),
		mcp.WithNumber("tail",
			mcp.Required(),
			mcp.Description("Number of lines to read from the end of the log"),
		),
	)

	ms.AddTool(startTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := request.RequireString("name")
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid name parameter", err), nil
		}
		command, err := request.RequireStringSlice("command")
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid command parameter", err), nil
		}
		workdir, err := request.RequireString("workdir")
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid workdir parameter", err), nil
		}
		daemon := NewDaemon(name, command, workdir)
		if err := daemon.Start(ctx); err != nil {
			return mcp.NewToolResultErrorFromErr(fmt.Sprintf("failed to start daemon %s", name), err), nil
		}
		s.Daemons[name] = daemon
		return mcp.NewToolResultText("Daemon started successfully"), nil
	})
	ms.AddTool(stopTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := request.RequireString("name")
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid name parameter", err), nil
		}
		daemon, ok := s.Daemons[name]
		if !ok {
			return mcp.NewToolResultError(fmt.Sprintf("daemon %s not found", name)), nil
		}
		status, err := daemon.Status()
		if err != nil {
			return mcp.NewToolResultErrorFromErr(fmt.Sprintf("failed to stop daemon %s", name), err), nil
		}
		if status != DaemonStatusRunning {
			delete(s.Daemons, name)
			return mcp.NewToolResultText("Daemon already stopped"), nil
		}
		if err := daemon.Stop(ctx); err != nil {
			return mcp.NewToolResultErrorFromErr(fmt.Sprintf("failed to stop daemon %s", name), err), nil
		}
		delete(s.Daemons, name)
		return mcp.NewToolResultText("Daemon stopped successfully"), nil
	})
	ms.AddTool(listTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if len(s.Daemons) == 0 {
			return mcp.NewToolResultText("No daemons running"), nil
		}
		names := slices.Collect(maps.Keys(s.Daemons))
		slices.Sort(names)
		result := &strings.Builder{}
		result.WriteString("Running daemons:\n")
		for _, name := range names {
			d := s.Daemons[name]
			status, err := d.Status()
			if err != nil {
				return mcp.NewToolResultErrorFromErr(fmt.Sprintf("failed to get status of daemon %s", name), err), nil
			}
			fmt.Fprintf(result, "  - %s[%s]:[%s]: %s\n", name, strings.Join(d.Commands, " "), d.Workdir, status)
		}
		return mcp.NewToolResultText(result.String()), nil
	})
	ms.AddTool(logsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := request.RequireString("name")
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid name parameter", err), nil
		}
		daemon, ok := s.Daemons[name]
		if !ok {
			return mcp.NewToolResultError(fmt.Sprintf("daemon %s not found", name)), nil
		}
		_tail, err := request.RequireInt("tail")
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid tail parameter", err), nil
		}
		tail := int64(_tail)
		if tail < 0 {
			return mcp.NewToolResultError("tail parameter must be non-negative"), nil
		}
		if tail == 0 {
			return mcp.NewToolResultText("No logs available"), nil
		}
		if tail > daemon.Logger.Lines() {
			tail = daemon.Logger.Lines()
		}
		offset := daemon.Logger.Lines() - tail
		offset = max(0, offset)
		lines, err := daemon.Logger.ReadLine(offset)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return mcp.NewToolResultText("No logs available"), nil
			}
			return mcp.NewToolResultErrorFromErr("failed to read logs", err), nil
		}
		result := &strings.Builder{}
		result.WriteString("Daemon logs:\n")
		for i, line := range lines {
			fmt.Fprintf(result, "  %d: %s\n", int64(i)+1+offset, line)
		}
		return mcp.NewToolResultText(result.String()), nil
	})

	if err := server.ServeStdio(ms); err != nil {
		slog.Error("Server error", slog.Any("error", err))
	}
	slog.Info("Server stop successfully")
	for name, daemon := range s.Daemons {
		if status, err := daemon.Status(); err != nil {
			slog.Error("Failed to get daemon status", slog.String("name", name), slog.Any("error", err))
			continue
		} else if status != DaemonStatusRunning {
			slog.Debug("Daemon already stopped", slog.String("name", name), slog.String("status", string(status)))
			delete(s.Daemons, name)
			continue
		}
		if err := daemon.Stop(context.Background()); err != nil {
			slog.Error("Failed to stop daemon", slog.String("name", name), slog.Any("error", err))
			continue
		}
	}

	return nil
}
