package collector

import (
	"context"
	"log"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func (c *Collector) collectDbStats(ctx context.Context) {
	dbs, err := c.client.ListDatabaseNames(ctx, bson.D{})
	if err != nil {
		log.Printf("[%s] listDatabases: %v", c.node, err)
		return
	}

	for _, dbName := range dbs {
		if isSystemDB(dbName) {
			continue
		}

		var result bson.M
		err := c.client.Database(dbName).RunCommand(ctx, bson.D{{Key: "dbStats", Value: 1}}).Decode(&result)
		if err != nil {
			continue
		}

		if v, ok := toFloat64(result["dataSize"]); ok {
			c.metrics.DbDataSize.WithLabelValues(c.node, dbName).Set(v)
		}
		if v, ok := toFloat64(result["storageSize"]); ok {
			c.metrics.DbStorageSize.WithLabelValues(c.node, dbName).Set(v)
		}
		if v, ok := toFloat64(result["indexSize"]); ok {
			c.metrics.DbIndexSize.WithLabelValues(c.node, dbName).Set(v)
		}
		if v, ok := toFloat64(result["collections"]); ok {
			c.metrics.DbCollections.WithLabelValues(c.node, dbName).Set(v)
		}
		if v, ok := toFloat64(result["objects"]); ok {
			c.metrics.DbObjects.WithLabelValues(c.node, dbName).Set(v)
		}
	}
}
