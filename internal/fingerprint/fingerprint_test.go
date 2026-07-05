package fingerprint

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"netatlas/internal/model"
)

func TestBuiltinRulesMatchOperatingSystemAndVendor(t *testing.T) {
	engine := New(BuiltinRules())
	event := model.DNSEvent{
		Timestamp: time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC),
		ClientIP:  "192.168.1.10",
		QueryName: "download.windowsupdate.com",
		RawHash:   "hash",
	}

	matches := engine.MatchEvent(event)
	if !hasEvidence(matches, CategoryOperatingSystem, "Windows") {
		t.Fatalf("matches missing Windows evidence: %#v", matches)
	}
}

func TestLoadCustomRules(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fingerprints.json")
	content := `{"rules":[{"id":"software-test","category":"software","target":"Test App","score":45,"domain_suffixes":["example.com"]}]}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	engine, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	matches := engine.MatchEvent(model.DNSEvent{
		Timestamp: time.Now(),
		ClientIP:  "192.168.1.10",
		QueryName: "api.example.com",
		RawHash:   "hash",
	})
	if !hasEvidence(matches, CategorySoftware, "Test App") {
		t.Fatalf("matches missing custom software evidence: %#v", matches)
	}
}

func TestConfidence(t *testing.T) {
	tests := map[int]string{
		20: "low",
		40: "medium",
		70: "high",
	}
	for score, want := range tests {
		if got := Confidence(score); got != want {
			t.Fatalf("Confidence(%d) = %q, want %q", score, got, want)
		}
	}
}

func TestNegativeRuleContributesNegativeScore(t *testing.T) {
	engine := New([]Rule{
		{
			ID:       "not-mobile",
			Category: CategoryDeviceType,
			Target:   "Mobile phone",
			Score:    20,
			Domains:  []string{"desktop-update.example"},
			Negative: true,
		},
	})
	matches := engine.MatchEvent(model.DNSEvent{
		Timestamp: time.Now(),
		ClientIP:  "192.168.1.10",
		QueryName: "desktop-update.example",
		RawHash:   "hash",
	})
	if len(matches) != 1 {
		t.Fatalf("len(matches) = %d, want 1", len(matches))
	}
	if matches[0].Score != -20 {
		t.Fatalf("Score = %d, want -20", matches[0].Score)
	}
}

func hasEvidence(items []Evidence, category, target string) bool {
	for _, item := range items {
		if item.Category == category && item.Target == target {
			return true
		}
	}
	return false
}
