package mcp

import (
	"context"

	"github.com/aface/sortie/internal/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ListWorkflowsArgs is the typed input schema for sortie_list_workflows.
type ListWorkflowsArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"Absolute path to the project repo root. Defaults to the git toplevel of the MCP process's cwd."`
}

func registerListWorkflows(s *server.MCPServer, c *client.Client) {
	tool := mcp.NewTool(
		"sortie_list_workflows",
		mcp.WithDescription("List the workflows configured for a sortie project. Returns three groups: 'tasks' (used by sortie_create_task), 'one_off' (run-menu shortcuts), and 'init' (project bootstrap). Each entry includes name, description, first_step_is_tmux, and a step summary."),
		mcp.WithInputSchema[ListWorkflowsArgs](),
	)
	s.AddTool(tool, mcp.NewTypedToolHandler(func(_ context.Context, _ mcp.CallToolRequest, args ListWorkflowsArgs) (*mcp.CallToolResult, error) {
		return handleListWorkflows(c, args)
	}))
}

func handleListWorkflows(c *client.Client, args ListWorkflowsArgs) (*mcp.CallToolResult, error) {
	projectPath, err := resolveProjectPath(args.ProjectPath)
	if err != nil {
		return resultErr("%v", err)
	}

	resp, err := c.ListWorkflows(projectPath)
	if err != nil {
		return resultErr("list workflows failed: %v", err)
	}

	return jsonResult(resp)
}
