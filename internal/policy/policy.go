package policy

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/ppiankov/mongopulse/internal/snapshot"
)

type Policy struct {
	MaxReplicationLagSeconds float64 `yaml:"max_replication_lag_seconds"`
	MaxConnectionUtilization float64 `yaml:"max_connection_utilization"`
	MaxUnusedIndexes         int     `yaml:"max_unused_indexes"`
	MinOplogWindowHours      float64 `yaml:"min_oplog_window_hours"`
	MaxSlowOps               int     `yaml:"max_slow_ops"`
}

type PolicyViolation struct {
	Rule      string `json:"rule"`
	Actual    string `json:"actual"`
	Threshold string `json:"threshold"`
	Severity  string `json:"severity"`
}

func Defaults() Policy {
	return Policy{
		MaxReplicationLagSeconds: 10,
		MaxConnectionUtilization: 0.9,
		MaxUnusedIndexes:         10,
		MinOplogWindowHours:      12,
		MaxSlowOps:               5,
	}
}

func Load(path string) (Policy, error) {
	p := Defaults()

	data, err := os.ReadFile(path)
	if err != nil {
		return Policy{}, fmt.Errorf("read policy file: %w", err)
	}

	if err := yaml.Unmarshal(data, &p); err != nil {
		return Policy{}, fmt.Errorf("parse policy file: %w", err)
	}

	if p.MaxConnectionUtilization < 0 || p.MaxConnectionUtilization > 1 {
		return Policy{}, fmt.Errorf("max_connection_utilization must be between 0 and 1, got %f", p.MaxConnectionUtilization)
	}

	return p, nil
}

func Evaluate(p Policy, snap snapshot.Snapshot) []PolicyViolation {
	var violations []PolicyViolation

	if snap.ReplSet != nil {
		for _, m := range snap.ReplSet.Members {
			if m.LagSecs > p.MaxReplicationLagSeconds {
				violations = append(violations, PolicyViolation{
					Rule:      "max_replication_lag_seconds",
					Actual:    fmt.Sprintf("%.1fs (%s)", m.LagSecs, m.Name),
					Threshold: fmt.Sprintf("%.1fs", p.MaxReplicationLagSeconds),
					Severity:  "critical",
				})
			}
		}
	}

	if snap.Connections.Ratio > p.MaxConnectionUtilization {
		violations = append(violations, PolicyViolation{
			Rule:      "max_connection_utilization",
			Actual:    fmt.Sprintf("%.2f", snap.Connections.Ratio),
			Threshold: fmt.Sprintf("%.2f", p.MaxConnectionUtilization),
			Severity:  "warning",
		})
	}

	if snap.UnusedIndexes > p.MaxUnusedIndexes {
		violations = append(violations, PolicyViolation{
			Rule:      "max_unused_indexes",
			Actual:    fmt.Sprintf("%d", snap.UnusedIndexes),
			Threshold: fmt.Sprintf("%d", p.MaxUnusedIndexes),
			Severity:  "warning",
		})
	}

	if snap.OplogWindowHours > 0 && snap.OplogWindowHours < p.MinOplogWindowHours {
		violations = append(violations, PolicyViolation{
			Rule:      "min_oplog_window_hours",
			Actual:    fmt.Sprintf("%.1fh", snap.OplogWindowHours),
			Threshold: fmt.Sprintf("%.1fh", p.MinOplogWindowHours),
			Severity:  "critical",
		})
	}

	if snap.SlowOps > p.MaxSlowOps {
		violations = append(violations, PolicyViolation{
			Rule:      "max_slow_ops",
			Actual:    fmt.Sprintf("%d", snap.SlowOps),
			Threshold: fmt.Sprintf("%d", p.MaxSlowOps),
			Severity:  "warning",
		})
	}

	return violations
}

func SampleYAML() string {
	return `# mongopulse policy-as-code configuration
max_replication_lag_seconds: 10
max_connection_utilization: 0.9
max_unused_indexes: 10
min_oplog_window_hours: 12
max_slow_ops: 5
`
}
