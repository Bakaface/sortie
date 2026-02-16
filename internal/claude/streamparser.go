package claude

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// StreamParser parses Claude's --output-format stream-json NDJSON events
// into human-readable log lines for display in logs and the TUI.
type StreamParser struct {
	// State tracking for the current content block
	blockType  string // "text", "tool_use", "thinking"
	toolName   string
	inputJSON  strings.Builder
	textAccum  strings.Builder
	thinkAccum strings.Builder
}

// streamEvent represents a top-level NDJSON event from Claude's stream-json output.
type streamEvent struct {
	Type         string        `json:"type"`
	Message      *streamMsg    `json:"message,omitempty"`
	Index        int           `json:"index"`
	ContentBlock *contentBlock `json:"content_block,omitempty"`
	Delta        *streamDelta  `json:"delta,omitempty"`
}

type streamMsg struct {
	Role string `json:"role"`
}

type contentBlock struct {
	Type string `json:"type"` // "text", "tool_use", "thinking"
	Name string `json:"name"` // tool name for tool_use blocks
}

type streamDelta struct {
	Type      string `json:"type"`
	Text      string `json:"text"`
	Thinking  string `json:"thinking"`
	PartialJSON string `json:"partial_json"`
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

	var ev streamEvent
	if err := json.Unmarshal(line, &ev); err != nil {
		return nil
	}

	ts := timestamp()

	switch ev.Type {
	case "message_start":
		return []string{fmt.Sprintf("[%s] --- Assistant turn ---", ts)}

	case "content_block_start":
		p.resetBlock()
		if ev.ContentBlock != nil {
			p.blockType = ev.ContentBlock.Type
			if ev.ContentBlock.Type == "tool_use" {
				p.toolName = ev.ContentBlock.Name
				return []string{fmt.Sprintf("[%s] Tool: %s", ts, p.toolName)}
			}
		}

	case "content_block_delta":
		if ev.Delta != nil {
			switch {
			case ev.Delta.Text != "":
				p.textAccum.WriteString(ev.Delta.Text)
			case ev.Delta.Thinking != "":
				p.thinkAccum.WriteString(ev.Delta.Thinking)
			case ev.Delta.PartialJSON != "":
				p.inputJSON.WriteString(ev.Delta.PartialJSON)
			}
		}

	case "content_block_stop":
		return p.flushBlock(ts)

	case "result":
		// Tool result events
		if ev.Delta != nil && ev.Delta.Text != "" {
			return []string{fmt.Sprintf("[%s] Result: %s", ts, truncate(ev.Delta.Text, 200))}
		}
	}

	return nil
}

// flushBlock emits formatted lines when a content block finishes.
func (p *StreamParser) flushBlock(ts string) []string {
	var lines []string

	switch p.blockType {
	case "tool_use":
		summary := summarizeToolInput(p.toolName, p.inputJSON.String())
		if summary != "" {
			lines = append(lines, fmt.Sprintf("[%s]   Input: %s", ts, summary))
		}

	case "text":
		text := strings.TrimSpace(p.textAccum.String())
		if text != "" {
			// Emit each line of text output with timestamp
			for _, l := range strings.Split(text, "\n") {
				l = strings.TrimRight(l, " \t")
				if l != "" {
					lines = append(lines, fmt.Sprintf("[%s] %s", ts, l))
				}
			}
		}

	case "thinking":
		text := strings.TrimSpace(p.thinkAccum.String())
		if text != "" {
			lines = append(lines, fmt.Sprintf("[%s] Thinking: %s", ts, truncate(text, 200)))
		}
	}

	p.resetBlock()
	return lines
}

func (p *StreamParser) resetBlock() {
	p.blockType = ""
	p.toolName = ""
	p.inputJSON.Reset()
	p.textAccum.Reset()
	p.thinkAccum.Reset()
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
