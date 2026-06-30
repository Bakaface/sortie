package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Bakaface/sortie/internal/client"
	"github.com/Bakaface/sortie/internal/config"
	"github.com/Bakaface/sortie/internal/daemon"
	mcppkg "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// shortSocketPath returns a sun_path-friendly Unix socket path.
func shortSocketPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "mcp")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, fmt.Sprintf("%d.sock", os.Getpid()))
}

// fakeDaemon is a hand-rolled stand-in for the real daemon that lets us
// assert on the wire protocol the MCP server uses. It reads one request,
// looks it up in the handlers map, and writes the configured response.
type fakeDaemon struct {
	t        *testing.T
	listener net.Listener
	handlers map[daemon.MessageType]func(*daemon.Message) *daemon.Message

	mu       sync.Mutex
	received []daemon.MessageType
}

func newFakeDaemon(t *testing.T) *fakeDaemon {
	t.Helper()
	listener, err := net.Listen("unix", shortSocketPath(t))
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	t.Cleanup(func() { listener.Close() })

	f := &fakeDaemon{
		t:        t,
		listener: listener,
		handlers: map[daemon.MessageType]func(*daemon.Message) *daemon.Message{},
	}
	go f.serve()
	return f
}

func (f *fakeDaemon) socketPath() string {
	return f.listener.Addr().String()
}

func (f *fakeDaemon) handle(msgType daemon.MessageType, h func(*daemon.Message) *daemon.Message) {
	f.handlers[msgType] = h
}

func (f *fakeDaemon) requestTypes() []daemon.MessageType {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]daemon.MessageType, len(f.received))
	copy(out, f.received)
	return out
}

func (f *fakeDaemon) serve() {
	for {
		conn, err := f.listener.Accept()
		if err != nil {
			return
		}
		go f.handleConn(conn)
	}
}

func (f *fakeDaemon) handleConn(conn net.Conn) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	for scanner.Scan() {
		msg, err := daemon.DecodeMessage(scanner.Bytes())
		if err != nil {
			return
		}
		f.mu.Lock()
		f.received = append(f.received, msg.Type)
		f.mu.Unlock()

		h, ok := f.handlers[msg.Type]
		var resp *daemon.Message
		if !ok {
			resp, _ = daemon.NewMessage(daemon.MsgError, daemon.ErrorResponse{Message: fmt.Sprintf("unhandled: %s", msg.Type)})
		} else {
			resp = h(msg)
		}
		data, _ := daemon.EncodeMessage(resp)
		if _, err := conn.Write(data); err != nil {
			return
		}
	}
}

// startMCPServer wires the MCP server in-process against a fake daemon and
// returns a connected MCP client. The MCP server is otherwise identical to
// what `sortie mcp` runs in production — same registerTools call.
func startMCPServer(t *testing.T, fake *fakeDaemon) *mcppkg.Client {
	t.Helper()

	cfg := &config.Config{}
	cfg.Daemon.SocketPath = fake.socketPath()

	c := client.New(cfg)
	if err := c.Connect(); err != nil {
		t.Fatalf("daemon client connect: %v", err)
	}
	t.Cleanup(func() { c.Close() })

	s := server.NewMCPServer("sortie-test", "0.0.0", server.WithToolCapabilities(false))
	registerTools(s, c)

	mcpClient, err := mcppkg.NewInProcessClient(s)
	if err != nil {
		t.Fatalf("new in-process client: %v", err)
	}
	t.Cleanup(func() { mcpClient.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := mcpClient.Start(ctx); err != nil {
		t.Fatalf("mcp client start: %v", err)
	}
	if _, err := mcpClient.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo:      mcp.Implementation{Name: "test", Version: "0.0.0"},
		},
	}); err != nil {
		t.Fatalf("mcp initialize: %v", err)
	}
	return mcpClient
}

func TestMCP_ListsToolsAdvertisedToClients(t *testing.T) {
	fake := newFakeDaemon(t)
	c := startMCPServer(t, fake)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	got := map[string]bool{}
	for _, tool := range resp.Tools {
		got[tool.Name] = true
	}
	for _, want := range []string{"create_task", "list_workflows", "get_task", "update_step_context"} {
		if !got[want] {
			t.Errorf("tool %q not advertised; got %v", want, got)
		}
	}
}

func TestMCP_ListWorkflows_FromExplicitProjectPath(t *testing.T) {
	fake := newFakeDaemon(t)
	fake.handle(daemon.MsgListWorkflows, func(msg *daemon.Message) *daemon.Message {
		var req daemon.ListWorkflowsRequest
		_ = msg.DecodePayload(&req)
		if !filepath.IsAbs(req.ProjectPath) {
			resp, _ := daemon.NewMessage(daemon.MsgError, daemon.ErrorResponse{
				Message: fmt.Sprintf("expected absolute path, got %q", req.ProjectPath),
			})
			return resp
		}
		resp, _ := daemon.NewMessage(daemon.MsgListWorkflows, daemon.ListWorkflowsResponse{
			ProjectPath: req.ProjectPath,
			ProjectName: "test-project",
			Workflows: []daemon.WorkflowSummary{
				{Name: "implement", Description: "Plan + implement", FirstStepIsTmux: false,
					Steps: []daemon.WorkflowStepSummary{{Name: "plan"}, {Name: "implement"}}},
				{Name: "tmux-session", FirstStepIsTmux: true,
					Steps: []daemon.WorkflowStepSummary{{Name: "session", Tmux: true}}},
			},
		})
		return resp
	})

	c := startMCPServer(t, fake)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "list_workflows",
			Arguments: map[string]any{"project_path": "/tmp/some-repo"},
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool returned error: %v", textOf(res))
	}

	var payload daemon.ListWorkflowsResponse
	if err := json.Unmarshal([]byte(textOf(res)), &payload); err != nil {
		t.Fatalf("unmarshal: %v\nraw: %s", err, textOf(res))
	}
	if payload.ProjectName != "test-project" {
		t.Errorf("ProjectName: got %q, want test-project", payload.ProjectName)
	}
	if len(payload.Workflows) != 2 {
		t.Fatalf("Workflows: got %d, want 2", len(payload.Workflows))
	}
	if !payload.Workflows[1].FirstStepIsTmux {
		t.Errorf("expected second workflow to be flagged FirstStepIsTmux")
	}
}

func TestMCP_ListWorkflows_PinFieldsAndFullySpec(t *testing.T) {
	worktreeTrue := true
	fake := newFakeDaemon(t)
	fake.handle(daemon.MsgListWorkflows, func(msg *daemon.Message) *daemon.Message {
		resp, _ := daemon.NewMessage(daemon.MsgListWorkflows, daemon.ListWorkflowsResponse{
			ProjectPath: "/tmp/proj",
			ProjectName: "pinned-project",
			Workflows: []daemon.WorkflowSummary{
				{
					Name:      "pinned-impl",
					Worktree:  &worktreeTrue,
					Branch:    "feat/{{task.slug}}",
					Checkout:  "",
					Target:    "main",
					FullySpec: true,
					Steps:     []daemon.WorkflowStepSummary{{Name: "implement"}},
				},
				{
					Name:      "unpinned-impl",
					FullySpec: false,
					Steps:     []daemon.WorkflowStepSummary{{Name: "implement"}},
				},
			},
		})
		return resp
	})

	c := startMCPServer(t, fake)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "list_workflows",
			Arguments: map[string]any{"project_path": "/tmp/proj"},
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool returned error: %v", textOf(res))
	}

	var payload daemon.ListWorkflowsResponse
	if err := json.Unmarshal([]byte(textOf(res)), &payload); err != nil {
		t.Fatalf("unmarshal: %v\nraw: %s", err, textOf(res))
	}
	if len(payload.Workflows) != 2 {
		t.Fatalf("Workflows: got %d, want 2", len(payload.Workflows))
	}

	pinned := payload.Workflows[0]
	if pinned.Worktree == nil || !*pinned.Worktree {
		t.Errorf("pinned.Worktree: want *true, got %v", pinned.Worktree)
	}
	if pinned.Branch != "feat/{{task.slug}}" {
		t.Errorf("pinned.Branch: got %q, want feat/{{task.slug}}", pinned.Branch)
	}
	if pinned.Target != "main" {
		t.Errorf("pinned.Target: got %q, want main", pinned.Target)
	}
	if !pinned.FullySpec {
		t.Errorf("pinned.FullySpec: want true")
	}

	unpinned := payload.Workflows[1]
	if unpinned.FullySpec {
		t.Errorf("unpinned.FullySpec: want false")
	}
}

func TestMCP_CreateTask_PassesAllFields(t *testing.T) {
	fake := newFakeDaemon(t)

	var captured daemon.CreateTaskRequest
	fake.handle(daemon.MsgCreateTask, func(msg *daemon.Message) *daemon.Message {
		_ = msg.DecodePayload(&captured)
		resp, _ := daemon.NewMessage(daemon.MsgCreateTask, daemon.CreateTaskResponse{
			Task: daemon.TaskInfo{
				ID:     42,
				Title:  "Implement login page",
				Status: "init",
			},
		})
		return resp
	})

	c := startMCPServer(t, fake)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "create_task",
			Arguments: map[string]any{
				"description":   "Implement the login page",
				"project_path":  "/tmp/proj",
				"workflow":      "implement",
				"priority":      "high",
				"branch_name":   "feat/{{task.slug}}",
				"target_branch": "develop",
				"images":        []string{"/tmp/a.png"},
				"blocked_by":    []int{7, 8},
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool returned error: %v", textOf(res))
	}

	if captured.Description != "Implement the login page" {
		t.Errorf("Description: %q", captured.Description)
	}
	if captured.ProjectPath != "/tmp/proj" {
		t.Errorf("ProjectPath: %q", captured.ProjectPath)
	}
	if captured.Workflow != "implement" {
		t.Errorf("Workflow: %q", captured.Workflow)
	}
	if captured.Priority != "high" {
		t.Errorf("Priority: %q", captured.Priority)
	}
	if captured.BranchName != "feat/{{task.slug}}" {
		t.Errorf("BranchName: %q", captured.BranchName)
	}
	if captured.TargetBranch != "develop" {
		t.Errorf("TargetBranch: %q", captured.TargetBranch)
	}
	if len(captured.Images) != 1 || captured.Images[0] != "/tmp/a.png" {
		t.Errorf("Images: %v", captured.Images)
	}
	if len(captured.BlockedBy) != 2 || captured.BlockedBy[0] != 7 || captured.BlockedBy[1] != 8 {
		t.Errorf("BlockedBy: %v", captured.BlockedBy)
	}

	var out daemon.TaskInfo
	if err := json.Unmarshal([]byte(textOf(res)), &out); err != nil {
		t.Fatalf("unmarshal task: %v", err)
	}
	if out.ID != 42 {
		t.Errorf("returned task ID: %d, want 42", out.ID)
	}
}

func TestMCP_CreateTask_RejectsCwdOutsideRepo(t *testing.T) {
	// When project_path isn't supplied, the tool must fall back to
	// `git rev-parse --show-toplevel` on cwd. From an arbitrary tempdir
	// that's not a git repo, the call must fail cleanly rather than
	// pass an empty path to the daemon (which would silently create a
	// wrong-rooted project row).
	tmpDir, err := os.MkdirTemp("", "mcp-no-git")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	origCwd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origCwd) })

	fake := newFakeDaemon(t)
	// Don't register a create handler — if the tool wrongly delegates to
	// the daemon, we'll see an "unhandled" error instead of our cwd error.
	c := startMCPServer(t, fake)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "create_task",
			Arguments: map[string]any{"description": "hello"},
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected tool error for non-git cwd, got success: %s", textOf(res))
	}
	if !strings.Contains(textOf(res), "not inside a git repository") {
		t.Errorf("error should mention missing git repo; got %q", textOf(res))
	}

	for _, msgType := range fake.requestTypes() {
		if msgType == daemon.MsgCreateTask {
			t.Errorf("create_task should not be sent when cwd resolution fails")
		}
	}
}

func TestMCP_GetTask_AggregatesSections(t *testing.T) {
	fake := newFakeDaemon(t)
	fake.handle(daemon.MsgGetTask, func(msg *daemon.Message) *daemon.Message {
		resp, _ := daemon.NewMessage(daemon.MsgGetTask, daemon.GetTaskResponse{
			Task: daemon.TaskInfo{ID: 99, Title: "demo", Status: "running"},
		})
		return resp
	})
	fake.handle(daemon.MsgGetTaskSteps, func(msg *daemon.Message) *daemon.Message {
		resp, _ := daemon.NewMessage(daemon.MsgGetTaskSteps, daemon.GetTaskStepsResponse{
			Steps: []daemon.TaskStepDetail{
				{Name: "plan", Status: "completed", Context: "outline"},
				{Name: "implement", Status: "running"},
			},
		})
		return resp
	})
	fake.handle(daemon.MsgGetStepContexts, func(msg *daemon.Message) *daemon.Message {
		resp, _ := daemon.NewMessage(daemon.MsgGetStepContexts, daemon.GetStepContextsResponse{
			Steps: map[string]string{"plan": "outline"},
		})
		return resp
	})

	c := startMCPServer(t, fake)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "get_task",
			Arguments: map[string]any{
				"task_id":               99,
				"include_steps":         true,
				"include_step_contexts": true,
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool returned error: %v", textOf(res))
	}

	var out GetTaskResult
	if err := json.Unmarshal([]byte(textOf(res)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Task == nil || out.Task.ID != 99 {
		t.Errorf("task: %+v", out.Task)
	}
	if len(out.Steps) != 2 {
		t.Errorf("steps: got %d, want 2", len(out.Steps))
	}
	if out.StepContexts["plan"] != "outline" {
		t.Errorf("StepContexts: %v", out.StepContexts)
	}

	// Verify the optional sections were actually requested.
	seen := map[daemon.MessageType]bool{}
	for _, mt := range fake.requestTypes() {
		seen[mt] = true
	}
	for _, want := range []daemon.MessageType{daemon.MsgGetTask, daemon.MsgGetTaskSteps, daemon.MsgGetStepContexts} {
		if !seen[want] {
			t.Errorf("expected request %s", want)
		}
	}
}

func TestMCP_GetTask_OmitsSectionsWhenNotRequested(t *testing.T) {
	fake := newFakeDaemon(t)
	fake.handle(daemon.MsgGetTask, func(msg *daemon.Message) *daemon.Message {
		resp, _ := daemon.NewMessage(daemon.MsgGetTask, daemon.GetTaskResponse{
			Task: daemon.TaskInfo{ID: 1, Title: "t", Status: "pending"},
		})
		return resp
	})

	c := startMCPServer(t, fake)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "get_task",
			Arguments: map[string]any{"task_id": 1},
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool error: %s", textOf(res))
	}

	for _, mt := range fake.requestTypes() {
		switch mt {
		case daemon.MsgGetTaskSteps, daemon.MsgGetStepContexts, daemon.MsgGetOutput, daemon.MsgGetLogs:
			t.Errorf("did not expect optional request %s when flags off", mt)
		}
	}
}

func TestMCP_GetTask_RejectsInvalidID(t *testing.T) {
	fake := newFakeDaemon(t)
	c := startMCPServer(t, fake)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "get_task",
			Arguments: map[string]any{"task_id": 0},
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected error for task_id=0")
	}

	if len(fake.requestTypes()) != 0 {
		t.Errorf("daemon should not be contacted for invalid id; got %v", fake.requestTypes())
	}
}

func TestMCP_UpdateStepContext_ForwardsPayload(t *testing.T) {
	fake := newFakeDaemon(t)

	var captured daemon.UpdateActiveStepContextRequest
	fake.handle(daemon.MsgUpdateActiveStepContext, func(msg *daemon.Message) *daemon.Message {
		_ = msg.DecodePayload(&captured)
		resp, _ := daemon.NewMessage(daemon.MsgOK, daemon.OKResponse{Message: "ok"})
		return resp
	})

	c := startMCPServer(t, fake)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "update_step_context",
			Arguments: map[string]any{
				"task_id":   42,
				"step_name": "implement",
				"context":   "canonical artifact body",
				"mode":      "append",
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool returned error: %v", textOf(res))
	}

	if captured.TaskID != 42 {
		t.Errorf("TaskID: %d, want 42", captured.TaskID)
	}
	if captured.StepName != "implement" {
		t.Errorf("StepName: %q", captured.StepName)
	}
	if captured.Context != "canonical artifact body" {
		t.Errorf("Context: %q", captured.Context)
	}
	if captured.Mode != "append" {
		t.Errorf("Mode: %q", captured.Mode)
	}
}

func TestMCP_UpdateStepContext_RejectsInvalidArgs(t *testing.T) {
	fake := newFakeDaemon(t)
	c := startMCPServer(t, fake)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cases := []struct {
		name      string
		arguments map[string]any
		wantErr   string
	}{
		{
			name:      "task_id zero",
			arguments: map[string]any{"task_id": 0, "step_name": "implement", "context": "x"},
			wantErr:   "task_id must be a positive integer",
		},
		{
			name:      "empty step_name",
			arguments: map[string]any{"task_id": 1, "step_name": "  ", "context": "x"},
			wantErr:   "step_name is required",
		},
		{
			name:      "invalid mode",
			arguments: map[string]any{"task_id": 1, "step_name": "implement", "context": "x", "mode": "overwrite"},
			wantErr:   "invalid mode",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := c.CallTool(ctx, mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Name:      "update_step_context",
					Arguments: tc.arguments,
				},
			})
			if err != nil {
				t.Fatalf("CallTool: %v", err)
			}
			if !res.IsError {
				t.Fatalf("expected tool error, got success: %s", textOf(res))
			}
			if !strings.Contains(textOf(res), tc.wantErr) {
				t.Errorf("error should contain %q, got %q", tc.wantErr, textOf(res))
			}
		})
	}

	// None of the rejected calls should have touched the daemon.
	for _, mt := range fake.requestTypes() {
		if mt == daemon.MsgUpdateActiveStepContext {
			t.Errorf("invalid arg call leaked to daemon")
		}
	}
}

func TestMCP_UpdateStepContext_DefaultsModeToReplace(t *testing.T) {
	fake := newFakeDaemon(t)

	var captured daemon.UpdateActiveStepContextRequest
	fake.handle(daemon.MsgUpdateActiveStepContext, func(msg *daemon.Message) *daemon.Message {
		_ = msg.DecodePayload(&captured)
		resp, _ := daemon.NewMessage(daemon.MsgOK, daemon.OKResponse{Message: "ok"})
		return resp
	})

	c := startMCPServer(t, fake)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "update_step_context",
			Arguments: map[string]any{
				"task_id":   7,
				"step_name": "implement",
				"context":   "value",
			},
		},
	}); err != nil {
		t.Fatalf("CallTool: %v", err)
	}

	if captured.Mode != "replace" {
		t.Errorf("expected default mode=replace, got %q", captured.Mode)
	}
}

// textOf collapses the content slice into a single string, since our tools
// always return one TextContent block.
func textOf(res *mcp.CallToolResult) string {
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}
