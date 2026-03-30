package claude

import (
	"testing"
)

func TestParseLineAssistantText(t *testing.T) {
	p := NewStreamParser()

	// Real assistant event from Claude 2.1.33 --verbose --output-format stream-json
	line := []byte(`{"type":"assistant","message":{"model":"claude-opus-4-6","id":"msg_01DFMh6KsTwq7cpstP9ZkCc2","type":"message","role":"assistant","content":[{"type":"text","text":"Hello! How can I help you today?"}],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":3,"output_tokens":1}},"parent_tool_use_id":null,"session_id":"a916df4b-5faf-47be-b0d8-2fdc8562bc71","uuid":"4a901e05-1b57-4d76-84fe-e99d83c9aafd"}`)

	lines := p.ParseLine(line)
	if len(lines) == 0 {
		t.Fatal("expected parsed lines from assistant event, got none")
	}

	// Should contain the text
	found := false
	for _, l := range lines {
		if contains(l, "Hello! How can I help you today?") {
			found = true
		}
	}
	if !found {
		t.Error("expected text content line")
	}
}

func TestParseLineSystemInit(t *testing.T) {
	p := NewStreamParser()

	line := []byte(`{"type":"system","subtype":"init","cwd":"/home/test","session_id":"abc","tools":["Bash"],"model":"claude-opus-4-6"}`)
	lines := p.ParseLine(line)
	if len(lines) != 0 {
		t.Errorf("expected no lines from system event, got %d: %v", len(lines), lines)
	}
}

func TestParseLineToolUse(t *testing.T) {
	p := NewStreamParser()

	line := []byte(`{"type":"assistant","message":{"id":"msg_01","role":"assistant","content":[{"type":"tool_use","name":"Bash","input":{"command":"go build ./..."}}]},"session_id":"abc"}`)
	lines := p.ParseLine(line)
	if len(lines) == 0 {
		t.Fatal("expected parsed lines from tool_use event, got none")
	}

	foundTool := false
	foundInput := false
	for _, l := range lines {
		t.Logf("parsed: %s", l)
		if contains(l, "Tool: Bash") {
			foundTool = true
		}
		if contains(l, "go build") {
			foundInput = true
		}
	}
	if !foundTool {
		t.Error("expected 'Tool: Bash' line")
	}
	if !foundInput {
		t.Error("expected tool input line")
	}
}

func TestParseLineResult(t *testing.T) {
	p := NewStreamParser()

	// Real result event - note "result" is a string, not an object
	line := []byte(`{"type":"result","subtype":"success","is_error":false,"duration_ms":3276,"duration_api_ms":3138,"num_turns":1,"result":"Hello!","total_cost_usd":0.15364,"session_id":"abc"}`)
	lines := p.ParseLine(line)

	// Log what we get - currently this may fail due to "result" being a string vs struct
	for _, l := range lines {
		t.Logf("parsed: %s", l)
	}

	if len(lines) == 0 {
		t.Error("expected 'Done' line from result event, got none (result field type mismatch?)")
	}
}

func TestParseLineThinking(t *testing.T) {
	p := NewStreamParser()

	line := []byte(`{"type":"assistant","message":{"id":"msg_02","role":"assistant","content":[{"type":"thinking","thinking":"Let me analyze the codebase structure..."}]},"session_id":"abc"}`)
	lines := p.ParseLine(line)
	if len(lines) == 0 {
		t.Fatal("expected parsed lines from thinking block, got none")
	}

	found := false
	for _, l := range lines {
		t.Logf("parsed: %s", l)
		if contains(l, "Thinking:") {
			found = true
		}
	}
	if !found {
		t.Error("expected 'Thinking:' line")
	}
}

func TestStreamParser_ResultText(t *testing.T) {
	parser := NewStreamParser()

	// Simulate a result event with result text
	resultLine := `{"type":"result","subtype":"success","result":"Here is the final output from Claude.","duration_ms":5000,"total_cost_usd":0.05}`

	lines := parser.ParseLine([]byte(resultLine))
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	got := parser.ResultText()
	want := "Here is the final output from Claude."
	if got != want {
		t.Errorf("ResultText() = %q, want %q", got, want)
	}
}

func TestStreamParser_ResultTextEmpty(t *testing.T) {
	parser := NewStreamParser()

	// No result event parsed yet
	if got := parser.ResultText(); got != "" {
		t.Errorf("ResultText() before any parsing = %q, want empty", got)
	}
}

func TestStreamParserSessionID(t *testing.T) {
	p := NewStreamParser()

	line := []byte(`{"type":"system","subtype":"init","session_id":"test-session-123"}`)
	p.ParseLine(line)

	got := p.SessionID()
	want := "test-session-123"
	if got != want {
		t.Errorf("SessionID() = %q, want %q", got, want)
	}
}

func TestStreamParserSessionID_FirstOnly(t *testing.T) {
	p := NewStreamParser()

	// Feed two system init lines with different session IDs
	first := []byte(`{"type":"system","subtype":"init","session_id":"first-session-id"}`)
	second := []byte(`{"type":"system","subtype":"init","session_id":"second-session-id"}`)

	p.ParseLine(first)
	p.ParseLine(second)

	// Only the first session ID should be captured
	got := p.SessionID()
	want := "first-session-id"
	if got != want {
		t.Errorf("SessionID() = %q, want %q (should capture first only)", got, want)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
