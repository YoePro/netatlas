# arpscout

`arpscout` is the first NetAtlas Layer 2 discovery sensor.

It currently supports passive discovery by reading the Linux neighbour table with `ip neigh show` and emitting normalized JSON observations.

## Build

```bash
go build -o bin/arpscout ./cmd/arpscout
```

## Commands

```bash
arpscout help
arpscout check -config config.ini
arpscout info
arpscout identity -config config.ini
arpscout passive
arpscout passive -iface eth0,wlan0
arpscout passive -include-incomplete
arpscout scan -dry-run -format text
arpscout scan 192.168.1.0/24 -dry-run -format text
arpscout scan -iface eth0 -dry-run
arpscout scan -all -dry-run -format text
arpscout changes -previous prev.json -current current.json -format text
arpscout daemon -config config.ini
arpscout daemon -config config.ini -once
```

## Transport

`arpscout daemon` can upload observations to NetAtlas Core when `[arpscout_transport] enabled = true`.
If Core is unavailable, batches are appended to the configured JSONL spool file.
No local database is used.

## Operational Notes

- Run `arpscout check -config config.ini` before enabling daemon mode on a new host.
- Active scans use conservative safety limits and refuse more than 1024 targets per scan.
- Daemon active scans require an explicit `scan_cidr` and at least a 1 minute scan interval.
- If Core is unavailable, transport falls back to the configured JSONL spool file.

## Current Scope

Implemented:

- ARP-0.1 Concept & Scope
- ARP-0.2 Passive ARP Reader
- ARP-0.3 Sensor Identity
- ARP-0.4 Active ARP Scan
- ARP-0.5 Vendor Identification
- ARP-0.5.1 MAC & Network Classification
- ARP-0.6 Change Detection
- ARP-0.7 Sensor Daemon
- ARP-0.8 Observation Transport
- ARP-0.9 Hardening & Operational Safety
- ARP-1.0 Stable ARP Sensor

Planned:

- Neo4J observation persistence
- Graph mapping
