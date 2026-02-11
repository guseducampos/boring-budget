package config

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	AppDirName      = ".boring-budget"
	DefaultDBFile   = "boring-budget.db"
	DefaultDataPerm = 0o755
)

func DefaultDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}

	dir := filepath.Join(home, AppDirName)
	if err := os.MkdirAll(dir, DefaultDataPerm); err != nil {
		return "", fmt.Errorf("create data directory %q: %w", dir, err)
	}

	return dir, nil
}

func DefaultDBPath() (string, error) {
	dir, err := DefaultDataDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, DefaultDBFile), nil
}
