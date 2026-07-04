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
