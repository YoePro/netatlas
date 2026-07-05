package ingest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOffsetStoreEnsureDirCreatesParentWithSafePermissions(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state", "dnslog.offset.json")
	store := NewOffsetStore(statePath)

	if err := store.EnsureDir(); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(filepath.Dir(statePath))
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", filepath.Dir(statePath))
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("permissions = %o, want 700", got)
	}
}

func TestOffsetStoreEnsureDirFailsClearlyWhenParentCannotBeCreated(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "state")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	store := NewOffsetStore(filepath.Join(blocker, "dnslog.offset.json"))
	if err := store.EnsureDir(); err == nil {
		t.Fatal("EnsureDir returned nil error with file as parent path")
	}
}

func TestOffsetStoreSaveWritesVersionedMetadata(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "dns.log")
	statePath := filepath.Join(dir, "state", "dnslog.offset.json")
	if err := os.WriteFile(logPath, []byte("one\n"), 0o644); err != nil {
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
	state.Offset = info.Size()

	store := NewOffsetStore(statePath)
	if err := store.Save(state); err != nil {
		t.Fatal(err)
	}

	saved, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.Version != 2 {
		t.Fatalf("Version = %d, want 2", saved.Version)
	}
	if saved.Basename != "dns.log" {
		t.Fatalf("Basename = %q, want dns.log", saved.Basename)
	}
	if saved.UpdatedAt.IsZero() {
		t.Fatal("UpdatedAt is zero")
	}
}
