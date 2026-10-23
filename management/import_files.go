package management

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const importFilesDir = "./data/imports"

func saveImportSnapshot(playlistID uint, filename string, source io.Reader) (string, int64, error) {
	return saveImportSnapshotTo(importFilesDir, playlistID, filename, source)
}

func saveImportSnapshotTo(directory string, playlistID uint, filename string, source io.Reader) (string, int64, error) {
	if err := os.MkdirAll(directory, 0750); err != nil {
		return "", 0, fmt.Errorf("create import snapshot directory: %w", err)
	}

	path := filepath.Join(directory, fmt.Sprintf("playlist-%d-%s", playlistID, filename))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return "", 0, fmt.Errorf("open import snapshot %q: %w", path, err)
	}
	if err := file.Chmod(0600); err != nil {
		_ = file.Close()
		return "", 0, fmt.Errorf("set import snapshot permissions %q: %w", path, err)
	}

	size, copyErr := io.Copy(file, source)
	closeErr := file.Close()
	if copyErr != nil {
		return "", size, fmt.Errorf("write import snapshot %q: %w", path, copyErr)
	}
	if closeErr != nil {
		return "", size, fmt.Errorf("close import snapshot %q: %w", path, closeErr)
	}
	return path, size, nil
}
