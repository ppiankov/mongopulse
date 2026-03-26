package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ppiankov/mongopulse/internal/snapshot"
)

var diffFormat string

var diffCmd = &cobra.Command{
	Use:   "diff <old.json> <new.json>",
	Short: "Compare two status snapshots",
	Args:  cobra.ExactArgs(2),
	RunE:  runDiff,
}

func init() {
	diffCmd.Flags().StringVar(&diffFormat, "format", "text", "Output format: text or json")
	rootCmd.AddCommand(diffCmd)
}

type snapshotDelta struct {
	OldStatus   string   `json:"old_status"`
	NewStatus   string   `json:"new_status"`
	Changed     bool     `json:"changed"`
	NewIssues   []string `json:"new_issues,omitempty"`
	Resolved    []string `json:"resolved,omitempty"`
	ConnDelta   int      `json:"connection_delta"`
	OpsCountOld int      `json:"active_ops_old"`
	OpsCountNew int      `json:"active_ops_new"`
}

func runDiff(cmd *cobra.Command, args []string) error {
	oldSnaps, err := loadSnapshots(args[0])
	if err != nil {
		return fmt.Errorf("old file: %w", err)
	}
	newSnaps, err := loadSnapshots(args[1])
	if err != nil {
		return fmt.Errorf("new file: %w", err)
	}

	if len(oldSnaps) == 0 || len(newSnaps) == 0 {
		return fmt.Errorf("empty snapshot files")
	}

	old := oldSnaps[0]
	new := newSnaps[0]

	delta := snapshotDelta{
		OldStatus:   string(old.Status),
		NewStatus:   string(new.Status),
		Changed:     old.Status != new.Status,
		ConnDelta:   new.Connections.Current - old.Connections.Current,
		OpsCountOld: old.ActiveOps,
		OpsCountNew: new.ActiveOps,
	}

	if old.Status == snapshot.Healthy && new.Status != snapshot.Healthy {
		delta.NewIssues = append(delta.NewIssues, fmt.Sprintf("status degraded: %s -> %s", old.Status, new.Status))
	}
	if old.Status != snapshot.Healthy && new.Status == snapshot.Healthy {
		delta.Resolved = append(delta.Resolved, fmt.Sprintf("status recovered: %s -> %s", old.Status, new.Status))
	}

	if new.Connections.Ratio > 0.9 && old.Connections.Ratio <= 0.9 {
		delta.NewIssues = append(delta.NewIssues, "connection saturation >90%")
	}
	if old.Connections.Ratio > 0.9 && new.Connections.Ratio <= 0.9 {
		delta.Resolved = append(delta.Resolved, "connection saturation resolved")
	}

	if new.SlowOps > 0 && old.SlowOps == 0 {
		delta.NewIssues = append(delta.NewIssues, fmt.Sprintf("slow ops appeared: %d", new.SlowOps))
	}
	if old.SlowOps > 0 && new.SlowOps == 0 {
		delta.Resolved = append(delta.Resolved, "slow ops resolved")
	}

	switch diffFormat {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(delta)
	default:
		fmt.Printf("Status: %s -> %s\n", delta.OldStatus, delta.NewStatus)
		fmt.Printf("Connections: %+d\n", delta.ConnDelta)
		fmt.Printf("Active ops: %d -> %d\n", delta.OpsCountOld, delta.OpsCountNew)
		for _, issue := range delta.NewIssues {
			fmt.Printf("  NEW: %s\n", issue)
		}
		for _, r := range delta.Resolved {
			fmt.Printf("  RESOLVED: %s\n", r)
		}
		if !delta.Changed && len(delta.NewIssues) == 0 && len(delta.Resolved) == 0 {
			fmt.Println("  No significant changes")
		}
	}

	if len(delta.NewIssues) > 0 {
		exitCode = 1
	}
	return nil
}

func loadSnapshots(path string) ([]snapshot.Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var snaps []snapshot.Snapshot
	if err := json.Unmarshal(data, &snaps); err != nil {
		// Try single snapshot.
		var snap snapshot.Snapshot
		if err2 := json.Unmarshal(data, &snap); err2 != nil {
			return nil, err
		}
		return []snapshot.Snapshot{snap}, nil
	}
	return snaps, nil
}
