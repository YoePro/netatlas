package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dnslog/internal/config"
)

func TestPrintSystemCheck(t *testing.T) {
	var buf bytes.Buffer
	printSystemCheck(&buf)
	output := buf.String()
	for _, want := range []string{"Detected:", "Recommended:", "worker_count", "runtime_mode"} {
		if !strings.Contains(output, want) {
			t.Fatalf("system-check output missing %q:\n%s", want, output)
		}
	}
}

func TestRunPreflightDryRun(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "dns.log")
	statePath := filepath.Join(dir, "state", "offset.json")
	if err := os.WriteFile(logPath, []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		LogFilePath:     logPath,
		OffsetStatePath: statePath,
		DryRun:          true,
	}
	var buf bytes.Buffer
	if err := runPreflight(context.Background(), &buf, cfg); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "neo4j: skipped") {
		t.Fatalf("preflight output missing dry-run neo4j skip:\n%s", buf.String())
	}
}

func TestRunBenchmarkDoesNotMoveOffset(t *testing.T) {
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
		RuntimeMode:         "max",
		ProgressInterval:    0,
		ParseFailureSamples: 0,
		IgnoreReverseLookup: true,
	}

	var buf bytes.Buffer
	if err := runBenchmark(context.Background(), &buf, cfg, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("benchmark created offset state or stat failed unexpectedly: %v", err)
	}
	if !strings.Contains(buf.String(), "Telemetry:") {
		t.Fatalf("benchmark output missing telemetry:\n%s", buf.String())
	}
}

func TestRunBenchmarkSweepsWorkerCounts(t *testing.T) {
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
		RuntimeMode:         "max",
		ParseFailureSamples: 0,
		IgnoreReverseLookup: true,
	}

	var buf bytes.Buffer
	if err := runBenchmark(context.Background(), &buf, cfg, "1,2"); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	if !strings.Contains(output, "Benchmark worker_count=1") || !strings.Contains(output, "Benchmark worker_count=2") {
		t.Fatalf("benchmark sweep output missing worker headers:\n%s", output)
	}
	if strings.Count(output, "Telemetry:") != 2 {
		t.Fatalf("benchmark sweep telemetry count = %d, want 2:\n%s", strings.Count(output, "Telemetry:"), output)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("benchmark created configured offset state or stat failed unexpectedly: %v", err)
	}
}

func TestBenchmarkWorkerCounts(t *testing.T) {
	tests := []struct {
		spec       string
		configured int
		want       []int
	}{
		{spec: "", configured: 3, want: []int{3}},
		{spec: "1,2,2,4", configured: 3, want: []int{1, 2, 4}},
		{spec: "2-4", configured: 3, want: []int{2, 3, 4}},
	}

	for _, tt := range tests {
		got, err := benchmarkWorkerCounts(tt.spec, tt.configured)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(tt.want) {
			t.Fatalf("benchmarkWorkerCounts(%q) length = %d, want %d", tt.spec, len(got), len(tt.want))
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Fatalf("benchmarkWorkerCounts(%q) = %#v, want %#v", tt.spec, got, tt.want)
			}
		}
	}
}
