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
arpscout info
arpscout identity -config config.ini
arpscout passive
arpscout passive -iface eth0,wlan0
arpscout passive -include-incomplete
```

## Current Scope

Implemented:

- ARP-0.1 Concept & Scope
- ARP-0.2 Passive ARP Reader
- ARP-0.3 Sensor Identity

Planned:

- Neo4J observation persistence
- Active ARP scan
- Change detection
- Vendor lookup
- Daemon mode
- NetAtlas core upload
- Graph mapping
