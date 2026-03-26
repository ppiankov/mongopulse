package baseline

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ppiankov/mongopulse/internal/policy"
)

func TestSaveAndLoad(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.json")

	original := Baseline{}
	original.Add("max_slow_ops", "node1", 0)
	original.Add("max_connection_utilization", "node2", 24*time.Hour)

	if err := Save(path, original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(loaded.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(loaded.Entries))
	}

	if loaded.Entries[0].Rule != "max_slow_ops" {
		t.Errorf("expected rule max_slow_ops, got %s", loaded.Entries[0].Rule)
	}
	if loaded.Entries[0].Resource != "node1" {
		t.Errorf("expected resource node1, got %s", loaded.Entries[0].Resource)
	}
	if loaded.Entries[1].ExpiresAt == nil {
		t.Error("expected ExpiresAt to be set for entry with expiry")
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	t.Parallel()

	_, err := Load("/nonexistent/baseline.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("{not json}"), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestIsKnown_Exists(t *testing.T) {
	t.Parallel()

	b := Baseline{}
	b.Add("max_slow_ops", "node1", 0)

	if !b.IsKnown("max_slow_ops", "node1") {
		t.Error("expected IsKnown to return true for existing entry")
	}
}

func TestIsKnown_NotFound(t *testing.T) {
	t.Parallel()

	b := Baseline{}
	b.Add("max_slow_ops", "node1", 0)

	if b.IsKnown("max_slow_ops", "node2") {
		t.Error("expected IsKnown to return false for unknown resource")
	}
	if b.IsKnown("other_rule", "node1") {
		t.Error("expected IsKnown to return false for unknown rule")
	}
}

func TestIsKnown_Expired(t *testing.T) {
	t.Parallel()

	b := Baseline{}
	now := time.Now()
	past := now.Add(-1 * time.Hour)
	b.Entries = append(b.Entries, BaselineEntry{
		ID:           "max_slow_ops:node1",
		Rule:         "max_slow_ops",
		Resource:     "node1",
		SuppressedAt: now.Add(-2 * time.Hour),
		ExpiresAt:    &past,
	})

	if b.IsKnown("max_slow_ops", "node1") {
		t.Error("expected IsKnown to return false for expired entry")
	}
}

func TestIsKnown_NotExpired(t *testing.T) {
	t.Parallel()

	b := Baseline{}
	b.Add("max_slow_ops", "node1", 24*time.Hour)

	if !b.IsKnown("max_slow_ops", "node1") {
		t.Error("expected IsKnown to return true for non-expired entry")
	}
}

func TestAdd_WithExpiry(t *testing.T) {
	t.Parallel()

	b := Baseline{}
	before := time.Now()
	b.Add("max_slow_ops", "node1", 48*time.Hour)
	after := time.Now()

	if len(b.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(b.Entries))
	}

	e := b.Entries[0]
	if e.ID != "max_slow_ops:node1" {
		t.Errorf("expected ID max_slow_ops:node1, got %s", e.ID)
	}
	if e.ExpiresAt == nil {
		t.Fatal("expected ExpiresAt to be set")
	}

	expectedMin := before.Add(48 * time.Hour)
	expectedMax := after.Add(48 * time.Hour)
	if e.ExpiresAt.Before(expectedMin) || e.ExpiresAt.After(expectedMax) {
		t.Errorf("ExpiresAt %v not in expected range [%v, %v]", *e.ExpiresAt, expectedMin, expectedMax)
	}
}

func TestAdd_WithoutExpiry(t *testing.T) {
	t.Parallel()

	b := Baseline{}
	b.Add("max_slow_ops", "node1", 0)

	if b.Entries[0].ExpiresAt != nil {
		t.Error("expected ExpiresAt to be nil when no expiry set")
	}
}

func TestFilterViolations(t *testing.T) {
	t.Parallel()

	b := Baseline{}
	b.Add("max_slow_ops", "10", 0)

	violations := []policy.PolicyViolation{
		{Rule: "max_slow_ops", Actual: "10", Threshold: "5", Severity: "warning"},
		{Rule: "max_connection_utilization", Actual: "0.95", Threshold: "0.90", Severity: "warning"},
	}

	filtered := FilterViolations(violations, b)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 violation after filter, got %d", len(filtered))
	}
	if filtered[0].Rule != "max_connection_utilization" {
		t.Errorf("expected max_connection_utilization, got %s", filtered[0].Rule)
	}
}

func TestFromViolations(t *testing.T) {
	t.Parallel()

	violations := []policy.PolicyViolation{
		{Rule: "max_slow_ops", Actual: "10", Threshold: "5", Severity: "warning"},
		{Rule: "max_connection_utilization", Actual: "0.95", Threshold: "0.90", Severity: "warning"},
	}

	b := FromViolations(violations, 24*time.Hour)
	if len(b.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(b.Entries))
	}
	if b.Entries[0].Rule != "max_slow_ops" {
		t.Errorf("expected max_slow_ops, got %s", b.Entries[0].Rule)
	}
	if b.Entries[0].ExpiresAt == nil {
		t.Error("expected ExpiresAt to be set")
	}
}

func TestFromViolations_NoExpiry(t *testing.T) {
	t.Parallel()

	violations := []policy.PolicyViolation{
		{Rule: "max_slow_ops", Actual: "10", Threshold: "5", Severity: "warning"},
	}

	b := FromViolations(violations, 0)
	if b.Entries[0].ExpiresAt != nil {
		t.Error("expected ExpiresAt to be nil")
	}
}

func TestIsKnown_EmptyBaseline(t *testing.T) {
	t.Parallel()

	b := Baseline{}
	if b.IsKnown("any_rule", "any_resource") {
		t.Error("expected IsKnown to return false on empty baseline")
	}
}
