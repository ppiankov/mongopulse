package collector

import "context"

func (c *Collector) collectOpcounters(ctx context.Context, serverStatus map[string]interface{}) {
	ops, ok := serverStatus["opcounters"].(map[string]interface{})
	if !ok {
		return
	}

	for _, op := range []string{"insert", "query", "update", "delete", "getmore", "command"} {
		if v, ok := toFloat64(ops[op]); ok {
			setCounter(c.metrics.OpsTotal, v, c.node, op)
		}
	}
}
