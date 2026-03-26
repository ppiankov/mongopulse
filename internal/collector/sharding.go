package collector

import (
	"context"
	"log/slog"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Sharding metrics (WO-15).
func (c *Collector) collectSharding(ctx context.Context) {
	// Only works when connected to mongos.
	var result bson.M
	if err := c.client.Database("admin").RunCommand(ctx, bson.D{{Key: "serverStatus", Value: 1}}).Decode(&result); err != nil {
		return
	}
	// Check if process is mongos.
	process, _ := result["process"].(string)
	if process != "mongos" {
		return
	}

	// Balancer status.
	var balancer bson.M
	if err := c.client.Database("config").RunCommand(ctx, bson.D{{Key: "balancerStatus", Value: 1}}).Decode(&balancer); err == nil {
		running := 0.0
		if mode, ok := balancer["mode"].(string); ok && mode == "full" {
			running = 1.0
		}
		c.metrics.ShardingBalancerRunning.WithLabelValues(c.node).Set(running)
	}

	// Chunks per shard.
	pipeline := bson.A{
		bson.D{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: bson.D{
				{Key: "shard", Value: "$shard"},
				{Key: "ns", Value: "$ns"},
			}},
			{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
		}}},
	}
	cursor, err := c.client.Database("config").Collection("chunks").Aggregate(ctx, pipeline)
	if err != nil {
		slog.Warn("sharding chunks aggregation failed", "node", c.node, "error", err)
		return
	}
	defer cursor.Close(ctx)

	var jumboCount float64
	for cursor.Next(ctx) {
		var doc bson.M
		if err := cursor.Decode(&doc); err != nil {
			continue
		}
		id, _ := doc["_id"].(bson.M)
		shard, _ := id["shard"].(string)
		ns, _ := id["ns"].(string)
		count, _ := toFloat64(doc["count"])
		c.metrics.ShardingChunks.WithLabelValues(c.node, shard, ns).Set(count)
	}

	// Jumbo chunks count.
	jumboFilter := bson.D{{Key: "jumbo", Value: true}}
	jumboN, err := c.client.Database("config").Collection("chunks").CountDocuments(ctx, jumboFilter)
	if err == nil {
		jumboCount = float64(jumboN)
	}
	c.metrics.ShardingJumboChunks.WithLabelValues(c.node).Set(jumboCount)

	// Sharded collections count.
	collCount, err := c.client.Database("config").Collection("collections").CountDocuments(ctx, bson.D{})
	if err == nil {
		c.metrics.ShardingCollections.WithLabelValues(c.node).Set(float64(collCount))
	}
}

// Balancer activity (WO-31).
func (c *Collector) collectBalancerActivity(ctx context.Context) {
	// Only works on mongos.
	var result bson.M
	if err := c.client.Database("admin").RunCommand(ctx, bson.D{{Key: "serverStatus", Value: 1}}).Decode(&result); err != nil {
		return
	}
	process, _ := result["process"].(string)
	if process != "mongos" {
		return
	}

	// Recent migrations from config.changelog.
	changelog := c.client.Database("config").Collection("changelog")

	// Count moveChunk events.
	moveFilter := bson.D{{Key: "what", Value: "moveChunk.from"}}
	moveCount, err := changelog.CountDocuments(ctx, moveFilter)
	if err == nil {
		setCounter(c.metrics.BalancerMigrations, float64(moveCount), c.node)
	}

	// Count split events.
	splitFilter := bson.D{{Key: "what", Value: "split"}}
	splitCount, err := changelog.CountDocuments(ctx, splitFilter)
	if err == nil {
		setCounter(c.metrics.BalancerSplits, float64(splitCount), c.node)
	}

	// Count failed migrations.
	failFilter := bson.D{{Key: "what", Value: "moveChunk.error"}}
	failCount, err := changelog.CountDocuments(ctx, failFilter)
	if err == nil {
		setCounter(c.metrics.BalancerMigrationFailures, float64(failCount), c.node)
	}

	// Shard key skew: max/min chunk ratio per namespace.
	pipeline := bson.A{
		bson.D{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: bson.D{
				{Key: "ns", Value: "$ns"},
				{Key: "shard", Value: "$shard"},
			}},
			{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
		}}},
		bson.D{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$_id.ns"},
			{Key: "max", Value: bson.D{{Key: "$max", Value: "$count"}}},
			{Key: "min", Value: bson.D{{Key: "$min", Value: "$count"}}},
		}}},
	}
	cursor, err := c.client.Database("config").Collection("chunks").Aggregate(ctx, pipeline)
	if err != nil {
		return
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var doc bson.M
		if err := cursor.Decode(&doc); err != nil {
			continue
		}
		ns, _ := doc["_id"].(string)
		maxChunks, _ := toFloat64(doc["max"])
		minChunks, _ := toFloat64(doc["min"])
		if minChunks > 0 {
			c.metrics.ShardKeySkew.WithLabelValues(c.node, ns).Set(maxChunks / minChunks)
		}
	}
}
