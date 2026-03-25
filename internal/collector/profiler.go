package collector

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sort"
	"sync"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type queryStats struct {
	count    int
	totalMs  float64
	samples  []float64
	baseline float64
}

var (
	profilerState   = make(map[string]map[string]*queryStats) // node -> fingerprint -> stats
	profilerStateMu sync.Mutex
)

func (c *Collector) collectProfiler(ctx context.Context) {
	dbs, err := c.client.ListDatabaseNames(ctx, bson.D{})
	if err != nil {
		return
	}

	profilerStateMu.Lock()
	if profilerState[c.node] == nil {
		profilerState[c.node] = make(map[string]*queryStats)
	}
	nodeState := profilerState[c.node]
	profilerStateMu.Unlock()

	for _, dbName := range dbs {
		if isSystemDB(dbName) {
			continue
		}

		// Check if profiling is enabled.
		var profileResult bson.M
		err := c.client.Database(dbName).RunCommand(ctx, bson.D{{Key: "profile", Value: -1}}).Decode(&profileResult)
		if err != nil {
			continue
		}
		level, _ := profileResult["was"].(int32)
		if level == 0 {
			continue
		}

		// Read system.profile entries.
		coll := c.client.Database(dbName).Collection("system.profile")
		cursor, err := coll.Find(ctx, bson.D{}, findOneDesc())
		if err != nil {
			continue
		}

		var entries int
		for cursor.Next(ctx) {
			var doc bson.M
			if err := cursor.Decode(&doc); err != nil {
				continue
			}
			entries++

			opType, _ := doc["op"].(string)
			ns, _ := doc["ns"].(string)
			millis, ok := toFloat64(doc["millis"])
			if !ok {
				continue
			}

			// Extract collection from ns.
			collName := extractCollFromNS(ns)

			c.metrics.SlowQueriesTotal.WithLabelValues(c.node, dbName, collName, opType).Inc()

			// Fingerprint the query shape.
			fp := fingerprint(doc)
			key := dbName + ":" + fp

			profilerStateMu.Lock()
			qs, exists := nodeState[key]
			if !exists {
				qs = &queryStats{}
				nodeState[key] = qs
			}
			qs.count++
			qs.totalMs += millis
			qs.samples = append(qs.samples, millis)

			// Keep only last 100 samples.
			if len(qs.samples) > 100 {
				qs.samples = qs.samples[len(qs.samples)-100:]
			}

			mean := qs.totalMs / float64(qs.count)
			p95 := percentile(qs.samples, 95)

			c.metrics.QueryMeanMs.WithLabelValues(c.node, dbName, fp).Set(mean)
			c.metrics.QueryP95Ms.WithLabelValues(c.node, dbName, fp).Set(p95)

			// Regression detection: after baseline is set, check if mean exceeds threshold.
			if qs.count > 10 && qs.baseline == 0 {
				qs.baseline = mean
			}
			if qs.baseline > 0 && mean > qs.baseline*c.cfg.RegressionThreshold {
				c.metrics.QueryRegressionTotal.WithLabelValues(c.node, dbName, fp).Inc()
				log.Printf("[%s] query regression: %s mean=%.1fms baseline=%.1fms", c.node, fp[:12], mean, qs.baseline)
			}
			profilerStateMu.Unlock()

			if entries >= c.cfg.StmtLimit {
				break
			}
		}
		cursor.Close(ctx)

		c.metrics.ProfilerEntries.WithLabelValues(c.node, dbName).Add(float64(entries))
	}
}

func fingerprint(doc bson.M) string {
	// Normalize query by extracting command shape (keys only, no values).
	cmd, ok := doc["command"].(bson.M)
	if !ok {
		cmd = doc
	}

	shape := extractKeys(cmd)
	b, _ := json.Marshal(shape)
	h := sha256.Sum256(b)
	return fmt.Sprintf("%x", h[:8])
}

func extractKeys(m bson.M) map[string]interface{} {
	out := make(map[string]interface{})
	for k, v := range m {
		if sub, ok := v.(bson.M); ok {
			out[k] = extractKeys(sub)
		} else {
			out[k] = 1
		}
	}
	return out
}

func extractCollFromNS(ns string) string {
	for i := 0; i < len(ns); i++ {
		if ns[i] == '.' {
			return ns[i+1:]
		}
	}
	return ns
}

func percentile(samples []float64, pct float64) float64 {
	if len(samples) == 0 {
		return 0
	}
	sorted := make([]float64, len(samples))
	copy(sorted, samples)
	sort.Float64s(sorted)

	idx := (pct / 100.0) * float64(len(sorted)-1)
	lower := int(math.Floor(idx))
	upper := int(math.Ceil(idx))
	if lower == upper || upper >= len(sorted) {
		return sorted[lower]
	}
	frac := idx - float64(lower)
	return sorted[lower]*(1-frac) + sorted[upper]*frac
}
