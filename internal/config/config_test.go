package config

import (
	"os"
	"testing"
	"time"
)

func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"MONGO_DSN", "DATABASE_URL", "METRICS_PORT", "POLL_INTERVAL",
		"SLOW_QUERY_THRESHOLD", "REGRESSION_THRESHOLD", "STMT_LIMIT",
		"TELEGRAM_BOT_TOKEN", "TELEGRAM_CHAT_ID", "ALERT_WEBHOOK_URL",
		"ALERT_COOLDOWN", "GRAFANA_URL", "GRAFANA_TOKEN", "GRAFANA_DASHBOARD_UID",
	} {
		t.Setenv(key, "")
		os.Unsetenv(key)
	}
}

func TestLoad_NoDSN(t *testing.T) {
	clearEnv(t)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when MONGO_DSN is not set")
	}
}

func TestLoad_SingleDSN(t *testing.T) {
	clearEnv(t)
	t.Setenv("MONGO_DSN", "mongodb://localhost:27017")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.DSN) != 1 {
		t.Fatalf("expected 1 DSN, got %d", len(cfg.DSN))
	}
	if cfg.DSN[0] != "mongodb://localhost:27017" {
		t.Errorf("DSN[0] = %q, want %q", cfg.DSN[0], "mongodb://localhost:27017")
	}
}

func TestLoad_CommaSeparatedDSN(t *testing.T) {
	clearEnv(t)
	t.Setenv("MONGO_DSN", "mongodb://host1:27017, mongodb://host2:27017, mongodb://host3:27017")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.DSN) != 3 {
		t.Fatalf("expected 3 DSNs, got %d", len(cfg.DSN))
	}
	if cfg.DSN[1] != "mongodb://host2:27017" {
		t.Errorf("DSN[1] = %q, want %q", cfg.DSN[1], "mongodb://host2:27017")
	}
}

func TestLoad_DATABASE_URL_Fallback(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "mongodb://fallback:27017")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.DSN) != 1 || cfg.DSN[0] != "mongodb://fallback:27017" {
		t.Errorf("expected DATABASE_URL fallback, got %v", cfg.DSN)
	}
}

func TestLoad_Defaults(t *testing.T) {
	clearEnv(t)
	t.Setenv("MONGO_DSN", "mongodb://localhost:27017")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.MetricsPort != 9216 {
		t.Errorf("MetricsPort = %d, want 9216", cfg.MetricsPort)
	}
	if cfg.PollInterval != 5*time.Second {
		t.Errorf("PollInterval = %v, want 5s", cfg.PollInterval)
	}
	if cfg.SlowQueryThreshold != 5*time.Second {
		t.Errorf("SlowQueryThreshold = %v, want 5s", cfg.SlowQueryThreshold)
	}
	if cfg.RegressionThreshold != 2.0 {
		t.Errorf("RegressionThreshold = %f, want 2.0", cfg.RegressionThreshold)
	}
	if cfg.StmtLimit != 50 {
		t.Errorf("StmtLimit = %d, want 50", cfg.StmtLimit)
	}
	if cfg.AlertCooldown != 5*time.Minute {
		t.Errorf("AlertCooldown = %v, want 5m", cfg.AlertCooldown)
	}
}

func TestLoad_InvalidMetricsPort(t *testing.T) {
	clearEnv(t)
	t.Setenv("MONGO_DSN", "mongodb://localhost:27017")
	t.Setenv("METRICS_PORT", "not-a-number")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid METRICS_PORT")
	}
}

func TestLoad_InvalidPollInterval(t *testing.T) {
	clearEnv(t)
	t.Setenv("MONGO_DSN", "mongodb://localhost:27017")
	t.Setenv("POLL_INTERVAL", "bad")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid POLL_INTERVAL")
	}
}

func TestLoad_InvalidSlowQueryThreshold(t *testing.T) {
	clearEnv(t)
	t.Setenv("MONGO_DSN", "mongodb://localhost:27017")
	t.Setenv("SLOW_QUERY_THRESHOLD", "xyz")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid SLOW_QUERY_THRESHOLD")
	}
}

func TestLoad_InvalidRegressionThreshold(t *testing.T) {
	clearEnv(t)
	t.Setenv("MONGO_DSN", "mongodb://localhost:27017")
	t.Setenv("REGRESSION_THRESHOLD", "abc")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid REGRESSION_THRESHOLD")
	}
}

func TestLoad_InvalidStmtLimit(t *testing.T) {
	clearEnv(t)
	t.Setenv("MONGO_DSN", "mongodb://localhost:27017")
	t.Setenv("STMT_LIMIT", "nope")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid STMT_LIMIT")
	}
}

func TestLoad_InvalidAlertCooldown(t *testing.T) {
	clearEnv(t)
	t.Setenv("MONGO_DSN", "mongodb://localhost:27017")
	t.Setenv("ALERT_COOLDOWN", "???")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid ALERT_COOLDOWN")
	}
}

func TestLoad_CustomValues(t *testing.T) {
	clearEnv(t)
	t.Setenv("MONGO_DSN", "mongodb://localhost:27017")
	t.Setenv("METRICS_PORT", "8080")
	t.Setenv("POLL_INTERVAL", "10s")
	t.Setenv("SLOW_QUERY_THRESHOLD", "2s")
	t.Setenv("REGRESSION_THRESHOLD", "3.5")
	t.Setenv("STMT_LIMIT", "100")
	t.Setenv("ALERT_COOLDOWN", "10m")
	t.Setenv("TELEGRAM_BOT_TOKEN", "tok123")
	t.Setenv("TELEGRAM_CHAT_ID", "chat456")
	t.Setenv("ALERT_WEBHOOK_URL", "https://hooks.example.com")
	t.Setenv("GRAFANA_URL", "https://grafana.example.com")
	t.Setenv("GRAFANA_TOKEN", "graftok")
	t.Setenv("GRAFANA_DASHBOARD_UID", "dash1")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MetricsPort != 8080 {
		t.Errorf("MetricsPort = %d, want 8080", cfg.MetricsPort)
	}
	if cfg.PollInterval != 10*time.Second {
		t.Errorf("PollInterval = %v, want 10s", cfg.PollInterval)
	}
	if cfg.SlowQueryThreshold != 2*time.Second {
		t.Errorf("SlowQueryThreshold = %v, want 2s", cfg.SlowQueryThreshold)
	}
	if cfg.RegressionThreshold != 3.5 {
		t.Errorf("RegressionThreshold = %f, want 3.5", cfg.RegressionThreshold)
	}
	if cfg.StmtLimit != 100 {
		t.Errorf("StmtLimit = %d, want 100", cfg.StmtLimit)
	}
	if cfg.AlertCooldown != 10*time.Minute {
		t.Errorf("AlertCooldown = %v, want 10m", cfg.AlertCooldown)
	}
	if cfg.TelegramBotToken != "tok123" {
		t.Errorf("TelegramBotToken = %q, want %q", cfg.TelegramBotToken, "tok123")
	}
	if cfg.TelegramChatID != "chat456" {
		t.Errorf("TelegramChatID = %q, want %q", cfg.TelegramChatID, "chat456")
	}
	if cfg.AlertWebhookURL != "https://hooks.example.com" {
		t.Errorf("AlertWebhookURL = %q", cfg.AlertWebhookURL)
	}
	if cfg.GrafanaURL != "https://grafana.example.com" {
		t.Errorf("GrafanaURL = %q", cfg.GrafanaURL)
	}
	if cfg.GrafanaToken != "graftok" {
		t.Errorf("GrafanaToken = %q", cfg.GrafanaToken)
	}
	if cfg.DashboardUID != "dash1" {
		t.Errorf("DashboardUID = %q", cfg.DashboardUID)
	}
}

func TestSplitDSN_Empty(t *testing.T) {
	t.Parallel()
	result := splitDSN("")
	if len(result) != 0 {
		t.Errorf("expected empty slice, got %v", result)
	}
}

func TestSplitDSN_Whitespace(t *testing.T) {
	t.Parallel()
	result := splitDSN("  ,  , ")
	if len(result) != 0 {
		t.Errorf("expected empty slice for whitespace-only entries, got %v", result)
	}
}

func TestSplitDSN_Trim(t *testing.T) {
	t.Parallel()
	result := splitDSN("  a , b , c  ")
	if len(result) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(result))
	}
	if result[0] != "a" || result[1] != "b" || result[2] != "c" {
		t.Errorf("unexpected result: %v", result)
	}
}
