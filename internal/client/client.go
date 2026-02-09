package client

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/aface/ralph-tamer-kit/internal/config"
	"github.com/aface/ralph-tamer-kit/internal/daemon"
)

type Client struct {
	cfg  *config.Config
	conn net.Conn
	mu   sync.Mutex

	msgChan chan *daemon.Message
	errChan chan error
	done    chan struct{}
}

func New(cfg *config.Config) *Client {
	return &Client{
		cfg:     cfg,
		msgChan: make(chan *daemon.Message, 100),
		errChan: make(chan error, 1),
		done:    make(chan struct{}),
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
	close(c.done)
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *Client) readLoop() {
	scanner := bufio.NewScanner(c.conn)
	for scanner.Scan() {
		msg, err := daemon.DecodeMessage(scanner.Bytes())
		if err != nil {
			log.Printf("IPC decode error: %v", err)
			continue
		}

		select {
		case c.msgChan <- msg:
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
	return c.msgChan
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
	if err := c.send(msgType, payload); err != nil {
		return nil, err
	}

	select {
	case msg := <-c.msgChan:
		return msg, nil
	case err := <-c.errChan:
		return nil, err
	case <-c.done:
		return nil, fmt.Errorf("client closed")
	}
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
	msg, err := c.sendAndWait(daemon.MsgListAgents, nil)
	if err != nil {
		return nil, err
	}

	if msg.Type == daemon.MsgError {
		var errResp daemon.ErrorResponse
		msg.DecodePayload(&errResp)
		return nil, fmt.Errorf("%s", errResp.Message)
	}

	var resp daemon.AgentListResponse
	if err := msg.DecodePayload(&resp); err != nil {
		return nil, err
	}

	return resp.Agents, nil
}

func (c *Client) ListTasks() ([]daemon.TaskInfo, error) {
	msg, err := c.sendAndWait(daemon.MsgListTasks, nil)
	if err != nil {
		return nil, err
	}

	if msg.Type == daemon.MsgError {
		var errResp daemon.ErrorResponse
		msg.DecodePayload(&errResp)
		return nil, fmt.Errorf("%s", errResp.Message)
	}

	var resp daemon.TaskListResponse
	if err := msg.DecodePayload(&resp); err != nil {
		return nil, err
	}

	return resp.Tasks, nil
}

func (c *Client) StartAgent(taskID int64) error {
	msg, err := c.sendAndWait(daemon.MsgStartAgent, daemon.StartAgentRequest{TaskID: taskID})
	if err != nil {
		return err
	}

	if msg.Type == daemon.MsgError {
		var errResp daemon.ErrorResponse
		msg.DecodePayload(&errResp)
		return fmt.Errorf("%s", errResp.Message)
	}

	return nil
}

func (c *Client) StopAgent(agentID string) error {
	msg, err := c.sendAndWait(daemon.MsgStopAgent, daemon.StopAgentRequest{AgentID: agentID})
	if err != nil {
		return err
	}

	if msg.Type == daemon.MsgError {
		var errResp daemon.ErrorResponse
		msg.DecodePayload(&errResp)
		return fmt.Errorf("%s", errResp.Message)
	}

	return nil
}

func (c *Client) Subscribe() error {
	return c.send(daemon.MsgSubscribe, nil)
}

func (c *Client) Unsubscribe() error {
	return c.send(daemon.MsgUnsubscribe, nil)
}

func (c *Client) GetOutput(agentID string, fromLine int) ([]string, int, error) {
	msg, err := c.sendAndWait(daemon.MsgGetOutput, daemon.GetOutputRequest{
		AgentID:  agentID,
		FromLine: fromLine,
	})
	if err != nil {
		return nil, 0, err
	}

	if msg.Type == daemon.MsgError {
		var errResp daemon.ErrorResponse
		msg.DecodePayload(&errResp)
		return nil, 0, fmt.Errorf("%s", errResp.Message)
	}

	var resp daemon.OutputChunkResponse
	if err := msg.DecodePayload(&resp); err != nil {
		return nil, 0, err
	}

	return resp.Lines, resp.TotalLines, nil
}

func (c *Client) SendInput(agentID, input string) error {
	msg, err := c.sendAndWait(daemon.MsgSendInput, daemon.SendInputRequest{
		AgentID: agentID,
		Input:   input,
	})
	if err != nil {
		return err
	}

	if msg.Type == daemon.MsgError {
		var errResp daemon.ErrorResponse
		msg.DecodePayload(&errResp)
		return fmt.Errorf("%s", errResp.Message)
	}

	return nil
}

func (c *Client) GetTask(id int64) (*daemon.TaskInfo, error) {
	msg, err := c.sendAndWait(daemon.MsgGetTask, daemon.GetTaskRequest{TaskID: id})
	if err != nil {
		return nil, err
	}

	if msg.Type == daemon.MsgError {
		var errResp daemon.ErrorResponse
		msg.DecodePayload(&errResp)
		return nil, fmt.Errorf("%s", errResp.Message)
	}

	var resp daemon.GetTaskResponse
	if err := msg.DecodePayload(&resp); err != nil {
		return nil, err
	}

	return &resp.Task, nil
}

func (c *Client) ApproveTask(id int64) error {
	msg, err := c.sendAndWait(daemon.MsgApproveTask, daemon.ApproveTaskRequest{TaskID: id})
	if err != nil {
		return err
	}

	if msg.Type == daemon.MsgError {
		var errResp daemon.ErrorResponse
		msg.DecodePayload(&errResp)
		return fmt.Errorf("%s", errResp.Message)
	}

	return nil
}

func (c *Client) RejectTask(id int64) error {
	msg, err := c.sendAndWait(daemon.MsgRejectTask, daemon.RejectTaskRequest{TaskID: id})
	if err != nil {
		return err
	}

	if msg.Type == daemon.MsgError {
		var errResp daemon.ErrorResponse
		msg.DecodePayload(&errResp)
		return fmt.Errorf("%s", errResp.Message)
	}

	return nil
}

func (c *Client) RetryTask(id int64) error {
	msg, err := c.sendAndWait(daemon.MsgRetryTask, daemon.RetryTaskRequest{TaskID: id})
	if err != nil {
		return err
	}

	if msg.Type == daemon.MsgError {
		var errResp daemon.ErrorResponse
		msg.DecodePayload(&errResp)
		return fmt.Errorf("%s", errResp.Message)
	}

	return nil
}

func (c *Client) GetLogs(id int64, step string, tail int) ([]string, error) {
	msg, err := c.sendAndWait(daemon.MsgGetLogs, daemon.GetLogsRequest{
		TaskID: id,
		Step:   step,
		Tail:   tail,
	})
	if err != nil {
		return nil, err
	}

	if msg.Type == daemon.MsgError {
		var errResp daemon.ErrorResponse
		msg.DecodePayload(&errResp)
		return nil, fmt.Errorf("%s", errResp.Message)
	}

	var resp daemon.GetLogsResponse
	if err := msg.DecodePayload(&resp); err != nil {
		return nil, err
	}

	return resp.Lines, nil
}

func (c *Client) StopTask(id int64) error {
	// Agent IDs are formatted as task ID in string form
	agentID := fmt.Sprintf("%d", id)
	return c.StopAgent(agentID)
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
