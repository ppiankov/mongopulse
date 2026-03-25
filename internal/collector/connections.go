package collector

import "context"

func (c *Collector) collectConnections(ctx context.Context, serverStatus map[string]interface{}) {
	conns, ok := serverStatus["connections"].(map[string]interface{})
	if !ok {
		return
	}

	if v, ok := toFloat64(conns["current"]); ok {
		c.metrics.ConnCurrent.WithLabelValues(c.node).Set(v)
	}
	if v, ok := toFloat64(conns["available"]); ok {
		c.metrics.ConnAvailable.WithLabelValues(c.node).Set(v)
	}
	if v, ok := toFloat64(conns["totalCreated"]); ok {
		setCounter(c.metrics.ConnCreated, v, c.node)
	}
}
