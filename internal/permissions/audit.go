package permissions

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func WriteAuditLog(path, message string) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	entry := fmt.Sprintf("%s %s\n", time.Now().Format(time.RFC3339Nano), message)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(entry)
	return err
}
