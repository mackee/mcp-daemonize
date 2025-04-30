# mcp-daemonize

mcp-daemonize is a [Model Context Protocol (MCP)](https://modelcontextprotocol.io/introduction) server designed to provide seamless management of long-running daemons (such as development servers) for AI agents like Claude Code and Cline. It enables advanced automation and debugging capabilities by allowing AI agents to start, stop, and monitor daemons, as well as access their logs in real time.

## Motivation

While AI agents can execute commands, they are not well-suited for managing long-running development servers (e.g., `vite dev`). This is because the agent typically waits for the command to finish, which is not suitable for servers that are meant to run continuously during development. As a workaround, users often start development servers in a separate shell, but this prevents the AI agent from directly accessing the server's logs, making autonomous debugging impossible.

mcp-daemonize solves these problems by providing:
- An interface to start daemons without waiting for their termination.
- Tools to stop running daemons.
- Tools to view real-time logs of running daemons.

With these features, mcp-daemonize enables AI agents to fully manage development servers, monitor their output, and perform automated debugging, greatly expanding the possibilities for autonomous coding.

## Use Cases

- Starting and stopping development servers (e.g., Vite, Next.js, etc.) from an AI agent.
- Viewing and analyzing real-time logs of running daemons for debugging.
- Automating development workflows that require persistent background processes.
- Enabling AI agents to manage, monitor, and debug long-running processes autonomously.

## Prerequisites

1. Go 1.24.2 or later installed.
2. (Optional) Docker, if you wish to run mcp-daemonize in a container.

## Installation

### Download from GitHub Releases

Visit the [Releases page](https://github.com/mackee/mcp-daemonize/releases) to download the latest version.

1. Download the archive for your platform from the latest release page.
2. Extract the archive and move the binary to a directory in your PATH.

```bash
wget https://github.com/mackee/mcp-daemonize/releases/download/vX.Y.Z/mcp-daemonize_X.Y.Z_Darwin_x86_64.tar.gz
tar -xzf mcp-daemonize_X.Y.Z_Darwin_x86_64.tar.gz
sudo mv mcp-daemonize /usr/local/bin/
```

### Build from Source

You can install the latest version using `go install` (Go 1.24.2 or later):

```bash
go install github.com/mackee/mcp-daemonize/cmd/mcp-daemonize@latest
```

### Usage with MCP Host (e.g., Claude Code, Cline)

Add the following configuration to your MCP host settings (e.g., `mcp.json`):

```json
{
  "servers": {
    "daemonize": {
      "command": "/path/to/mcp-daemonize",
      "args": [],
      "env": {}
    }
  }
}
```

Replace `/path/to/mcp-daemonize` with the actual path to the built binary.

## Usage

mcp-daemonize provides the following tools for AI agents:

### Tools

- **daemonize_start**
  - Start a long-running process (e.g., a development server) as a daemon.
  - **Parameters:**
    - `name` (string, required): Name of the daemon.
    - `command` (string[], optional): Command to run (e.g., `["npm", "run", "dev"]`).
    - `workdir` (string, required): Working directory for the daemon (absolute path).

- **daemonize_stop**
  - Stop a running daemon by name.
  - **Parameters:**
    - `name` (string, required): Name of the daemon to stop.

- **daemonize_list**
  - List all currently running daemons.
  - **Parameters:** None

- **daemonize_logs**
  - Retrieve the latest logs from a running daemon.
  - **Parameters:**
    - `name` (string, required): Name of the daemon.
    - `tail` (number, required): Number of lines to read from the end of the log.


## Example Workflow

1. Start a development server as a daemon using `daemonize_start`.
2. Use `daemonize_logs` to monitor the server's output in real time.
3. Stop the server with `daemonize_stop` when finished.

## License

This project is licensed under the terms of the MIT open source license. See [LICENSE](./LICENSE) for details.
