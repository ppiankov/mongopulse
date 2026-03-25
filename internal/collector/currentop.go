package collector

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func (c *Collector) collectCurrentOp(ctx context.Context) error {
	var result bson.M
	err := c.client.Database("admin").RunCommand(ctx, bson.D{
		{Key: "currentOp", Value: 1},
		{Key: "active", Value: true},
	}).Decode(&result)
	if err != nil {
		return err
	}

	inprog, _ := result["inprog"].(bson.A)

	var active, slow float64
	var longestSecs float64

	thresholdSecs := c.cfg.SlowQueryThreshold.Seconds()

	for _, raw := range inprog {
		op, ok := raw.(bson.M)
		if !ok {
			continue
		}
		active++

		secs, ok := toFloat64(op["secs_running"])
		if ok {
			if secs > longestSecs {
				longestSecs = secs
			}
			if secs > thresholdSecs {
				slow++
			}
		}
	}

	c.metrics.ActiveOps.WithLabelValues(c.node).Set(active)
	c.metrics.SlowOps.WithLabelValues(c.node).Set(slow)
	c.metrics.LongestOps.WithLabelValues(c.node).Set(longestSecs)

	return nil
}
