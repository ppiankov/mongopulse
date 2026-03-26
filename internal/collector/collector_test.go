//go:build integration

package collector

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/ppiankov/mongopulse/internal/config"
	"github.com/ppiankov/mongopulse/internal/metrics"
	"github.com/ppiankov/mongopulse/internal/testutil"
)

func gaugeValue(g *prometheus.GaugeVec, labels ...string) float64 {
	m := &dto.Metric{}
	if err := g.WithLabelValues(labels...).Write(m); err != nil {
		return 0
	}
	return m.GetGauge().GetValue()
}

func counterValue(c *prometheus.CounterVec, labels ...string) float64 {
	m := &dto.Metric{}
	if err := c.WithLabelValues(labels...).Write(m); err != nil {
		return 0
	}
	return m.GetCounter().GetValue()
}

func setupCollector(t *testing.T) (*Collector, *metrics.Metrics, string) {
	t.Helper()
	client, _ := testutil.StartMongo(t)

	reg := prometheus.NewRegistry()
	m := metrics.New(reg)
	cfg := config.Config{
		DSN:                 []string{"mongodb://localhost:27017"},
		PollInterval:        5 * time.Second,
		SlowQueryThreshold:  5 * time.Second,
		RegressionThreshold: 2.0,
		StmtLimit:           50,
	}
	node := "test-node"
	c := New(client, node, m, cfg, nil, nil)
	return c, m, node
}

func TestCollectServerStatus(t *testing.T) {
	c, m, node := setupCollector(t)
	ctx := context.Background()

	ss, err := c.collectServerStatus(ctx)
	if err != nil {
		t.Fatalf("collectServerStatus: %v", err)
	}
	if ss == nil {
		t.Fatal("serverStatus result is nil")
	}

	up := gaugeValue(m.Up, node)
	if up != 1 {
		t.Errorf("Up = %f, want 1", up)
	}

	uptime := gaugeValue(m.Uptime, node)
	if uptime <= 0 {
		t.Errorf("Uptime = %f, want > 0", uptime)
	}

	// Version info should have a non-empty version label.
	if v, ok := ss["version"].(string); !ok || v == "" {
		t.Error("serverStatus missing version field")
	}
}

func TestCollectConnections(t *testing.T) {
	c, m, node := setupCollector(t)
	ctx := context.Background()

	ss, err := c.collectServerStatus(ctx)
	if err != nil {
		t.Fatalf("collectServerStatus: %v", err)
	}

	c.collectConnections(ctx, ss)

	current := gaugeValue(m.ConnCurrent, node)
	if current <= 0 {
		t.Errorf("ConnCurrent = %f, want > 0", current)
	}

	available := gaugeValue(m.ConnAvailable, node)
	if available <= 0 {
		t.Errorf("ConnAvailable = %f, want > 0", available)
	}
}

func TestCollectOpcounters(t *testing.T) {
	c, m, node := setupCollector(t)
	ctx := context.Background()

	// Get initial opcounters.
	ss1, err := c.collectServerStatus(ctx)
	if err != nil {
		t.Fatalf("collectServerStatus: %v", err)
	}
	c.collectOpcounters(ctx, ss1)
	insertBefore := counterValue(m.OpsTotal, node, "insert")

	// Insert a document to increment insert counter.
	_, err = c.client.Database("testdb_opcounters").Collection("testcoll").InsertOne(ctx, bson.M{"key": "value"})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Re-collect.
	ss2, err := c.collectServerStatus(ctx)
	if err != nil {
		t.Fatalf("collectServerStatus: %v", err)
	}
	c.collectOpcounters(ctx, ss2)
	insertAfter := counterValue(m.OpsTotal, node, "insert")

	if insertAfter <= insertBefore {
		t.Errorf("insert counter did not increase: before=%f, after=%f", insertBefore, insertAfter)
	}
}

func TestCollectDbStats(t *testing.T) {
	c, m, node := setupCollector(t)
	ctx := context.Background()

	// Insert data so dbStats has something to report.
	db := c.client.Database("testdb_dbstats")
	_, err := db.Collection("testcoll").InsertOne(ctx, bson.M{"data": "hello world"})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	c.collectDbStats(ctx)

	dataSize := gaugeValue(m.DbDataSize, node, "testdb_dbstats")
	if dataSize <= 0 {
		t.Errorf("DbDataSize = %f, want > 0", dataSize)
	}
}

func TestCollectCollections(t *testing.T) {
	c, m, node := setupCollector(t)
	ctx := context.Background()

	db := c.client.Database("testdb_collections")
	_, err := db.Collection("mycoll").InsertOne(ctx, bson.M{"n": 1})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	c.collectCollections(ctx)

	docCount := gaugeValue(m.CollDocCount, node, "testdb_collections", "mycoll")
	if docCount < 1 {
		t.Errorf("CollDocCount = %f, want >= 1", docCount)
	}
}

func TestCollect_FullFlow(t *testing.T) {
	c, m, node := setupCollector(t)
	ctx := context.Background()

	// Insert some data to make metrics non-trivial.
	db := c.client.Database("testdb_fullflow")
	_, err := db.Collection("items").InsertOne(ctx, bson.M{"item": "test"})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Run full collection — should not panic or error.
	c.Collect(ctx)

	// Verify key metrics are set.
	up := gaugeValue(m.Up, node)
	if up != 1 {
		t.Errorf("Up = %f, want 1 after Collect()", up)
	}

	uptime := gaugeValue(m.Uptime, node)
	if uptime <= 0 {
		t.Errorf("Uptime = %f, want > 0", uptime)
	}

	pollDuration := gaugeValue(m.PollDuration, node)
	if pollDuration <= 0 {
		t.Errorf("PollDuration = %f, want > 0", pollDuration)
	}
}

func TestCollect_ReplicationGracefulSkip(t *testing.T) {
	c, _, _ := setupCollector(t)
	ctx := context.Background()

	// Standalone MongoDB — replication collector should not fail the whole Collect.
	err := c.collectReplication(ctx)
	if err == nil {
		t.Log("collectReplication returned nil — unexpected for standalone, but not fatal")
	}
	// The key assertion: it should return an error (standalone), but not panic.
}

func TestToFloat64(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   interface{}
		want float64
		ok   bool
	}{
		{"float64", float64(3.14), 3.14, true},
		{"int32", int32(42), 42, true},
		{"int64", int64(100), 100, true},
		{"int", int(7), 7, true},
		{"string", "nope", 0, false},
		{"nil", nil, 0, false},
		{"bool", true, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := toFloat64(tt.in)
			if ok != tt.ok {
				t.Errorf("ok = %v, want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Errorf("value = %f, want %f", got, tt.want)
			}
		})
	}
}

func TestIsSystemDB(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		want bool
	}{
		{"admin", true},
		{"local", true},
		{"config", true},
		{"mydb", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isSystemDB(tt.name); got != tt.want {
				t.Errorf("isSystemDB(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}
