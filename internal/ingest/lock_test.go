package ingest

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAcquireLockRefusesActiveLock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "offset.json.lock")

	lock, err := AcquireLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()

	if _, err := AcquireLock(path); err == nil {
		t.Fatal("AcquireLock returned nil error for active lock")
	}
}

func TestAcquireLockRemovesStaleLock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "offset.json.lock")
	content := fmt.Sprintf(`{"pid":99999999,"created_at":%q}`+"\n", time.Now().Format(time.RFC3339))
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	lock, err := AcquireLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("lock file missing after stale lock replacement: %v", err)
	}
}
