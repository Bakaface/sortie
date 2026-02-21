package client

import (
	"bufio"
	"encoding/json"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/aface/ralph-tamer-kit/internal/daemon"
)

// mockServer simulates a daemon that reads requests and sends back typed responses.
// It processes requests sequentially (like the real server).
func mockServer(t *testing.T, listener net.Listener, handler func(msg *daemon.Message) *daemon.Message) {
	t.Helper()

	conn, err := listener.Accept()
	if err != nil {
		return
	}
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		msg, err := daemon.DecodeMessage(scanner.Bytes())
		if err != nil {
			t.Logf("mock server decode error: %v", err)
			continue
		}

		resp := handler(msg)
		data, err := daemon.EncodeMessage(resp)
		if err != nil {
			t.Logf("mock server encode error: %v", err)
			continue
		}
		conn.Write(data)
	}
}

func TestSendAndWait_ConcurrentCallsGetCorrectResponses(t *testing.T) {
	// Create a Unix socket pair for testing
	listener, err := net.Listen("unix", t.TempDir()+"/test.sock")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	defer listener.Close()

	// Start mock server that responds with a message type matching the request:
	// - list_tasks -> task_list with empty tasks
	// - approve_task -> ok
	// - ping -> pong
	go mockServer(t, listener, func(msg *daemon.Message) *daemon.Message {
		switch msg.Type {
		case daemon.MsgListTasks:
			// Simulate some processing time to widen the race window
			time.Sleep(10 * time.Millisecond)
			resp, _ := daemon.NewMessage(daemon.MsgTaskList, daemon.TaskListResponse{Tasks: []daemon.TaskInfo{}})
			return resp
		case daemon.MsgApproveTask:
			resp, _ := daemon.NewMessage(daemon.MsgOK, daemon.OKResponse{Message: "task approved and resumed"})
			return resp
		case daemon.MsgPing:
			resp, _ := daemon.NewMessage(daemon.MsgPong, nil)
			return resp
		default:
			resp, _ := daemon.NewMessage(daemon.MsgError, daemon.ErrorResponse{Message: "unknown"})
			return resp
		}
	})

	// Connect client
	conn, err := net.Dial("unix", listener.Addr().String())
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	c := &Client{
		conn:     conn,
		respChan: make(chan *daemon.Message, 100),
		subChan:  make(chan *daemon.Message, 100),
		errChan:  make(chan error, 1),
		done:     make(chan struct{}),
	}
	go c.readLoop()
	defer c.Close()

	// Run multiple concurrent sendAndWait calls and verify each gets the correct response type.
	// Before the fix, concurrent calls could receive each other's responses.
	const iterations = 20
	var wg sync.WaitGroup

	errCh := make(chan string, iterations*2)

	for range iterations {
		wg.Add(2)

		// Goroutine 1: ListTasks expects MsgTaskList response
		go func() {
			defer wg.Done()
			msg, err := c.sendAndWait(daemon.MsgListTasks, nil)
			if err != nil {
				errCh <- "ListTasks error: " + err.Error()
				return
			}
			if msg.Type != daemon.MsgTaskList {
				errCh <- "ListTasks got wrong response type: " + string(msg.Type)
			}
		}()

		// Goroutine 2: ApproveTask expects MsgOK response
		go func() {
			defer wg.Done()
			msg, err := c.sendAndWait(daemon.MsgApproveTask, daemon.ApproveTaskRequest{TaskID: 1})
			if err != nil {
				errCh <- "ApproveTask error: " + err.Error()
				return
			}
			if msg.Type != daemon.MsgOK {
				errCh <- "ApproveTask got wrong response type: " + string(msg.Type)
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for errMsg := range errCh {
		t.Error(errMsg)
	}
}

func TestSendAndWait_BroadcastsNotRoutedToResponses(t *testing.T) {
	// Verify that broadcast messages (task_update, agent_update) go to subChan,
	// not respChan, so they don't interfere with sendAndWait.
	listener, err := net.Listen("unix", t.TempDir()+"/test.sock")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	defer listener.Close()

	go mockServer(t, listener, func(msg *daemon.Message) *daemon.Message {
		if msg.Type == daemon.MsgPing {
			resp, _ := daemon.NewMessage(daemon.MsgPong, nil)
			return resp
		}
		resp, _ := daemon.NewMessage(daemon.MsgError, daemon.ErrorResponse{Message: "unknown"})
		return resp
	})

	conn, err := net.Dial("unix", listener.Addr().String())
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	c := &Client{
		conn:     conn,
		respChan: make(chan *daemon.Message, 100),
		subChan:  make(chan *daemon.Message, 100),
		errChan:  make(chan error, 1),
		done:     make(chan struct{}),
	}
	go c.readLoop()
	defer c.Close()

	// Inject a broadcast message directly into the connection to simulate
	// a server-pushed update arriving while a request is in flight.
	// We can't easily do this with a simple mock, so instead verify the
	// isBroadcast routing logic.
	broadcastTypes := []daemon.MessageType{daemon.MsgAgentUpdate, daemon.MsgTaskUpdate}
	for _, bt := range broadcastTypes {
		if !isBroadcast(bt) {
			t.Errorf("expected %s to be classified as broadcast", bt)
		}
	}

	nonBroadcastTypes := []daemon.MessageType{daemon.MsgOK, daemon.MsgError, daemon.MsgTaskList, daemon.MsgPong}
	for _, nbt := range nonBroadcastTypes {
		if isBroadcast(nbt) {
			t.Errorf("expected %s to NOT be classified as broadcast", nbt)
		}
	}

	// Verify a normal ping-pong still works
	msg, err := c.sendAndWait(daemon.MsgPing, nil)
	if err != nil {
		t.Fatalf("ping failed: %v", err)
	}
	if msg.Type != daemon.MsgPong {
		t.Errorf("expected pong, got %s", msg.Type)
	}
}

func TestSend_FireAndForget(t *testing.T) {
	// Verify that send() (used by Unsubscribe) works independently of sendAndWait.
	listener, err := net.Listen("unix", t.TempDir()+"/test.sock")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	defer listener.Close()

	var receivedMsg daemon.MessageType
	var serverDone sync.WaitGroup
	serverDone.Add(1)
	go func() {
		defer serverDone.Done()
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		scanner := bufio.NewScanner(conn)
		if scanner.Scan() {
			msg, _ := daemon.DecodeMessage(scanner.Bytes())
			if msg != nil {
				receivedMsg = msg.Type
			}
		}
	}()

	conn, err := net.Dial("unix", listener.Addr().String())
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	c := &Client{
		conn:     conn,
		respChan: make(chan *daemon.Message, 100),
		subChan:  make(chan *daemon.Message, 100),
		errChan:  make(chan error, 1),
		done:     make(chan struct{}),
	}
	defer c.Close()

	err = c.send(daemon.MsgUnsubscribe, nil)
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}

	conn.Close()
	serverDone.Wait()

	if receivedMsg != daemon.MsgUnsubscribe {
		t.Errorf("expected server to receive unsubscribe, got %s", receivedMsg)
	}
}

func TestContinueTaskRequest_JSONEncoding(t *testing.T) {
	req := daemon.ContinueTaskRequest{TaskID: 42}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded daemon.ContinueTaskRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.TaskID != 42 {
		t.Errorf("expected task_id=42, got %d", decoded.TaskID)
	}
}

// Verify JSON encoding consistency (regression test for the original bug scenario).
func TestApproveTaskRequest_JSONEncoding(t *testing.T) {
	req := daemon.ApproveTaskRequest{TaskID: 26}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded daemon.ApproveTaskRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.TaskID != 26 {
		t.Errorf("expected task_id=26, got %d", decoded.TaskID)
	}
}
