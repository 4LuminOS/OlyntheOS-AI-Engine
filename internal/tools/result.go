package tools

// Result is a normalized tool execution result for agent streaming.
type Result struct {
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}
