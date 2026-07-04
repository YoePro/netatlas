package parser

import "testing"

func TestParseSimpleLine(t *testing.T) {
	event, err := ParseLine("2026-07-03T12:00:00Z 192.168.1.10 Example.COM. a", ServerMeta{
		Name: "dns-primary",
		Role: "primary",
	})
	if err != nil {
		t.Fatalf("ParseLine returned error: %v", err)
	}

	if event.QueryName != "example.com" {
		t.Fatalf("QueryName = %q, want %q", event.QueryName, "example.com")
	}
	if event.QueryType != "A" {
		t.Fatalf("QueryType = %q, want %q", event.QueryType, "A")
	}
	if event.ServerName != "dns-primary" || event.ServerRole != "primary" {
		t.Fatalf("server metadata = %q/%q", event.ServerName, event.ServerRole)
	}
	if event.RawHash == "" {
		t.Fatal("RawHash is empty")
	}
}

func TestParseBindQueryLine(t *testing.T) {
	line := "04-Jul-2026 22:13:01.123 queries: info: client @0x7f00 192.168.1.10#53111 (Example.COM): query: Example.COM IN AAAA +E(0)K (192.168.1.1)"

	event, err := ParseLine(line, ServerMeta{Name: "dns-secondary", Role: "secondary"})
	if err != nil {
		t.Fatalf("ParseLine returned error: %v", err)
	}

	if event.ClientIP != "192.168.1.10" {
		t.Fatalf("ClientIP = %q, want %q", event.ClientIP, "192.168.1.10")
	}
	if event.QueryName != "example.com" {
		t.Fatalf("QueryName = %q, want %q", event.QueryName, "example.com")
	}
	if event.QueryType != "AAAA" {
		t.Fatalf("QueryType = %q, want %q", event.QueryType, "AAAA")
	}
}
