# NetAtlas

NetAtlas is a LAN observability project for turning local network signals into a useful graph of devices, services, and risk-relevant behaviour.

The first collector is `dnslog`, a DNS log ingester that reads DNS server logs, normalizes query observations, and writes them to Neo4J. The current binary remains named `dnslog` while the wider project becomes NetAtlas.

DNS observations are attached to persistent `Device` entities. Today the first stable identity is based on client IP address; future collectors can add stronger identities from DHCP, ARP, Home Assistant, and other sources without replacing the DNS collector.

## What NetAtlas Is

- A local-first observability graph for your own network.
- A way to correlate DNS behaviour with future collectors such as DHCP, ARP, Home Assistant, firewall logs, and Fail2ban.
- A tool for operational understanding and risk analysis, such as identifying noisy devices, unusual domains, repeated failures, and stale or suspicious network behaviour.

## What NetAtlas Is Not

- Not a surveillance product.
- Not a browsing-history archive.
- Not intended to store endless raw logs.
- Not a DNS configuration manager.

NetAtlas should preserve useful network knowledge and evidence, not hoard private raw data.

## Current Collector: dnslog

Build:

```bash
go build -o bin/dnslog ./cmd/dnslog
```

Run help:

```bash
./bin/dnslog help
```

Useful commands:

```bash
./bin/dnslog preflight -config config.ini
./bin/dnslog benchmark -config config.ini -input sample-normal.log -benchmark-workers 1-4
./bin/dnslog queries
./bin/dnslog analytics
```

Device-oriented validation:

```bash
./bin/dnslog queries unique-devices
./bin/dnslog queries device-client-map
./bin/dnslog analytics top-devices
```

Fingerprinting:

NetAtlas loads DNS fingerprint rules from `[fingerprints] rules_path` when the file exists. If no custom file exists, built-in rules cover the first operating system, device type, infrastructure, software, and vendor signals. See `config/fingerprints.example.json` for the JSON shape.

## Configuration

Use `config.example.ini` as the tracked template. Keep `config.ini` local because it can contain credentials.

The default mode is safe for local validation:

- `dry_run = true`
- `dry_run_updates_offset = false`
- local state is written under `state/` only when explicitly enabled or when running non-dry-run ingestion

## Repository Notes

Generated binaries, logs, local credentials, and state are ignored by Git.

Before pushing changes:

```bash
go test ./...
go build -o bin/dnslog ./cmd/dnslog
```

## UI Server

Build and run the NetAtlas UI:

```bash
go build -o bin/netatlas ./cmd/netatlas
./bin/netatlas -config config.ini
```

The UI listens according to `[ui]` in the config file. The example config uses `0.0.0.0:8080`.
