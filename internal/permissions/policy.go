package permissions

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Policy struct {
	EnabledTools    map[string]bool
	AllowedPaths    []string
	AllowedCommands []string
}

func Default() *Policy {
	return &Policy{EnabledTools: map[string]bool{}}
}

func Load(path string) (*Policy, error) {
	policy := Default()
	if path == "" {
		return policy, nil
	}
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return policy, nil
		}
		return nil, err
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
		switch {
		case key == "enabled_tool" || strings.HasPrefix(key, "tool."):
			policy.EnabledTools[strings.TrimPrefix(key, "tool.")] = strings.EqualFold(value, "true") || value == "1"
		case key == "allowed_path":
			policy.AllowedPaths = append(policy.AllowedPaths, value)
		case key == "allowed_command":
			policy.AllowedCommands = append(policy.AllowedCommands, value)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read permissions: %w", err)
	}
	return policy, nil
}

func (p *Policy) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var builder strings.Builder
	for tool, enabled := range p.EnabledTools {
		builder.WriteString("tool.")
		builder.WriteString(tool)
		builder.WriteString(" = ")
		builder.WriteString(strings.ToLower(fmt.Sprint(enabled)))
		builder.WriteString("\n")
	}
	for _, allowedPath := range p.AllowedPaths {
		builder.WriteString("allowed_path = \"")
		builder.WriteString(allowedPath)
		builder.WriteString("\"\n")
	}
	for _, allowedCommand := range p.AllowedCommands {
		builder.WriteString("allowed_command = \"")
		builder.WriteString(allowedCommand)
		builder.WriteString("\"\n")
	}
	return os.WriteFile(path, []byte(builder.String()), 0o644)
}

func (p *Policy) IsEnabled(tool string) bool {
	if p == nil || len(p.EnabledTools) == 0 {
		return true
	}
	enabled, ok := p.EnabledTools[tool]
	return !ok || enabled
}

func (p *Policy) PathAllowed(path string) bool {
	if p == nil || len(p.AllowedPaths) == 0 {
		return true
	}
	cleaned := filepath.Clean(path)
	for _, allowed := range p.AllowedPaths {
		if strings.HasPrefix(cleaned, filepath.Clean(allowed)) {
			return true
		}
	}
	return false
}

func (p *Policy) CommandAllowed(command string) bool {
	if p == nil || len(p.AllowedCommands) == 0 {
		return true
	}
	for _, allowed := range p.AllowedCommands {
		if command == allowed {
			return true
		}
	}
	return false
}

func (p *Policy) Summary() map[string]any {
	return map[string]any{
		"enabled_tools":   len(p.EnabledTools),
		"allowed_paths":   len(p.AllowedPaths),
		"allowed_commands": len(p.AllowedCommands),
	}
}
