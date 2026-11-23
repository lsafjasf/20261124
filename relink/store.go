package relink

import (
	"fmt"
	"os"
	"path/filepath"
)

type Store interface {
	Load(path string) ([]byte, error)
	Save(path string, data []byte) error
}

type FileStore struct{}

func (FileStore) Load(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (FileStore) Save(path string, data []byte) error {
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".relink-*.tmp")
	if err != nil {
		return fmt.Errorf("create project temp file: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)

	if err := temp.Chmod(mode); err != nil {
		temp.Close()
		return fmt.Errorf("set project temp permissions: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return fmt.Errorf("write project temp file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("sync project temp file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close project temp file: %w", err)
	}
	if err := replaceFile(tempName, path); err != nil {
		return err
	}
	if directory, err := os.Open(dir); err == nil {
		_ = directory.Sync()
		_ = directory.Close()
	}
	return nil
}

func replaceFile(tempName, target string) error {
	if err := os.Rename(tempName, target); err == nil {
		return nil
	}

	backup := target + ".relink-backup"
	_ = os.Remove(backup)
	if err := os.Rename(target, backup); err != nil {
		if os.IsNotExist(err) {
			if renameErr := os.Rename(tempName, target); renameErr != nil {
				return fmt.Errorf("replace project: %w", renameErr)
			}
			return nil
		}
		return fmt.Errorf("move old project aside: %w", err)
	}
	if err := os.Rename(tempName, target); err != nil {
		_ = os.Rename(backup, target)
		return fmt.Errorf("install relinked project: %w", err)
	}
	_ = os.Remove(backup)
	return nil
}
