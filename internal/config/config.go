package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// MaskDSN redacts the password portion of mongodb:// URIs.
func MaskDSN(dsn string) string {
	if dsn == "" {
		return ""
	}
	u, err := url.Parse(dsn)
	if err != nil {
		return dsn
	}
	if u.User == nil {
		return dsn
	}
	if _, hasPass := u.User.Password(); !hasPass {
		return dsn
	}
	// Rebuild manually to avoid URL-encoding the mask characters.
	var b strings.Builder
	b.WriteString(u.Scheme)
	b.WriteString("://")
	b.WriteString(u.User.Username())
	b.WriteString(":REDACTED@")
	b.WriteString(u.Host)
	if u.Path != "" {
		b.WriteString(u.Path)
	}
	if u.RawQuery != "" {
		b.WriteByte('?')
		b.WriteString(u.RawQuery)
	}
	if u.Fragment != "" {
		b.WriteByte('#')
		b.WriteString(u.Fragment)
	}
	return b.String()
}

type Config struct {
	// Required: comma-separated for multi-target.
	DSN []string

	MetricsPort         int
	PollInterval        time.Duration
	SlowQueryThreshold  time.Duration
	RegressionThreshold float64
	StmtLimit           int

	// Alerting (all optional).
	TelegramBotToken string
	TelegramChatID   string
	AlertWebhookURL  string
	AlertCooldown    time.Duration

	// Grafana annotations (optional).
	GrafanaURL   string
	GrafanaToken string
	DashboardUID string
}

func Load() (Config, error) {
	dsn := os.Getenv("MONGO_DSN")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		return Config{}, fmt.Errorf("MONGO_DSN or DATABASE_URL must be set")
	}

	targets := splitDSN(dsn)
	if len(targets) == 0 {
		return Config{}, fmt.Errorf("MONGO_DSN must contain at least one URI")
	}

	port := 9216
	if v := os.Getenv("METRICS_PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid METRICS_PORT: %w", err)
		}
		port = p
	}

	pollInterval := 5 * time.Second
	if v := os.Getenv("POLL_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid POLL_INTERVAL: %w", err)
		}
		pollInterval = d
	}

	slowThreshold := 5 * time.Second
	if v := os.Getenv("SLOW_QUERY_THRESHOLD"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid SLOW_QUERY_THRESHOLD: %w", err)
		}
		slowThreshold = d
	}

	regressionThreshold := 2.0
	if v := os.Getenv("REGRESSION_THRESHOLD"); v != "" {
		r, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return Config{}, fmt.Errorf("invalid REGRESSION_THRESHOLD: %w", err)
		}
		regressionThreshold = r
	}

	stmtLimit := 50
	if v := os.Getenv("STMT_LIMIT"); v != "" {
		s, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid STMT_LIMIT: %w", err)
		}
		stmtLimit = s
	}

	alertCooldown := 5 * time.Minute
	if v := os.Getenv("ALERT_COOLDOWN"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid ALERT_COOLDOWN: %w", err)
		}
		alertCooldown = d
	}

	return Config{
		DSN:                 targets,
		MetricsPort:         port,
		PollInterval:        pollInterval,
		SlowQueryThreshold:  slowThreshold,
		RegressionThreshold: regressionThreshold,
		StmtLimit:           stmtLimit,
		TelegramBotToken:    os.Getenv("TELEGRAM_BOT_TOKEN"),
		TelegramChatID:      os.Getenv("TELEGRAM_CHAT_ID"),
		AlertWebhookURL:     os.Getenv("ALERT_WEBHOOK_URL"),
		AlertCooldown:       alertCooldown,
		GrafanaURL:          os.Getenv("GRAFANA_URL"),
		GrafanaToken:        os.Getenv("GRAFANA_TOKEN"),
		DashboardUID:        os.Getenv("GRAFANA_DASHBOARD_UID"),
	}, nil
}

func splitDSN(raw string) []string {
	var out []string
	for _, s := range strings.Split(raw, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
