package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ModelManager handles downloading, verifying, and storing GGUF models.
type ModelManager struct {
	modelDir string
}

// NewModelManager creates a new model manager with the given directory.
func NewModelManager(modelDir string) (*ModelManager, error) {
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create model directory: %w", err)
	}
	return &ModelManager{modelDir: modelDir}, nil
}

// DownloadModel downloads a GGUF model from a URL and verifies its SHA256 checksum.
// Returns the local path to the downloaded model or an error.
func (m *ModelManager) DownloadModel(url, expectedSHA256 string, onProgress func(current, total int64)) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch model: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Extract model filename from URL
	parts := strings.Split(strings.TrimSuffix(url, "/"), "/")
	filename := parts[len(parts)-1]
	if !strings.HasSuffix(filename, ".gguf") {
		filename = fmt.Sprintf("model_%d.gguf", time.Now().Unix())
	}

	modelPath := filepath.Join(m.modelDir, filename)

	// Create temporary file for safe writing
	tmpFile, err := os.CreateTemp(m.modelDir, ".model-download-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tmpFile.Close()

	// Download with progress tracking and SHA256 verification
	hasher := sha256.New()
	written, err := io.CopyN(io.MultiWriter(tmpFile, hasher), resp.Body, resp.ContentLength)
	if err != nil && err != io.EOF {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("download failed: %w", err)
	}

	if onProgress != nil {
		onProgress(written, resp.ContentLength)
	}

	// Verify checksum if provided
	if expectedSHA256 != "" {
		actualSHA256 := fmt.Sprintf("%x", hasher.Sum(nil))
		if !strings.EqualFold(actualSHA256, expectedSHA256) {
			os.Remove(tmpFile.Name())
			return "", fmt.Errorf("checksum mismatch: expected %s, got %s", expectedSHA256, actualSHA256)
		}
	}

	// Atomically move temp file to model path
	if err := os.Rename(tmpFile.Name(), modelPath); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to finalize model: %w", err)
	}

	if err := os.Chmod(modelPath, 0o644); err != nil {
		log.Printf("warning: failed to set model permissions: %v", err)
	}

	return modelPath, nil
}

// ListModels returns all downloaded GGUF models in the model directory.
func (m *ModelManager) ListModels() ([]string, error) {
	entries, err := os.ReadDir(m.modelDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read model directory: %w", err)
	}

	var models []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".gguf") {
			models = append(models, filepath.Join(m.modelDir, entry.Name()))
		}
	}
	return models, nil
}

// DeleteModel removes a model file from storage.
func (m *ModelManager) DeleteModel(modelPath string) error {
	// Security: ensure model is within modelDir
	abs, err := filepath.Abs(modelPath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	modelDirAbs, err := filepath.Abs(m.modelDir)
	if err != nil {
		return fmt.Errorf("invalid model directory: %w", err)
	}

	if !strings.HasPrefix(abs, modelDirAbs+string(filepath.Separator)) {
		return fmt.Errorf("path outside model directory")
	}

	if err := os.Remove(abs); err != nil {
		return fmt.Errorf("failed to delete model: %w", err)
	}
	return nil
}

// GetModelInfo returns file info (size, modified time) for a model.
func (m *ModelManager) GetModelInfo(modelPath string) (map[string]any, error) {
	info, err := os.Stat(modelPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat model: %w", err)
	}

	return map[string]any{
		"name":     filepath.Base(modelPath),
		"path":     modelPath,
		"size":     info.Size(),
		"modified": info.ModTime().Unix(),
	}, nil
}

func main() {
	var (
		modelDir = flag.String("model-dir", filepath.Join(os.Getenv("HOME"), ".local/share/lumin/models"), "Directory to store models")
		url      = flag.String("url", "", "URL to download model from")
		sha256   = flag.String("sha256", "", "Expected SHA256 checksum (optional)")
		list     = flag.Bool("list", false, "List downloaded models")
		delete   = flag.String("delete", "", "Delete model by path")
	)
	flag.Parse()

	mm, err := NewModelManager(*modelDir)
	if err != nil {
		log.Fatalf("failed to initialize model manager: %v", err)
	}

	if *list {
		models, err := mm.ListModels()
		if err != nil {
			log.Fatalf("failed to list models: %v", err)
		}
		for _, model := range models {
			info, err := mm.GetModelInfo(model)
			if err != nil {
				log.Printf("error: %v", err)
				continue
			}
			fmt.Printf("%s (%.2f MB, modified %s)\n", info["name"], float64(info["size"].(int64))/1024/1024, time.Unix(info["modified"].(int64), 0).Format("2006-01-02 15:04:05"))
		}
		return
	}

	if *delete != "" {
		if err := mm.DeleteModel(*delete); err != nil {
			log.Fatalf("failed to delete model: %v", err)
		}
		fmt.Printf("Model deleted: %s\n", *delete)
		return
	}

	if *url == "" {
		flag.Usage()
		return
	}

	fmt.Printf("Downloading model from %s...\n", *url)
	modelPath, err := mm.DownloadModel(*url, *sha256, func(current, total int64) {
		percent := float64(current) / float64(total) * 100
		fmt.Printf("  Progress: %.1f%% (%d/%d bytes)\n", percent, current, total)
	})
	if err != nil {
		log.Fatalf("download failed: %v", err)
	}

	fmt.Printf("Model downloaded successfully: %s\n", modelPath)
	info, _ := mm.GetModelInfo(modelPath)
	fmt.Printf("  Size: %.2f MB\n", float64(info["size"].(int64))/1024/1024)
}
