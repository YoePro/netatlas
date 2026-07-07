package arpscout

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strings"
	"time"
)

type PassiveOptions struct {
	Interfaces        []string
	IncludeIncomplete bool
	Now               time.Time
}

func ReadPassive(options PassiveOptions) ([]Observation, error) {
	if len(options.Interfaces) == 0 {
		return readInterface("", options)
	}

	var observations []Observation
	for _, iface := range options.Interfaces {
		items, err := readInterface(iface, options)
		if err != nil {
			return nil, err
		}
		observations = append(observations, items...)
	}
	return observations, nil
}

func readInterface(iface string, options PassiveOptions) ([]Observation, error) {
	args := []string{"neigh", "show"}
	if strings.TrimSpace(iface) != "" {
		args = append(args, "dev", iface)
	}

	cmd := exec.Command("ip", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("run ip %s: %w", strings.Join(args, " "), err)
	}
	return ParseIPNeigh(strings.NewReader(string(out)), options)
}

func ParseIPNeigh(r io.Reader, options PassiveOptions) ([]Observation, error) {
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}

	scanner := bufio.NewScanner(r)
	var observations []Observation
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		observation, ok := parseIPNeighLine(line, now)
		if !ok {
			continue
		}
		if observation.MAC == "" && !options.IncludeIncomplete {
			continue
		}
		if strings.EqualFold(observation.State, "INCOMPLETE") && !options.IncludeIncomplete {
			continue
		}
		if len(options.Interfaces) > 0 && !interfaceAllowed(observation.Interface, options.Interfaces) {
			continue
		}
		EnrichObservation(&observation)
		observations = append(observations, observation)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read ip neigh output: %w", err)
	}
	return observations, nil
}

func parseIPNeighLine(line string, observed time.Time) (Observation, bool) {
	fields := strings.Fields(line)
	if len(fields) == 0 || net.ParseIP(fields[0]) == nil {
		return Observation{}, false
	}

	observation := Observation{
		IP:       fields[0],
		Source:   "passive_neigh",
		Observed: observed,
	}

	for i := 1; i < len(fields); i++ {
		switch fields[i] {
		case "dev":
			if i+1 < len(fields) {
				observation.Interface = fields[i+1]
				i++
			}
		case "lladdr":
			if i+1 < len(fields) {
				observation.MAC = strings.ToLower(fields[i+1])
				i++
			}
		default:
			if isNeighborState(fields[i]) {
				observation.State = strings.ToUpper(fields[i])
			}
		}
	}

	return observation, true
}

func isNeighborState(value string) bool {
	switch strings.ToUpper(value) {
	case "INCOMPLETE", "REACHABLE", "STALE", "DELAY", "PROBE", "FAILED", "NOARP", "PERMANENT":
		return true
	default:
		return false
	}
}

func interfaceAllowed(iface string, allowed []string) bool {
	for _, value := range allowed {
		if strings.TrimSpace(value) == iface {
			return true
		}
	}
	return false
}
