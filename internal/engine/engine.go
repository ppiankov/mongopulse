package engine

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/ppiankov/mongopulse/internal/alerter"
	"github.com/ppiankov/mongopulse/internal/annotator"
	"github.com/ppiankov/mongopulse/internal/collector"
	"github.com/ppiankov/mongopulse/internal/config"
	"github.com/ppiankov/mongopulse/internal/metrics"
	"github.com/ppiankov/mongopulse/internal/retry"
)

type Target struct {
	DSN       string
	Node      string
	Client    *mongo.Client
	Collector *collector.Collector
}

type Engine struct {
	targets   []*Target
	cfg       config.Config
	metrics   *metrics.Metrics
	alerter   *alerter.Alerter
	annotator *annotator.Annotator
}

func New(cfg config.Config, m *metrics.Metrics, al *alerter.Alerter, an *annotator.Annotator) *Engine {
	return &Engine{cfg: cfg, metrics: m, alerter: al, annotator: an}
}

func (e *Engine) Connect(ctx context.Context) error {
	rc := retry.DefaultConfig()
	for i, dsn := range e.cfg.DSN {
		node := nodeLabel(dsn, i)

		var client *mongo.Client
		err := retry.Do(ctx, rc, "connect-"+node, func(ctx context.Context) error {
			c, err := mongo.Connect(options.Client().ApplyURI(dsn))
			if err != nil {
				return fmt.Errorf("connect: %w", err)
			}
			if err := c.Ping(ctx, nil); err != nil {
				return fmt.Errorf("ping: %w", err)
			}
			client = c
			return nil
		})
		if err != nil {
			return fmt.Errorf("%s: %w", node, err)
		}

		t := &Target{
			DSN:       dsn,
			Node:      node,
			Client:    client,
			Collector: collector.New(client, node, e.metrics, e.cfg, e.alerter, e.annotator),
		}
		e.targets = append(e.targets, t)
		log.Printf("connected to %s", node)
	}
	return nil
}

func (e *Engine) Run(ctx context.Context) {
	ticker := time.NewTicker(e.cfg.PollInterval)
	defer ticker.Stop()

	e.poll(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.poll(ctx)
		}
	}
}

func (e *Engine) Close(ctx context.Context) {
	for _, t := range e.targets {
		if err := t.Client.Disconnect(ctx); err != nil {
			log.Printf("disconnect %s: %v", t.Node, err)
		}
	}
}

func (e *Engine) Targets() []*Target {
	return e.targets
}

func (e *Engine) poll(ctx context.Context) {
	for _, t := range e.targets {
		t.Collector.Collect(ctx)
	}
}

func nodeLabel(dsn string, index int) string {
	// Extract host:port from mongodb://user:pass@host:port/db?opts
	// Strip scheme.
	s := dsn
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	// Strip credentials.
	if i := strings.LastIndex(s, "@"); i >= 0 {
		s = s[i+1:]
	}
	// Strip path and query.
	if i := strings.IndexAny(s, "/?"); i >= 0 {
		s = s[:i]
	}
	if s != "" {
		return s
	}
	return fmt.Sprintf("node-%d", index)
}
