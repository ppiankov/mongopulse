package collector

import (
	"context"
	"sync"
	"time"
)

type connSample struct {
	ts      time.Time
	current float64
}

type connPredictState struct {
	samples []connSample
	max     float64
}

var (
	connPredictStates   = make(map[string]*connPredictState)
	connPredictStatesMu sync.Mutex
)

func (c *Collector) collectConnPrediction(ctx context.Context, serverStatus map[string]interface{}) {
	conns, ok := serverStatus["connections"].(map[string]interface{})
	if !ok {
		return
	}

	current, ok := toFloat64(conns["current"])
	if !ok {
		return
	}
	available, ok := toFloat64(conns["available"])
	if !ok {
		return
	}
	maxConns := current + available

	connPredictStatesMu.Lock()
	defer connPredictStatesMu.Unlock()

	if connPredictStates[c.node] == nil {
		connPredictStates[c.node] = &connPredictState{max: maxConns}
	}
	ps := connPredictStates[c.node]
	ps.max = maxConns

	now := time.Now()
	ps.samples = append(ps.samples, connSample{ts: now, current: current})

	// Keep only last 60 samples (5min at 5s interval).
	if len(ps.samples) > 60 {
		ps.samples = ps.samples[len(ps.samples)-60:]
	}

	// Utilization ratio.
	if maxConns > 0 {
		c.metrics.ConnUtilization.WithLabelValues(c.node).Set(current / maxConns)
	}

	// Need at least 2 samples for trend.
	if len(ps.samples) < 2 {
		return
	}

	// Compute linear trend (connections per hour).
	first := ps.samples[0]
	last := ps.samples[len(ps.samples)-1]
	elapsed := last.ts.Sub(first.ts).Hours()
	if elapsed <= 0 {
		return
	}

	trend := (last.current - first.current) / elapsed
	c.metrics.ConnTrendPerHour.WithLabelValues(c.node).Set(trend)

	// Estimate hours to exhaustion.
	if trend <= 0 {
		c.metrics.ConnExhaustionHours.WithLabelValues(c.node).Set(-1)
		return
	}

	remaining := maxConns - current
	hours := remaining / trend
	c.metrics.ConnExhaustionHours.WithLabelValues(c.node).Set(hours)
}
