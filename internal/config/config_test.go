package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestLoadReadsGraphAndFilterSettings(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.ini")

	content := `[neo4j]
uri = neo4j://example:7687
user = dnslog
password = secret
database = dns

[server]
name = ns1
role = secondary

[log]
file_path = /var/log/named/queries.log

[ingest]
batch_size = 100
worker_count = 2
flush_interval = 2s
max_write_retries = 5
retry_delay = 250ms
offset_state_path = /var/lib/dnslog/offset.json
dry_run = false
dry_run_updates_offset = true
runtime_mode = low
log_mode = verbose
genesis = custom
genesis_after = 2026-07-05T00:00:00+02:00
progress_interval = 3s
parse_failure_samples = 7
parse_failure_path = /var/lib/dnslog/parse-failures.log

[filter]
ignore_reverse_lookup = false
ignored_domains = telemetry.example.com, noise.example.net
local_domains = lan, home.arpa
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Neo4jURI != "neo4j://example:7687" || cfg.Neo4jDatabase != "dns" {
		t.Fatalf("neo4j config = %q/%q", cfg.Neo4jURI, cfg.Neo4jDatabase)
	}
	if cfg.ServerName != "ns1" || cfg.ServerRole != "secondary" {
		t.Fatalf("server config = %q/%q", cfg.ServerName, cfg.ServerRole)
	}
	if cfg.BatchSize != 100 || cfg.WorkerCount != 2 {
		t.Fatalf("ingest sizes = %d/%d", cfg.BatchSize, cfg.WorkerCount)
	}
	if cfg.FlushInterval != 2*time.Second || cfg.RetryDelay != 250*time.Millisecond {
		t.Fatalf("durations = %s/%s", cfg.FlushInterval, cfg.RetryDelay)
	}
	if cfg.Genesis != "custom" || cfg.GenesisAfter != "2026-07-05T00:00:00+02:00" {
		t.Fatalf("genesis = %q/%q", cfg.Genesis, cfg.GenesisAfter)
	}
	if !cfg.DryRunUpdatesOffset || cfg.RuntimeMode != "low" || cfg.LogMode != "verbose" {
		t.Fatalf("runtime config = dry_run_updates_offset:%t runtime_mode:%q log_mode:%q", cfg.DryRunUpdatesOffset, cfg.RuntimeMode, cfg.LogMode)
	}
	if cfg.ProgressInterval != 3*time.Second || cfg.ParseFailureSamples != 7 || cfg.ParseFailurePath != "/var/lib/dnslog/parse-failures.log" {
		t.Fatalf("ops config = progress:%s samples:%d path:%q", cfg.ProgressInterval, cfg.ParseFailureSamples, cfg.ParseFailurePath)
	}
	if cfg.IgnoreReverseLookup {
		t.Fatal("IgnoreReverseLookup = true, want false")
	}
	if !reflect.DeepEqual(cfg.IgnoredDomains, []string{"telemetry.example.com", "noise.example.net"}) {
		t.Fatalf("IgnoredDomains = %#v", cfg.IgnoredDomains)
	}
	if !reflect.DeepEqual(cfg.LocalDomains, []string{"lan", "home.arpa"}) {
		t.Fatalf("LocalDomains = %#v", cfg.LocalDomains)
	}
}

func TestLoadRejectsInvalidGenesis(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.ini")

	content := `[ingest]
genesis = forever
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(configPath); err == nil {
		t.Fatal("Load returned nil error for invalid genesis")
	}
}

func TestLoadRejectsInvalidRuntimeAndLogMode(t *testing.T) {
	tests := map[string]string{
		"runtime": "[ingest]\nruntime_mode = turbo\n",
		"log":     "[ingest]\nlog_mode = chatty\n",
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			configPath := filepath.Join(dir, "config.ini")
			if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(configPath); err == nil {
				t.Fatal("Load returned nil error")
			}
		})
	}
}
