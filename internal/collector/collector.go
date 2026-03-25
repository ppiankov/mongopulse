package collector

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/ppiankov/mongopulse/internal/alerter"
	"github.com/ppiankov/mongopulse/internal/annotator"
	"github.com/ppiankov/mongopulse/internal/config"
	"github.com/ppiankov/mongopulse/internal/metrics"
)

type Collector struct {
	client    *mongo.Client
	node      string
	metrics   *metrics.Metrics
	cfg       config.Config
	alerter   *alerter.Alerter
	annotator *annotator.Annotator
}

func New(client *mongo.Client, node string, m *metrics.Metrics, cfg config.Config, al *alerter.Alerter, an *annotator.Annotator) *Collector {
	return &Collector{
		client:    client,
		node:      node,
		metrics:   m,
		cfg:       cfg,
		alerter:   al,
		annotator: an,
	}
}

func (c *Collector) Collect(ctx context.Context) {
	start := time.Now()

	ss, err := c.collectServerStatus(ctx)
	if err != nil {
		log.Printf("[%s] serverStatus: %v", c.node, err)
		c.metrics.Up.WithLabelValues(c.node).Set(0)
		c.metrics.PollErrors.WithLabelValues(c.node).Inc()
		c.fireAlert(alerter.AlertNodeDown, fmt.Sprintf("serverStatus failed: %v", err))
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

	// Differentiator collectors.
	c.collectProfiler(ctx)
	c.collectIndexUsage(ctx)
	c.collectTopology(ctx)
	c.collectConnPrediction(ctx, ss)

	// Sharding (mongos only, no-op on mongod).
	c.collectSharding(ctx)
	c.collectBalancerActivity(ctx)

	// Alert checks.
	c.checkAlerts(ss)

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

func (c *Collector) checkAlerts(ss map[string]interface{}) {
	// Connection saturation >90%.
	if conns, ok := ss["connections"].(map[string]interface{}); ok {
		cur, _ := toFloat64(conns["current"])
		avail, _ := toFloat64(conns["available"])
		if total := cur + avail; total > 0 && cur/total > 0.9 {
			c.fireAlert(alerter.AlertConnSaturation, fmt.Sprintf("connections %.0f%% (%.0f/%.0f)", cur/total*100, cur, total))
			c.annotate("connection saturation spike", "connections", "alert")
		}
	}

	// Cache pressure >80%.
	if wt, ok := ss["wiredTiger"].(map[string]interface{}); ok {
		if cache, ok := wt["cache"].(map[string]interface{}); ok {
			used, _ := toFloat64(cache["bytes currently in the cache"])
			max, _ := toFloat64(cache["maximum bytes configured"])
			if max > 0 && used/max > 0.8 {
				c.fireAlert(alerter.AlertCachePressure, fmt.Sprintf("cache %.0f%%", used/max*100))
				c.annotate("cache pressure spike", "wiredtiger", "alert")
			}
		}
	}
}

func (c *Collector) fireAlert(t alerter.AlertType, msg string) {
	if c.alerter == nil {
		return
	}
	c.alerter.Fire(alerter.Alert{
		Type:    t,
		Node:    c.node,
		Message: msg,
		Time:    time.Now(),
	})
}

func (c *Collector) annotate(text string, tags ...string) {
	if c.annotator == nil {
		return
	}
	c.annotator.Annotate(fmt.Sprintf("[%s] %s", c.node, text), tags...)
}
