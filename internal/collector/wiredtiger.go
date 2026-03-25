package collector

import "context"

func (c *Collector) collectWiredTiger(ctx context.Context, serverStatus map[string]interface{}) {
	wt, ok := serverStatus["wiredTiger"].(map[string]interface{})
	if !ok {
		return
	}

	cache, ok := wt["cache"].(map[string]interface{})
	if !ok {
		return
	}

	if v, ok := toFloat64(cache["bytes currently in the cache"]); ok {
		c.metrics.WTCacheBytesUsed.WithLabelValues(c.node).Set(v)
	}
	if v, ok := toFloat64(cache["maximum bytes configured"]); ok {
		c.metrics.WTCacheBytesMax.WithLabelValues(c.node).Set(v)
	}
	if v, ok := toFloat64(cache["tracked dirty bytes in the cache"]); ok {
		c.metrics.WTCacheDirtyBytes.WithLabelValues(c.node).Set(v)
	}
	if v, ok := toFloat64(cache["pages read into cache"]); ok {
		setCounter(c.metrics.WTCacheReadPages, v, c.node)
	}
	if v, ok := toFloat64(cache["pages written from cache"]); ok {
		setCounter(c.metrics.WTCacheWritPages, v, c.node)
	}
	if v, ok := toFloat64(cache["unmodified pages evicted"]); ok {
		setCounter(c.metrics.WTCacheEvictions, v, c.node)
	}
}
