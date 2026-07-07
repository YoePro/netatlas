package arpscout

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadTransportConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.ini")
	content := `[arpscout_transport]
enabled = true
core_url = http://core.local
token = secret
spool_path = state/custom.jsonl
dry_run_path = state/dry.jsonl
timeout = 3s
retries = 4
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadTransportConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Enabled || cfg.CoreURL != "http://core.local" || cfg.Token != "secret" {
		t.Fatalf("config = %#v", cfg)
	}
	if cfg.SpoolPath != "state/custom.jsonl" || cfg.DryRunPath != "state/dry.jsonl" || cfg.Timeout != 3*time.Second || cfg.Retries != 4 {
		t.Fatalf("config = %#v", cfg)
	}
}

func TestCoreClientSendsRegisterHeartbeatAndBatch(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("Authorization = %q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client, err := NewCoreClient(TransportConfig{CoreURL: server.URL, Token: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	identity := Identity{SensorID: "sensor-1"}
	if err := client.Register(context.Background(), identity); err != nil {
		t.Fatal(err)
	}
	if err := client.Heartbeat(context.Background(), identity, DaemonStatus{Iterations: 1}); err != nil {
		t.Fatal(err)
	}
	if err := client.UploadBatch(context.Background(), ObservationBatch{SensorID: "sensor-1"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"/api/arpscout/register", "/api/arpscout/heartbeat", "/api/arpscout/observations"}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("paths = %#v", paths)
		}
	}
}

func TestUploaderSpoolsBatchWhenCoreFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spool.jsonl")
	uploader := &Uploader{
		Transport: failingTransport{},
		SpoolPath: path,
		Retries:   0,
	}
	err := uploader.UploadBatch(context.Background(), ObservationBatch{
		SensorID:     "sensor-1",
		Observations: []Observation{{IP: "192.168.1.10"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var batch ObservationBatch
	if err := json.Unmarshal(data[:len(data)-1], &batch); err != nil {
		t.Fatal(err)
	}
	if batch.SensorID != "sensor-1" || len(batch.Observations) != 1 {
		t.Fatalf("batch = %#v", batch)
	}
}

func TestUploaderDryRunWritesBatchWithoutCore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dry.jsonl")
	uploader := &Uploader{DryRunPath: path}
	if err := uploader.UploadBatch(context.Background(), ObservationBatch{SensorID: "sensor-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

type failingTransport struct{}

func (failingTransport) Register(context.Context, Identity) error {
	return nil
}

func (failingTransport) Heartbeat(context.Context, Identity, DaemonStatus) error {
	return nil
}

func (failingTransport) UploadBatch(context.Context, ObservationBatch) error {
	return errTestTransportFailure{}
}

type errTestTransportFailure struct{}

func (errTestTransportFailure) Error() string {
	return "test transport failure"
}
