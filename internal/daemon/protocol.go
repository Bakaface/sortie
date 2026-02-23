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
	MsgAgentList      MessageType = "agent_list"
	MsgTaskList     MessageType = "task_list"
	MsgAgentUpdate  MessageType = "agent_update"
	MsgTaskUpdate   MessageType = "task_update"
	MsgOutputChunk  MessageType = "output_chunk"
	MsgError        MessageType = "error"
	MsgOK           MessageType = "ok"
	MsgPing         MessageType = "ping"
	MsgPong         MessageType = "pong"
	MsgShutdown     MessageType = "shutdown"
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
}

type ListTasksRequest struct {
	ProjectID int64 `json:"project_id,omitempty"` // 0 means all projects
}

type CreateTaskRequest struct {
	Description string   `json:"description"`
	Workflow    string   `json:"workflow,omitempty"`
	Priority    string   `json:"priority,omitempty"`
	ProjectPath string   `json:"project_path,omitempty"` // resolved to project_id by daemon
	Images      []string `json:"images,omitempty"`
}

type ContinueTaskRequest struct {
	TaskID int64 `json:"task_id"`
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
	Branch       string     `json:"branch"`
	WorktreePath string     `json:"worktree_path,omitempty"`
	ErrorMessage string     `json:"error_message,omitempty"`
	Context      string     `json:"context,omitempty"`
	Images       []string   `json:"images,omitempty"`
	BlockedBy    []int64    `json:"blocked_by,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
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
	TaskID int64    `json:"task_id"`
	Step   string   `json:"step"`
	Lines  []string `json:"lines"`
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
