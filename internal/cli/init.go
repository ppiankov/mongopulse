package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var initFormat string

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Print default configuration template",
	RunE:  runInit,
}

func init() {
	initCmd.Flags().StringVar(&initFormat, "format", "env", "Output format: env or json")
	rootCmd.AddCommand(initCmd)
}

type configSchema struct {
	Name        string `json:"name"`
	Default     string `json:"default"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

func runInit(cmd *cobra.Command, args []string) error {
	vars := []configSchema{
		{Name: "MONGO_DSN", Default: "mongodb://localhost:27017", Required: true, Description: "MongoDB connection URI (comma-separated for multi-target)"},
		{Name: "METRICS_PORT", Default: "9216", Required: false, Description: "Port for Prometheus /metrics endpoint"},
		{Name: "POLL_INTERVAL", Default: "5s", Required: false, Description: "How often to poll MongoDB for metrics"},
		{Name: "SLOW_QUERY_THRESHOLD", Default: "5s", Required: false, Description: "Queries slower than this are counted as slow"},
		{Name: "REGRESSION_THRESHOLD", Default: "2.0", Required: false, Description: "Multiplier over baseline mean to flag query regression"},
		{Name: "STMT_LIMIT", Default: "50", Required: false, Description: "Max profiler entries to process per poll"},
		{Name: "TELEGRAM_BOT_TOKEN", Default: "", Required: false, Description: "Telegram bot token for alerts (optional)"},
		{Name: "TELEGRAM_CHAT_ID", Default: "", Required: false, Description: "Telegram chat ID for alerts (optional)"},
		{Name: "ALERT_WEBHOOK_URL", Default: "", Required: false, Description: "Slack/generic webhook URL for alerts (optional)"},
		{Name: "ALERT_COOLDOWN", Default: "5m", Required: false, Description: "Minimum interval between repeated alerts of same type"},
		{Name: "GRAFANA_URL", Default: "", Required: false, Description: "Grafana base URL for anomaly annotations (optional)"},
		{Name: "GRAFANA_TOKEN", Default: "", Required: false, Description: "Grafana API token (optional)"},
		{Name: "GRAFANA_DASHBOARD_UID", Default: "", Required: false, Description: "Grafana dashboard UID for annotations (optional)"},
	}

	switch initFormat {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(vars)
	case "env":
		for _, v := range vars {
			req := ""
			if v.Required {
				req = " (required)"
			}
			fmt.Fprintf(os.Stdout, "# %s%s\n", v.Description, req)
			fmt.Fprintf(os.Stdout, "%s=%s\n\n", v.Name, v.Default)
		}
		return nil
	default:
		return fmt.Errorf("unknown format %q: use env or json", initFormat)
	}
}
