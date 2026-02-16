package claude

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// StreamParser parses Claude's --verbose --output-format stream-json NDJSON events
// into human-readable log lines for display in logs and the TUI.
//
// The verbose stream-json format emits one complete content block per line:
//   - {"type":"system","subtype":"init",...} — session initialization
//   - {"type":"assistant","message":{"content":[{one block}]}} — assistant output
//   - {"type":"user","message":{"content":[...]}} — tool results
//   - {"type":"result",...} — final result summary
type StreamParser struct {
	lastMsgID string // track message ID to detect new turns
}

// streamEvent represents a top-level NDJSON event from Claude's verbose stream-json output.
// Note: the "result" JSON key has different types per event (string for result events,
// absent for others), so we use json.RawMessage and decode result events separately.
type streamEvent struct {
	Type    string     `json:"type"`    // "system", "assistant", "user", "result"
	Subtype string     `json:"subtype"` // for system events: "init"
	Message *streamMsg `json:"message,omitempty"`
}

// resultEvent is decoded separately for "result" type events because the top-level
// "result" key is a string (the final text), which conflicts with reusing streamEvent.
type resultEvent struct {
	DurationMs   float64 `json:"duration_ms"`
	TotalCostUSD float64 `json:"total_cost_usd"`
}

type streamMsg struct {
	ID      string         `json:"id"`
	Role    string         `json:"role"`
	Content []contentBlock `json:"content"`
}

type contentBlock struct {
	Type     string          `json:"type"`  // "text", "tool_use", "thinking", "tool_result"
	Text     string          `json:"text"`  // for text blocks
	Thinking string          `json:"thinking"` // for thinking blocks
	Name     string          `json:"name"`  // tool name for tool_use blocks
	Input    json.RawMessage `json:"input"` // raw JSON for tool_use input
	Content  string          `json:"content"` // for tool_result blocks
}

func NewStreamParser() *StreamParser {
	return &StreamParser{}
}

// ParseLine processes one NDJSON line and returns zero or more formatted log lines.
func (p *StreamParser) ParseLine(line []byte) []string {
	line = trimBOM(line)
	if len(line) == 0 {
		return nil
	}

	// First pass: extract event type
	var ev streamEvent
	if err := json.Unmarshal(line, &ev); err != nil {
		return nil
	}

	ts := timestamp()

	switch ev.Type {
	case "assistant":
		return p.parseAssistant(&ev, ts)

	case "result":
		// Decode result events separately because the "result" key is a string
		// (the final text output), not an object — which would break streamEvent unmarshal.
		var res resultEvent
		if err := json.Unmarshal(line, &res); err == nil {
			return []string{fmt.Sprintf("[%s] Done (%.1fs, $%.4f)", ts, res.DurationMs/1000, res.TotalCostUSD)}
		}
	}

	return nil
}

// parseAssistant extracts human-readable lines from an assistant event.
// Each event contains one complete content block.
func (p *StreamParser) parseAssistant(ev *streamEvent, ts string) []string {
	if ev.Message == nil {
		return nil
	}

	var lines []string

	// Detect new assistant turn
	if ev.Message.ID != p.lastMsgID {
		p.lastMsgID = ev.Message.ID
		lines = append(lines, fmt.Sprintf("[%s] --- Assistant turn ---", ts))
	}

	for _, block := range ev.Message.Content {
		switch block.Type {
		case "text":
			text := strings.TrimSpace(block.Text)
			if text != "" {
				for _, l := range strings.Split(text, "\n") {
					l = strings.TrimRight(l, " \t")
					if l != "" {
						lines = append(lines, fmt.Sprintf("[%s] %s", ts, l))
					}
				}
			}

		case "tool_use":
			lines = append(lines, fmt.Sprintf("[%s] Tool: %s", ts, block.Name))
			inputStr := string(block.Input)
			summary := summarizeToolInput(block.Name, inputStr)
			if summary != "" {
				lines = append(lines, fmt.Sprintf("[%s]   Input: %s", ts, summary))
			}

		case "thinking":
			text := strings.TrimSpace(block.Thinking)
			if text != "" {
				lines = append(lines, fmt.Sprintf("[%s] Thinking: %s", ts, truncate(text, 200)))
			}
		}
	}

	return lines
}

// summarizeToolInput extracts the most useful field from a tool's JSON input.
func summarizeToolInput(toolName, rawJSON string) string {
	if rawJSON == "" {
		return ""
	}

	var input map[string]any
	if err := json.Unmarshal([]byte(rawJSON), &input); err != nil {
		return truncate(rawJSON, 200)
	}

	switch toolName {
	case "Bash":
		if cmd, ok := input["command"].(string); ok {
			return truncate(cmd, 200)
		}
	case "Read":
		if fp, ok := input["file_path"].(string); ok {
			return fp
		}
	case "Edit":
		if fp, ok := input["file_path"].(string); ok {
			return fp
		}
	case "Write":
		if fp, ok := input["file_path"].(string); ok {
			return fp
		}
	case "Grep":
		if pat, ok := input["pattern"].(string); ok {
			return fmt.Sprintf("pattern=%s", truncate(pat, 150))
		}
	case "Glob":
		if pat, ok := input["pattern"].(string); ok {
			return fmt.Sprintf("pattern=%s", truncate(pat, 150))
		}
	case "Task":
		if desc, ok := input["description"].(string); ok {
			return truncate(desc, 200)
		}
	}

	return truncate(rawJSON, 200)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func timestamp() string {
	return time.Now().Format("15:04:05")
}

// trimBOM removes a UTF-8 BOM if present.
func trimBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	return b
}
