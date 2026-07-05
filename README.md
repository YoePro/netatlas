# NetAtlas

NetAtlas is a graph-based LAN observability platform.

Instead of storing isolated log files, NetAtlas correlates observations from multiple network sources into a continuously evolving knowledge graph built on Neo4J.

The project is designed for private LAN environments and focuses on network understanding, infrastructure optimization, troubleshooting, and security analysis.

NetAtlas is **not** intended to become a general-purpose log archive or a surveillance platform.

## Design principles

- Correlate observations from multiple sources.
- Build knowledge rather than collect logs.
- Make conclusions explainable.
- Keep resource usage low.
- Preserve privacy whenever practical.

## Current collectors

- dnslog

## Planned collectors

- dhcplog
- arplog
- fail2banlog
- halog (Home Assistant)
- firewalllog

## Planned observation sources

- DNS
- DHCP
- ARP
- Home Assistant
- Fail2ban
- Firewall logs
- mDNS
- SSDP
- LLDP

## Technology

- Go
- Neo4J

## Project status

Early development.

The first collector (`dnslog`) is under active development and provides the foundation for future observation sources.

## License

License to be determined.
