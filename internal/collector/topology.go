package collector

import (
	"context"
	"log"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type topoState struct {
	lastPrimary  string
	lastElection time.Time
	elections    []time.Time // ring buffer for storm detection
}

var (
	topoStates   = make(map[string]*topoState) // node -> state
	topoStatesMu sync.Mutex
)

func (c *Collector) collectTopology(ctx context.Context) {
	var result bson.M
	err := c.client.Database("admin").RunCommand(ctx, bson.D{{Key: "replSetGetStatus", Value: 1}}).Decode(&result)
	if err != nil {
		return
	}

	set, _ := result["set"].(string)
	members, _ := result["members"].(bson.A)

	topoStatesMu.Lock()
	defer topoStatesMu.Unlock()

	if topoStates[c.node] == nil {
		topoStates[c.node] = &topoState{}
	}
	ts := topoStates[c.node]

	var currentPrimary string

	for _, raw := range members {
		m, ok := raw.(bson.M)
		if !ok {
			continue
		}
		state, _ := toFloat64(m["state"])
		name, _ := m["name"].(string)

		// Map MongoDB state numbers to role gauge.
		var role float64
		switch int(state) {
		case 1: // PRIMARY
			role = 1
			currentPrimary = name
		case 2: // SECONDARY
			role = 2
		case 7: // ARBITER
			role = 3
		default:
			role = 0
		}
		c.metrics.TopoRole.WithLabelValues(c.node, set).Set(role)
	}

	// Detect primary change.
	now := time.Now()
	if ts.lastPrimary != "" && currentPrimary != ts.lastPrimary {
		c.metrics.TopoPrimaryChanges.WithLabelValues(c.node, set).Inc()
		ts.lastElection = now
		ts.elections = append(ts.elections, now)
		log.Printf("[%s] primary changed: %s -> %s", c.node, ts.lastPrimary, currentPrimary)
	}
	ts.lastPrimary = currentPrimary

	// Time since last election.
	if !ts.lastElection.IsZero() {
		c.metrics.TopoLastElectionSecs.WithLabelValues(c.node, set).Set(now.Sub(ts.lastElection).Seconds())
	}

	// Election storm detection: >3 elections in 10 minutes.
	cutoff := now.Add(-10 * time.Minute)
	var recent []time.Time
	for _, t := range ts.elections {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	ts.elections = recent

	if len(recent) > 3 {
		c.metrics.TopoElectionStorm.WithLabelValues(c.node, set).Set(1)
		log.Printf("[%s] election storm: %d elections in 10min", c.node, len(recent))
	} else {
		c.metrics.TopoElectionStorm.WithLabelValues(c.node, set).Set(0)
	}
}
