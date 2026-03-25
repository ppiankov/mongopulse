package collector

import "context"

func (c *Collector) collectCursors(ctx context.Context, serverStatus map[string]interface{}) {
	m, ok := serverStatus["metrics"].(map[string]interface{})
	if !ok {
		return
	}
	cur, ok := m["cursor"].(map[string]interface{})
	if !ok {
		return
	}

	if open, ok := cur["open"].(map[string]interface{}); ok {
		if v, ok := toFloat64(open["total"]); ok {
			c.metrics.CursorsOpen.WithLabelValues(c.node).Set(v)
		}
		if v, ok := toFloat64(open["pinned"]); ok {
			c.metrics.CursorsPinned.WithLabelValues(c.node).Set(v)
		}
		if v, ok := toFloat64(open["noTimeout"]); ok {
			c.metrics.CursorsNoTimeout.WithLabelValues(c.node).Set(v)
		}
	}
	if v, ok := toFloat64(cur["timedOut"]); ok {
		setCounter(c.metrics.CursorsTimedOut, v, c.node)
	}
}
