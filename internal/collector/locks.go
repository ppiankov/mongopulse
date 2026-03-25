package collector

import "context"

func (c *Collector) collectLocks(ctx context.Context, serverStatus map[string]interface{}) {
	locks, ok := serverStatus["locks"].(map[string]interface{})
	if !ok {
		return
	}

	for lockType, raw := range locks {
		lock, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}

		if active, ok := lock["acquireCount"].(map[string]interface{}); ok {
			for mode, val := range active {
				if v, ok := toFloat64(val); ok {
					c.metrics.LocksActive.WithLabelValues(c.node, lockType, mode).Set(v)
				}
			}
		}

		if waiting, ok := lock["acquireWaitCount"].(map[string]interface{}); ok {
			for mode, val := range waiting {
				if v, ok := toFloat64(val); ok {
					c.metrics.LocksWaiting.WithLabelValues(c.node, lockType, mode).Set(v)
				}
			}
		}
	}
}
