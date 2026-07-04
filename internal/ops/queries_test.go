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
