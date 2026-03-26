package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/spf13/cobra"

	"github.com/ppiankov/mongopulse/internal/baseline"
	"github.com/ppiankov/mongopulse/internal/config"
	"github.com/ppiankov/mongopulse/internal/policy"
	"github.com/ppiankov/mongopulse/internal/snapshot"
)

var (
	statusFormat         string
	statusUnhealthy      bool
	statusPolicy         string
	statusBaseline       string
	statusBaselineCreate bool
	statusExpires        string
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "One-shot cluster health snapshot",
	RunE:  runStatus,
}

func init() {
	statusCmd.Flags().StringVar(&statusFormat, "format", "text", "Output format: text or json")
	statusCmd.Flags().BoolVar(&statusUnhealthy, "unhealthy", false, "Only show unhealthy nodes")
	statusCmd.Flags().StringVar(&statusPolicy, "policy", "", "Path to policy YAML file for policy-as-code enforcement")
	statusCmd.Flags().StringVar(&statusBaseline, "baseline", "", "Path to baseline JSON file for suppressing known conditions")
	statusCmd.Flags().BoolVar(&statusBaselineCreate, "baseline-create", false, "Save current conditions as a baseline file (requires --baseline)")
	statusCmd.Flags().StringVar(&statusExpires, "expires", "", "Expiry duration for baseline entries (e.g. 90d, 24h)")
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var snaps []snapshot.Snapshot

	for _, dsn := range cfg.DSN {
		client, err := mongo.Connect(options.Client().ApplyURI(dsn).SetTimeout(10 * time.Second))
		if err != nil {
			return fmt.Errorf("connect: %w", err)
		}

		node := dsn // simplified; engine.nodeLabel parses in serve mode
		snap := snapshot.Take(ctx, client, node, cfg.SlowQueryThreshold.Seconds())
		client.Disconnect(ctx)

		if statusUnhealthy && !snap.IsUnhealthy() {
			continue
		}
		snaps = append(snaps, snap)
	}

	var violations []policy.PolicyViolation
	if statusPolicy != "" {
		pol, err := policy.Load(statusPolicy)
		if err != nil {
			return fmt.Errorf("policy: %w", err)
		}
		for _, s := range snaps {
			violations = append(violations, policy.Evaluate(pol, s)...)
		}
	}

	if statusBaselineCreate && statusBaseline != "" {
		expires, err := parseDuration(statusExpires)
		if err != nil {
			return fmt.Errorf("expires: %w", err)
		}
		b := baseline.FromViolations(violations, expires)
		if err := baseline.Save(statusBaseline, b); err != nil {
			return fmt.Errorf("baseline save: %w", err)
		}
		fmt.Printf("Baseline saved to %s (%d conditions)\n", statusBaseline, len(b.Entries))
		return nil
	}

	if statusBaseline != "" {
		b, err := baseline.Load(statusBaseline)
		if err != nil {
			return fmt.Errorf("baseline: %w", err)
		}
		violations = baseline.FilterViolations(violations, b)
	}

	switch statusFormat {
	case "json":
		type statusOutput struct {
			Snapshots        []snapshot.Snapshot      `json:"snapshots"`
			PolicyViolations []policy.PolicyViolation `json:"policy_violations,omitempty"`
		}
		out := statusOutput{Snapshots: snaps, PolicyViolations: violations}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	default:
		for _, s := range snaps {
			printSnapshot(s)
		}
		if len(violations) > 0 {
			fmt.Println("Policy violations:")
			for _, v := range violations {
				fmt.Printf("  [%s] %s: actual=%s threshold=%s\n", v.Severity, v.Rule, v.Actual, v.Threshold)
			}
		}
	}

	if len(violations) > 0 {
		exitCode = 6
		return nil
	}

	// Exit codes: 0=healthy, 1=degraded, 2=critical.
	for _, s := range snaps {
		switch s.Status {
		case snapshot.Critical:
			exitCode = 2
		case snapshot.Degraded:
			if exitCode < 1 {
				exitCode = 1
			}
		}
	}

	return nil
}

func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}

	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, fmt.Errorf("invalid day duration %q: %w", s, err)
		}
		if days <= 0 {
			return 0, fmt.Errorf("duration must be positive, got %q", s)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}

	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: %w", s, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("duration must be positive, got %q", s)
	}
	return d, nil
}

func printSnapshot(s snapshot.Snapshot) {
	fmt.Printf("Node: %s  Status: %s\n", s.Node, s.Status)
	fmt.Printf("  Version: %s  Uptime: %.0fh\n", s.Version, s.Uptime/3600)
	fmt.Printf("  Connections: %d/%d (%.0f%%)\n", s.Connections.Current, s.Connections.Current+s.Connections.Available, s.Connections.Ratio*100)
	fmt.Printf("  Cache: %.1f/%.1f MB (%.0f%%)\n",
		s.WiredTiger.CacheUsedBytes/1024/1024,
		s.WiredTiger.CacheMaxBytes/1024/1024,
		s.WiredTiger.CacheRatio*100)
	fmt.Printf("  Active ops: %d  Slow ops: %d\n", s.ActiveOps, s.SlowOps)
	if s.ReplSet != nil {
		fmt.Printf("  Replica set: %s (%s)\n", s.ReplSet.Set, s.ReplSet.State)
		for _, m := range s.ReplSet.Members {
			if m.LagSecs > 0 {
				fmt.Printf("    %s: %s (lag: %.1fs)\n", m.Name, m.State, m.LagSecs)
			} else {
				fmt.Printf("    %s: %s\n", m.Name, m.State)
			}
		}
	}
	fmt.Println()
}
