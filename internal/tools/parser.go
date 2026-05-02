package tools

import (
	"encoding/json"
	"strings"
)

// Call represents a structured tool invocation
type Call struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ParserState tracks where we are in parsing a tool call
type ParserState int

const (
	StateText ParserState = iota
	StateBracket1
	StateBracket2
	StateTool
	StateContent
)

// StreamParser detects and extracts tool calls from a token stream without full buffering.
// It uses a state machine to find [[tool:...]] blocks and emits them as they complete.
type StreamParser struct {
	state      ParserState
	buffer     strings.Builder
	bracketPos int // count of '[' or ']' seen
	toolStart  int  // position where [[tool: started
}

// NewStreamParser creates a parser for the streaming inference loop
func NewStreamParser() *StreamParser {
	return &StreamParser{state: StateText}
}

// Feed processes a chunk of text and returns any complete tool calls found
func (p *StreamParser) Feed(chunk string) []Call {
	var calls []Call

	for _, ch := range chunk {
		p.buffer.WriteRune(ch)
		text := p.buffer.String()

		// State machine: search for [[tool: ... ]]
		switch p.state {
		case StateText:
			// Look for first [
			if ch == '[' {
				p.state = StateBracket1
				p.bracketPos = 1
			}

		case StateBracket1:
			if ch == '[' {
				// Found [[, check for "tool:"
				p.state = StateBracket2
				p.bracketPos = 2
			} else if ch == ']' {
				// Found ], reset bracket position
				p.bracketPos = 1
			} else {
				// False alarm, go back to searching
				p.state = StateText
				p.bracketPos = 0
			}

		case StateBracket2:
			// We have [[, now look for tool:
			if strings.HasSuffix(text, "[[tool:") {
				p.state = StateTool
				p.toolStart = len(text) - 7 // position of [[tool:
				p.buffer.Reset()
			} else if strings.HasSuffix(text, "]") {
				// False alarm, backtrack
				p.state = StateText
			}

		case StateTool:
			// Inside a tool call, look for ]]
			if strings.Contains(text, "]]") {
				idx := strings.Index(text, "]]")
				toolContent := text[:idx]

				// Parse the tool call
				call := p.parseToolCall(toolContent)
				if call != nil {
					calls = append(calls, *call)
				}

				// After ]], reset and look for more
				p.state = StateText
				p.buffer.Reset()
			}
		}
	}

	return calls
}

// parseToolCall extracts name and arguments from "tool_name {...json...}"
func (p *StreamParser) parseToolCall(content string) *Call {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}

	// Find the first { to split name from arguments
	braceIdx := strings.Index(content, "{")
	var name string
	var argsText string

	if braceIdx > 0 {
		name = strings.TrimSpace(content[:braceIdx])
		argsText = strings.TrimSpace(content[braceIdx:])
	} else {
		// No arguments, just the tool name
		name = content
		argsText = "{}"
	}

	// Validate name is alphanumeric + underscores
	for _, ch := range name {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_') {
			return nil
		}
	}

	// Try to parse arguments as JSON
	var args json.RawMessage
	if argsText == "{}" || argsText == "" {
		args = json.RawMessage("{}")
	} else {
		// Validate it's valid JSON
		if !json.Valid([]byte(argsText)) {
			return nil
		}
		args = json.RawMessage(argsText)
	}

	return &Call{
		Name:      name,
		Arguments: args,
	}
}

// Flush returns any buffered content (for end-of-stream cleanup)
func (p *StreamParser) Flush() string {
	defer p.buffer.Reset()
	return p.buffer.String()
}
