package collector

import (
	"context"
	"log/slog"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func (c *Collector) collectCollections(ctx context.Context) {
	dbs, err := c.client.ListDatabaseNames(ctx, bson.D{})
	if err != nil {
		slog.Warn("listDatabases failed", "node", c.node, "error", err)
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
			var result bson.M
			err := db.RunCommand(ctx, bson.D{{Key: "collStats", Value: collName}}).Decode(&result)
			if err != nil {
				continue
			}

			if v, ok := toFloat64(result["count"]); ok {
				c.metrics.CollDocCount.WithLabelValues(c.node, dbName, collName).Set(v)
			}
			if v, ok := toFloat64(result["size"]); ok {
				c.metrics.CollSizeBytes.WithLabelValues(c.node, dbName, collName).Set(v)
			}
			if v, ok := toFloat64(result["nindexes"]); ok {
				c.metrics.CollIndexCount.WithLabelValues(c.node, dbName, collName).Set(v)
			}
			if v, ok := toFloat64(result["totalIndexSize"]); ok {
				c.metrics.CollIndexSize.WithLabelValues(c.node, dbName, collName).Set(v)
			}
		}
	}
}

func isSystemDB(name string) bool {
	return name == "admin" || name == "local" || name == "config"
}
