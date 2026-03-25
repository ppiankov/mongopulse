package collector

import "context"

func (c *Collector) collectNetwork(ctx context.Context, serverStatus map[string]interface{}) {
	net, ok := serverStatus["network"].(map[string]interface{})
	if !ok {
		return
	}

	if v, ok := toFloat64(net["bytesIn"]); ok {
		setCounter(c.metrics.NetBytesIn, v, c.node)
	}
	if v, ok := toFloat64(net["bytesOut"]); ok {
		setCounter(c.metrics.NetBytesOut, v, c.node)
	}
	if v, ok := toFloat64(net["numRequests"]); ok {
		setCounter(c.metrics.NetRequests, v, c.node)
	}
}
