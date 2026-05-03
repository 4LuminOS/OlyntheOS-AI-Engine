package agent

import (
	"encoding/json"
	"strings"
)

// ─── Parsed tool call ─────────────────────────────────────────────────────────

// ParsedToolCall is a structured tool call extracted from the model output.
// The model writes raw JSON inside <tool_call>…</tool_call> tags;
// this struct is what you get after parsing that JSON.
type ParsedToolCall struct {
	// Name is the tool identifier — must match a registered tool in
	// internal/tools/executor.go (e.g. "fs_read", "plasma_set_theme").
	Name string `json:"name"`

	// Arguments is the key→value map of parameters.
	// Values are always strings at this layer; tools cast as needed.
	Arguments map[string]string `json:"arguments"`
}

// ─── Parse result ─────────────────────────────────────────────────────────────

// ParseKind tells the agent loop what the parser found.
type ParseKind int

const (
	// ParseKindText means this is a normal text token — forward to UI.
	ParseKindText ParseKind = iota

	// ParseKindToolCall means a complete tool call was extracted.
	// result.ToolCall is populated; result.Text is empty.
	ParseKindToolCall
)

// ParseResult is returned by StreamParser.Feed() for every token.
type ParseResult struct {
	Kind     ParseKind
	Text     string          // set when Kind == ParseKindText
	ToolCall *ParsedToolCall // set when Kind == ParseKindToolCall
}

// ─── Parser states ────────────────────────────────────────────────────────────

type parserState int

const (
	// stateText — reading normal assistant text, forwarding to UI.
	stateText parserState = iota

	// stateMaybeOpen — we've seen a '<', accumulating chars to check
	// if this is the opening <tool_call> tag.
	stateMaybeOpen

	// stateInsideCall — inside <tool_call>…</tool_call>, accumulating JSON.
	stateInsideCall

	// stateMaybeClose — inside a call, seen '<', checking for </tool_call>.
	stateMaybeClose
)

const (
	openTag  = "<tool_call>"
	closeTag = "</tool_call>"
)

// ─── StreamParser ─────────────────────────────────────────────────────────────

// StreamParser is a zero-allocation state machine that watches the token
// stream from the model and detects embedded tool calls.
//
// The model streams text like:
//
//	"I'll change your wallpaper now.\n<tool_call>\n{\"name\":\"plasma_set_theme\",\"arguments\":{\"theme\":\"BreezeDark\"}}\n</tool_call>"
//
// The parser:
//   - Forwards all text tokens before <tool_call> to the UI in real time.
//   - Buffers everything between <tool_call> and </tool_call>.
//   - Parses the buffered JSON and returns a ParseKindToolCall result.
//
// Usage:
//
//	p := NewStreamParser()
//	for token := range tokenChannel {
//	    result := p.Feed(token)
//	    switch result.Kind {
//	    case ParseKindText:     sendToUI(result.Text)
//	    case ParseKindToolCall: executeToolCall(result.ToolCall)
//	    }
//	}
type StreamParser struct {
	state   parserState
	tagBuf  strings.Builder // accumulates chars while matching a tag
	callBuf strings.Builder // accumulates JSON between the tags
}

// NewStreamParser creates a fresh parser. Create one per agent iteration.
func NewStreamParser() *StreamParser {
	return &StreamParser{}
}

// Reset clears all state so the parser can be reused for the next iteration.
func (p *StreamParser) Reset() {
	p.state = stateText
	p.tagBuf.Reset()
	p.callBuf.Reset()
}

// Feed processes one token from the model output stream.
// It returns a ParseResult telling the caller what to do with it.
//
// This is called for EVERY token — it must be fast.
func (p *StreamParser) Feed(token string) ParseResult {
	// Process the token character by character so we can detect
	// multi-character tag boundaries that might straddle token edges.
	for _, ch := range token {
		c := string(ch)
		result := p.feedChar(c)
		if result != nil {
			return *result
		}
	}
	// Default: if we're in text state, return the token as-is for efficiency.
	if p.state == stateText {
		return ParseResult{Kind: ParseKindText, Text: token}
	}
	// We're mid-tag or mid-call — nothing to emit yet.
	return ParseResult{Kind: ParseKindText, Text: ""}
}

// feedChar handles a single character and returns a result if one is ready,
// or nil if we're still accumulating.
func (p *StreamParser) feedChar(c string) *ParseResult {
	switch p.state {

	// ── Normal text output ────────────────────────────────────────────────────
	case stateText:
		if c == "<" {
			// Might be the start of <tool_call> — switch to probe mode.
			p.state = stateMaybeOpen
			p.tagBuf.Reset()
			p.tagBuf.WriteString(c)
			return nil
		}
		// Plain text — emit immediately.
		return &ParseResult{Kind: ParseKindText, Text: c}

	// ── Probing for <tool_call> ───────────────────────────────────────────────
	case stateMaybeOpen:
		p.tagBuf.WriteString(c)
		accumulated := p.tagBuf.String()

		if accumulated == openTag {
			// Confirmed — we're inside a tool call block.
			p.state = stateInsideCall
			p.callBuf.Reset()
			p.tagBuf.Reset()
			return nil
		}

		if !strings.HasPrefix(openTag, accumulated) {
			// This is NOT a tool_call tag — flush tagBuf as text and go back.
			flushed := accumulated
			p.state = stateText
			p.tagBuf.Reset()
			return &ParseResult{Kind: ParseKindText, Text: flushed}
		}

		// Still a valid prefix — keep probing.
		return nil

	// ── Accumulating JSON inside <tool_call>…</tool_call> ────────────────────
	case stateInsideCall:
		if c == "<" {
			// Might be the closing </tool_call> tag.
			p.state = stateMaybeClose
			p.tagBuf.Reset()
			p.tagBuf.WriteString(c)
			return nil
		}
		p.callBuf.WriteString(c)
		return nil

	// ── Probing for </tool_call> ──────────────────────────────────────────────
	case stateMaybeClose:
		p.tagBuf.WriteString(c)
		accumulated := p.tagBuf.String()

		if accumulated == closeTag {
			// Full closing tag found — parse the buffered JSON.
			p.state = stateText
			raw := strings.TrimSpace(p.callBuf.String())
			p.callBuf.Reset()
			p.tagBuf.Reset()

			call, err := parseToolCallJSON(raw)
			if err != nil {
				// Malformed JSON — treat as text so the model can try again.
				return &ParseResult{
					Kind: ParseKindText,
					Text: openTag + raw + closeTag, // show raw to UI for debugging
				}
			}
			return &ParseResult{Kind: ParseKindToolCall, ToolCall: call}
		}

		if !strings.HasPrefix(closeTag, accumulated) {
			// Not a close tag — flush back into the call buffer as content.
			p.callBuf.WriteString(accumulated)
			p.state = stateInsideCall
			p.tagBuf.Reset()
			return nil
		}

		// Still a valid prefix of </tool_call> — keep probing.
		return nil
	}

	return nil
}

// ─── JSON parsing ─────────────────────────────────────────────────────────────

// rawToolCall is the JSON structure the model outputs inside <tool_call> tags.
// We support two common formats that different models use.
type rawToolCall struct {
	Name      string `json:"name"`

	// Format 1 (preferred): {"name":"fs_read","arguments":{"path":"/tmp"}}
	Arguments map[string]json.RawMessage `json:"arguments"`

	// Format 2 (some models): {"name":"fs_read","parameters":{"path":"/tmp"}}
	Parameters map[string]json.RawMessage `json:"parameters"`
}

// parseToolCallJSON parses the JSON block between <tool_call> tags into a
// ParsedToolCall. Arguments are always converted to strings for simplicity —
// the individual tool wrappers handle type conversion.
func parseToolCallJSON(raw string) (*ParsedToolCall, error) {
	var rtc rawToolCall
	if err := json.Unmarshal([]byte(raw), &rtc); err != nil {
		return nil, err
	}

	// Normalise: use arguments if present, fall back to parameters.
	rawArgs := rtc.Arguments
	if rawArgs == nil {
		rawArgs = rtc.Parameters
	}

	args := make(map[string]string, len(rawArgs))
	for k, v := range rawArgs {
		// Unquote JSON strings; leave numbers/bools as their raw text.
		var s string
		if err := json.Unmarshal(v, &s); err == nil {
			args[k] = s
		} else {
			args[k] = string(v) // number, bool, or nested object as raw JSON
		}
	}

	return &ParsedToolCall{
		Name:      rtc.Name,
		Arguments: args,
	}, nil
}

// ─── Qwen3 thinking block stripper ───────────────────────────────────────────

// Qwen3 models (especially in "thinking" mode) emit <think>…</think> blocks
// before the actual response. These are the model's internal reasoning and
// should NOT be shown to the user or treated as tool calls.
//
// StripThinkingBlock removes any leading <think>…</think> block from a
// complete response string. Used in post-processing, not in the stream parser
// (since we handle thinking blocks at the stream level via a separate state).
func StripThinkingBlock(response string) string {
	const thinkOpen  = "<think>"
	const thinkClose = "</think>"

	start := strings.Index(response, thinkOpen)
	if start == -1 {
		return response
	}
	end := strings.Index(response, thinkClose)
	if end == -1 {
		return response
	}
	return strings.TrimSpace(response[end+len(thinkClose):])
}