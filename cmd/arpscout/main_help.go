package main

import (
	"fmt"
	"io"
)

func printHelp(w io.Writer) {
	fmt.Fprint(w, `arpscout

Local Layer 2 discovery sensor for NetAtlas.

Usage:
  arpscout help
  arpscout check [flags]
  arpscout changes [flags]
  arpscout daemon [flags]
  arpscout info
  arpscout identity [flags]
  arpscout passive [flags]
  arpscout scan [flags]

Commands:
  check
      Validate config, scan safety, interface names, and active scan permissions.

  changes
      Compare two observation JSON files and emit detected change events.

  daemon
      Run continuously with periodic passive reads, optional active scans, and runtime status.

  info
      Print sensor role, discovery modes, and initial event types as JSON.

  identity
      Print stable sensor identity, hostname, site, interfaces, version, and capabilities as JSON.

  passive
      Read Linux neighbour table with "ip neigh show" and print normalized ARP observations as JSON.

  scan
      Actively scan a CIDR or discovered local LAN target with ARP requests.

Observations:
  passive and scan output include vendor, MAC classification, and network classification.
  vendor is null when OUI lookup is not applicable, for example local or multicast MAC addresses.

Check flags:
  -config string
      Path to NetAtlas INI config. Defaults to config.ini.

  -format string
      Output format: text or json. Defaults to text.

Changes flags:
  -previous string
      Previous observation JSON file.

  -current string
      Current observation JSON file.

  -gateway string
      Gateway IP to monitor for MAC changes.

  -format string
      Output format: json or text. Defaults to json.

Daemon flags:
  -config string
      Path to NetAtlas INI config. Defaults to config.ini.

  -iface string
      Comma-separated interfaces to read.

  -interval duration
      Passive read interval. Config key: passive_interval.

  -status-interval duration
      Runtime status log interval. Config key: status_interval.

  -include-incomplete
      Include INCOMPLETE neighbour entries.

  -gateway string
      Gateway IP to monitor for MAC changes.

  -active-scan
      Enable periodic active ARP scans.

  -scan-cidr string
      IPv4 CIDR for periodic active ARP scans.

  -scan-interval duration
      Periodic active scan interval.

  -max-scan-hosts int
      Maximum active scan targets.

  -format string
      Output format: text or json. Defaults to text.

  -once
      Run one daemon iteration and exit.

Transport config:
  [arpscout_transport] enabled=true sends daemon batches to NetAtlas Core.
  core_url, token, spool_path, dry_run_path, timeout, and retries are read from the config file.
  When Core is unavailable, observation batches are appended to the file spool.

Identity flags:
  -config string
      Path to NetAtlas INI config. Defaults to config.ini.

  -iface string
      Comma-separated interfaces to include in identity, overriding [arpscout] interfaces.

Passive flags:
  -iface string
      Comma-separated interfaces to read or filter, for example eth0,wlan0.

  -include-incomplete
      Include INCOMPLETE neighbour entries. They are ignored by default.

Scan flags:
  -cidr string
      IPv4 CIDR to scan, for example 192.168.1.0/24. You can also pass the CIDR as a positional argument.

  -iface string
      Interface to send ARP requests on, or interface to use for target discovery when CIDR is omitted.

  -all
      Scan all discovered networks, including normally skipped virtual or VPN networks.

  -dry-run
      Plan the scan and print target IPs without sending ARP requests.

  -max-hosts int
      Maximum number of targets allowed in one scan. Defaults to 256.

  -timeout duration
      Per-target arping timeout. Defaults to 1s.

  -interval duration
      Delay between ARP requests. Defaults to 25ms.

  -format string
      Output format: json or text. Defaults to json.

  -debug
      Include debug details such as full planned target lists.

Notes:
  arpscout scan shells out to "arping" for active discovery.
  scan without CIDR auto-selects a normal physical LAN target and skips loopback, virtual bridges, VPN and container networks by default.
  daemon keeps change state in memory only; it does not write a local database.
  transport uses HTTP and a JSONL file spool; it does not use a local database.
`)
}
