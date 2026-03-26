//go:build integration

package engine

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ppiankov/mongopulse/internal/config"
	"github.com/ppiankov/mongopulse/internal/metrics"
	"github.com/ppiankov/mongopulse/internal/testutil"
)

func TestNodeLabel_HostPort(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		dsn   string
		index int
		want  string
	}{
		{"simple", "mongodb://localhost:27017", 0, "localhost:27017"},
		{"with_db", "mongodb://localhost:27017/testdb", 0, "localhost:27017"},
		{"with_query", "mongodb://localhost:27017/?replicaSet=rs0", 0, "localhost:27017"},
		{"no_port", "mongodb://myhost/testdb", 0, "myhost"},
		{"no_scheme", "localhost:27017", 0, "localhost:27017"},
		{"empty_fallback", "", 3, "node-3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := nodeLabel(tt.dsn, tt.index)
			if got != tt.want {
				t.Errorf("nodeLabel(%q, %d) = %q, want %q", tt.dsn, tt.index, got, tt.want)
			}
		})
	}
}

func TestConnect_RealMongoDB(t *testing.T) {
	_, uri := testutil.StartMongo(t)

	reg := prometheus.NewRegistry()
	m := metrics.New(reg)

	cfg := config.Config{
		DSN:                 []string{uri},
		PollInterval:        5 * time.Second,
		SlowQueryThreshold:  5 * time.Second,
		RegressionThreshold: 2.0,
		StmtLimit:           50,
	}

	eng := New(cfg, m, nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := eng.Connect(ctx); err != nil {
		t.Fatalf("Connect() error: %v", err)
	}
	defer eng.Close(ctx)

	targets := eng.Targets()
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}

	if targets[0].Client == nil {
		t.Error("target client is nil")
	}
	if targets[0].Collector == nil {
		t.Error("target collector is nil")
	}
	if targets[0].Node == "" {
		t.Error("target node label is empty")
	}
}

func TestTargets_EmptyBeforeConnect(t *testing.T) {
	t.Parallel()
	reg := prometheus.NewRegistry()
	m := metrics.New(reg)
	cfg := config.Config{DSN: []string{"mongodb://localhost:27017"}}
	eng := New(cfg, m, nil, nil)
	if len(eng.Targets()) != 0 {
		t.Errorf("expected 0 targets before Connect, got %d", len(eng.Targets()))
	}
}
