package ingest

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReadFileResumesFromSavedOffset(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "dns.log")
	statePath := filepath.Join(dir, "state.json")

	if err := os.WriteFile(logPath, []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(logPath)
	if err != nil {
		t.Fatal(err)
	}
	state, err := CurrentState(logPath, info)
	if err != nil {
		t.Fatal(err)
	}
	state.Offset = int64(len("one\n"))

	store := NewOffsetStore(statePath)
	if err := store.Save(state); err != nil {
		t.Fatal(err)
	}

	lines := readAll(t, logPath, store)
	if got, want := len(lines), 2; got != want {
		t.Fatalf("line count = %d, want %d", got, want)
	}
	if lines[0].Text != "two" || lines[1].Text != "three" {
		t.Fatalf("lines = %#v", lines)
	}
}

func TestReadFileStartsAtBeginningAfterRotation(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.log")
	logPath := filepath.Join(dir, "dns.log")
	statePath := filepath.Join(dir, "state.json")

	if err := os.WriteFile(oldPath, []byte("old-one\nold-two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldInfo, err := os.Stat(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	oldState, err := CurrentState(logPath, oldInfo)
	if err != nil {
		t.Fatal(err)
	}
	oldState.Offset = oldInfo.Size()

	store := NewOffsetStore(statePath)
	if err := store.Save(oldState); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(logPath, []byte("new-one\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	lines := readAll(t, logPath, store)
	if got, want := len(lines), 1; got != want {
		t.Fatalf("line count = %d, want %d", got, want)
	}
	if lines[0].Text != "new-one" {
		t.Fatalf("line = %q, want %q", lines[0].Text, "new-one")
	}
}

func readAll(t *testing.T, path string, store *OffsetStore) []RawLine {
	t.Helper()

	out := make(chan RawLine)
	errs := make(chan error, 1)
	go func() {
		errs <- ReadFile(context.Background(), path, store, out)
		close(out)
	}()

	var lines []RawLine
	for line := range out {
		lines = append(lines, line)
	}
	if err := <-errs; err != nil {
		t.Fatal(err)
	}

	return lines
}
