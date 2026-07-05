package ingest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

type LockFile struct {
	path string
}

type lockState struct {
	PID       int       `json:"pid"`
	CreatedAt time.Time `json:"created_at"`
}

func AcquireLock(path string) (*LockFile, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create lock directory: %w", err)
	}
	state := lockState{PID: os.Getpid(), CreatedAt: time.Now()}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode lock file: %w", err)
	}
	data = append(data, '\n')

	for {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			if _, err := file.Write(data); err != nil {
				_ = file.Close()
				_ = os.Remove(path)
				return nil, fmt.Errorf("write lock file: %w", err)
			}
			if err := file.Close(); err != nil {
				_ = os.Remove(path)
				return nil, fmt.Errorf("close lock file: %w", err)
			}
			return &LockFile{path: path}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("create lock file %q: %w", path, err)
		}
		stale, err := lockIsStale(path)
		if err != nil {
			return nil, err
		}
		if !stale {
			return nil, fmt.Errorf("offset state is locked by another dnslog process: %s", path)
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("remove stale lock file %q: %w", path, err)
		}
	}
}

func (l *LockFile) Release() error {
	if l == nil || l.path == "" {
		return nil
	}
	if err := os.Remove(l.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove lock file %q: %w", l.path, err)
	}
	return nil
}

func lockIsStale(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read lock file %q: %w", path, err)
	}
	var state lockState
	if err := json.Unmarshal(data, &state); err != nil {
		return false, fmt.Errorf("parse lock file %q: %w", path, err)
	}
	if state.PID <= 0 {
		return true, nil
	}
	err = syscall.Kill(state.PID, 0)
	return errors.Is(err, syscall.ESRCH), nil
}
