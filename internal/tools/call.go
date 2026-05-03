package tools

import "encoding/json"

// Call is the shared JSON-RPC payload shape for tool.call requests.
type Call struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}
