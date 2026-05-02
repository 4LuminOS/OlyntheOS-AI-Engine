package ipc

import (
	"context"
	"net"
	"os"
	"path/filepath"
)

type Server struct {
	listener net.Listener
	handler  *Handler
}

// Listen creates a Unix domain socket listener and returns a new Server.
func Listen(ctx context.Context, socketPath string, handler *Handler) (*Server, error) {
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o755); err != nil {
		return nil, err
	}
	if err := os.RemoveAll(socketPath); err != nil {
		return nil, err
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}
	_ = ctx
	return &Server{listener: listener, handler: handler}, nil
}

// NewServer creates a Server from an existing listener (e.g., from systemd socket activation).
func NewServer(listener net.Listener, handler *Handler) *Server {
	return &Server{listener: listener, handler: handler}
}

func (s *Server) Serve(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		_ = s.listener.Close()
	}()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				return err
			}
		}
		go s.handler.HandleConn(conn)
	}
}

func (s *Server) Close() error {
	return s.listener.Close()
}
