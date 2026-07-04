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

func TestPrintHelpMentionsCoreUsage(t *testing.T) {
	var buf bytes.Buffer
	printHelp(&buf)
	output := buf.String()

	for _, want := range []string{
		"dnslog help",
		"dnslog queries",
		"Reader -> Parser workers",
		"config.example.ini",
		"DNSLOG_DRY_RUN",
		"(:DnsServer)-[:OBSERVED]->(:DnsEvent)",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("help output missing %q", want)
		}
	}
}
