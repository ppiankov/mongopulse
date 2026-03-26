package baseline

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/ppiankov/mongopulse/internal/policy"
)

type BaselineEntry struct {
	ID           string     `json:"id"`
	Rule         string     `json:"rule"`
	Resource     string     `json:"resource"`
	SuppressedAt time.Time  `json:"suppressed_at"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
}

type Baseline struct {
	Entries []BaselineEntry `json:"entries"`
}

func Load(path string) (Baseline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Baseline{}, fmt.Errorf("read baseline: %w", err)
	}

	var b Baseline
	if err := json.Unmarshal(data, &b); err != nil {
		return Baseline{}, fmt.Errorf("parse baseline: %w", err)
	}

	return b, nil
}

func Save(path string, b Baseline) error {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal baseline: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write baseline: %w", err)
	}

	return nil
}

func (b *Baseline) IsKnown(rule, resource string) bool {
	now := time.Now()
	for _, e := range b.Entries {
		if e.Rule == rule && e.Resource == resource {
			if e.ExpiresAt != nil && now.After(*e.ExpiresAt) {
				return false
			}
			return true
		}
	}
	return false
}

func (b *Baseline) Add(rule, resource string, expires time.Duration) {
	now := time.Now()
	entry := BaselineEntry{
		ID:           fmt.Sprintf("%s:%s", rule, resource),
		Rule:         rule,
		Resource:     resource,
		SuppressedAt: now,
	}

	if expires > 0 {
		exp := now.Add(expires)
		entry.ExpiresAt = &exp
	}

	b.Entries = append(b.Entries, entry)
}

func FilterViolations(violations []policy.PolicyViolation, b Baseline) []policy.PolicyViolation {
	var filtered []policy.PolicyViolation
	for _, v := range violations {
		if !b.IsKnown(v.Rule, v.Actual) {
			filtered = append(filtered, v)
		}
	}
	return filtered
}

func FromViolations(violations []policy.PolicyViolation, expires time.Duration) Baseline {
	var b Baseline
	for _, v := range violations {
		b.Add(v.Rule, v.Actual, expires)
	}
	return b
}
