package ingest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

type FileID struct {
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
}

type OffsetState struct {
	Path   string `json:"path"`
	Offset int64  `json:"offset"`
	Size   int64  `json:"size"`
	FileID FileID `json:"file_id"`
}

type OffsetStore struct {
	path string
}

func NewOffsetStore(path string) *OffsetStore {
	return &OffsetStore{path: path}
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

	return state, nil
}

func (s *OffsetStore) Save(state OffsetState) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create offset state directory: %w", err)
	}

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

	return OffsetState{
		Path:   path,
		Size:   info.Size(),
		FileID: fileID,
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
