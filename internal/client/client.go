package client

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/aface/sortie/internal/config"
	"github.com/aface/sortie/internal/daemon"
)

type Client struct {
	cfg  *config.Config
	conn net.Conn
	mu   sync.Mutex

	respChan  chan *daemon.Message // request-response messages
	subChan   chan *daemon.Message // subscription broadcast messages
	errChan   chan error
	done      chan struct{}
	closeOnce sync.Once
}

func New(cfg *config.Config) *Client {
	return &Client{
		cfg:      cfg,
		respChan: make(chan *daemon.Message, 100),
		subChan:  make(chan *daemon.Message, 100),
		errChan:  make(chan error, 1),
		done:     make(chan struct{}),
	}
}

func (c *Client) Connect() error {
	conn, err := net.Dial("unix", c.cfg.Daemon.SocketPath)
	if err != nil {
		return fmt.Errorf("failed to connect to daemon: %w", err)
	}
	c.conn = conn

	go c.readLoop()

	return nil
}

func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		close(c.done)
	})
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// isBroadcast returns true for message types that are pushed by the daemon
// to subscribers, as opposed to responses to client requests.
func isBroadcast(t daemon.MessageType) bool {
	switch t {
	case daemon.MsgAgentUpdate, daemon.MsgTaskUpdate, daemon.MsgTmuxActivity:
		return true
	default:
		return false
	}
}

func (c *Client) readLoop() {
	scanner := bufio.NewScanner(c.conn)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // 10MB buffer for large log responses
	for scanner.Scan() {
		msg, err := daemon.DecodeMessage(scanner.Bytes())
		if err != nil {
			log.Printf("IPC decode error: %v", err)
			continue
		}

		ch := c.respChan
		if isBroadcast(msg.Type) {
			ch = c.subChan
		}

		select {
		case ch <- msg:
		case <-c.done:
			return
		}
	}

	if err := scanner.Err(); err != nil {
		select {
		case c.errChan <- err:
		default:
		}
	}
}

func (c *Client) Messages() <-chan *daemon.Message {
	return c.subChan
}

func (c *Client) Errors() <-chan error {
	return c.errChan
}

func (c *Client) send(msgType daemon.MessageType, payload any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	msg, err := daemon.NewMessage(msgType, payload)
	if err != nil {
		return err
	}

	data, err := daemon.EncodeMessage(msg)
	if err != nil {
		return err
	}

	_, err = c.conn.Write(data)
	return err
}

func (c *Client) sendAndWait(msgType daemon.MessageType, payload any) (*daemon.Message, error) {
	// Hold the lock for the entire send+wait cycle to prevent concurrent
	// sendAndWait calls from receiving each other's responses.
	c.mu.Lock()
	defer c.mu.Unlock()

	msg, err := daemon.NewMessage(msgType, payload)
	if err != nil {
		return nil, err
	}

	data, err := daemon.EncodeMessage(msg)
	if err != nil {
		return nil, err
	}

	if _, err = c.conn.Write(data); err != nil {
		return nil, err
	}

	// Read responses, skipping any broadcast messages that may have leaked
	// into respChan (e.g. due to a missing isBroadcast entry).
	for {
		select {
		case resp := <-c.respChan:
			if isBroadcast(resp.Type) {
				// Discard stale broadcast that ended up in the response channel
				continue
			}
			return resp, nil
		case err := <-c.errChan:
			return nil, err
		case <-c.done:
			return nil, fmt.Errorf("client closed")
		}
	}
}

// request sends a message and waits for a non-error response.
func (c *Client) request(msgType daemon.MessageType, payload any) (*daemon.Message, error) {
	msg, err := c.sendAndWait(msgType, payload)
	if err != nil {
		return nil, err
	}
	if msg.Type == daemon.MsgError {
		var errResp daemon.ErrorResponse
		msg.DecodePayload(&errResp)
		return nil, errors.New(errResp.Message)
	}
	return msg, nil
}

// requestOK sends a message and returns an error if the response is not successful.
func (c *Client) requestOK(msgType daemon.MessageType, payload any) error {
	_, err := c.request(msgType, payload)
	return err
}

func (c *Client) Ping() error {
	msg, err := c.sendAndWait(daemon.MsgPing, nil)
	if err != nil {
		return err
	}
	if msg.Type != daemon.MsgPong {
		return fmt.Errorf("unexpected response: %s", msg.Type)
	}
	return nil
}

func (c *Client) ListAgents() ([]daemon.AgentInfo, error) {
	msg, err := c.request(daemon.MsgListAgents, nil)
	if err != nil {
		return nil, err
	}

	var resp daemon.AgentListResponse
	if err := msg.DecodePayload(&resp); err != nil {
		return nil, err
	}

	return resp.Agents, nil
}

func (c *Client) ListTasks() ([]daemon.TaskInfo, error) {
	return c.ListTasksFiltered(0)
}

// ListTasksFiltered returns tasks optionally filtered by project ID.
// A projectID of 0 returns all tasks.
func (c *Client) ListTasksFiltered(projectID int64) ([]daemon.TaskInfo, error) {
	var payload any
	if projectID > 0 {
		payload = daemon.ListTasksRequest{ProjectID: projectID}
	}

	msg, err := c.request(daemon.MsgListTasks, payload)
	if err != nil {
		return nil, err
	}

	var resp daemon.TaskListResponse
	if err := msg.DecodePayload(&resp); err != nil {
		return nil, err
	}

	return resp.Tasks, nil
}

// ListTasksByProjectName returns tasks filtered by project name (repo basename).
func (c *Client) ListTasksByProjectName(name string) ([]daemon.TaskInfo, error) {
	msg, err := c.request(daemon.MsgListTasks, daemon.ListTasksRequest{ProjectName: name})
	if err != nil {
		return nil, err
	}

	var resp daemon.TaskListResponse
	if err := msg.DecodePayload(&resp); err != nil {
		return nil, err
	}

	return resp.Tasks, nil
}

func (c *Client) StartAgent(taskID int64) error {
	return c.requestOK(daemon.MsgStartAgent, daemon.StartAgentRequest{TaskID: taskID})
}

func (c *Client) StopAgent(agentID string) error {
	return c.requestOK(daemon.MsgStopAgent, daemon.StopAgentRequest{AgentID: agentID})
}

func (c *Client) Subscribe() error {
	_, err := c.sendAndWait(daemon.MsgSubscribe, nil)
	return err
}

func (c *Client) Unsubscribe() error {
	return c.requestOK(daemon.MsgUnsubscribe, nil)
}

func (c *Client) GetOutput(agentID string, fromLine int) ([]string, int, error) {
	msg, err := c.request(daemon.MsgGetOutput, daemon.GetOutputRequest{
		AgentID:  agentID,
		FromLine: fromLine,
	})
	if err != nil {
		return nil, 0, err
	}

	var resp daemon.OutputChunkResponse
	if err := msg.DecodePayload(&resp); err != nil {
		return nil, 0, err
	}

	return resp.Lines, resp.TotalLines, nil
}

func (c *Client) SendInput(agentID, input string) error {
	return c.requestOK(daemon.MsgSendInput, daemon.SendInputRequest{
		AgentID: agentID,
		Input:   input,
	})
}

func (c *Client) GetTask(id int64) (*daemon.TaskInfo, error) {
	msg, err := c.request(daemon.MsgGetTask, daemon.GetTaskRequest{TaskID: id})
	if err != nil {
		return nil, err
	}

	var resp daemon.GetTaskResponse
	if err := msg.DecodePayload(&resp); err != nil {
		return nil, err
	}

	return &resp.Task, nil
}

func (c *Client) RetryTask(id int64) error {
	return c.requestOK(daemon.MsgRetryTask, daemon.RetryTaskRequest{TaskID: id})
}

func (c *Client) CreateTask(description, workflow, branchName, projectPath string, worktree bool, images []string) (*daemon.TaskInfo, error) {
	msg, err := c.request(daemon.MsgCreateTask, daemon.CreateTaskRequest{
		Description: description,
		Workflow:    workflow,
		BranchName:  branchName,
		ProjectPath: projectPath,
		Worktree:    &worktree,
		Images:      images,
	})
	if err != nil {
		return nil, err
	}

	var resp daemon.CreateTaskResponse
	if err := msg.DecodePayload(&resp); err != nil {
		return nil, err
	}

	return &resp.Task, nil
}

// CreateTaskWithOptions creates a task with all available options.
func (c *Client) CreateTaskWithOptions(req daemon.CreateTaskRequest) (*daemon.TaskInfo, error) {
	msg, err := c.request(daemon.MsgCreateTask, req)
	if err != nil {
		return nil, err
	}

	var resp daemon.CreateTaskResponse
	if err := msg.DecodePayload(&resp); err != nil {
		return nil, err
	}

	return &resp.Task, nil
}

func (c *Client) ContinueTask(id int64, workflow, prompt string) error {
	return c.requestOK(daemon.MsgContinueTask, daemon.ContinueTaskRequest{TaskID: id, Workflow: workflow, Prompt: prompt})
}

func (c *Client) FinalizeTask(id int64) error {
	return c.requestOK(daemon.MsgFinalizeTask, daemon.FinalizeTaskRequest{TaskID: id})
}

func (c *Client) UpdateTaskPriority(id int64, priority string) error {
	return c.requestOK(daemon.MsgUpdatePriority, daemon.UpdatePriorityRequest{
		TaskID:   id,
		Priority: priority,
	})
}

func (c *Client) UpdateTaskField(id int64, field, value string) error {
	return c.requestOK(daemon.MsgUpdateField, daemon.UpdateFieldRequest{
		TaskID: id,
		Field:  field,
		Value:  value,
	})
}

func (c *Client) DeleteTask(id int64) error {
	return c.requestOK(daemon.MsgDeleteTask, daemon.DeleteTaskRequest{TaskID: id})
}

func (c *Client) RevertTask(id int64) error {
	return c.requestOK(daemon.MsgRevertTask, daemon.RevertTaskRequest{TaskID: id})
}

func (c *Client) AddTaskDependency(taskID, blockedByID int64) error {
	return c.requestOK(daemon.MsgUpdateDependency, daemon.UpdateDependencyRequest{
		TaskID:    taskID,
		BlockedBy: blockedByID,
		Action:    "add",
	})
}

func (c *Client) RemoveTaskDependency(taskID, blockedByID int64) error {
	return c.requestOK(daemon.MsgUpdateDependency, daemon.UpdateDependencyRequest{
		TaskID:    taskID,
		BlockedBy: blockedByID,
		Action:    "remove",
	})
}

func (c *Client) DetachBranch(id int64) error {
	return c.requestOK(daemon.MsgDetachBranch, daemon.DetachBranchRequest{TaskID: id})
}

func (c *Client) AttachBranch(id int64) error {
	return c.requestOK(daemon.MsgAttachBranch, daemon.AttachBranchRequest{TaskID: id})
}

func (c *Client) GetLogs(id int64, step string, tail int, offset int) ([]string, int, error) {
	msg, err := c.request(daemon.MsgGetLogs, daemon.GetLogsRequest{
		TaskID: id,
		Step:   step,
		Tail:   tail,
		Offset: offset,
	})
	if err != nil {
		return nil, 0, err
	}

	var resp daemon.GetLogsResponse
	if err := msg.DecodePayload(&resp); err != nil {
		return nil, 0, err
	}

	return resp.Lines, resp.TotalLines, nil
}

func (c *Client) StopTask(id int64) error {
	// Agent IDs are formatted as task ID in string form
	agentID := fmt.Sprintf("%d", id)
	return c.StopAgent(agentID)
}

// GetStepContexts fetches all completed step contexts for a task.
func (c *Client) GetStepContexts(taskID int64) (map[string]string, error) {
	msg, err := c.request(daemon.MsgGetStepContexts, daemon.GetStepContextsRequest{TaskID: taskID})
	if err != nil {
		return nil, err
	}
	var result daemon.GetStepContextsResponse
	if err := msg.DecodePayload(&result); err != nil {
		return nil, err
	}
	return result.Steps, nil
}

// GetTaskSteps fetches the full per-step state for a task, in workflow config
// order. Includes steps that have not started yet (Status == "pending").
func (c *Client) GetTaskSteps(taskID int64) ([]daemon.TaskStepDetail, error) {
	msg, err := c.request(daemon.MsgGetTaskSteps, daemon.GetTaskStepsRequest{TaskID: taskID})
	if err != nil {
		return nil, err
	}
	var result daemon.GetTaskStepsResponse
	if err := msg.DecodePayload(&result); err != nil {
		return nil, err
	}
	return result.Steps, nil
}

// UpdateStepContext overwrites the captured context for a completed step.
func (c *Client) UpdateStepContext(taskID int64, stepName, context string) error {
	return c.requestOK(daemon.MsgUpdateStepContext, daemon.UpdateStepContextRequest{
		TaskID:   taskID,
		StepName: stepName,
		Context:  context,
	})
}

// ListWorkflows returns the workflows configured for the project rooted at
// projectPath, grouped by kind (tasks, one-off, init).
func (c *Client) ListWorkflows(projectPath string) (*daemon.ListWorkflowsResponse, error) {
	msg, err := c.request(daemon.MsgListWorkflows, daemon.ListWorkflowsRequest{ProjectPath: projectPath})
	if err != nil {
		return nil, err
	}
	var resp daemon.ListWorkflowsResponse
	if err := msg.DecodePayload(&resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func ParseAgentUpdate(msg *daemon.Message) (*daemon.AgentInfo, error) {
	if msg.Type != daemon.MsgAgentUpdate {
		return nil, fmt.Errorf("not an agent update message")
	}

	var resp daemon.AgentUpdateResponse
	if err := json.Unmarshal(msg.Payload, &resp); err != nil {
		return nil, err
	}

	return &resp.Agent, nil
}
