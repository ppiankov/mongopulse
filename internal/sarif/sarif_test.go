package sarif

import (
	"encoding/json"
	"testing"

	"github.com/ppiankov/mongopulse/internal/doctor"
	"github.com/ppiankov/mongopulse/internal/snapshot"
)

func TestFromSnapshot_Healthy(t *testing.T) {
	snaps := []snapshot.Snapshot{
		{Node: "localhost:27017", Status: snapshot.Healthy},
	}
	log := FromSnapshot(snaps, "1.0.0")

	if len(log.Runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(log.Runs))
	}
	if len(log.Runs[0].Results) != 0 {
		t.Errorf("healthy snapshot should produce zero results, got %d", len(log.Runs[0].Results))
	}
	if log.Version != "2.1.0" {
		t.Errorf("expected SARIF version 2.1.0, got %s", log.Version)
	}
	if log.Runs[0].Tool.Driver.Name != "mongopulse" {
		t.Errorf("expected tool name mongopulse, got %s", log.Runs[0].Tool.Driver.Name)
	}
	if log.Runs[0].Tool.Driver.Version != "1.0.0" {
		t.Errorf("expected tool version 1.0.0, got %s", log.Runs[0].Tool.Driver.Version)
	}
}

func TestFromSnapshot_Degraded(t *testing.T) {
	snaps := []snapshot.Snapshot{
		{Node: "node1:27017", Status: snapshot.Degraded},
	}
	log := FromSnapshot(snaps, "1.0.0")

	if len(log.Runs[0].Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(log.Runs[0].Results))
	}
	r := log.Runs[0].Results[0]
	if r.Level != "warning" {
		t.Errorf("degraded should map to warning, got %s", r.Level)
	}
	if r.RuleID != "snapshot/degraded" {
		t.Errorf("expected ruleId snapshot/degraded, got %s", r.RuleID)
	}
	if len(r.Locations) != 1 {
		t.Fatalf("expected 1 location, got %d", len(r.Locations))
	}
	if r.Locations[0].PhysicalLocation.ArtifactLocation.URI != "node1:27017" {
		t.Errorf("unexpected URI: %s", r.Locations[0].PhysicalLocation.ArtifactLocation.URI)
	}
}

func TestFromSnapshot_Critical(t *testing.T) {
	snaps := []snapshot.Snapshot{
		{Node: "node2:27017", Status: snapshot.Critical},
	}
	log := FromSnapshot(snaps, "0.5.0")

	if len(log.Runs[0].Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(log.Runs[0].Results))
	}
	r := log.Runs[0].Results[0]
	if r.Level != "error" {
		t.Errorf("critical should map to error, got %s", r.Level)
	}
	if r.RuleID != "snapshot/critical" {
		t.Errorf("expected ruleId snapshot/critical, got %s", r.RuleID)
	}
}

func TestFromSnapshot_Mixed(t *testing.T) {
	snaps := []snapshot.Snapshot{
		{Node: "healthy:27017", Status: snapshot.Healthy},
		{Node: "degraded:27017", Status: snapshot.Degraded},
		{Node: "critical:27017", Status: snapshot.Critical},
	}
	log := FromSnapshot(snaps, "1.0.0")

	if len(log.Runs[0].Results) != 2 {
		t.Fatalf("expected 2 results (degraded + critical), got %d", len(log.Runs[0].Results))
	}
}

func TestFromSnapshot_Empty(t *testing.T) {
	log := FromSnapshot(nil, "1.0.0")

	if len(log.Runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(log.Runs))
	}
	if len(log.Runs[0].Results) != 0 {
		t.Errorf("nil snapshots should produce zero results, got %d", len(log.Runs[0].Results))
	}
}

func TestFromDoctorReport_AllPass(t *testing.T) {
	r := doctor.Report{
		Tool:   doctor.ToolInfo{Name: "mongopulse", Version: "1.0.0"},
		Status: doctor.StatusPass,
		Checks: []doctor.Check{
			{Name: "connectivity", Status: doctor.StatusPass, Message: "connected"},
			{Name: "server_version", Status: doctor.StatusPass, Message: "7.0.0"},
		},
	}
	log := FromDoctorReport(r)

	if len(log.Runs[0].Results) != 0 {
		t.Errorf("all-pass report should produce zero results, got %d", len(log.Runs[0].Results))
	}
}

func TestFromDoctorReport_Fail(t *testing.T) {
	r := doctor.Report{
		Tool:   doctor.ToolInfo{Name: "mongopulse", Version: "1.0.0"},
		Status: doctor.StatusFail,
		Checks: []doctor.Check{
			{Name: "connectivity", Status: doctor.StatusFail, Message: "connect: connection refused"},
		},
	}
	log := FromDoctorReport(r)

	if len(log.Runs[0].Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(log.Runs[0].Results))
	}
	res := log.Runs[0].Results[0]
	if res.Level != "error" {
		t.Errorf("fail should map to error, got %s", res.Level)
	}
	if res.RuleID != "doctor/connectivity" {
		t.Errorf("expected ruleId doctor/connectivity, got %s", res.RuleID)
	}
	if res.Message.Text != "connect: connection refused" {
		t.Errorf("unexpected message: %s", res.Message.Text)
	}
}

func TestFromDoctorReport_Warn(t *testing.T) {
	r := doctor.Report{
		Tool:   doctor.ToolInfo{Name: "mongopulse", Version: "1.0.0"},
		Status: doctor.StatusWarn,
		Checks: []doctor.Check{
			{Name: "connectivity", Status: doctor.StatusPass, Message: "connected"},
			{Name: "replication", Status: doctor.StatusWarn, Message: "not a replica set member"},
			{Name: "profiling", Status: doctor.StatusWarn, Message: "profiling disabled"},
		},
	}
	log := FromDoctorReport(r)

	if len(log.Runs[0].Results) != 2 {
		t.Fatalf("expected 2 results (warn checks only), got %d", len(log.Runs[0].Results))
	}
	for _, res := range log.Runs[0].Results {
		if res.Level != "warning" {
			t.Errorf("warn should map to warning, got %s", res.Level)
		}
	}
}

func TestFromDoctorReport_MixedStatuses(t *testing.T) {
	r := doctor.Report{
		Tool:   doctor.ToolInfo{Name: "mongopulse", Version: "2.0.0"},
		Status: doctor.StatusFail,
		Checks: []doctor.Check{
			{Name: "connectivity", Status: doctor.StatusPass, Message: "connected"},
			{Name: "replication", Status: doctor.StatusWarn, Message: "standalone"},
			{Name: "permissions", Status: doctor.StatusFail, Message: "access denied"},
		},
	}
	log := FromDoctorReport(r)

	if len(log.Runs[0].Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(log.Runs[0].Results))
	}
	if log.Runs[0].Results[0].Level != "warning" {
		t.Errorf("first result should be warning, got %s", log.Runs[0].Results[0].Level)
	}
	if log.Runs[0].Results[1].Level != "error" {
		t.Errorf("second result should be error, got %s", log.Runs[0].Results[1].Level)
	}
}

func TestSarifLog_ValidJSON(t *testing.T) {
	snaps := []snapshot.Snapshot{
		{Node: "n1:27017", Status: snapshot.Degraded},
		{Node: "n2:27017", Status: snapshot.Critical},
	}
	log := FromSnapshot(snaps, "1.0.0")

	data, err := json.Marshal(log)
	if err != nil {
		t.Fatalf("failed to marshal SARIF log: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	if parsed["version"] != "2.1.0" {
		t.Errorf("expected version 2.1.0 in JSON output, got %v", parsed["version"])
	}
	if parsed["$schema"] != schema {
		t.Errorf("expected schema URI in JSON output, got %v", parsed["$schema"])
	}
}

func TestDoctorSarifLog_ValidJSON(t *testing.T) {
	r := doctor.Report{
		Tool:   doctor.ToolInfo{Name: "mongopulse", Version: "1.0.0"},
		Status: doctor.StatusFail,
		Checks: []doctor.Check{
			{Name: "connectivity", Status: doctor.StatusFail, Message: "connection refused"},
		},
	}
	log := FromDoctorReport(r)

	data, err := json.Marshal(log)
	if err != nil {
		t.Fatalf("failed to marshal SARIF log: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
}
