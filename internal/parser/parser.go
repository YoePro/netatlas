package parser

import (
	"errors"
	"net"
	"regexp"
	"strings"
	"time"

	"netatlas/internal/model"
	"netatlas/internal/util"
)

const (
	IgnoreCategoryBindNoise            = "bind_noise"
	IgnoreCategoryConfig               = "config"
	IgnoreCategoryFiltered             = "filtered"
	IgnoreCategoryNetwork              = "network"
	IgnoreCategoryNotify               = "notify"
	IgnoreCategoryRateLimit            = "rate_limit"
	IgnoreCategoryResolver             = "resolver"
	IgnoreCategorySocket               = "socket"
	IgnoreCategoryTimeout              = "timeout"
	IgnoreCategoryXferIn               = "xfer_in"
	IgnoreCategoryXferOut              = "xfer_out"
	IgnoreCategoryZoneload             = "zoneload"
	NotableCategorySecurityDeniedCache = "security_denied_cache"
	SourceCategoryQuery                = "queries"
	SourceCategoryQueryErr             = "query-errors"
)

var (
	ErrIgnored     = errors.New("ignored log line")
	ErrNotable     = errors.New("notable bind log line")
	ErrUnsupported = errors.New("unsupported log format")
)

var queryFailedPattern = regexp.MustCompile(`\(([^)]+)\): query failed \(([^)]+)\) for ([^/]+)/([^/]+)/([^[:space:]]+)`)

type ServerMeta struct {
	Name string
	Role string
}

type Options struct {
	Server              ServerMeta
	IgnoreReverseLookup bool
	IgnoredDomains      []string
	LocalDomains        []string
}

func ParseLine(line string, opts Options) (model.DNSEvent, error) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return model.DNSEvent{}, ErrIgnored
	}
	if isBenignBindNoise(line) {
		return model.DNSEvent{}, ErrIgnored
	}
	if event, ok := parseBindQueryError(line, opts.Server); ok {
		return filterEvent(event, opts)
	}
	if isNotableBindLine(line) {
		return model.DNSEvent{}, ErrNotable
	}
	if isIgnoredBindCategory(line) {
		return model.DNSEvent{}, ErrIgnored
	}

	if event, ok := parseSimple(line, opts.Server); ok {
		return filterEvent(event, opts)
	}
	if event, ok := parseBindQuery(line, opts.Server); ok {
		return filterEvent(event, opts)
	}

	return model.DNSEvent{}, ErrUnsupported
}

func ExtractTimestamp(line string) (time.Time, bool) {
	parts := strings.Fields(strings.TrimSpace(line))
	if len(parts) == 0 {
		return time.Time{}, false
	}
	if timestamp, err := time.Parse(time.RFC3339, parts[0]); err == nil {
		return timestamp, true
	}
	return bindTimestamp(parts)
}

func isBenignBindNoise(line string) bool {
	return strings.Contains(line, "query failed (timed out)") ||
		strings.Contains(line, ": Transfer started.")
}

func parseSimple(line string, server ServerMeta) (model.DNSEvent, bool) {
	parts := strings.Fields(line)
	if len(parts) < 4 {
		return model.DNSEvent{}, false
	}

	timestamp, err := time.Parse(time.RFC3339, parts[0])
	if err != nil {
		return model.DNSEvent{}, false
	}

	event := baseEvent(line, timestamp, server, parts[1], parts[2], parts[3])
	event.SourceCategory = "simple"
	if len(parts) > 4 {
		event.ResponseCode = strings.ToUpper(parts[4])
	}
	if len(parts) > 5 {
		event.AnswerIP = parts[5]
	}

	return event, true
}

func isIgnoredBindCategory(line string) bool {
	return IgnoredCategory(line) != ""
}

func IgnoredCategory(line string) string {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return IgnoreCategoryBindNoise
	}
	if strings.Contains(line, "query failed (timed out)") {
		return IgnoreCategoryTimeout
	}
	if strings.Contains(line, "rate limit drop") {
		return IgnoreCategoryRateLimit
	}
	if strings.Contains(line, "Accepting TCP connection failed") {
		return IgnoreCategorySocket
	}
	if strings.Contains(line, ": Transfer started.") {
		return IgnoreCategoryXferIn
	}

	parts := strings.Fields(line)
	if len(parts) < 3 {
		return ""
	}

	switch parts[2] {
	case "dnssec:", "general:", "lame-servers:", "trust-anchor-telemetry:":
		return IgnoreCategoryBindNoise
	case "config:":
		return IgnoreCategoryConfig
	case "network:":
		return IgnoreCategoryNetwork
	case "notify:":
		return IgnoreCategoryNotify
	case "rate-limit:":
		return IgnoreCategoryRateLimit
	case "resolver:":
		return IgnoreCategoryResolver
	case "xfer-in:":
		return IgnoreCategoryXferIn
	case "xfer-out:":
		return IgnoreCategoryXferOut
	case "zoneload:":
		return IgnoreCategoryZoneload
	default:
		return ""
	}
}

func isNotableBindLine(line string) bool {
	return NotableCategory(line) != ""
}

func NotableCategory(line string) string {
	if strings.Contains(line, "security:") &&
		strings.Contains(line, "query (cache)") &&
		strings.Contains(line, "denied") {
		return NotableCategorySecurityDeniedCache
	}
	if containsNotableRCode(line, "REFUSED") || containsNotableRCode(line, "SERVFAIL") {
		return "rcode"
	}
	return ""
}

func containsNotableRCode(line string, rcode string) bool {
	return strings.Contains(line, "query failed ("+rcode+")") ||
		strings.Contains(line, "unexpected rcode ("+rcode+")")
}

func parseBindQuery(line string, server ServerMeta) (model.DNSEvent, bool) {
	parts := strings.Fields(line)
	if len(parts) < 12 {
		return model.DNSEvent{}, false
	}

	timestamp, ok := bindTimestamp(parts)
	if !ok {
		return model.DNSEvent{}, false
	}

	clientIdx := indexOf(parts, "client")
	queryIdx := indexOf(parts, "query:")
	if clientIdx < 0 || queryIdx < 0 || len(parts) <= clientIdx+2 || len(parts) <= queryIdx+3 {
		return model.DNSEvent{}, false
	}

	clientIP := clientAddress(parts[clientIdx+2])
	queryName := parts[queryIdx+1]
	queryType := parts[queryIdx+3]

	event := baseEvent(line, timestamp, server, clientIP, queryName, queryType)
	event.QueryClass = strings.ToUpper(parts[queryIdx+2])
	event.SourceCategory = SourceCategoryQuery
	if len(parts) > queryIdx+4 {
		event.Protocol = protocolFromFlags(parts[queryIdx+4:])
	}

	return event, true
}

func parseBindQueryError(line string, server ServerMeta) (model.DNSEvent, bool) {
	parts := strings.Fields(line)
	if len(parts) < 6 {
		return model.DNSEvent{}, false
	}
	timestamp, ok := bindTimestamp(parts)
	if !ok {
		return model.DNSEvent{}, false
	}

	if !strings.Contains(line, "query failed (") {
		return model.DNSEvent{}, false
	}
	matches := queryFailedPattern.FindStringSubmatch(line)
	if len(matches) != 6 {
		return model.DNSEvent{}, false
	}

	event := baseEvent(line, timestamp, server, "", matches[3], matches[5])
	event.ClientIP = queryErrorClientIP(parts)
	if event.ClientIP == "" {
		return model.DNSEvent{}, false
	}
	event.QueryClass = strings.ToUpper(matches[4])
	event.ResponseCode = strings.ToUpper(matches[2])
	event.SourceCategory = SourceCategoryQueryErr

	return event, true
}

func queryErrorClientIP(parts []string) string {
	clientIdx := indexOf(parts, "client")
	if clientIdx < 0 || len(parts) <= clientIdx+2 {
		return ""
	}
	return clientAddress(parts[clientIdx+2])
}

func bindTimestamp(parts []string) (time.Time, bool) {
	if len(parts) < 2 {
		return time.Time{}, false
	}

	value := parts[0] + " " + parts[1]
	layouts := []string{
		"02-Jan-2006 15:04:05.000",
		"02-Jan-2006 15:04:05",
	}

	for _, layout := range layouts {
		timestamp, err := time.ParseInLocation(layout, value, time.Local)
		if err == nil {
			return timestamp, true
		}
	}

	return time.Time{}, false
}

func baseEvent(line string, timestamp time.Time, server ServerMeta, clientIP, queryName, queryType string) model.DNSEvent {
	return model.DNSEvent{
		Timestamp:  timestamp,
		ServerName: server.Name,
		ServerRole: server.Role,
		ClientIP:   clientIP,
		QueryName:  normalizeDomain(queryName),
		QueryClass: "IN",
		QueryType:  strings.ToUpper(queryType),
		RawLine:    line,
		RawHash:    util.SHA256Hex(line),
	}
}

func normalizeDomain(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "()")
	value = strings.TrimSuffix(value, ".")
	return strings.ToLower(value)
}

func filterEvent(event model.DNSEvent, opts Options) (model.DNSEvent, error) {
	if shouldIgnoreDomain(event.QueryName, opts) {
		return model.DNSEvent{}, ErrIgnored
	}
	return event, nil
}

func shouldIgnoreDomain(domain string, opts Options) bool {
	if domain == "" {
		return true
	}
	if opts.IgnoreReverseLookup && isReverseLookup(domain) {
		return true
	}
	for _, ignored := range opts.IgnoredDomains {
		if domain == normalizeDomain(ignored) {
			return true
		}
	}
	for _, local := range opts.LocalDomains {
		local = normalizeDomain(local)
		if local == "" {
			continue
		}
		if domain == local || strings.HasSuffix(domain, "."+local) {
			return true
		}
	}
	return false
}

func isReverseLookup(domain string) bool {
	return domain == "in-addr.arpa" ||
		domain == "ip6.arpa" ||
		strings.HasSuffix(domain, ".in-addr.arpa") ||
		strings.HasSuffix(domain, ".ip6.arpa")
}

func clientAddress(value string) string {
	value = strings.Trim(value, "()")
	if host, _, err := net.SplitHostPort(value); err == nil {
		return host
	}
	if idx := strings.LastIndex(value, "#"); idx >= 0 {
		return value[:idx]
	}
	return value
}

func protocolFromFlags(flags []string) string {
	for _, flag := range flags {
		switch {
		case strings.Contains(flag, "T"):
			return "tcp"
		case strings.Contains(flag, "D"):
			return "udp"
		}
	}
	return ""
}

func indexOf(parts []string, needle string) int {
	for idx, part := range parts {
		if part == needle {
			return idx
		}
	}
	return -1
}
