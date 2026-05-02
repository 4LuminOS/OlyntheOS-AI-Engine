package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	enginecontext "lumin-engine/internal/context"
	"lumin-engine/internal/config"
	"lumin-engine/internal/hardware"
	"lumin-engine/internal/inference"
	"lumin-engine/internal/ipc"
	"lumin-engine/internal/permissions"
	"lumin-engine/internal/tools"
	"github.com/coreos/go-systemd/v22/activation"
)

type engine struct {
	model      *inference.Model
	policy     *permissions.Policy
	executor   *tools.Executor
	config     config.Config
	probe      hardware.ProbeResult
	ctxManager *enginecontext.Manager
}

func (e *engine) Generate(prompt string, maxTokens int) (string, error) {
	if err := e.model.EnsureLoaded(); err != nil {
		return "", err
	}
	return e.model.Generate(prompt, maxTokens)
}

func (e *engine) Health() map[string]any {
	return map[string]any{
		"status":      "ok",
		"model":       e.model.Status(),
		"hardware":    e.probe,
		"max_context":  e.ctxManager.MaxTokens,
		"permissions":  e.policy.Summary(),
		"socket_path":  e.config.SocketPath,
		"audit_log":    e.config.AuditLogPath,
	}
}

func (e *engine) LoadModel(path string) error {
	return e.model.Load(path)
}

func (e *engine) UnloadModel() error {
	return e.model.Unload()
}

func (e *engine) Tool(name string, args []byte) (any, error) {
	return e.executor.Execute(name, args)
}

func main() {
	configPath := flag.String("config", "", "path to config file")
	socketPath := flag.String("socket", "", "unix socket path")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if *socketPath != "" {
		cfg.SocketPath = *socketPath
	}

	if err := os.MkdirAll(filepath.Dir(cfg.SocketPath), 0o755); err != nil {
		log.Fatalf("prepare socket directory: %v", err)
	}

	policy, err := permissions.Load(cfg.PermissionsPath)
	if err != nil {
		log.Fatalf("load permissions: %v", err)
	}

	probe := hardware.ProbeSystem()
	model := inference.NewModel(cfg.ModelPath, cfg.MaxContextTokens)
	if cfg.ModelPath != "" {
		if err := model.Load(""); err != nil {
			log.Fatalf("load model: %v", err)
		}
	}
	ctxManager := enginecontext.NewManager(cfg.MaxContextTokens)
	backend := &engine{
		model:      model,
		policy:     policy,
		executor:   tools.NewExecutor(policy, cfg.AuditLogPath),
		config:     cfg,
		probe:      probe,
		ctxManager: ctxManager,
	}

	handler := ipc.NewHandler(backend)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Try systemd socket activation first; fall back to unix socket creation
	var listener net.Listener
	listeners, err := activation.Listeners()
	if err != nil {
		log.Fatalf("check systemd activation: %v", err)
	}

	if len(listeners) > 0 {
		// Socket activation: use listener from systemd
		listener = listeners[0]
		log.Printf("lumin-engine listening on systemd-activated socket")
	} else {
		// Manual socket: create unix domain socket
		if err := os.MkdirAll(filepath.Dir(cfg.SocketPath), 0o755); err != nil {
			log.Fatalf("prepare socket directory: %v", err)
		}
		if err := os.Remove(cfg.SocketPath); err != nil && !os.IsNotExist(err) {
			log.Fatalf("remove stale socket: %v", err)
		}

		unixListener, err := net.ListenUnix("unix", &net.UnixAddr{Name: cfg.SocketPath, Net: "unix"})
		if err != nil {
			log.Fatalf("listen on socket: %v", err)
		}
		if err := os.Chmod(cfg.SocketPath, 0o666); err != nil {
			log.Fatalf("chmod socket: %v", err)
		}
		listener = unixListener
		log.Printf("lumin-engine listening on %s", cfg.SocketPath)
	}
	defer listener.Close()

	server := ipc.NewServer(listener, handler)
	if err := server.Serve(ctx); err != nil && err != context.Canceled {
		log.Fatalf("serve: %v", err)
	}
	fmt.Println("lumin-engine stopped")
}
