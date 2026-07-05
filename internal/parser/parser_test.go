package parser

import (
	"testing"
	"time"
)

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

func TestExtractTimestamp(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		layout string
		want   string
	}{
		{
			name:   "simple",
			line:   "2026-07-03T12:00:00Z 192.168.1.10 Example.COM. a NOERROR 93.184.216.34",
			layout: time.RFC3339Nano,
			want:   "2026-07-03T12:00:00Z",
		},
		{
			name:   "bind",
			line:   "03-Jul-2026 12:00:00.123 queries: info: client @0x123 192.168.1.10#53000 (example.com): query: example.com IN A + (192.168.1.2)",
			layout: "2006-01-02T15:04:05.999",
			want:   "2026-07-03T12:00:00.123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ExtractTimestamp(tt.line)
			if !ok {
				t.Fatal("ExtractTimestamp returned ok=false")
			}
			if got.Format(tt.layout) != tt.want {
				t.Fatalf("timestamp = %s, want %s", got.Format(time.RFC3339Nano), tt.want)
			}
		})
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
	if event.QueryClass != "IN" {
		t.Fatalf("QueryClass = %q, want %q", event.QueryClass, "IN")
	}
	if event.SourceCategory != SourceCategoryQuery {
		t.Fatalf("SourceCategory = %q, want %q", event.SourceCategory, SourceCategoryQuery)
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
		"04-Jul-2026 23:29:48.128 resolver: info: (ns3.t-s2-msedge.net): query failed (timed out) for ns3.t-s2-msedge.net/IN/AAAA at query.c:7851",
		"04-Jul-2026 23:29:48.128 network: info: listening on IPv6 interface lo, ::1#53",
		"04-Jul-2026 23:29:48.128 network: info: no longer listening on 192.168.1.5#53",
		"04-Jul-2026 23:29:48.128 resolver: info: resolver priming query complete",
		"04-Jul-2026 23:29:48.128 notify: info: zone example.com/IN: notify from 192.0.2.1#53",
		"04-Jul-2026 23:29:48.128 config: info: automatic empty zone: 10.IN-ADDR.ARPA",
		"04-Jul-2026 23:29:48.128 zoneload: info: zone example.com/IN: loaded serial 1",
		"04-Jul-2026 23:29:48.128 trust-anchor-telemetry: info: validating ./DNSKEY: success",
		"04-Jul-2026 23:29:48.128 query-errors: info: client @0x7f00 192.168.1.20#53000 (example.com): rate limit drop for example.com/IN/A",
		"04-Jul-2026 23:29:48.128 default: info: Accepting TCP connection failed: socket is not connected",
		"04-Jul-2026 23:29:48.128 xfer-in: info: zone badrum.hemma/IN: Transfer started.",
		"04-Jul-2026 23:29:48.128 rate-limit: info: limit responses to 192.0.2.1/24",
	}

	for _, line := range lines {
		_, err := ParseLine(line, Options{Server: ServerMeta{Name: "dns-primary", Role: "primary"}})
		if err != ErrIgnored {
			t.Fatalf("ParseLine(%q) error = %v, want %v", line, err, ErrIgnored)
		}
	}
}

func TestIgnoredCategory(t *testing.T) {
	tests := map[string]string{
		"04-Jul-2026 23:29:48.128 network: info: listening on IPv6 interface lo, ::1#53":                                                           IgnoreCategoryNetwork,
		"04-Jul-2026 23:29:48.128 resolver: info: resolver priming query complete":                                                                 IgnoreCategoryResolver,
		"04-Jul-2026 23:29:48.128 notify: info: zone example.com/IN: notify from 192.0.2.1#53":                                                     IgnoreCategoryNotify,
		"04-Jul-2026 23:29:48.128 config: info: automatic empty zone: 10.IN-ADDR.ARPA":                                                             IgnoreCategoryConfig,
		"04-Jul-2026 23:29:48.128 zoneload: info: zone example.com/IN: loaded serial 1":                                                            IgnoreCategoryZoneload,
		"04-Jul-2026 23:29:48.128 trust-anchor-telemetry: info: validating ./DNSKEY: success":                                                      IgnoreCategoryBindNoise,
		"04-Jul-2026 23:29:48.128 query-errors: info: client @0x7f00 192.168.1.20#53000 (example.com): rate limit drop for example.com/IN/A":       IgnoreCategoryRateLimit,
		"04-Jul-2026 23:29:48.128 default: info: Accepting TCP connection failed: socket is not connected":                                         IgnoreCategorySocket,
		"04-Jul-2026 23:29:48.128 xfer-in: info: zone badrum.hemma/IN: Transfer started.":                                                          IgnoreCategoryXferIn,
		"04-Jul-2026 23:29:48.128 rate-limit: info: limit responses to 192.0.2.1/24":                                                               IgnoreCategoryRateLimit,
		"04-Jul-2026 23:29:48.128 resolver: info: (ns3.t-s2-msedge.net): query failed (timed out) for ns3.t-s2-msedge.net/IN/AAAA at query.c:7851": IgnoreCategoryTimeout,
	}

	for line, want := range tests {
		if got := IgnoredCategory(line); got != want {
			t.Fatalf("IgnoredCategory(%q) = %q, want %q", line, got, want)
		}
	}
}

func TestParseLineMarksInterestingBindRCodesAsNotable(t *testing.T) {
	lines := []string{
		"04-Jul-2026 23:29:48.128 resolver: info: (version.pdns): query failed (REFUSED) for version.pdns/CH/NS at query.c:5932",
		"04-Jul-2026 23:29:48.128 xfer-in: info: zone rpz/IN: refresh: unexpected rcode (SERVFAIL) from primary 192.168.1.5#53 (source 0.0.0.0#0)",
	}

	for _, line := range lines {
		_, err := ParseLine(line, Options{Server: ServerMeta{Name: "dns-primary", Role: "primary"}})
		if err != ErrNotable {
			t.Fatalf("ParseLine(%q) error = %v, want %v", line, err, ErrNotable)
		}
	}
}

func TestParseLineMarksSecurityDeniedCacheAsNotable(t *testing.T) {
	line := "04-Jul-2026 23:29:48.128 security: info: client @0x7f00 192.168.1.20#53000 (blocked.example): query (cache) 'blocked.example/A/IN' denied"

	_, err := ParseLine(line, Options{Server: ServerMeta{Name: "dns-primary", Role: "primary"}})
	if err != ErrNotable {
		t.Fatalf("ParseLine error = %v, want %v", err, ErrNotable)
	}
	if got := NotableCategory(line); got != NotableCategorySecurityDeniedCache {
		t.Fatalf("NotableCategory = %q, want %q", got, NotableCategorySecurityDeniedCache)
	}
}

func TestParseBindQueryErrorLine(t *testing.T) {
	line := "04-Jul-2026 23:29:48.128 query-errors: info: client @0x7f00 192.168.1.20#53000 (version.pdns): query failed (REFUSED) for version.pdns/CH/NS at query.c:5932"

	event, err := ParseLine(line, Options{Server: ServerMeta{Name: "dns-primary", Role: "primary"}})
	if err != nil {
		t.Fatalf("ParseLine returned error: %v", err)
	}
	if event.ClientIP != "192.168.1.20" {
		t.Fatalf("ClientIP = %q, want %q", event.ClientIP, "192.168.1.20")
	}
	if event.QueryName != "version.pdns" {
		t.Fatalf("QueryName = %q, want %q", event.QueryName, "version.pdns")
	}
	if event.QueryClass != "CH" {
		t.Fatalf("QueryClass = %q, want %q", event.QueryClass, "CH")
	}
	if event.QueryType != "NS" {
		t.Fatalf("QueryType = %q, want %q", event.QueryType, "NS")
	}
	if event.ResponseCode != "REFUSED" {
		t.Fatalf("ResponseCode = %q, want %q", event.ResponseCode, "REFUSED")
	}
	if event.SourceCategory != SourceCategoryQueryErr {
		t.Fatalf("SourceCategory = %q, want %q", event.SourceCategory, SourceCategoryQueryErr)
	}
}
