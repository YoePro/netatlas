package parser

import "testing"

func TestParseSimpleLine(t *testing.T) {
	event, err := ParseLine("2026-07-03T12:00:00Z 192.168.1.10 Example.COM. a NOERROR 93.184.216.34", Options{
		Server: ServerMeta{
			Name: "dns-primary",
			Role: "primary",
		},
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
	if event.ResponseCode != "NOERROR" {
		t.Fatalf("ResponseCode = %q, want %q", event.ResponseCode, "NOERROR")
	}
	if event.AnswerIP != "93.184.216.34" {
		t.Fatalf("AnswerIP = %q, want %q", event.AnswerIP, "93.184.216.34")
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

	event, err := ParseLine(line, Options{Server: ServerMeta{Name: "dns-secondary", Role: "secondary"}})
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

func TestParseLineIgnoresReverseLookup(t *testing.T) {
	_, err := ParseLine("2026-07-03T12:00:00Z 192.168.1.10 10.1.168.192.in-addr.arpa. ptr", Options{
		Server:              ServerMeta{Name: "dns-primary", Role: "primary"},
		IgnoreReverseLookup: true,
	})
	if err != ErrIgnored {
		t.Fatalf("ParseLine error = %v, want %v", err, ErrIgnored)
	}
}

func TestParseLineIgnoresLocalDomains(t *testing.T) {
	_, err := ParseLine("2026-07-03T12:00:00Z 192.168.1.10 printer.lan. a", Options{
		Server:       ServerMeta{Name: "dns-primary", Role: "primary"},
		LocalDomains: []string{"lan"},
	})
	if err != ErrIgnored {
		t.Fatalf("ParseLine error = %v, want %v", err, ErrIgnored)
	}
}

func TestParseLineIgnoresExactDomains(t *testing.T) {
	_, err := ParseLine("2026-07-03T12:00:00Z 192.168.1.10 telemetry.example.com. a", Options{
		Server:         ServerMeta{Name: "dns-primary", Role: "primary"},
		IgnoredDomains: []string{"telemetry.example.com"},
	})
	if err != ErrIgnored {
		t.Fatalf("ParseLine error = %v, want %v", err, ErrIgnored)
	}
}

func TestParseLineIgnoresKnownBindNonQueryCategories(t *testing.T) {
	lines := []string{
		"04-Jul-2026 23:22:48.092 dnssec: info: validating chatgpt.com/A: no valid signature found",
		"04-Jul-2026 23:23:16.267 general: warning: zone kok.hemma/IN: dump failed: file not found",
		"04-Jul-2026 23:29:48.128 lame-servers: info: insecurity proof failed resolving 'cloudflare.net/DNSKEY/IN': 192.168.1.5#53",
	}

	for _, line := range lines {
		_, err := ParseLine(line, Options{Server: ServerMeta{Name: "dns-primary", Role: "primary"}})
		if err != ErrIgnored {
			t.Fatalf("ParseLine(%q) error = %v, want %v", line, err, ErrIgnored)
		}
	}
}
