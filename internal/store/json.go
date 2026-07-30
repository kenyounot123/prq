package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

var (
	makeDir    = os.MkdirAll
	createTemp = os.CreateTemp
	renameFile = os.Rename
)

func readJSON(path string, destination any) error {
	contents, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("read %s: %w", path, ErrNotFound)
		}
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(contents, destination); err != nil {
		return fmt.Errorf("decode %s: %w: %v", path, ErrInvalid, err)
	}
	return nil
}

func writeJSONAtomic(path string, value any) (err error) {
	directory := filepath.Dir(path)
	if err := makeDir(directory, 0o700); err != nil {
		return fmt.Errorf("create store directory %s: %w", directory, err)
	}

	file, err := createTemp(directory, ".prq-*")
	if err != nil {
		return fmt.Errorf("create temporary store file in %s: %w", directory, err)
	}
	temporaryPath := file.Name()
	closed := false
	defer func() {
		if !closed {
			if closeErr := file.Close(); err == nil && closeErr != nil {
				err = fmt.Errorf("close temporary store file %s: %w", temporaryPath, closeErr)
			}
		}
		if err != nil {
			_ = os.Remove(temporaryPath)
		}
	}()

	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("set permissions on temporary store file %s: %w", temporaryPath, err)
	}
	if err := json.NewEncoder(file).Encode(value); err != nil {
		return fmt.Errorf("encode temporary store file %s: %w", temporaryPath, err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync temporary store file %s: %w", temporaryPath, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close temporary store file %s: %w", temporaryPath, err)
	}
	closed = true
	if err := renameFile(temporaryPath, path); err != nil {
		return fmt.Errorf("replace store file %s: %w", path, err)
	}
	return nil
}
