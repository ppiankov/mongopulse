package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/spf13/cobra"

	"github.com/ppiankov/mongopulse/internal/config"
	"github.com/ppiankov/mongopulse/internal/snapshot"
)

var (
	watchInterval string
	watchFormat   string
	watchSlackURL string
	watchOnce     bool
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Continuous anomaly detection with delta reporting",
	RunE:  runWatch,
}

func init() {
	watchCmd.Flags().StringVar(&watchInterval, "interval", "5m", "Poll interval")
	watchCmd.Flags().StringVar(&watchFormat, "format", "text", "Output format: text or json")
	watchCmd.Flags().StringVar(&watchSlackURL, "slack-webhook", "", "Slack webhook URL for notifications")
	watchCmd.Flags().BoolVar(&watchOnce, "once", false, "Run once and exit (CI mode)")
	rootCmd.AddCommand(watchCmd)
}

type watchDelta struct {
	Timestamp string   `json:"timestamp"`
	NewIssues []string `json:"new_issues,omitempty"`
	Resolved  []string `json:"resolved,omitempty"`
	Status    string   `json:"status"`
}

func runWatch(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	interval, err := time.ParseDuration(watchInterval)
	if err != nil {
		return fmt.Errorf("invalid interval: %w", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(cfg.DSN[0]).SetTimeout(10 * time.Second))
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer client.Disconnect(ctx)

	var prevSnap *snapshot.Snapshot

	for {
		snap := snapshot.Take(ctx, client, cfg.DSN[0], cfg.SlowQueryThreshold.Seconds())

		if prevSnap != nil {
			delta := computeDelta(prevSnap, &snap)
			outputDelta(delta)

			if watchSlackURL != "" && (len(delta.NewIssues) > 0) {
				sendSlackNotification(watchSlackURL, delta)
			}

			if watchOnce {
				switch snap.Status {
				case snapshot.Critical:
					exitCode = 2
				case snapshot.Degraded:
					exitCode = 1
				}
				return nil
			}
		} else {
			fmt.Fprintf(os.Stderr, "Baseline established (%s)\n", snap.Status)
			if watchOnce {
				switch snap.Status {
				case snapshot.Critical:
					exitCode = 2
				case snapshot.Degraded:
					exitCode = 1
				}
				return nil
			}
		}

		prevSnap = &snap

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(interval):
		}
	}
}

func computeDelta(prev, curr *snapshot.Snapshot) watchDelta {
	d := watchDelta{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Status:    string(curr.Status),
	}

	if prev.Status == snapshot.Healthy && curr.Status != snapshot.Healthy {
		d.NewIssues = append(d.NewIssues, fmt.Sprintf("status changed: %s -> %s", prev.Status, curr.Status))
	}
	if prev.Status != snapshot.Healthy && curr.Status == snapshot.Healthy {
		d.Resolved = append(d.Resolved, fmt.Sprintf("status recovered: %s -> %s", prev.Status, curr.Status))
	}

	if curr.Connections.Ratio > 0.9 && prev.Connections.Ratio <= 0.9 {
		d.NewIssues = append(d.NewIssues, fmt.Sprintf("connection saturation: %.0f%%", curr.Connections.Ratio*100))
	}
	if prev.Connections.Ratio > 0.9 && curr.Connections.Ratio <= 0.9 {
		d.Resolved = append(d.Resolved, "connection saturation resolved")
	}

	if curr.WiredTiger.CacheRatio > 0.8 && prev.WiredTiger.CacheRatio <= 0.8 {
		d.NewIssues = append(d.NewIssues, fmt.Sprintf("cache pressure: %.0f%%", curr.WiredTiger.CacheRatio*100))
	}
	if prev.WiredTiger.CacheRatio > 0.8 && curr.WiredTiger.CacheRatio <= 0.8 {
		d.Resolved = append(d.Resolved, "cache pressure resolved")
	}

	if curr.SlowOps > 0 && prev.SlowOps == 0 {
		d.NewIssues = append(d.NewIssues, fmt.Sprintf("slow ops detected: %d", curr.SlowOps))
	}
	if prev.SlowOps > 0 && curr.SlowOps == 0 {
		d.Resolved = append(d.Resolved, "slow ops resolved")
	}

	return d
}

func outputDelta(d watchDelta) {
	if len(d.NewIssues) == 0 && len(d.Resolved) == 0 {
		return
	}

	switch watchFormat {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.Encode(d)
	default:
		fmt.Printf("[%s] Status: %s\n", d.Timestamp, d.Status)
		for _, issue := range d.NewIssues {
			fmt.Printf("  NEW: %s\n", issue)
		}
		for _, r := range d.Resolved {
			fmt.Printf("  RESOLVED: %s\n", r)
		}
	}
}

func sendSlackNotification(url string, d watchDelta) {
	text := fmt.Sprintf("*mongopulse watch* — %s\n", d.Status)
	for _, issue := range d.NewIssues {
		text += fmt.Sprintf("• NEW: %s\n", issue)
	}
	for _, r := range d.Resolved {
		text += fmt.Sprintf("• RESOLVED: %s\n", r)
	}

	body, _ := json.Marshal(map[string]string{"text": text})
	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "slack notification failed: %v\n", err)
		return
	}
	resp.Body.Close()
}
