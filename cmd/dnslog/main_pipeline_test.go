package main

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"netatlas/internal/config"
	"netatlas/internal/model"
)

type recordingStore struct {
	mu      sync.Mutex
	writes  int
	written int
}

func (s *recordingStore) Close(ctx context.Context) error {
	return nil
}

func (s *recordingStore) WriteBatch(ctx context.Context, batch []model.DNSEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writes++
	s.written += len(batch)
	return nil
}

func TestRunDoesNotSaveOffsetInDryRun(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "dns.log")
	statePath := filepath.Join(dir, "state", "offset.json")

	if err := os.WriteFile(logPath, []byte("2026-07-03T12:00:00Z 192.168.1.10 example.com. A NOERROR 93.184.216.34\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		LogFilePath:         logPath,
		ServerName:          "dns-primary",
		ServerRole:          "primary",
		BatchSize:           10,
		WorkerCount:         1,
		FlushInterval:       time.Second,
		MaxWriteRetries:     0,
		RetryDelay:          0,
		OffsetStatePath:     statePath,
		DryRun:              true,
		IgnoreReverseLookup: true,
	}
	store := &recordingStore{}
	runStats := &stats{}

	if err := run(context.Background(), cfg, store, runStats); err != nil {
		t.Fatal(err)
	}
	if store.written != 1 {
		t.Fatalf("written = %d, want 1", store.written)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("offset state exists in dry-run or stat failed unexpectedly: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(statePath)); !os.IsNotExist(err) {
		t.Fatalf("offset state directory exists in dry-run or stat failed unexpectedly: %v", err)
	}
}

func TestRunCanSaveOffsetInDryRunWhenExplicitlyEnabled(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "dns.log")
	statePath := filepath.Join(dir, "state", "offset.json")

	if err := os.WriteFile(logPath, []byte("2026-07-03T12:00:00Z 192.168.1.10 example.com. A NOERROR 93.184.216.34\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		LogFilePath:         logPath,
		ServerName:          "dns-primary",
		ServerRole:          "primary",
		BatchSize:           10,
		WorkerCount:         1,
		FlushInterval:       time.Second,
		MaxWriteRetries:     0,
		RetryDelay:          0,
		OffsetStatePath:     statePath,
		DryRun:              true,
		DryRunUpdatesOffset: true,
		RuntimeMode:         "max",
		ProgressInterval:    0,
		ParseFailureSamples: 0,
		IgnoreReverseLookup: true,
	}
	store := &recordingStore{}
	runStats := &stats{}

	if err := run(context.Background(), cfg, store, runStats); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("offset state was not saved with dry_run_updates_offset=true: %v", err)
	}
	if _, err := os.Stat(statePath + ".lock"); !os.IsNotExist(err) {
		t.Fatalf("lock file remains after run or stat failed unexpectedly: %v", err)
	}
}

func TestRunSavesOffsetWhenNotDryRun(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "dns.log")
	statePath := filepath.Join(dir, "state", "offset.json")

	if err := os.WriteFile(logPath, []byte("2026-07-03T12:00:00Z 192.168.1.10 example.com. A NOERROR 93.184.216.34\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		LogFilePath:         logPath,
		ServerName:          "dns-primary",
		ServerRole:          "primary",
		BatchSize:           10,
		WorkerCount:         1,
		FlushInterval:       time.Second,
		MaxWriteRetries:     0,
		RetryDelay:          0,
		OffsetStatePath:     statePath,
		DryRun:              false,
		IgnoreReverseLookup: true,
	}
	store := &recordingStore{}
	runStats := &stats{}

	if err := run(context.Background(), cfg, store, runStats); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("offset state was not saved when dry-run=false: %v", err)
	}
}

func TestRunSkipsRowsOlderThanGenesis(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "dns.log")
	statePath := filepath.Join(dir, "state", "offset.json")

	content := "" +
		"2026-07-03T12:00:00Z 192.168.1.10 old.example.com. A NOERROR 93.184.216.34\n" +
		"2026-07-05T12:00:00Z 192.168.1.10 fresh.example.com. A NOERROR 93.184.216.34\n"
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		LogFilePath:         logPath,
		ServerName:          "dns-primary",
		ServerRole:          "primary",
		BatchSize:           10,
		WorkerCount:         1,
		FlushInterval:       time.Second,
		MaxWriteRetries:     0,
		RetryDelay:          0,
		OffsetStatePath:     statePath,
		DryRun:              true,
		Genesis:             "custom",
		GenesisAfter:        "2026-07-04T00:00:00Z",
		IgnoreReverseLookup: true,
	}
	store := &recordingStore{}
	runStats := &stats{}

	if err := run(context.Background(), cfg, store, runStats); err != nil {
		t.Fatal(err)
	}
	if store.written != 1 {
		t.Fatalf("written = %d, want 1", store.written)
	}
	if runStats.lines != 2 {
		t.Fatalf("lines = %d, want 2", runStats.lines)
	}
	if runStats.parsed != 1 {
		t.Fatalf("parsed = %d, want 1", runStats.parsed)
	}
	if runStats.skippedGenesis != 1 {
		t.Fatalf("skippedGenesis = %d, want 1", runStats.skippedGenesis)
	}
	if runStats.parseFailures != 0 {
		t.Fatalf("parseFailures = %d, want 0", runStats.parseFailures)
	}
}
