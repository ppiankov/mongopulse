//go:build integration

package doctor

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ppiankov/mongopulse/internal/testutil"
)

func TestRun_Standalone(t *testing.T) {
	_, uri := testutil.StartMongo(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	report := Run(ctx, uri, "0.1.0-test")

	if report.Tool.Name != "mongopulse" {
		t.Errorf("Tool.Name = %q, want %q", report.Tool.Name, "mongopulse")
	}
	if report.Tool.Version != "0.1.0-test" {
		t.Errorf("Tool.Version = %q, want %q", report.Tool.Version, "0.1.0-test")
	}
	if report.Timestamp == "" {
		t.Error("Timestamp is empty")
	}

	checkMap := make(map[string]Check)
	for _, c := range report.Checks {
		checkMap[c.Name] = c
	}

	// Connectivity must pass.
	if c, ok := checkMap["connectivity"]; !ok {
		t.Error("missing connectivity check")
	} else if c.Status != StatusPass {
		t.Errorf("connectivity status = %q, want %q: %s", c.Status, StatusPass, c.Message)
	}

	// Server version must pass and return a version string.
	if c, ok := checkMap["server_version"]; !ok {
		t.Error("missing server_version check")
	} else {
		if c.Status != StatusPass {
			t.Errorf("server_version status = %q, want %q: %s", c.Status, StatusPass, c.Message)
		}
		if c.Message == "" {
			t.Error("server_version message (version string) is empty")
		}
	}

	// Replication on standalone should warn.
	if c, ok := checkMap["replication"]; !ok {
		t.Error("missing replication check")
	} else if c.Status != StatusWarn {
		t.Errorf("replication status = %q, want %q (standalone): %s", c.Status, StatusWarn, c.Message)
	}

	// Profiling check: default level 0 = warn.
	if c, ok := checkMap["profiling"]; !ok {
		t.Error("missing profiling check")
	} else if c.Status != StatusWarn {
		t.Errorf("profiling status = %q, want %q (default level 0): %s", c.Status, StatusWarn, c.Message)
	}

	// Permissions should pass.
	if c, ok := checkMap["permissions"]; !ok {
		t.Error("missing permissions check")
	} else if c.Status != StatusPass {
		t.Errorf("permissions status = %q, want %q: %s", c.Status, StatusPass, c.Message)
	}

	// Overall status should be warn (not fail) on standalone.
	if report.Status != StatusWarn {
		t.Errorf("overall status = %q, want %q", report.Status, StatusWarn)
	}
}

func TestRun_JSONRoundtrip(t *testing.T) {
	_, uri := testutil.StartMongo(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	report := Run(ctx, uri, "0.1.0-test")

	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded Report
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if decoded.Tool.Name != report.Tool.Name {
		t.Errorf("decoded Tool.Name = %q, want %q", decoded.Tool.Name, report.Tool.Name)
	}
	if decoded.Status != report.Status {
		t.Errorf("decoded Status = %q, want %q", decoded.Status, report.Status)
	}
	if len(decoded.Checks) != len(report.Checks) {
		t.Errorf("decoded Checks count = %d, want %d", len(decoded.Checks), len(report.Checks))
	}
}

func TestRun_InvalidDSN(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	report := Run(ctx, "mongodb://invalid-host-that-does-not-exist:99999", "0.1.0-test")
	if report.Status != StatusFail {
		t.Errorf("expected fail status for invalid DSN, got %q", report.Status)
	}
}

func TestDowngrade(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		initial  Status
		input    Status
		expected Status
	}{
		{"pass_to_warn", StatusPass, StatusWarn, StatusWarn},
		{"pass_to_fail", StatusPass, StatusFail, StatusFail},
		{"warn_to_fail", StatusWarn, StatusFail, StatusFail},
		{"warn_stays_warn", StatusWarn, StatusWarn, StatusWarn},
		{"fail_stays_fail", StatusFail, StatusWarn, StatusFail},
		{"pass_stays_pass_on_pass", StatusPass, StatusPass, StatusPass},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := &Report{Status: tt.initial}
			r.downgrade(tt.input)
			if r.Status != tt.expected {
				t.Errorf("downgrade(%q → %q) = %q, want %q", tt.initial, tt.input, r.Status, tt.expected)
			}
		})
	}
}
