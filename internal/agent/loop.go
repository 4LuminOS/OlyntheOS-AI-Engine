// Package agent implements the LuminAI agentic loop.
// This is the heart of the engine — the think → act → observe cycle
// that turns a user message into real OS actions.
package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/yourusername/lumin-engine/internal/permissions"
	"github.com/yourusername/lumin-engine/internal/tools"
)

// MaxIterations caps the agent loop to prevent infinite cycles.
// Most real tasks finish in 3–8 iterations. If this is hit, the
// loop stops and tells the user the task may be incomplete.
const MaxIterations = 20

// ─── Message roles ────────────────────────────────────────────────────────────

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is one turn in the conversation context.
type Message struct {
	Role       Role
	Content    string
	ToolName   string // only set when Role == RoleTool
	TokenCount int    // filled in by the context manager after tokenising
}

// ─── Events streamed back to the caller ──────────────────────────────────────

// EventType describes what kind of event is being sent downstream.
type EventType string

const (
	// EventToken is a single text token to display in the UI immediately.
	EventToken EventType = "token"

	// EventToolCall signals that the AI is about to execute a tool.
	// The UI can show "AI is doing X…" while it runs.
	EventToolCall EventType = "tool_call"

	// EventToolResult signals the outcome of a tool call.
	EventToolResult EventType = "tool_result"

	// EventDone signals the loop has finished cleanly.
	EventDone EventType = "done"

	// EventError signals a fatal error — loop has stopped.
	EventError EventType = "error"
)

// Event is what the agent streams back to the IPC handler (and then to the UI).
type Event struct {
	Type EventType

	// EventToken
	Text string

	// EventToolCall
	ToolCall *ParsedToolCall

	// EventToolResult
	ToolResult *tools.Result

	// EventError
	Err error
}

// ─── Inference interface (implemented by internal/inference) ─────────────────

// Inferencer is the interface the agent loop calls to generate tokens.
// internal/inference.Model satisfies this interface.
type Inferencer interface {
	// Generate runs the model forward pass and streams tokens to out.
	// It closes out when the model outputs an EOS token or stop sequence.
	Generate(ctx context.Context, tokens []int32, out chan<- string) error

	// Tokenize converts text to token IDs using the model's built-in vocab.
	Tokenize(text string) []int32

	// Detokenize converts a single token ID back to its text piece.
	Detokenize(token int32) string
}

// ─── Context builder interface (implemented by internal/context) ──────────────

// ContextBuilder assembles the full prompt from the conversation history
// and returns the complete token sequence ready for inference.
type ContextBuilder interface {
	// Add appends a message to the conversation history.
	Add(msg Message)

	// BuildTokens returns the full tokenised prompt for the current history.
	// Automatically trims oldest non-system messages when the window is full.
	BuildTokens() []int32

	// Reset clears history except for the system prompt.
	Reset()
}

// ─── Agent ───────────────────────────────────────────────────────────────────

// Agent ties together inference, context management, tool execution,
// and permission checking into the agentic loop.
type Agent struct {
	model    Inferencer
	ctx      ContextBuilder
	executor *tools.Executor
	perms    *permissions.Policy
	log      *slog.Logger
}

// New creates a new Agent. Call Run() to start a request.
func New(
	model Inferencer,
	ctx ContextBuilder,
	executor *tools.Executor,
	perms *permissions.Policy,
	log *slog.Logger,
) *Agent {
	return &Agent{
		model:    model,
		ctx:      ctx,
		executor: executor,
		perms:    perms,
		log:      log,
	}
}

// ─── Run — the main agent loop ───────────────────────────────────────────────

// Run executes the agentic loop for a single user message.
//
// It streams Events to the out channel:
//   - EventToken  → forward to UI for live display
//   - EventToolCall → show "AI is doing X" in UI
//   - EventToolResult → show result in UI (optional)
//   - EventDone   → request complete, close UI input
//   - EventError  → something went wrong
//
// The caller (ipc/handler.go) is responsible for closing nothing —
// Run always closes out before returning.
//
// cancelCtx can be used to abort an in-progress request (e.g. user
// clicks Stop in the UI).
func (a *Agent) Run(cancelCtx context.Context, userMessage string, out chan<- Event) {
	defer close(out)

	a.log.Info("agent loop started", "message_len", len(userMessage))

	// Add the user's message to the shared context window.
	a.ctx.Add(Message{Role: RoleUser, Content: userMessage})

	parser := NewStreamParser()

	for iteration := range MaxIterations {
		a.log.Debug("agent iteration", "n", iteration+1)

		// ── STEP A: build full tokenised prompt ───────────────────────────────
		tokens := a.ctx.BuildTokens()
		if len(tokens) == 0 {
			out <- Event{Type: EventError, Err: errors.New("context produced empty token sequence")}
			return
		}

		// ── STEP B: run inference, stream tokens through the parser ───────────
		tokenCh := make(chan string, 128)
		inferErr := make(chan error, 1)

		go func() {
			inferErr <- a.model.Generate(cancelCtx, tokens, tokenCh)
		}()

		var assistantBuf strings.Builder
		parser.Reset()
		toolCallFound := false

		feedLoop:
		for {
			select {
			case <-cancelCtx.Done():
				out <- Event{Type: EventError, Err: fmt.Errorf("request cancelled")}
				return

			case token, ok := <-tokenCh:
				if !ok {
					// Channel closed — inference finished this pass.
					break feedLoop
				}

				result := parser.Feed(token)

				switch result.Kind {
				case ParseKindText:
					// Normal text token — stream straight to UI.
					assistantBuf.WriteString(result.Text)
					out <- Event{Type: EventToken, Text: result.Text}

				case ParseKindToolCall:
					// Tool call detected — stop collecting text, handle it.
					toolCallFound = true

					// Save what the assistant said before the tool call.
					if t := strings.TrimSpace(assistantBuf.String()); t != "" {
						a.ctx.Add(Message{Role: RoleAssistant, Content: t})
					}
					assistantBuf.Reset()

					// Drain the rest of the token channel (model output ends here).
					for range tokenCh {}

					// ── STEP C: execute the tool call ─────────────────────────
					a.log.Info("tool call", "tool", result.ToolCall.Name, "args", result.ToolCall.Arguments)
					out <- Event{Type: EventToolCall, ToolCall: result.ToolCall}

					toolResult := a.executeChecked(result.ToolCall)
					out <- Event{Type: EventToolResult, ToolResult: toolResult}

					// ── STEP D: inject result into context, loop again ────────
					content := toolResult.Output
					if toolResult.Error != "" {
						content = fmt.Sprintf("error: %s", toolResult.Error)
					}
					a.ctx.Add(Message{
						Role:     RoleTool,
						ToolName: result.ToolCall.Name,
						Content:  content,
					})

					break feedLoop
				}

			case err := <-inferErr:
				if err != nil && !errors.Is(err, context.Canceled) {
					out <- Event{Type: EventError, Err: fmt.Errorf("inference error: %w", err)}
					return
				}
			}
		}

		// Wait for inference goroutine to finish before next iteration.
		if err := <-inferErr; err != nil && !errors.Is(err, context.Canceled) {
			out <- Event{Type: EventError, Err: fmt.Errorf("inference error: %w", err)}
			return
		}

		if toolCallFound {
			// Tool was called — loop again so the model can see the result.
			continue
		}

		// ── STEP E: no tool call → model is done for this request ─────────────
		if final := strings.TrimSpace(assistantBuf.String()); final != "" {
			a.ctx.Add(Message{Role: RoleAssistant, Content: final})
		}
		a.log.Info("agent loop complete", "iterations", iteration+1)
		out <- Event{Type: EventDone}
		return
	}

	// Hit the max-iteration safety cap.
	a.log.Warn("agent loop hit max iterations", "max", MaxIterations)
	out <- Event{
		Type: EventToken,
		Text: "\n\n[I've reached my step limit — the task may be incomplete. You can ask me to continue.]",
	}
	out <- Event{Type: EventDone}
}

// ─── executeChecked — permission-gated tool execution ─────────────────────────

// executeChecked checks permissions before running a tool.
// If the capability is not granted, it returns an error result without
// touching the OS — the model will see the error and can tell the user.
func (a *Agent) executeChecked(call *ParsedToolCall) *tools.Result {
	// 1. Is this capability enabled by the user?
	if !a.perms.IsEnabled(call.Name) {
		return &tools.Result{
			Error: fmt.Sprintf(
				"capability '%s' is not enabled. The user can enable it in the LuminAI Control Panel.",
				call.Name,
			),
		}
	}

	// 2. For file-system tools, check the path allowlist.
	if path, ok := call.Arguments["path"]; ok {
		if !a.perms.PathAllowed(path) {
			return &tools.Result{
				Error: fmt.Sprintf(
					"path '%s' is not in the allowlist. The user can add it in the LuminAI Control Panel.",
					path,
				),
			}
		}
	}

	// 3. Execute via the tool executor (internal/tools/executor.go).
	result := a.executor.Execute(call.Name, call.Arguments)

	// 4. Audit log — always record what happened.
	a.log.Info("tool executed",
		"tool", call.Name,
		"ok", result.Error == "",
		"error", result.Error,
	)

	return result
}