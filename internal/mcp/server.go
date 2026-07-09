// Package mcp implements a Model Context Protocol server that lets Claude Code
// (or any MCP client) interact with a running sortie daemon over its Unix
// socket. The server speaks MCP over stdio and exposes a safe-by-default
// surface: creating tasks, listing workflows, and reading task state. No
// destructive operations (delete, stop, retry) are exposed in this version.
package mcp

import (
	"fmt"

	"github.com/Bakaface/sortie/internal/client"
	"github.com/Bakaface/sortie/internal/config"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	serverName    = "sortie"
	serverVersion = "0.1.0"
)

// Serve constructs the MCP server, registers tools, and runs it over stdio
// until stdin closes. The supplied config is used to find the daemon socket;
// it is not required for the project the caller's tools target — those are
// resolved per-call via git rev-parse on the client cwd.
func Serve(cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("config is required")
	}

	// Build a daemon client up front so we can fail fast if the daemon isn't
	// running. The connection is held for the lifetime of the MCP process.
	c := client.New(cfg)
	if err := c.Connect(); err != nil {
		return fmt.Errorf("sortie daemon not reachable on %s: %w", cfg.SocketPath, err)
	}
	defer c.Close()

	s := server.NewMCPServer(
		serverName,
		serverVersion,
		server.WithToolCapabilities(false),
	)

	registerTools(s, c)

	return server.ServeStdio(s)
}

// registerTools wires the public tool surface. Keep this list aligned with
// the "Safe-by-default" promise: no destructive operations.
//
// create_tasks_and_wait and wait_for_tasks are intentionally additive: they
// spawn children and gate the caller's own step, but never delete, stop, or
// retry foreign tasks. They mutate task_waits_on edges only, which the engine
// clears automatically on the parent's resume.
//
// update_step_context is included even though it mutates DB state because the
// daemon enforces strict safety constraints: it only writes to the task's
// currently-active step (verified against tasks.current_step AND task_steps
// row status), and it cannot affect any other task. It cannot delete data,
// stop agents, or retry/revert tasks. The worst-case is that an agent
// overwrites its own in-flight context — recoverable by re-running the step.
func registerTools(s *server.MCPServer, c *client.Client) {
	registerCreateTask(s, c)
	registerListWorkflows(s, c)
	registerGetTask(s, c)
	registerCreateTasksAndWait(s, c)
	registerWaitForTasks(s, c)
	registerUpdateStepContext(s, c)
}

// resultErr builds an MCP error result without losing the underlying error
// message. Returning (result, nil) is the MCP convention for tool-level
// errors — the protocol-level error channel is reserved for transport faults.
func resultErr(format string, args ...any) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultError(fmt.Sprintf(format, args...)), nil
}
