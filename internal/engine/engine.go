package engine

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/ppiankov/mongopulse/internal/collector"
	"github.com/ppiankov/mongopulse/internal/config"
	"github.com/ppiankov/mongopulse/internal/metrics"
)

type Target struct {
	DSN       string
	Node      string
	Client    *mongo.Client
	Collector *collector.Collector
}

type Engine struct {
	targets []*Target
	cfg     config.Config
	metrics *metrics.Metrics
}

func New(cfg config.Config, m *metrics.Metrics) *Engine {
	return &Engine{cfg: cfg, metrics: m}
}

func (e *Engine) Connect(ctx context.Context) error {
	for i, dsn := range e.cfg.DSN {
		node := nodeLabel(dsn, i)

		client, err := mongo.Connect(options.Client().ApplyURI(dsn))
		if err != nil {
			return fmt.Errorf("connect %s: %w", node, err)
		}

		if err := client.Ping(ctx, nil); err != nil {
			return fmt.Errorf("ping %s: %w", node, err)
		}

		t := &Target{
			DSN:       dsn,
			Node:      node,
			Client:    client,
			Collector: collector.New(client, node, e.metrics, e.cfg),
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
	// Extract host from mongodb://host:port/...
	// Simple approach: use index-based label, overridden if parseable.
	return fmt.Sprintf("node-%d", index)
}
