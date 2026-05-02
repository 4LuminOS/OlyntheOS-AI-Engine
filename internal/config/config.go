package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	SocketPath       string
	PermissionsPath  string
	AuditLogPath     string
	ModelName        string
	ModelPath        string
	MaxContextTokens int
}

func Default() Config {
	return Config{
		SocketPath:       "/tmp/lumin-engine.sock",
		PermissionsPath:  "/etc/lumin/permissions.toml",
		AuditLogPath:     "/var/log/lumin-engine/audit.log",
		ModelName:        "default",
		ModelPath:        "",
		MaxContextTokens: 8192,
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		return cfg, nil
	}
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), "\"')")
		switch key {
		case "socket_path":
			cfg.SocketPath = value
		case "permissions_path":
			cfg.PermissionsPath = value
		case "audit_log_path":
			cfg.AuditLogPath = value
		case "model_name":
			cfg.ModelName = value
		case "model_path":
			cfg.ModelPath = value
		case "max_context_tokens":
			if n, err := strconv.Atoi(value); err == nil {
				cfg.MaxContextTokens = n
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if !filepath.IsAbs(cfg.SocketPath) {
		cfg.SocketPath = filepath.Clean(cfg.SocketPath)
	}
	return cfg, nil
}
