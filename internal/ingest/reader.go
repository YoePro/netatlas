package ingest

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

type RawLine struct {
	Seq       uint64
	Text      string
	NextState OffsetState
}

func ReadFile(ctx context.Context, path string, offsetStore *OffsetStore, out chan<- RawLine) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open log file %q: %w", path, err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat log file %q: %w", path, err)
	}

	current, err := CurrentState(path, info)
	if err != nil {
		return fmt.Errorf("identify log file %q: %w", path, err)
	}

	saved, err := offsetStore.Load()
	if err != nil {
		return err
	}

	startOffset := startOffset(saved, current)
	if startOffset > 0 {
		if _, err := file.Seek(startOffset, io.SeekStart); err != nil {
			return fmt.Errorf("seek log file %q to offset %d: %w", path, startOffset, err)
		}
	}

	reader := bufio.NewReader(file)
	offset := startOffset
	var seq uint64

	for {
		rawLine, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("read log file %q: %w", path, err)
		}
		if rawLine == "" && errors.Is(err, io.EOF) {
			return nil
		}

		line := strings.TrimSuffix(strings.TrimSuffix(rawLine, "\n"), "\r")
		offset += int64(len(rawLine))
		current.Offset = offset
		current.Size = info.Size()
		seq++

		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- RawLine{Seq: seq, Text: line, NextState: current}:
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
	}
}

func startOffset(saved OffsetState, current OffsetState) int64 {
	if saved.Offset <= 0 {
		return 0
	}
	if !SameFile(saved.FileID, current.FileID) {
		log.Printf("log rotation detected for %q: file identity changed", current.Path)
		return 0
	}
	if saved.Offset > current.Size {
		log.Printf("log rotation detected for %q: saved offset %d is beyond current size %d", current.Path, saved.Offset, current.Size)
		return 0
	}
	return saved.Offset
}

func IsMissingState(err error) bool {
	return errors.Is(err, os.ErrNotExist)
}
