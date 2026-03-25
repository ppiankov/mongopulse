package collector

import (
	"context"
	"log"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func (c *Collector) collectIndexUsage(ctx context.Context) {
	dbs, err := c.client.ListDatabaseNames(ctx, bson.D{})
	if err != nil {
		return
	}

	for _, dbName := range dbs {
		if isSystemDB(dbName) {
			continue
		}
		db := c.client.Database(dbName)
		colls, err := db.ListCollectionNames(ctx, bson.D{})
		if err != nil {
			continue
		}

		for _, collName := range colls {
			c.collectIndexStatsForColl(ctx, db, dbName, collName)
		}
	}
}

func (c *Collector) collectIndexStatsForColl(ctx context.Context, db *mongo.Database, dbName, collName string) {
	pipeline := mongo.Pipeline{
		{{Key: "$indexStats", Value: bson.D{}}},
	}

	cursor, err := db.Collection(collName).Aggregate(ctx, pipeline)
	if err != nil {
		log.Printf("[%s] indexStats %s.%s: %v", c.node, dbName, collName, err)
		return
	}
	defer cursor.Close(ctx)

	var totalIndexes, unusedIndexes float64

	for cursor.Next(ctx) {
		var doc bson.M
		if err := cursor.Decode(&doc); err != nil {
			continue
		}

		indexName, _ := doc["name"].(string)
		if indexName == "_id_" {
			continue // _id index is always required.
		}

		totalIndexes++

		var ops float64
		if accesses, ok := doc["accesses"].(bson.M); ok {
			if v, ok := toFloat64(accesses["ops"]); ok {
				ops = v
			}
		}

		c.metrics.IndexOpsTotal.WithLabelValues(c.node, dbName, collName, indexName).Add(ops)

		// Get index size from collStats.
		if ops == 0 {
			unusedIndexes++
			c.metrics.IndexUnused.WithLabelValues(c.node, dbName, collName, indexName).Set(1)
		} else {
			c.metrics.IndexUnused.WithLabelValues(c.node, dbName, collName, indexName).Set(0)
		}
	}

	c.metrics.IndexesTotal.WithLabelValues(c.node, dbName, collName).Set(totalIndexes)
	c.metrics.IndexesUnusedTotal.WithLabelValues(c.node, dbName, collName).Set(unusedIndexes)
}
