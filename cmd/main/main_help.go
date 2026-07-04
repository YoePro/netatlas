package main

import (
	"fmt"
	"io"
)

func printHelp(w io.Writer) {
	fmt.Fprint(w, `dnslog

Purpose:
  dnslog reads DNS query logs from one DNS server, normalizes each event,
  enriches it with server metadata, and writes structured observations to Neo4J.

Current scope:
  - Reads one configured DNS log file and exits when current content is consumed.
  - Maintains a persistent offset state for restart-safe ingestion.
  - Detects log rotation by file identity and resumes safely.
  - Parses DNS events in parallel and writes batches to Neo4J.
  - Creates the initial Neo4J graph schema when dry_run is false.
  - Maintains aggregate Client-to-Domain query relationships.
  - Prints performance baseline metrics after each ingestion run.
  - Supports dry-run mode for local validation without touching Neo4J.

Usage:
  dnslog [flags]
  dnslog help
  dnslog queries
  dnslog queries <name>
  dnslog -h

Flags:
  -config string
        Path to INI config file. Defaults to config.ini.

Configuration:
  The config file is INI based. See config.example.ini for a complete example.

  [neo4j]
    uri, user, password, database

  [server]
    name, role

  [log]
    file_path

  [ingest]
    batch_size, worker_count, flush_interval, max_write_retries,
    retry_delay, offset_state_path, dry_run

  [filter]
    ignore_reverse_lookup, ignored_domains, local_domains

Environment overrides:
  DNSLOG_NEO4J_URI
  DNSLOG_NEO4J_USER
  DNSLOG_NEO4J_PASSWORD
  DNSLOG_NEO4J_DATABASE
  DNSLOG_FILE_PATH
  DNSLOG_SERVER_NAME
  DNSLOG_SERVER_ROLE
  DNSLOG_BATCH_SIZE
  DNSLOG_WORKER_COUNT
  DNSLOG_FLUSH_INTERVAL
  DNSLOG_MAX_WRITE_RETRIES
  DNSLOG_RETRY_DELAY
  DNSLOG_OFFSET_STATE_PATH
  DNSLOG_DRY_RUN
  DNSLOG_IGNORE_REVERSE_LOOKUP
  DNSLOG_IGNORED_DOMAINS
  DNSLOG_LOCAL_DOMAINS

Pipeline:
  Reader -> Parser workers -> Ordered offset commit -> Batch writer -> Neo4J

Graph model:
  (:DnsServer)-[:OBSERVED]->(:DnsEvent)
  (:Client)-[:ASKED]->(:DnsEvent)
  (:DnsEvent)-[:FOR_DOMAIN]->(:Domain)
  (:DnsEvent)-[:QUERY_TYPE]->(:QueryType)
  (:DnsEvent)-[:ANSWERED_WITH]->(:IpAddress)
  (:Client)-[:QUERIED]->(:Domain)

Aggregate relationship properties:
  count, firstSeen, lastSeen, nxCount, queryTypes, serverSeenOn, lastResponseCode

Performance baseline:
  Every ingestion run reports total time, parse throughput, write throughput,
  peak memory, CPU time, CPU utilization, and batch efficiency.

Operational queries:
  dnslog queries
      Print all built-in Cypher validation queries.

  dnslog queries top-client-domain
      Print one named Cypher query.

Examples:
  dnslog help
  dnslog queries
  dnslog -config config.ini
  DNSLOG_DRY_RUN=true dnslog -config config.ini

Notes:
  config.ini is intended to stay local because it can contain credentials.
  Use config.example.ini as the tracked template.
`)
}
