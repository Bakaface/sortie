package daemon

import (
	"encoding/json"
	"fmt"
	"time"
)

type MessageType string

const (
	MsgListAgents   MessageType = "list_agents"
	MsgListTasks    MessageType = "list_tasks"
	MsgStartAgent   MessageType = "start_agent"
	MsgStopAgent    MessageType = "stop_agent"
	MsgSubscribe    MessageType = "subscribe"
	MsgUnsubscribe  MessageType = "unsubscribe"
	MsgSendInput    MessageType = "send_input"
	MsgGetOutput    MessageType = "get_output"
	MsgGetTask      MessageType = "get_task"
	MsgRetryTask    MessageType = "retry_task"
	MsgGetLogs      MessageType = "get_logs"
	MsgCreateTask   MessageType = "create_task"
	MsgContinueTask   MessageType = "continue_task"
	MsgFinalizeTask   MessageType = "finalize_task"
	MsgDeleteTask     MessageType = "delete_task"
	MsgUpdatePriority MessageType = "update_priority"
	MsgUpdateField    MessageType = "update_field"
	MsgAgentList      MessageType = "agent_list"
	MsgTaskList     MessageType = "task_list"
	MsgAgentUpdate  MessageType = "agent_update"
	MsgTaskUpdate   MessageType = "task_update"
	MsgOutputChunk  MessageType = "output_chunk"
	MsgError        MessageType = "error"
	MsgOK           MessageType = "ok"
	MsgPing         MessageType = "ping"
	MsgPong         MessageType = "pong"
	MsgShutdown       MessageType = "shutdown"
	MsgTmuxActivity   MessageType = "tmux_activity"
	MsgRevertTask         MessageType = "revert_task"
	MsgUpdateDependency   MessageType = "update_dependency"
	MsgDetachBranch       MessageType = "detach_branch"
	MsgAttachBranch       MessageType = "attach_branch"
	MsgGetStepContexts    MessageType = "get_step_contexts"
)

type Message struct {
	Type    MessageType     `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

func NewMessage(msgType MessageType, payload any) (*Message, error) {
	var raw json.RawMessage
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		raw = data
	}
	return &Message{Type: msgType, Payload: raw}, nil
}

func (m *Message) DecodePayload(v any) error {
	if m.Payload == nil {
		return nil
	}
	return json.Unmarshal(m.Payload, v)
}

type ListAgentsRequest struct{}

type StartAgentRequest struct {
	TaskID int64 `json:"task_id"`
}

type StopAgentRequest struct {
	AgentID string `json:"agent_id"`
}

type SubscribeRequest struct{}

type UnsubscribeRequest struct{}

type SendInputRequest struct {
	AgentID string `json:"agent_id"`
	Input   string `json:"input"`
}

type GetOutputRequest struct {
	AgentID  string `json:"agent_id"`
	FromLine int    `json:"from_line"`
}

type GetTaskRequest struct {
	TaskID int64 `json:"task_id"`
}

type RetryTaskRequest struct {
	TaskID int64 `json:"task_id"`
}

type GetLogsRequest struct {
	TaskID int64  `json:"task_id"`
	Step   string `json:"step"`
	Tail   int    `json:"tail"`
	Offset int    `json:"offset"` // skip first N lines (for incremental loading)
}

type ListTasksRequest struct {
	ProjectID   int64  `json:"project_id,omitempty"`    // 0 means all projects
	ProjectName string `json:"project_name,omitempty"`  // filter by project name (repo basename)
}

type CreateTaskRequest struct {
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description"`
	Workflow    string   `json:"workflow,omitempty"`
	Priority    string   `json:"priority,omitempty"`
	BranchName     string   `json:"branch_name,omitempty"`  // user-provided branch template
	TargetBranch   string   `json:"target_branch,omitempty"`
	CheckoutBranch string   `json:"checkout_branch,omitempty"`
	ProjectPath    string   `json:"project_path,omitempty"` // resolved to project_id by daemon
	Worktree       *bool    `json:"worktree,omitempty"`     // nil means default (true)
	TmuxDirect     bool     `json:"tmux_direct,omitempty"`  // when true, skip workflow and go straight to tmux
	Images      []string `json:"images,omitempty"`
	BlockedBy   []int64  `json:"blocked_by,omitempty"`   // task IDs that block this task
}

type ContinueTaskRequest struct {
	TaskID   int64  `json:"task_id"`
	Workflow string `json:"workflow,omitempty"`
	Prompt   string `json:"prompt,omitempty"`
}

type FinalizeTaskRequest struct {
	TaskID int64 `json:"task_id"`
}

type DeleteTaskRequest struct {
	TaskID int64 `json:"task_id"`
}

type UpdatePriorityRequest struct {
	TaskID   int64  `json:"task_id"`
	Priority string `json:"priority"`
}

type UpdateFieldRequest struct {
	TaskID int64  `json:"task_id"`
	Field  string `json:"field"`
	Value  string `json:"value"`
}

type RevertTaskRequest struct {
	TaskID int64 `json:"task_id"`
}

type UpdateDependencyRequest struct {
	TaskID    int64  `json:"task_id"`
	BlockedBy int64  `json:"blocked_by"`
	Action    string `json:"action"` // "add" or "remove"
}

type DetachBranchRequest struct {
	TaskID int64 `json:"task_id"`
}

type AttachBranchRequest struct {
	TaskID int64 `json:"task_id"`
}

type GetStepContextsRequest struct {
	TaskID int64 `json:"task_id"`
}

type GetStepContextsResponse struct {
	Steps map[string]string `json:"steps"` // step_name -> context
}

type CreateTaskResponse struct {
	Task TaskInfo `json:"task"`
}

type AgentState string

const (
	AgentPending         AgentState = "pending"
	AgentStarting        AgentState = "starting"
	AgentRunning         AgentState = "running"
	AgentWaitingForInput AgentState = "waiting_for_input"
	AgentCompleted       AgentState = "completed"
	AgentFailed          AgentState = "failed"
	AgentStopped         AgentState = "stopped"
)

type AgentInfo struct {
	ID          string     `json:"id"`
	TaskID      int64      `json:"task_id"`
	Description string     `json:"description"`
	WorkDir     string     `json:"work_dir"`
	State       AgentState `json:"state"`
	StartedAt   time.Time  `json:"started_at"`
	Error       string     `json:"error,omitempty"`
}

type TaskInfo struct {
	ID           int64      `json:"id"`
	ProjectID    int64      `json:"project_id"`
	ProjectName  string     `json:"project_name,omitempty"`
	ProjectPath  string     `json:"project_path,omitempty"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	Slug         string     `json:"slug"`
	Workflow     string     `json:"workflow,omitempty"`
	Status       string     `json:"status"`
	Priority     string     `json:"priority"`
	StepIndex     int        `json:"step_index"`
	CurrentStep   string     `json:"current_step"`
	LoopIteration int        `json:"loop_iteration,omitempty"`
	BranchName     string     `json:"branch_name,omitempty"`
	Branch         string     `json:"branch"`
	TargetBranch   string     `json:"target_branch,omitempty"`
	CheckoutBranch string     `json:"checkout_branch,omitempty"`
	Worktree         bool       `json:"worktree"`
	WorktreePath     string     `json:"worktree_path,omitempty"`
	WorktreeDetached bool       `json:"worktree_detached,omitempty"`
	ErrorMessage string     `json:"error_message,omitempty"`
	Context      string     `json:"context,omitempty"`
	Images       []string   `json:"images,omitempty"`
	Commits      []string   `json:"commits,omitempty"`
	BlockedBy    []int64    `json:"blocked_by,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	TmuxActivity string     `json:"tmux_activity,omitempty"`
}

type TaskListResponse struct {
	Tasks []TaskInfo `json:"tasks"`
}

type TaskUpdateResponse struct {
	Task TaskInfo `json:"task"`
}

type AgentListResponse struct {
	Agents []AgentInfo `json:"agents"`
}

type AgentUpdateResponse struct {
	Agent AgentInfo `json:"agent"`
}

type OutputChunkResponse struct {
	AgentID    string   `json:"agent_id"`
	Lines      []string `json:"lines"`
	TotalLines int      `json:"total_lines"`
}

type GetTaskResponse struct {
	Task TaskInfo `json:"task"`
}

type GetLogsResponse struct {
	TaskID     int64    `json:"task_id"`
	Step       string   `json:"step"`
	Lines      []string `json:"lines"`
	TotalLines int      `json:"total_lines"` // total line count before offset/tail
}

type TmuxActivityResponse struct {
	TaskID   int64  `json:"task_id"`
	Activity string `json:"activity"`
}

type ErrorResponse struct {
	Message string `json:"message"`
}

type OKResponse struct {
	Message string `json:"message,omitempty"`
}

func EncodeMessage(msg *Message) ([]byte, error) {
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func DecodeMessage(data []byte) (*Message, error) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("failed to decode message: %w", err)
	}
	return &msg, nil
}
