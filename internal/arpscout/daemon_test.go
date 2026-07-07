package arpscout

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadDaemonConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.ini")
	content := `[arpscout]
interfaces = eth0,wlan0
include_incomplete = true
passive_interval = 2s
status_interval = 10s
gateway_ip = 192.168.1.1
active_scan = true
scan_cidr = 192.168.1.0/24
scan_interval = 1m
max_scan_hosts = 64
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadDaemonConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Interfaces) != 2 || cfg.Interfaces[0] != "eth0" || !cfg.IncludeIncomplete {
		t.Fatalf("config = %#v", cfg)
	}
	if cfg.PassiveInterval != 2*time.Second || cfg.StatusInterval != 10*time.Second || cfg.ScanInterval != time.Minute {
		t.Fatalf("interval config = %#v", cfg)
	}
	if !cfg.ActiveScan || cfg.ScanCIDR != "192.168.1.0/24" || cfg.MaxScanHosts != 64 {
		t.Fatalf("scan config = %#v", cfg)
	}
}

func TestRunDaemonEmitsBatchAndChanges(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	calls := 0
	var events []DaemonEvent
	err := RunDaemon(ctx, DaemonOptions{
		Config: DaemonConfig{
			PassiveInterval: time.Hour,
			StatusInterval:  time.Hour,
		},
		PassiveReader: func(options PassiveOptions) ([]Observation, error) {
			calls++
			cancel()
			return []Observation{{IP: "192.168.1.10", MAC: "b8:27:eb:12:34:56", Source: "test"}}, nil
		},
		Output: func(event DaemonEvent) {
			events = append(events, event)
		},
		Now: func() time.Time {
			return time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
		},
	})
	if err != context.Canceled {
		t.Fatalf("err = %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
	if len(events) < 1 || events[0].Kind != DaemonEventBatch || len(events[0].Changes) != 1 {
		t.Fatalf("events = %#v", events)
	}
}

func TestRunDaemonCanIncludeActiveScan(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	scans := 0
	var batch DaemonEvent
	err := RunDaemon(ctx, DaemonOptions{
		Config: DaemonConfig{
			Interfaces:      []string{"eth0"},
			PassiveInterval: time.Hour,
			StatusInterval:  time.Hour,
			ActiveScan:      true,
			ScanCIDR:        "192.168.1.0/30",
			ScanInterval:    time.Hour,
			MaxScanHosts:    8,
		},
		PassiveReader: func(options PassiveOptions) ([]Observation, error) {
			return nil, nil
		},
		Scanner: func(options ScanOptions) (ScanResult, error) {
			scans++
			cancel()
			return ScanResult{Observations: []Observation{{IP: "192.168.1.1", MAC: "b8:27:eb:12:34:56", Interface: options.Interface, Source: SourceActiveScan}}}, nil
		},
		Output: func(event DaemonEvent) {
			if event.Kind == DaemonEventBatch {
				batch = event
			}
		},
		Now: func() time.Time {
			return time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
		},
	})
	if err != context.Canceled {
		t.Fatalf("err = %v", err)
	}
	if scans != 1 {
		t.Fatalf("scans = %d, want 1", scans)
	}
	if len(batch.Observations) != 1 || batch.Observations[0].Interface != "eth0" {
		t.Fatalf("batch = %#v", batch)
	}
}

func TestRunDaemonUploadsBatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	transport := &recordingTransport{}
	err := RunDaemon(ctx, DaemonOptions{
		Identity: Identity{SensorID: "sensor-1"},
		Config: DaemonConfig{
			PassiveInterval: time.Hour,
			StatusInterval:  time.Hour,
		},
		PassiveReader: func(options PassiveOptions) ([]Observation, error) {
			cancel()
			return []Observation{{IP: "192.168.1.10", MAC: "b8:27:eb:12:34:56", Source: "test"}}, nil
		},
		Transport: transport,
	})
	if err != context.Canceled {
		t.Fatalf("err = %v", err)
	}
	if transport.registers != 1 || transport.uploads != 1 || transport.lastBatch.SensorID != "sensor-1" {
		t.Fatalf("transport = %#v", transport)
	}
}

type recordingTransport struct {
	registers  int
	heartbeats int
	uploads    int
	lastBatch  ObservationBatch
}

func (t *recordingTransport) Register(context.Context, Identity) error {
	t.registers++
	return nil
}

func (t *recordingTransport) Heartbeat(context.Context, Identity, DaemonStatus) error {
	t.heartbeats++
	return nil
}

func (t *recordingTransport) UploadBatch(_ context.Context, batch ObservationBatch) error {
	t.uploads++
	t.lastBatch = batch
	return nil
}
