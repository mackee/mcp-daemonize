package daemonize

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type Server struct {
	Daemons map[string]*Daemon
}

func New(ctx context.Context) *Server {
	return &Server{
		Daemons: make(map[string]*Daemon),
	}
}

func (s *Server) Start(sctx context.Context) error {
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
		name, ok := request.Params.Arguments["name"].(string)
		if !ok {
			return mcp.NewToolResultError("invalid name parameter"), nil
		}
		_command, ok := request.Params.Arguments["command"].([]any)
		if !ok {
			return mcp.NewToolResultError("invalid command parameter"), nil
		}
		command := make([]string, len(_command))
		for i, v := range _command {
			if str, ok := v.(string); ok {
				command[i] = str
			} else {
				return mcp.NewToolResultError("invalid command parameter"), nil
			}
		}
		workdir, ok := request.Params.Arguments["workdir"].(string)
		if !ok {
			return mcp.NewToolResultError("invalid workdir parameter"), nil
		}
		daemon := NewDaemon(name, command, workdir)
		if err := daemon.Start(sctx); err != nil {
			return mcp.NewToolResultErrorFromErr(fmt.Sprintf("failed to start daemon %s", name), err), nil
		}
		s.Daemons[name] = daemon
		return mcp.NewToolResultText("Daemon started successfully"), nil
	})
	ms.AddTool(stopTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, ok := request.Params.Arguments["name"].(string)
		if !ok {
			return mcp.NewToolResultError("invalid name parameter"), nil
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
		if err := daemon.Stop(sctx); err != nil {
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
		name, ok := request.Params.Arguments["name"].(string)
		if !ok {
			return mcp.NewToolResultError("invalid name parameter"), nil
		}
		daemon, ok := s.Daemons[name]
		if !ok {
			return mcp.NewToolResultError(fmt.Sprintf("daemon %s not found", name)), nil
		}
		tail, ok := request.Params.Arguments["tail"].(float64)
		if !ok {
			return mcp.NewToolResultError("invalid tail parameter"), nil
		}
		if tail < 0 {
			return mcp.NewToolResultError("tail parameter must be non-negative"), nil
		}
		if tail == 0 {
			return mcp.NewToolResultText("No logs available"), nil
		}
		if tail > float64(daemon.Logger.Lines()) {
			tail = float64(daemon.Logger.Lines())
		}
		offset := daemon.Logger.Lines() - int64(tail)
		offset = max(0, offset)
		lines, err := daemon.Logger.ReadLine(offset)
		if err != nil {
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
		fmt.Printf("Server error: %v\n", err)
	}
	return nil
}
