package ipc

import (
	"encoding/json"
	"errors"
	"net"

	"lumin-engine/internal/tools"
)

type Backend interface {
	Generate(prompt string, maxTokens int) (string, error)
	Health() map[string]any
	LoadModel(path string) error
	UnloadModel() error
	Tool(name string, args []byte) (any, error)
}

type Handler struct {
	backend Backend
}

func NewHandler(backend Backend) *Handler {
	return &Handler{backend: backend}
}

func (h *Handler) HandleConn(conn net.Conn) {
	defer conn.Close()
	var request Request
	if err := json.NewDecoder(conn).Decode(&request); err != nil {
		_ = json.NewEncoder(conn).Encode(Response{JSONRPC: "2.0", Error: &RPCError{Code: -32700, Message: err.Error()}})
		return
	}
	response := Response{JSONRPC: "2.0", ID: request.ID}
	result, err := h.dispatch(request.Method, request.Params)
	if err != nil {
		response.Error = &RPCError{Code: -32000, Message: err.Error()}
	} else {
		response.Result = result
	}
	_ = json.NewEncoder(conn).Encode(response)
}

func (h *Handler) dispatch(method string, params json.RawMessage) (any, error) {
	switch method {
	case "health":
		return h.backend.Health(), nil
	case "generate":
		var request struct {
			Prompt    string `json:"prompt"`
			MaxTokens int    `json:"max_tokens"`
		}
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, err
		}
		return h.backend.Generate(request.Prompt, request.MaxTokens)
	case "model.load":
		var request struct{ Path string `json:"path"` }
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, err
		}
		return nil, h.backend.LoadModel(request.Path)
	case "model.unload":
		return nil, h.backend.UnloadModel()
	case "tool.call":
		var request tools.Call
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, err
		}
		return h.backend.Tool(request.Name, request.Arguments)
	default:
		return nil, errors.New("unknown method: " + method)
	}
}
