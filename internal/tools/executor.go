package tools

import (
	"encoding/json"
	"fmt"

	"lumin-engine/internal/permissions"
)

// Executor routes and sandboxes tool calls through the permission policy
type Executor struct {
	policy       *permissions.Policy
	auditLogPath string // path to audit log file
}

// NewExecutor creates a new executor with a permission policy and audit log path
func NewExecutor(policy *permissions.Policy, auditLogPath string) *Executor {
	return &Executor{
		policy:       policy,
		auditLogPath: auditLogPath,
	}
}

// Execute checks permissions and runs the tool, with full audit trail
func (e *Executor) Execute(name string, raw json.RawMessage) (any, error) {
	// Check if tool is enabled
	if !e.policy.IsEnabled(name) {
		e.writeAuditLog(name, "DENIED", "tool disabled")
		return nil, fmt.Errorf("tool disabled: %s", name)
	}

	// Dispatch to handler
	var result any
	var execErr error

	switch name {
	case "fs_read":
		result, execErr = e.handleFsRead(raw)
	case "fs_write":
		result, execErr = e.handleFsWrite(raw)
	case "exec_safe":
		result, execErr = e.handleExecSafe(raw)
	case "notify":
		result, execErr = e.handleNotify(raw)
	case "web_fetch":
		result, execErr = e.handleWebFetch(raw)
	case "plasma_status":
		result, execErr = e.handlePlasmaStatus(raw)
	default:
		e.writeAuditLog(name, "UNKNOWN", "unknown tool")
		return nil, fmt.Errorf("unknown tool: %s", name)
	}

	// Audit result
	status := "OK"
	if execErr != nil {
		status = "ERROR"
	}
	e.writeAuditLog(name, status, execErr)

	return result, execErr
}

func (e *Executor) writeAuditLog(toolName, status string, detail any) {
	if e.auditLogPath == "" {
		return
	}

	msg := fmt.Sprintf(
		"[%s] tool=%s detail=%v",
		status,
		toolName,
		detail,
	)

	_ = permissions.WriteAuditLog(e.auditLogPath, msg)
}

func (e *Executor) handleFsRead(raw json.RawMessage) (any, error) {
	var request struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, fmt.Errorf("invalid fs_read params: %w", err)
	}

	// Check path is allowed
	if !e.policy.PathAllowed(request.Path) {
		return nil, fmt.Errorf("path not allowed: %s", request.Path)
	}

	return ReadFile(request.Path)
}

func (e *Executor) handleFsWrite(raw json.RawMessage) (any, error) {
	var request struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, fmt.Errorf("invalid fs_write params: %w", err)
	}

	if !e.policy.PathAllowed(request.Path) {
		return nil, fmt.Errorf("path not allowed: %s", request.Path)
	}

	return nil, WriteFile(request.Path, request.Content)
}

func (e *Executor) handleExecSafe(raw json.RawMessage) (any, error) {
	var request struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, fmt.Errorf("invalid exec_safe params: %w", err)
	}

	if !e.policy.CommandAllowed(request.Command) {
		return nil, fmt.Errorf("command not allowed: %s", request.Command)
	}

	return RunSafe(request.Command, request.Args...)
}

func (e *Executor) handleNotify(raw json.RawMessage) (any, error) {
	var request struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, fmt.Errorf("invalid notify params: %w", err)
	}

	return nil, Notify(request.Title, request.Body)
}

func (e *Executor) handleWebFetch(raw json.RawMessage) (any, error) {
	var request struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, fmt.Errorf("invalid web_fetch params: %w", err)
	}

	return FetchURL(request.URL)
}

func (e *Executor) handlePlasmaStatus(raw json.RawMessage) (any, error) {
	return PlasmaStatus(), nil
}
