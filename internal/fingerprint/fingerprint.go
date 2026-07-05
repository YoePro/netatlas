package fingerprint

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"netatlas/internal/model"
)

const (
	CategoryOperatingSystem = "operating_system"
	CategoryDeviceType      = "device_type"
	CategorySoftware        = "software"
	CategoryInfrastructure  = "infrastructure"
	CategoryVendor          = "vendor"
)

type RuleSet struct {
	Rules []Rule `json:"rules"`
}

type Rule struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Category       string   `json:"category"`
	Target         string   `json:"target"`
	Score          int      `json:"score"`
	Domains        []string `json:"domains"`
	DomainSuffixes []string `json:"domain_suffixes"`
	Negative       bool     `json:"negative"`
}

type Evidence struct {
	DeviceKey     string
	Category      string
	Target        string
	Score         int
	Confidence    string
	Timestamp     time.Time
	EvidenceHash  string
	FingerprintID string
	MatchedDomain string
}

type Engine struct {
	rules []Rule
}

func Load(path string) (*Engine, error) {
	if strings.TrimSpace(path) == "" {
		return New(BuiltinRules()), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return New(BuiltinRules()), nil
		}
		return nil, fmt.Errorf("read fingerprint rules %q: %w", path, err)
	}
	var ruleSet RuleSet
	if err := json.Unmarshal(data, &ruleSet); err != nil {
		return nil, fmt.Errorf("parse fingerprint rules %q: %w", path, err)
	}
	if len(ruleSet.Rules) == 0 {
		return nil, fmt.Errorf("fingerprint rules %q contains no rules", path)
	}
	return New(ruleSet.Rules), nil
}

func New(rules []Rule) *Engine {
	normalized := make([]Rule, 0, len(rules))
	for _, rule := range rules {
		rule.Category = normalizeToken(rule.Category)
		rule.Target = strings.TrimSpace(rule.Target)
		rule.ID = strings.TrimSpace(rule.ID)
		if rule.ID == "" {
			rule.ID = normalizeToken(rule.Name)
		}
		rule.Score = abs(rule.Score)
		if rule.Negative {
			rule.Score = -rule.Score
		}
		for i := range rule.Domains {
			rule.Domains[i] = normalizeDomain(rule.Domains[i])
		}
		for i := range rule.DomainSuffixes {
			rule.DomainSuffixes[i] = normalizeDomain(rule.DomainSuffixes[i])
		}
		if rule.Category == "" || rule.Target == "" || rule.Score == 0 {
			continue
		}
		normalized = append(normalized, rule)
	}
	return &Engine{rules: normalized}
}

func (e *Engine) MatchBatch(events []model.DNSEvent) []Evidence {
	seen := make(map[string]struct{})
	var evidence []Evidence
	for _, event := range events {
		for _, item := range e.MatchEvent(event) {
			key := item.DeviceKey + "|" + item.Category + "|" + item.Target + "|" + item.FingerprintID + "|" + item.EvidenceHash
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			evidence = append(evidence, item)
		}
	}
	return evidence
}

func (e *Engine) MatchEvent(event model.DNSEvent) []Evidence {
	if event.ClientIP == "" || event.QueryName == "" {
		return nil
	}
	domain := normalizeDomain(event.QueryName)
	var matches []Evidence
	for _, rule := range e.rules {
		if !ruleMatches(rule, domain) {
			continue
		}
		matches = append(matches, Evidence{
			DeviceKey:     "ip:" + event.ClientIP,
			Category:      rule.Category,
			Target:        rule.Target,
			Score:         rule.Score,
			Confidence:    Confidence(rule.Score),
			Timestamp:     event.Timestamp,
			EvidenceHash:  event.RawHash,
			FingerprintID: rule.ID,
			MatchedDomain: domain,
		})
	}
	return matches
}

func Confidence(score int) string {
	if score < 0 {
		score = -score
	}
	switch {
	case score >= 70:
		return "high"
	case score >= 40:
		return "medium"
	default:
		return "low"
	}
}

func ruleMatches(rule Rule, domain string) bool {
	for _, exact := range rule.Domains {
		if domain == exact {
			return true
		}
	}
	for _, suffix := range rule.DomainSuffixes {
		if domain == suffix || strings.HasSuffix(domain, "."+suffix) {
			return true
		}
	}
	return false
}

func normalizeDomain(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimSuffix(value, ".")
	return value
}

func normalizeToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
