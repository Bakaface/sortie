// Package mcp implements a Model Context Protocol server that lets Claude Code
// (or any MCP client) interact with a running sortie daemon over its Unix
// socket. The server speaks MCP over stdio and exposes a safe-by-default
// surface: creating tasks, listing workflows, and reading task state. No
// destructive operations (delete, stop, retry) are exposed in this version.
package mcp

import (
	"fmt"

	"github.com/aface/sortie/internal/client"
	"github.com/aface/sortie/internal/config"
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
		return fmt.Errorf("sortie daemon not reachable on %s: %w", cfg.Daemon.SocketPath, err)
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
func registerTools(s *server.MCPServer, c *client.Client) {
	registerCreateTask(s, c)
	registerListWorkflows(s, c)
	registerGetTask(s, c)
}

// resultErr builds an MCP error result without losing the underlying error
// message. Returning (result, nil) is the MCP convention for tool-level
// errors — the protocol-level error channel is reserved for transport faults.
func resultErr(format string, args ...any) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultError(fmt.Sprintf(format, args...)), nil
}
