package ingest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

type FileID struct {
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
}

type OffsetState struct {
	Version   int       `json:"version"`
	Path      string    `json:"path"`
	Basename  string    `json:"basename"`
	Offset    int64     `json:"offset"`
	Size      int64     `json:"size"`
	FileID    FileID    `json:"file_id"`
	UpdatedAt time.Time `json:"updated_at"`
}

type OffsetStore struct {
	path string
}

func NewOffsetStore(path string) *OffsetStore {
	return &OffsetStore{path: path}
}

func (s *OffsetStore) Path() string {
	return s.path
}

func (s *OffsetStore) EnsureDir() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create offset state directory %q: %w", dir, err)
	}
	return nil
}

func (s *OffsetStore) Load() (OffsetState, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return OffsetState{}, nil
		}
		return OffsetState{}, fmt.Errorf("read offset state %q: %w", s.path, err)
	}

	var state OffsetState
	if err := json.Unmarshal(data, &state); err != nil {
		return OffsetState{}, fmt.Errorf("parse offset state %q: %w", s.path, err)
	}
	if state.Version == 0 && (state.Path != "" || state.Offset > 0 || state.Size > 0 || state.FileID != (FileID{})) {
		state.Version = 1
	}

	return state, nil
}

func (s *OffsetStore) Save(state OffsetState) error {
	if err := s.EnsureDir(); err != nil {
		return err
	}
	if state.Version < 2 {
		state.Version = 2
	}
	if state.Basename == "" && state.Path != "" {
		state.Basename = filepath.Base(state.Path)
	}
	state.UpdatedAt = time.Now()

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode offset state: %w", err)
	}
	data = append(data, '\n')

	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("write offset state temp file: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("replace offset state: %w", err)
	}

	return nil
}

func CurrentState(path string, info os.FileInfo) (OffsetState, error) {
	fileID, err := FileIDFromInfo(info)
	if err != nil {
		return OffsetState{}, err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	return OffsetState{
		Version:   2,
		Path:      absPath,
		Basename:  filepath.Base(path),
		Size:      info.Size(),
		FileID:    fileID,
		UpdatedAt: time.Now(),
	}, nil
}

func FileIDFromInfo(info os.FileInfo) (FileID, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return FileID{}, fmt.Errorf("file info does not expose syscall.Stat_t")
	}

	return FileID{
		Device: uint64(stat.Dev),
		Inode:  stat.Ino,
	}, nil
}

func SameFile(a, b FileID) bool {
	return a.Device == b.Device && a.Inode == b.Inode
}
