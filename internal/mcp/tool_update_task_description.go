package mcp

import (
	"context"
	"strings"

	"github.com/Bakaface/sortie/internal/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// UpdateTaskDescriptionArgs is the typed input schema for update_task_description.
type UpdateTaskDescriptionArgs struct {
	TaskID      int64  `json:"task_id" jsonschema:"Task ID to edit. Required."`
	Description string `json:"description" jsonschema:"New task description. Replaces the existing description entirely. May contain {{tasks.<id>.<field>}} references; the daemon validates them and auto-adds newly referenced tasks as blockers."`
}

func registerUpdateTaskDescription(s *server.MCPServer, c *client.Client) {
	tool := mcp.NewTool(
		"update_task_description",
		mcp.WithDescription("Replace a sortie task's description. The daemon validates any {{tasks.<id>.<field>}} references against the new value before applying it (a bad reference leaves the description untouched) and auto-adds newly referenced tasks as blockers. Returns the updated TaskInfo as JSON."),
		mcp.WithInputSchema[UpdateTaskDescriptionArgs](),
	)
	s.AddTool(tool, mcp.NewTypedToolHandler(func(_ context.Context, _ mcp.CallToolRequest, args UpdateTaskDescriptionArgs) (*mcp.CallToolResult, error) {
		return handleUpdateTaskDescription(c, args)
	}))
}

func handleUpdateTaskDescription(c *client.Client, args UpdateTaskDescriptionArgs) (*mcp.CallToolResult, error) {
	if args.TaskID <= 0 {
		return resultErr("task_id must be a positive integer")
	}
	if strings.TrimSpace(args.Description) == "" {
		return resultErr("description must not be empty")
	}

	task, err := c.UpdateTaskField(args.TaskID, "description", args.Description)
	if err != nil {
		return resultErr("update task description failed: %v", err)
	}
	return jsonResult(task)
}
