package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestWantsHelp(t *testing.T) {
	for _, args := range [][]string{{"help"}, {"-h"}, {"--help"}} {
		if !wantsHelp(args) {
			t.Fatalf("wantsHelp(%v) = false, want true", args)
		}
	}

	if wantsHelp([]string{"-config", "config.ini"}) {
		t.Fatal("wantsHelp returned true for normal config flags")
	}
}

func TestWantsQueries(t *testing.T) {
	for _, args := range [][]string{{"queries"}, {"queries", "top-client-domain"}} {
		if !wantsQueries(args) {
			t.Fatalf("wantsQueries(%v) = false, want true", args)
		}
	}

	if wantsQueries([]string{"help"}) {
		t.Fatal("wantsQueries returned true for help")
	}
}

func TestWantsAnalytics(t *testing.T) {
	for _, args := range [][]string{{"analytics"}, {"analytics", "top-clients"}} {
		if !wantsAnalytics(args) {
			t.Fatalf("wantsAnalytics(%v) = false, want true", args)
		}
	}

	if wantsAnalytics([]string{"queries"}) {
		t.Fatal("wantsAnalytics returned true for queries")
	}
}

func TestWantsSystemCheck(t *testing.T) {
	if !wantsSystemCheck([]string{"system-check"}) {
		t.Fatal("wantsSystemCheck returned false")
	}
	if wantsSystemCheck([]string{"benchmark"}) {
		t.Fatal("wantsSystemCheck returned true for benchmark")
	}
}

func TestExtractCommand(t *testing.T) {
	command, args := extractCommand([]string{"benchmark", "-config", "config.ini"})
	if command != "benchmark" {
		t.Fatalf("command = %q, want benchmark", command)
	}
	if len(args) != 2 || args[0] != "-config" || args[1] != "config.ini" {
		t.Fatalf("args = %#v", args)
	}
}

func TestPrintOperationalQueries(t *testing.T) {
	var buf bytes.Buffer
	if err := printOperationalQueries(&buf, []string{"queries", "top-client-domain"}); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !strings.Contains(output, "# top-client-domain") {
		t.Fatalf("query output missing top-client-domain: %s", output)
	}
	if !strings.Contains(output, "QUERIED") {
		t.Fatalf("query output missing QUERIED: %s", output)
	}
}

func TestPrintAnalyticsReports(t *testing.T) {
	var buf bytes.Buffer
	if err := printAnalyticsReports(&buf, []string{"analytics", "top-clients"}); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !strings.Contains(output, "# top-clients") {
		t.Fatalf("analytics output missing top-clients: %s", output)
	}
	if !strings.Contains(output, "QUERIED") {
		t.Fatalf("analytics output missing QUERIED: %s", output)
	}
}

func TestPrintHelpMentionsCoreUsage(t *testing.T) {
	var buf bytes.Buffer
	printHelp(&buf)
	output := buf.String()

	for _, want := range []string{
		"dnslog help",
		"dnslog queries",
		"dnslog analytics",
		"dnslog system-check",
		"dnslog preflight",
		"dnslog benchmark",
		"-input",
		"-offset-state",
		"-quiet",
		"-verbose",
		"-benchmark-workers",
		"-debug",
		"-telemetry",
		"Reader -> Parser workers",
		"config.example.ini",
		"DNSLOG_DRY_RUN",
		"DNSLOG_GENESIS",
		"DNSLOG_FINGERPRINT_RULES_PATH",
		"skipped_genesis",
		"(:Device)-[:HAS_CLIENT]->(:Client)",
		"(:DnsServer)-[:OBSERVED]->(:DnsEvent)",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("help output missing %q", want)
		}
	}
}
