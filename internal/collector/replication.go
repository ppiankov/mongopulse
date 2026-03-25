package collector

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func (c *Collector) collectReplication(ctx context.Context) error {
	var result bson.M
	err := c.client.Database("admin").RunCommand(ctx, bson.D{{Key: "replSetGetStatus", Value: 1}}).Decode(&result)
	if err != nil {
		return err
	}

	set, _ := result["set"].(string)
	members, _ := result["members"].(bson.A)

	c.metrics.ReplMembersTotal.WithLabelValues(c.node, set).Set(float64(len(members)))

	var primaryOptime time.Time
	for _, raw := range members {
		m, ok := raw.(bson.M)
		if !ok {
			continue
		}
		state, _ := toFloat64(m["state"])
		if state == 1 { // PRIMARY
			primaryOptime = extractOptime(m)
		}
	}

	for _, raw := range members {
		m, ok := raw.(bson.M)
		if !ok {
			continue
		}
		name, _ := m["name"].(string)
		state, _ := toFloat64(m["state"])

		c.metrics.ReplMemberState.WithLabelValues(c.node, name, set).Set(state)

		if state == 2 && !primaryOptime.IsZero() { // SECONDARY
			memberOptime := extractOptime(m)
			if !memberOptime.IsZero() {
				lag := primaryOptime.Sub(memberOptime).Seconds()
				if lag < 0 {
					lag = 0
				}
				c.metrics.ReplLagSeconds.WithLabelValues(c.node, name).Set(lag)
			}
		}
	}

	// Oplog window.
	c.collectOplogWindow(ctx)

	return nil
}

func (c *Collector) collectOplogWindow(ctx context.Context) {
	oplog := c.client.Database("local").Collection("oplog.rs")

	// Get first entry.
	var first bson.M
	cursor, err := oplog.Find(ctx, bson.D{}, findOneAsc())
	if err != nil {
		return
	}
	defer cursor.Close(ctx)
	if !cursor.Next(ctx) {
		return
	}
	if err := cursor.Decode(&first); err != nil {
		return
	}

	// Get last entry.
	var last bson.M
	cursor2, err := oplog.Find(ctx, bson.D{}, findOneDesc())
	if err != nil {
		return
	}
	defer cursor2.Close(ctx)
	if !cursor2.Next(ctx) {
		return
	}
	if err := cursor2.Decode(&last); err != nil {
		return
	}

	firstTS := extractTimestamp(first)
	lastTS := extractTimestamp(last)
	if firstTS > 0 && lastTS > 0 {
		windowHours := float64(lastTS-firstTS) / 3600.0
		c.metrics.ReplOplogWindowH.WithLabelValues(c.node).Set(windowHours)
	}
}

func extractOptime(m bson.M) time.Time {
	if optimeDate, ok := m["optimeDate"].(time.Time); ok {
		return optimeDate
	}
	return time.Time{}
}

func extractTimestamp(doc bson.M) uint32 {
	if ts, ok := doc["ts"].(bson.Timestamp); ok {
		return ts.T
	}
	return 0
}
