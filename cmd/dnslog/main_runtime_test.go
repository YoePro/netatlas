package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dnslog/internal/config"
)

func TestParseFailureSamplerWritesSamples(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "parse-failures.log")
	parseFailures = newParseFailureSampler()

	cfg := &config.Config{
		ParseFailureSamples: 2,
		ParseFailurePath:    path,
	}
	recordParseFailure(cfg, errTestParseFailure{}, "unsupported one")
	recordParseFailure(cfg, errTestParseFailure{}, "unsupported one")
	recordParseFailure(cfg, errTestParseFailure{}, "unsupported two")
	recordParseFailure(cfg, errTestParseFailure{}, "unsupported three")

	if err := flushParseFailures(cfg); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	output := string(data)
	if strings.Count(output, "unsupported") != 2 {
		t.Fatalf("sample count output = %q", output)
	}
	if strings.Contains(output, "unsupported three") {
		t.Fatalf("sample limit was not respected: %q", output)
	}
}

type errTestParseFailure struct{}

func (errTestParseFailure) Error() string {
	return "test parse failure"
}
