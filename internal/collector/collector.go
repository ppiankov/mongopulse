package collector

import (
	"context"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/ppiankov/mongopulse/internal/config"
	"github.com/ppiankov/mongopulse/internal/metrics"
)

type Collector struct {
	client  *mongo.Client
	node    string
	metrics *metrics.Metrics
	cfg     config.Config
}

func New(client *mongo.Client, node string, m *metrics.Metrics, cfg config.Config) *Collector {
	return &Collector{
		client:  client,
		node:    node,
		metrics: m,
		cfg:     cfg,
	}
}

func (c *Collector) Collect(ctx context.Context) {
	start := time.Now()

	ss, err := c.collectServerStatus(ctx)
	if err != nil {
		log.Printf("[%s] serverStatus: %v", c.node, err)
		c.metrics.Up.WithLabelValues(c.node).Set(0)
		c.metrics.PollErrors.WithLabelValues(c.node).Inc()
		return
	}
	c.metrics.Up.WithLabelValues(c.node).Set(1)

	// Collectors that use serverStatus fields.
	c.collectConnections(ctx, ss)
	c.collectWiredTiger(ctx, ss)
	c.collectOpcounters(ctx, ss)
	c.collectCursors(ctx, ss)
	c.collectLocks(ctx, ss)
	c.collectNetwork(ctx, ss)

	// Collectors that issue their own commands.
	if err := c.collectReplication(ctx); err != nil {
		log.Printf("[%s] replication: %v", c.node, err)
	}
	if err := c.collectCurrentOp(ctx); err != nil {
		log.Printf("[%s] currentOp: %v", c.node, err)
	}

	c.collectCollections(ctx)
	c.collectDbStats(ctx)

	c.metrics.PollDuration.WithLabelValues(c.node).Set(time.Since(start).Seconds())
}

func (c *Collector) collectServerStatus(ctx context.Context) (map[string]interface{}, error) {
	var result bson.M
	err := c.client.Database("admin").RunCommand(ctx, bson.D{{Key: "serverStatus", Value: 1}}).Decode(&result)
	if err != nil {
		return nil, err
	}

	if v, ok := result["version"].(string); ok {
		c.metrics.Version.WithLabelValues(c.node, v).Set(1)
	}
	if v, ok := toFloat64(result["uptime"]); ok {
		c.metrics.Uptime.WithLabelValues(c.node).Set(v)
	}

	return result, nil
}
