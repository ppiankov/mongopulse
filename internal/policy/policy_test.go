package policy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ppiankov/mongopulse/internal/snapshot"
)

func TestDefaults(t *testing.T) {
	t.Parallel()
	d := Defaults()
	if d.MaxReplicationLagSeconds != 10 {
		t.Errorf("MaxReplicationLagSeconds = %f, want 10", d.MaxReplicationLagSeconds)
	}
	if d.MaxConnectionUtilization != 0.9 {
		t.Errorf("MaxConnectionUtilization = %f, want 0.9", d.MaxConnectionUtilization)
	}
	if d.MaxUnusedIndexes != 10 {
		t.Errorf("MaxUnusedIndexes = %d, want 10", d.MaxUnusedIndexes)
	}
	if d.MinOplogWindowHours != 12 {
		t.Errorf("MinOplogWindowHours = %f, want 12", d.MinOplogWindowHours)
	}
	if d.MaxSlowOps != 5 {
		t.Errorf("MaxSlowOps = %d, want 5", d.MaxSlowOps)
	}
}

func TestLoad_FullOverride(t *testing.T) {
	t.Parallel()
	content := `
max_replication_lag_seconds: 5
max_connection_utilization: 0.8
max_unused_indexes: 3
min_oplog_window_hours: 24
max_slow_ops: 2
`
	path := writeTemp(t, content)
	p, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if p.MaxReplicationLagSeconds != 5 {
		t.Errorf("MaxReplicationLagSeconds = %f, want 5", p.MaxReplicationLagSeconds)
	}
	if p.MaxConnectionUtilization != 0.8 {
		t.Errorf("MaxConnectionUtilization = %f, want 0.8", p.MaxConnectionUtilization)
	}
	if p.MaxUnusedIndexes != 3 {
		t.Errorf("MaxUnusedIndexes = %d, want 3", p.MaxUnusedIndexes)
	}
	if p.MinOplogWindowHours != 24 {
		t.Errorf("MinOplogWindowHours = %f, want 24", p.MinOplogWindowHours)
	}
	if p.MaxSlowOps != 2 {
		t.Errorf("MaxSlowOps = %d, want 2", p.MaxSlowOps)
	}
}

func TestLoad_PartialUsesDefaults(t *testing.T) {
	t.Parallel()
	content := `max_slow_ops: 1`
	path := writeTemp(t, content)
	p, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if p.MaxSlowOps != 1 {
		t.Errorf("MaxSlowOps = %d, want 1", p.MaxSlowOps)
	}
	if p.MaxReplicationLagSeconds != 10 {
		t.Errorf("MaxReplicationLagSeconds = %f, want default 10", p.MaxReplicationLagSeconds)
	}
	if p.MaxConnectionUtilization != 0.9 {
		t.Errorf("MaxConnectionUtilization = %f, want default 0.9", p.MaxConnectionUtilization)
	}
	if p.MaxUnusedIndexes != 10 {
		t.Errorf("MaxUnusedIndexes = %d, want default 10", p.MaxUnusedIndexes)
	}
	if p.MinOplogWindowHours != 12 {
		t.Errorf("MinOplogWindowHours = %f, want default 12", p.MinOplogWindowHours)
	}
}

func TestLoad_EmptyFileUsesDefaults(t *testing.T) {
	t.Parallel()
	path := writeTemp(t, "")
	p, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	d := Defaults()
	if p != d {
		t.Errorf("empty file policy = %+v, want defaults %+v", p, d)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	t.Parallel()
	_, err := Load("/nonexistent/policy.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	t.Parallel()
	path := writeTemp(t, "{{{{invalid yaml")
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoad_InvalidConnectionUtilization(t *testing.T) {
	t.Parallel()
	for _, val := range []string{"1.5", "-0.1"} {
		path := writeTemp(t, "max_connection_utilization: "+val)
		_, err := Load(path)
		if err == nil {
			t.Errorf("expected error for max_connection_utilization=%s", val)
		}
	}
}

func TestEvaluate_AllViolations(t *testing.T) {
	t.Parallel()
	p := Policy{
		MaxReplicationLagSeconds: 5,
		MaxConnectionUtilization: 0.8,
		MaxUnusedIndexes:         2,
		MinOplogWindowHours:      24,
		MaxSlowOps:               1,
	}
	snap := snapshot.Snapshot{
		Connections: snapshot.ConnectionsSnap{
			Current:   900,
			Available: 100,
			Ratio:     0.95,
		},
		ReplSet: &snapshot.ReplSetSnap{
			Set:   "rs0",
			State: "PRIMARY",
			Members: []snapshot.MemberSnap{
				{Name: "node1", State: "PRIMARY", LagSecs: 0},
				{Name: "node2", State: "SECONDARY", LagSecs: 15},
			},
		},
		SlowOps:          10,
		UnusedIndexes:    5,
		OplogWindowHours: 6,
	}

	violations := Evaluate(p, snap)
	rules := make(map[string]bool)
	for _, v := range violations {
		rules[v.Rule] = true
	}

	expected := []string{
		"max_replication_lag_seconds",
		"max_connection_utilization",
		"max_unused_indexes",
		"min_oplog_window_hours",
		"max_slow_ops",
	}
	for _, rule := range expected {
		if !rules[rule] {
			t.Errorf("missing violation for rule %q", rule)
		}
	}
	if len(violations) != len(expected) {
		t.Errorf("got %d violations, want %d", len(violations), len(expected))
	}
}

func TestEvaluate_HealthySnapshot(t *testing.T) {
	t.Parallel()
	p := Defaults()
	snap := snapshot.Snapshot{
		Connections: snapshot.ConnectionsSnap{
			Current:   100,
			Available: 900,
			Ratio:     0.1,
		},
		ReplSet: &snapshot.ReplSetSnap{
			Set:   "rs0",
			State: "PRIMARY",
			Members: []snapshot.MemberSnap{
				{Name: "node1", State: "PRIMARY", LagSecs: 0},
				{Name: "node2", State: "SECONDARY", LagSecs: 2},
			},
		},
		SlowOps:          1,
		UnusedIndexes:    3,
		OplogWindowHours: 48,
	}

	violations := Evaluate(p, snap)
	if len(violations) != 0 {
		t.Errorf("expected no violations, got %d: %+v", len(violations), violations)
	}
}

func TestEvaluate_NoReplSet(t *testing.T) {
	t.Parallel()
	p := Defaults()
	snap := snapshot.Snapshot{
		Connections: snapshot.ConnectionsSnap{Ratio: 0.5},
		SlowOps:     1,
	}
	violations := Evaluate(p, snap)
	if len(violations) != 0 {
		t.Errorf("expected no violations for standalone, got %d", len(violations))
	}
}

func TestEvaluate_ZeroOplogWindowSkipped(t *testing.T) {
	t.Parallel()
	p := Policy{MinOplogWindowHours: 24}
	snap := snapshot.Snapshot{
		OplogWindowHours: 0,
	}
	violations := Evaluate(p, snap)
	for _, v := range violations {
		if v.Rule == "min_oplog_window_hours" {
			t.Error("oplog window rule should be skipped when OplogWindowHours is 0")
		}
	}
}

func TestEvaluate_MultipleReplicaLagViolations(t *testing.T) {
	t.Parallel()
	p := Policy{MaxReplicationLagSeconds: 5}
	snap := snapshot.Snapshot{
		ReplSet: &snapshot.ReplSetSnap{
			Members: []snapshot.MemberSnap{
				{Name: "a", LagSecs: 10},
				{Name: "b", LagSecs: 20},
				{Name: "c", LagSecs: 1},
			},
		},
	}
	violations := Evaluate(p, snap)
	count := 0
	for _, v := range violations {
		if v.Rule == "max_replication_lag_seconds" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("expected 2 lag violations, got %d", count)
	}
}

func TestEvaluate_ViolationSeverity(t *testing.T) {
	t.Parallel()
	p := Policy{
		MaxReplicationLagSeconds: 1,
		MaxConnectionUtilization: 0.5,
		MaxUnusedIndexes:         0,
		MinOplogWindowHours:      100,
		MaxSlowOps:               0,
	}
	snap := snapshot.Snapshot{
		Connections:      snapshot.ConnectionsSnap{Ratio: 0.9},
		ReplSet:          &snapshot.ReplSetSnap{Members: []snapshot.MemberSnap{{Name: "n1", LagSecs: 5}}},
		SlowOps:          3,
		UnusedIndexes:    2,
		OplogWindowHours: 10,
	}

	violations := Evaluate(p, snap)
	severities := make(map[string]string)
	for _, v := range violations {
		severities[v.Rule] = v.Severity
	}

	if severities["max_replication_lag_seconds"] != "critical" {
		t.Error("replication lag should be critical severity")
	}
	if severities["min_oplog_window_hours"] != "critical" {
		t.Error("oplog window should be critical severity")
	}
	if severities["max_connection_utilization"] != "warning" {
		t.Error("connection utilization should be warning severity")
	}
	if severities["max_unused_indexes"] != "warning" {
		t.Error("unused indexes should be warning severity")
	}
	if severities["max_slow_ops"] != "warning" {
		t.Error("slow ops should be warning severity")
	}
}

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "policy.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	return path
}
