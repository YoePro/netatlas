package main

import (
	"fmt"
	"strings"
	"time"

	"netatlas/internal/config"
	"netatlas/internal/parser"
)

type genesisFilter struct {
	enabled bool
	after   time.Time
}

func newGenesisFilter(cfg *config.Config, now time.Time) (genesisFilter, error) {
	mode := strings.ToLower(strings.TrimSpace(cfg.Genesis))
	if mode == "" {
		mode = "all"
	}

	switch mode {
	case "all":
		return genesisFilter{}, nil
	case "today":
		localNow := now.In(time.Local)
		after := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, time.Local)
		return genesisFilter{enabled: true, after: after}, nil
	case "3h":
		return genesisFilter{enabled: true, after: now.Add(-3 * time.Hour)}, nil
	case "24h":
		return genesisFilter{enabled: true, after: now.Add(-24 * time.Hour)}, nil
	case "7d":
		return genesisFilter{enabled: true, after: now.Add(-7 * 24 * time.Hour)}, nil
	case "30d":
		return genesisFilter{enabled: true, after: now.Add(-30 * 24 * time.Hour)}, nil
	case "custom":
		after, err := time.Parse(time.RFC3339, strings.TrimSpace(cfg.GenesisAfter))
		if err != nil {
			return genesisFilter{}, fmt.Errorf("parse genesis_after: %w", err)
		}
		return genesisFilter{enabled: true, after: after}, nil
	default:
		return genesisFilter{}, fmt.Errorf("unsupported genesis value %q", cfg.Genesis)
	}
}

func (f genesisFilter) shouldSkip(line string) bool {
	if !f.enabled {
		return false
	}
	timestamp, ok := parser.ExtractTimestamp(line)
	if !ok {
		return false
	}
	return timestamp.Before(f.after)
}
