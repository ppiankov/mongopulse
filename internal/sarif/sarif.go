package sarif

import (
	"fmt"

	"github.com/ppiankov/mongopulse/internal/doctor"
	"github.com/ppiankov/mongopulse/internal/snapshot"
)

const schema = "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/main/sarif-2.1/schema/sarif-schema-2.1.0.json"
const sarifVersion = "2.1.0"

type SarifLog struct {
	Schema  string `json:"$schema"`
	Version string `json:"version"`
	Runs    []Run  `json:"runs"`
}

type Run struct {
	Tool    Tool     `json:"tool"`
	Results []Result `json:"results"`
}

type Tool struct {
	Driver ToolDriver `json:"driver"`
}

type ToolDriver struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Result struct {
	RuleID    string     `json:"ruleId"`
	Level     string     `json:"level"`
	Message   Message    `json:"message"`
	Locations []Location `json:"locations,omitempty"`
}

type Message struct {
	Text string `json:"text"`
}

type Location struct {
	PhysicalLocation PhysicalLocation `json:"physicalLocation"`
}

type PhysicalLocation struct {
	ArtifactLocation ArtifactLocation `json:"artifactLocation"`
	Region           *Region          `json:"region,omitempty"`
}

type ArtifactLocation struct {
	URI string `json:"uri"`
}

type Region struct {
	StartLine int `json:"startLine,omitempty"`
}

func FromSnapshot(snaps []snapshot.Snapshot, ver string) SarifLog {
	run := Run{
		Tool: Tool{Driver: ToolDriver{Name: "mongopulse", Version: ver}},
	}

	for _, s := range snaps {
		switch s.Status {
		case snapshot.Critical:
			run.Results = append(run.Results, Result{
				RuleID:  "snapshot/critical",
				Level:   "error",
				Message: Message{Text: fmt.Sprintf("node %s is critical", s.Node)},
				Locations: []Location{{
					PhysicalLocation: PhysicalLocation{
						ArtifactLocation: ArtifactLocation{URI: s.Node},
					},
				}},
			})
		case snapshot.Degraded:
			run.Results = append(run.Results, Result{
				RuleID:  "snapshot/degraded",
				Level:   "warning",
				Message: Message{Text: fmt.Sprintf("node %s is degraded", s.Node)},
				Locations: []Location{{
					PhysicalLocation: PhysicalLocation{
						ArtifactLocation: ArtifactLocation{URI: s.Node},
					},
				}},
			})
		}
	}

	return SarifLog{
		Schema:  schema,
		Version: sarifVersion,
		Runs:    []Run{run},
	}
}

func FromDoctorReport(r doctor.Report) SarifLog {
	run := Run{
		Tool: Tool{Driver: ToolDriver{Name: r.Tool.Name, Version: r.Tool.Version}},
	}

	for _, c := range r.Checks {
		var level string
		switch c.Status {
		case doctor.StatusFail:
			level = "error"
		case doctor.StatusWarn:
			level = "warning"
		default:
			continue
		}
		run.Results = append(run.Results, Result{
			RuleID:  fmt.Sprintf("doctor/%s", c.Name),
			Level:   level,
			Message: Message{Text: c.Message},
		})
	}

	return SarifLog{
		Schema:  schema,
		Version: sarifVersion,
		Runs:    []Run{run},
	}
}
