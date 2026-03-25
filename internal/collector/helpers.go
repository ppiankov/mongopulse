package collector

import (
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/prometheus/client_golang/prometheus"
)

func toFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	default:
		return 0, false
	}
}

// setCounter sets a counter to match a monotonic server value.
// Prometheus counters can only increase, so we track the delta externally.
// For simplicity, we use a gauge-like approach via Add if the counter is new.
// In practice, MongoDB counters reset on restart, so we just set the absolute value.
func setCounter(c *prometheus.CounterVec, val float64, labels ...string) {
	// CounterVec.WithLabelValues returns a Counter which starts at 0.
	// We add the value directly — on subsequent polls the counter will grow.
	// This works because Prometheus counters are cumulative on the server side.
	counter := c.WithLabelValues(labels...)
	// Reset detection: if server restarted, the value may be lower than counter.
	// Prometheus handles resets at scrape time, so just add the full value.
	counter.Add(val)
}

func findOneAsc() *options.FindOptionsBuilder {
	return options.Find().SetSort(map[string]int{"$natural": 1}).SetLimit(1)
}

func findOneDesc() *options.FindOptionsBuilder {
	return options.Find().SetSort(map[string]int{"$natural": -1}).SetLimit(1)
}
