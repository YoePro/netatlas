package main

import (
	"os"
	"path/filepath"
	"testing"

	"dnslog/internal/ingest"
)

func TestInputOffsetStatePathUsesInputBasename(t *testing.T) {
	got := inputOffsetStatePath("/var/log/named/query.log.1", "state/dnslog.offset.json")
	want := filepath.Join("state", "query.log.1-dnslog-offset.json")
	if got != want {
		t.Fatalf("inputOffsetStatePath = %q, want %q", got, want)
	}
}

func TestPrepareInputOffsetStateMovesCollidingState(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state", "dnslog.offset.json")
	firstDir := filepath.Join(dir, "first")
	secondDir := filepath.Join(dir, "second")
	if err := os.MkdirAll(firstDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(secondDir, 0o755); err != nil {
		t.Fatal(err)
	}

	firstInput := filepath.Join(firstDir, "named.log.1")
	secondInput := filepath.Join(secondDir, "named.log.1")
	if err := os.WriteFile(firstInput, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondInput, []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	candidate, err := prepareInputOffsetState(firstInput, statePath, false)
	if err != nil {
		t.Fatal(err)
	}
	firstInfo, err := os.Stat(firstInput)
	if err != nil {
		t.Fatal(err)
	}
	firstState, err := ingest.CurrentState(firstInput, firstInfo)
	if err != nil {
		t.Fatal(err)
	}
	firstState.Offset = firstInfo.Size()
	if err := ingest.NewOffsetStore(candidate).Save(firstState); err != nil {
		t.Fatal(err)
	}

	secondCandidate, err := prepareInputOffsetState(secondInput, statePath, false)
	if err != nil {
		t.Fatal(err)
	}
	if secondCandidate != candidate {
		t.Fatalf("state path changed = %q, want %q", secondCandidate, candidate)
	}
	if _, err := os.Stat(candidate); !os.IsNotExist(err) {
		t.Fatalf("candidate still exists after collision move or stat failed unexpectedly: %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(filepath.Dir(candidate), "named.log.1-dnslog-offset.dev-*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("collision archive count = %d, want 1 (%v)", len(matches), matches)
	}
}

func TestPrepareInputOffsetStateMigratesMatchingDefaultState(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "named.log")
	defaultStatePath := filepath.Join(dir, "state", "dnslog.offset.json")
	if err := os.WriteFile(input, []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(input)
	if err != nil {
		t.Fatal(err)
	}
	state, err := ingest.CurrentState(input, info)
	if err != nil {
		t.Fatal(err)
	}
	state.Offset = info.Size()
	if err := ingest.NewOffsetStore(defaultStatePath).Save(state); err != nil {
		t.Fatal(err)
	}

	inputStatePath, err := prepareInputOffsetState(input, defaultStatePath, false)
	if err != nil {
		t.Fatal(err)
	}
	if inputStatePath == defaultStatePath {
		t.Fatalf("input state path was not derived from input basename: %q", inputStatePath)
	}
	if _, err := os.Stat(defaultStatePath); !os.IsNotExist(err) {
		t.Fatalf("default state still exists after migration or stat failed unexpectedly: %v", err)
	}
	if _, err := os.Stat(inputStatePath); err != nil {
		t.Fatalf("input state was not created by migration: %v", err)
	}
}
