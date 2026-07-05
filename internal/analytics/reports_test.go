package analytics

import (
	"bytes"
	"strings"
	"testing"
)

func TestReportsCoverBasicAnalyticsScope(t *testing.T) {
	want := []string{
		"top-devices",
		"device-enrichment-summary",
		"top-clients",
		"top-domains",
		"top-nxdomain-clients",
		"top-nxdomain-domains",
		"new-domains-24h",
		"single-client-domains",
		"secondary-heavy-clients",
		"client-query-increase",
		"domain-query-increase",
		"first-last-seen",
	}

	for _, name := range want {
		report, ok := FindReport(name)
		if !ok {
			t.Fatalf("FindReport(%q) = false", name)
		}
		if strings.TrimSpace(report.Cypher) == "" {
			t.Fatalf("report %q has empty Cypher", name)
		}
	}
}

func TestPrintReportsPrintsNamedReport(t *testing.T) {
	var buf bytes.Buffer
	if err := PrintReports(&buf, "top-clients"); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	for _, want := range []string{"# top-clients", "QUERIED", "queries"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestPrintReportsReturnsErrorForUnknownName(t *testing.T) {
	var buf bytes.Buffer
	if err := PrintReports(&buf, "missing"); err == nil {
		t.Fatal("PrintReports returned nil error for unknown report")
	}
}
