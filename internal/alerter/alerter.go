package alerter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/ppiankov/mongopulse/internal/config"
)

type AlertType string

const (
	AlertNodeDown       AlertType = "node_down"
	AlertConnSaturation AlertType = "conn_saturation"
	AlertReplicaLag     AlertType = "replica_lag"
	AlertNoPrimary      AlertType = "no_primary"
	AlertCachePressure  AlertType = "cache_pressure"
	AlertSlowQuery      AlertType = "slow_query"
	AlertRegression     AlertType = "query_regression"
	AlertElectionStorm  AlertType = "election_storm"
	AlertOplogWindow    AlertType = "oplog_window"
	AlertConnExhaustion AlertType = "conn_exhaustion"
)

type Alert struct {
	Type    AlertType `json:"type"`
	Node    string    `json:"node"`
	Message string    `json:"message"`
	Time    time.Time `json:"time"`
}

type Alerter struct {
	telegramToken string
	telegramChat  string
	webhookURL    string
	cooldown      time.Duration
	client        *http.Client
	mu            sync.Mutex
	lastSent      map[AlertType]time.Time
}

func New(cfg config.Config) *Alerter {
	hasTelegram := cfg.TelegramBotToken != "" && cfg.TelegramChatID != ""
	hasWebhook := cfg.AlertWebhookURL != ""

	if !hasTelegram && !hasWebhook {
		return nil
	}

	return &Alerter{
		telegramToken: cfg.TelegramBotToken,
		telegramChat:  cfg.TelegramChatID,
		webhookURL:    cfg.AlertWebhookURL,
		cooldown:      cfg.AlertCooldown,
		client:        &http.Client{Timeout: 10 * time.Second},
		lastSent:      make(map[AlertType]time.Time),
	}
}

func (a *Alerter) Fire(alert Alert) {
	if a == nil {
		return
	}

	a.mu.Lock()
	last, ok := a.lastSent[alert.Type]
	if ok && time.Since(last) < a.cooldown {
		a.mu.Unlock()
		return
	}
	a.lastSent[alert.Type] = time.Now()
	a.mu.Unlock()

	if a.telegramToken != "" {
		go a.sendTelegram(alert)
	}
	if a.webhookURL != "" {
		go a.sendWebhook(alert)
	}
}

func (a *Alerter) sendTelegram(alert Alert) {
	text := fmt.Sprintf("<b>mongopulse [%s]</b>\n<b>%s</b> on <code>%s</code>\n%s",
		alert.Type, alert.Type, alert.Node, alert.Message)

	body, _ := json.Marshal(map[string]string{
		"chat_id":    a.telegramChat,
		"text":       text,
		"parse_mode": "HTML",
	})

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", a.telegramToken)
	resp, err := a.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		slog.Error("telegram alert failed", "error", err)
		return
	}
	resp.Body.Close()
}

func (a *Alerter) sendWebhook(alert Alert) {
	body, _ := json.Marshal(alert)
	resp, err := a.client.Post(a.webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		slog.Error("webhook alert failed", "error", err)
		return
	}
	resp.Body.Close()
}
