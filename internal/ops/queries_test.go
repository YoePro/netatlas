package ops

import (
	"bytes"
	"strings"
	"testing"
)

func TestOperationalQueriesCoverValidationScope(t *testing.T) {
	want := []string{
		"events-per-server",
		"unique-clients",
		"unique-devices",
		"device-client-map",
		"unenriched-devices",
		"merge-device-os-evidence",
		"merge-device-type-evidence",
		"merge-device-software-evidence",
		"merge-device-infrastructure-evidence",
		"merge-device-vendor-evidence",
		"unique-domains",
		"newest-events",
		"top-client-domain",
		"duplicate-protection",
		"primary-secondary-split",
		"aggregate-vs-events",
		"top-nxdomain",
	}

	for _, name := range want {
		query, ok := FindQuery(name)
		if !ok {
			t.Fatalf("FindQuery(%q) = false", name)
		}
		if strings.TrimSpace(query.Cypher) == "" {
			t.Fatalf("query %q has empty Cypher", name)
		}
	}
}

func TestEnrichmentQueriesPreserveEvidence(t *testing.T) {
	query, ok := FindQuery("merge-device-os-evidence")
	if !ok {
		t.Fatal("missing merge-device-os-evidence")
	}
	for _, want := range []string{
		"LIKELY_RUNNING",
		"confidence",
		"score",
		"evidenceCount",
		"evidenceHashes",
	} {
		if !strings.Contains(query.Cypher, want) {
			t.Fatalf("enrichment query missing %q", want)
		}
	}
}

func TestPrintQueriesPrintsAllQueries(t *testing.T) {
	var buf bytes.Buffer
	if err := PrintQueries(&buf, ""); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	for _, want := range []string{"# events-per-server", "# top-client-domain", "MATCH"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q", want)
		}
	}
}

func TestPrintQueriesReturnsErrorForUnknownName(t *testing.T) {
	var buf bytes.Buffer
	if err := PrintQueries(&buf, "missing"); err == nil {
		t.Fatal("PrintQueries returned nil error for unknown query")
	}
}
