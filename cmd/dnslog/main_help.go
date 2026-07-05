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
  - Can skip historical rows before a configured genesis boundary.
  - Can print opt-in runtime telemetry after ingestion.
  - Supports preflight checks, benchmark runs, progress output, and quiet cron mode.
  - Supports dry-run mode for local validation without touching Neo4J.

Usage:
  dnslog [flags]
  dnslog help
  dnslog queries
  dnslog queries <name>
  dnslog analytics
  dnslog analytics <name>
  dnslog system-check
  dnslog preflight [flags]
  dnslog benchmark [flags]
  dnslog -h

Flags:
  -config string
        Path to INI config file. Defaults to config.ini.

  -input string
        Override the configured log input file.

  -offset-state string
        Override the offset state file. Use with care for temporary inputs.

  -quiet
        Suppress normal output. Errors and notable final summaries remain visible.

  -verbose
        Print verbose diagnostics and enable telemetry.

  -benchmark-workers string
        Worker counts for dnslog benchmark. Supports comma lists and ranges, for example 1,2,4 or 1-4.

  -debug
        Print debug logging, including dry-run batch details.

  -telemetry
        Print runtime telemetry after ingestion.

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
    retry_delay, offset_state_path, dry_run, dry_run_updates_offset,
    runtime_mode, log_mode, genesis, genesis_after, progress_interval,
    parse_failure_samples, parse_failure_path

  [filter]
    ignore_reverse_lookup, ignored_domains, local_domains

  [fingerprints]
    rules_path

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
  DNSLOG_DRY_RUN_UPDATES_OFFSET
  DNSLOG_RUNTIME_MODE
  DNSLOG_LOG_MODE
  DNSLOG_GENESIS
  DNSLOG_GENESIS_AFTER
  DNSLOG_PROGRESS_INTERVAL
  DNSLOG_PARSE_FAILURE_SAMPLES
  DNSLOG_PARSE_FAILURE_PATH
  DNSLOG_FINGERPRINT_RULES_PATH
  DNSLOG_IGNORE_REVERSE_LOOKUP
  DNSLOG_IGNORED_DOMAINS
  DNSLOG_LOCAL_DOMAINS

Pipeline:
  Reader -> Parser workers -> Ordered offset commit -> Batch writer -> Neo4J

Input overrides:
  -input reads an alternate log file without editing config.ini.
  Without -offset-state, dnslog stores that input's offset in:
      state/<input-basename>-dnslog-offset.json
  If that state filename already belongs to a different input source, the old state
  is moved aside to a unique collision filename before the new input starts.
  -offset-state disables the automatic input-derived state path.

Graph model:
  (:Device)-[:HAS_CLIENT]->(:Client)
  (:DnsServer)-[:OBSERVED]->(:DnsEvent)
  (:Client)-[:ASKED]->(:DnsEvent)
  (:DnsEvent)-[:FOR_DOMAIN]->(:Domain)
  (:DnsEvent)-[:QUERY_TYPE]->(:QueryType)
  (:DnsEvent)-[:ANSWERED_WITH]->(:IpAddress)
  (:Client)-[:QUERIED]->(:Domain)
  Future enrichment:
  (:Device)-[:LIKELY_RUNNING]->(:OperatingSystem)
  (:Device)-[:LIKELY_IS]->(:DeviceType)
  (:Device)-[:LIKELY_HAS]->(:Software)
  (:Device)-[:LIKELY_INFRASTRUCTURE]->(:InfrastructureRole)
  (:Device)-[:LIKELY_VENDOR]->(:Vendor)

Aggregate relationship properties:
  count, firstSeen, lastSeen, nxCount, queryTypes, serverSeenOn, lastResponseCode

Telemetry:
  When -telemetry is set, dnslog reports total time, parse throughput, write throughput,
  skipped-by-genesis rows, peak memory, CPU time, CPU utilization, and batch efficiency.

Runtime modes:
  runtime_mode=max runs at configured throughput.
  runtime_mode=medium adds light cooperative pauses.
  runtime_mode=low adds stronger cooperative pauses for small machines.

Log modes:
  log_mode=quiet suppresses normal progress and boring successful summaries.
  log_mode=normal prints normal summaries.
  log_mode=verbose includes detailed parser failures and telemetry.

Summary counters:
  skipped_genesis counts rows older than the configured genesis boundary.
  notable counts non-query BIND lines with interesting rcodes such as REFUSED or SERVFAIL.
  notable_by_category splits notable lines into rcode and security_denied_cache.
  ignored_by_category splits ignored BIND noise into bind_noise, config, network, notify,
  rate_limit, resolver, socket, timeout, xfer_in, zoneload, and filtered.

Operational queries:
  dnslog queries
      Print all built-in Cypher validation queries.

  dnslog queries top-client-domain
      Print one named Cypher query.

Analytics reports:
  dnslog analytics
      Print all built-in Cypher analytics reports.

  dnslog analytics top-clients
      Print one named analytics report.

System commands:
  dnslog system-check
      Print detected CPU/memory/architecture and recommended ingest settings.

  dnslog preflight -config config.ini
      Check config, log file readability, offset state writability, and Neo4J/schema when dry_run=false.

  dnslog benchmark -config config.ini -input sample-normal.log
      Run a dry-run benchmark with telemetry and without moving offset state.

  dnslog benchmark -config config.ini -input sample-normal.log -benchmark-workers 1-4
      Run comparable benchmark passes for worker_count 1 through 4.

Examples:
  dnslog help
  dnslog queries
  dnslog analytics
  dnslog system-check
  dnslog preflight -config config.ini
  dnslog benchmark -config config.ini -input sample-normal.log
  dnslog benchmark -config config.ini -input sample-normal.log -benchmark-workers 1,2,4
  dnslog -config config.ini
  dnslog -config config.ini -input sample-normal.log
  dnslog -config config.ini -input named.log.1 -offset-state state/named-log-1.offset.json
  dnslog -config config.ini -quiet
  dnslog -config config.ini -verbose
  dnslog -config config.ini -debug
  dnslog -config config.ini -telemetry
  DNSLOG_DRY_RUN=true dnslog -config config.ini

Notes:
  config.ini is intended to stay local because it can contain credentials.
  Use config.example.ini as the tracked template.
  dry_run=true never updates the persistent offset state.
  dry_run_updates_offset=true is available for explicit test workflows, but defaults to false.
  When dry_run=false, the offset state directory is created automatically with owner-only permissions.
  genesis defaults to all. Supported values are all, today, 3h, 24h, 7d, 30d, and custom.
  genesis=custom uses genesis_after formatted as RFC3339, for example 2026-07-05T00:00:00+02:00.
  Parse failure samples are appended to parse_failure_path and capped by parse_failure_samples per run.
  Fingerprint rules are loaded from [fingerprints] rules_path when present; otherwise built-in DNS rules are used.
`)
}
