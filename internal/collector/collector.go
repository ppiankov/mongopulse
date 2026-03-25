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

	if err := c.collectServerStatus(ctx); err != nil {
		log.Printf("[%s] serverStatus: %v", c.node, err)
		c.metrics.Up.WithLabelValues(c.node).Set(0)
		c.metrics.PollErrors.WithLabelValues(c.node).Inc()
		return
	}
	c.metrics.Up.WithLabelValues(c.node).Set(1)
	c.metrics.PollDuration.WithLabelValues(c.node).Set(time.Since(start).Seconds())
}

func (c *Collector) collectServerStatus(ctx context.Context) error {
	var result bson.M
	err := c.client.Database("admin").RunCommand(ctx, bson.D{{Key: "serverStatus", Value: 1}}).Decode(&result)
	if err != nil {
		return err
	}

	if v, ok := result["version"].(string); ok {
		c.metrics.Version.WithLabelValues(c.node, v).Set(1)
	}
	if v, ok := result["uptime"].(float64); ok {
		c.metrics.Uptime.WithLabelValues(c.node).Set(v)
	} else if v, ok := result["uptime"].(int32); ok {
		c.metrics.Uptime.WithLabelValues(c.node).Set(float64(v))
	}

	return nil
}
