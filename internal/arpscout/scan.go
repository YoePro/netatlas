package arpscout

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"net"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const SourceActiveScan = "active_scan"

const (
	DefaultScanTimeout  = time.Second
	DefaultScanInterval = 25 * time.Millisecond
	DefaultMaxScanHosts = 256
)

var macPattern = regexp.MustCompile(`(?i)([0-9a-f]{2}:){5}[0-9a-f]{2}`)

type ScanOptions struct {
	CIDR                  string
	Interface             string
	DryRun                bool
	MaxHosts              int
	Timeout               time.Duration
	Interval              time.Duration
	Now                   time.Time
	Runner                ScanRunner
	IncludePlannedTargets bool
}

type ScanRunner func(ctx context.Context, target string, options ScanOptions) ([]byte, error)

type ScanResult struct {
	CIDR           string         `json:"cidr"`
	Interface      string         `json:"interface,omitempty"`
	DryRun         bool           `json:"dry_run"`
	Plan           ScanPlan       `json:"plan"`
	Statistics     ScanStatistics `json:"statistics"`
	Discoveries    []Observation  `json:"discoveries,omitempty"`
	PlannedTargets []string       `json:"planned_targets,omitempty"`
	ScannedTargets int            `json:"-"`
	Observations   []Observation  `json:"-"`
	StartedAt      time.Time      `json:"started_at"`
	FinishedAt     time.Time      `json:"finished_at"`
}

type ScanPlan struct {
	CIDR        string `json:"cidr"`
	Interface   string `json:"interface,omitempty"`
	TargetCount int    `json:"target_count"`
	FirstTarget string `json:"first_target,omitempty"`
	LastTarget  string `json:"last_target,omitempty"`
	AddressType string `json:"address_type"`
	NetworkType string `json:"network_type"`
}

type ScanStatistics struct {
	TargetCount         int            `json:"target_count"`
	ScannedCount        int            `json:"scanned_count"`
	ReplyCount          int            `json:"reply_count"`
	Duration            string         `json:"duration"`
	NonResponsiveRanges []AddressRange `json:"non_responsive_ranges,omitempty"`
}

type AddressRange struct {
	First string `json:"first"`
	Last  string `json:"last,omitempty"`
	Count int    `json:"count"`
}

func RunActiveScan(options ScanOptions) (ScanResult, error) {
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	if options.MaxHosts <= 0 {
		options.MaxHosts = DefaultMaxScanHosts
	}
	if options.Timeout <= 0 {
		options.Timeout = DefaultScanTimeout
	}
	if options.Interval < 0 {
		options.Interval = 0
	}
	if err := ValidateScanOptions(options); err != nil {
		return ScanResult{}, err
	}

	targets, err := PlanScanTargets(options.CIDR, options.MaxHosts)
	if err != nil {
		return ScanResult{}, err
	}

	result := ScanResult{
		CIDR:      strings.TrimSpace(options.CIDR),
		Interface: strings.TrimSpace(options.Interface),
		DryRun:    options.DryRun,
		Plan:      BuildScanPlan(options.CIDR, options.Interface, targets),
		Statistics: ScanStatistics{
			TargetCount: len(targets),
			Duration:    "0s",
		},
		StartedAt:  now,
		FinishedAt: now,
	}
	if options.IncludePlannedTargets {
		result.PlannedTargets = targets
	}
	if options.DryRun {
		return result, nil
	}

	runner := options.Runner
	if runner == nil {
		runner = runArping
	}

	var observations []Observation
	for i, target := range targets {
		ctx, cancel := context.WithTimeout(context.Background(), options.Timeout+250*time.Millisecond)
		out, err := runner(ctx, target, options)
		cancel()
		result.ScannedTargets++
		if err == nil {
			observation, ok := ParseArpingOutput(target, out, now)
			if ok {
				observation.Interface = strings.TrimSpace(options.Interface)
				EnrichObservation(&observation)
				observations = append(observations, observation)
			}
		}
		if options.Interval > 0 && i < len(targets)-1 {
			time.Sleep(options.Interval)
		}
	}

	result.Observations = observations
	result.Discoveries = observations
	result.FinishedAt = time.Now()
	result.Statistics.ScannedCount = result.ScannedTargets
	result.Statistics.ReplyCount = len(observations)
	result.Statistics.Duration = result.FinishedAt.Sub(result.StartedAt).Round(time.Millisecond).String()
	result.Statistics.NonResponsiveRanges = CompressAddressRanges(nonResponsiveTargets(targets, observations))
	return result, nil
}

func BuildScanPlan(cidr, iface string, targets []string) ScanPlan {
	network := ClassifyNetwork(networkIP(cidr), iface)
	plan := ScanPlan{
		CIDR:        strings.TrimSpace(cidr),
		Interface:   strings.TrimSpace(iface),
		TargetCount: len(targets),
		AddressType: network.AddressType,
		NetworkType: network.NetworkType,
	}
	if len(targets) > 0 {
		plan.FirstTarget = targets[0]
		plan.LastTarget = targets[len(targets)-1]
	}
	return plan
}

func PlanScanTargets(cidr string, maxHosts int) ([]string, error) {
	cidr = strings.TrimSpace(cidr)
	if cidr == "" {
		return nil, fmt.Errorf("cidr is required")
	}
	if maxHosts <= 0 {
		return nil, fmt.Errorf("max hosts must be greater than zero")
	}

	ip, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("parse cidr %q: %w", cidr, err)
	}
	if ip.To4() == nil {
		return nil, fmt.Errorf("active ARP scan only supports IPv4 CIDR ranges")
	}

	ones, bits := network.Mask.Size()
	if bits != 32 {
		return nil, fmt.Errorf("active ARP scan only supports IPv4 CIDR ranges")
	}

	start := uint64(ipv4ToUint32(network.IP))
	size := uint64(1) << uint64(bits-ones)
	first := start
	last := start + size - 1
	if ones <= 30 && size > 2 {
		first++
		last--
	}

	count := int(last-first) + 1
	if count > maxHosts {
		return nil, fmt.Errorf("cidr %s contains %d scan targets, above safety limit %d", cidr, count, maxHosts)
	}

	targets := make([]string, 0, count)
	for value := first; value <= last; value++ {
		targets = append(targets, uint32ToIPv4(uint32(value)).String())
		if value == last {
			break
		}
	}
	return targets, nil
}

func ParseArpingOutput(target string, output []byte, observed time.Time) (Observation, bool) {
	mac := strings.ToLower(macPattern.FindString(string(output)))
	if mac == "" {
		return Observation{}, false
	}
	observation := Observation{
		IP:       target,
		MAC:      mac,
		State:    "REACHABLE",
		Source:   SourceActiveScan,
		Observed: observed,
	}
	EnrichObservation(&observation)
	return observation, true
}

func runArping(ctx context.Context, target string, options ScanOptions) ([]byte, error) {
	args := arpingArgs(target, options)
	cmd := exec.CommandContext(ctx, "arping", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if stderr.Len() > 0 {
			return out, fmt.Errorf("run arping %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
		}
		return out, fmt.Errorf("run arping %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

func arpingArgs(target string, options ScanOptions) []string {
	waitSeconds := int(math.Ceil(options.Timeout.Seconds()))
	if waitSeconds < 1 {
		waitSeconds = 1
	}
	args := []string{"-c", "1", "-w", fmt.Sprintf("%d", waitSeconds)}
	if strings.TrimSpace(options.Interface) != "" {
		args = append(args, "-I", strings.TrimSpace(options.Interface))
	}
	args = append(args, target)
	return args
}

func ipv4ToUint32(ip net.IP) uint32 {
	value := ip.To4()
	return uint32(value[0])<<24 | uint32(value[1])<<16 | uint32(value[2])<<8 | uint32(value[3])
}

func uint32ToIPv4(value uint32) net.IP {
	return net.IPv4(byte(value>>24), byte(value>>16), byte(value>>8), byte(value))
}

func nonResponsiveTargets(targets []string, observations []Observation) []string {
	responsive := make(map[string]struct{}, len(observations))
	for _, observation := range observations {
		responsive[observation.IP] = struct{}{}
	}
	items := make([]string, 0, len(targets))
	for _, target := range targets {
		if _, ok := responsive[target]; !ok {
			items = append(items, target)
		}
	}
	return items
}

func CompressAddressRanges(addresses []string) []AddressRange {
	if len(addresses) == 0 {
		return nil
	}
	ranges := make([]AddressRange, 0)
	var start, previous uint32
	var startText, previousText string
	for i, address := range addresses {
		ip := net.ParseIP(address)
		if ip == nil || ip.To4() == nil {
			continue
		}
		value := ipv4ToUint32(ip)
		if i == 0 || value != previous+1 {
			if i > 0 {
				ranges = append(ranges, addressRange(startText, previousText, int(previous-start)+1))
			}
			start = value
			startText = address
		}
		previous = value
		previousText = address
	}
	if previousText != "" {
		ranges = append(ranges, addressRange(startText, previousText, int(previous-start)+1))
	}
	return ranges
}

func addressRange(first, last string, count int) AddressRange {
	item := AddressRange{First: first, Count: count}
	if last != first {
		item.Last = last
	}
	return item
}
