package parser

import (
	"errors"
	"net"
	"strings"
	"time"

	"dnslog/internal/model"
	"dnslog/internal/util"
)

var (
	ErrIgnored     = errors.New("ignored log line")
	ErrUnsupported = errors.New("unsupported log format")
)

type ServerMeta struct {
	Name string
	Role string
}

func ParseLine(line string, server ServerMeta) (model.DNSEvent, error) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return model.DNSEvent{}, ErrIgnored
	}

	if event, ok := parseSimple(line, server); ok {
		return event, nil
	}
	if event, ok := parseBindQuery(line, server); ok {
		return event, nil
	}

	return model.DNSEvent{}, ErrUnsupported
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

	return baseEvent(line, timestamp, server, parts[1], parts[2], parts[3]), true
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
	if len(parts) > queryIdx+4 {
		event.Protocol = protocolFromFlags(parts[queryIdx+4:])
	}

	return event, true
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
