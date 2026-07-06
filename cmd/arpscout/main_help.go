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
  arpscout info
  arpscout identity [flags]
  arpscout passive [flags]

Commands:
  info
      Print sensor role, discovery modes, and initial event types as JSON.

  identity
      Print stable sensor identity, hostname, site, interfaces, version, and capabilities as JSON.

  passive
      Read Linux neighbour table with "ip neigh show" and print normalized ARP observations as JSON.

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

Notes:
  arpscout currently performs passive discovery only.
  Active scans, local storage, change detection, vendor lookup, daemon mode, and core upload are roadmap items.
`)
}
