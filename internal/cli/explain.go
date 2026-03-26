package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/spf13/cobra"

	"github.com/ppiankov/mongopulse/internal/config"
)

var explainFormat string

var explainCmd = &cobra.Command{
	Use:   "explain <db.collection>",
	Short: "Structured collection intelligence",
	Args:  cobra.ExactArgs(1),
	RunE:  runExplain,
}

func init() {
	explainCmd.Flags().StringVar(&explainFormat, "format", "text", "Output format: text or json")
	rootCmd.AddCommand(explainCmd)
}

type collExplain struct {
	Database    string         `json:"database"`
	Collection  string         `json:"collection"`
	Documents   int64          `json:"documents"`
	SizeBytes   int64          `json:"size_bytes"`
	AvgObjSize  int64          `json:"avg_obj_size"`
	StorageSize int64          `json:"storage_size"`
	Capped      bool           `json:"capped"`
	Indexes     []indexExplain `json:"indexes"`
	UnusedCount int            `json:"unused_index_count"`
	TotalIdx    int            `json:"total_index_count"`
}

type indexExplain struct {
	Name   string `json:"name"`
	Ops    int64  `json:"ops"`
	Unused bool   `json:"unused"`
}

func runExplain(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	dbName, collName := splitNS(args[0])
	if collName == "" {
		return fmt.Errorf("usage: mongopulse explain <db.collection>")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(cfg.DSN[0]).SetTimeout(10 * time.Second))
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer client.Disconnect(ctx)

	db := client.Database(dbName)

	// collStats.
	var stats bson.M
	if err := db.RunCommand(ctx, bson.D{{Key: "collStats", Value: collName}}).Decode(&stats); err != nil {
		return fmt.Errorf("collStats: %w", err)
	}

	exp := collExplain{
		Database:   dbName,
		Collection: collName,
	}

	if v, ok := stats["count"].(int32); ok {
		exp.Documents = int64(v)
	} else if v, ok := stats["count"].(int64); ok {
		exp.Documents = v
	}
	if v, ok := stats["size"].(int32); ok {
		exp.SizeBytes = int64(v)
	} else if v, ok := stats["size"].(int64); ok {
		exp.SizeBytes = v
	}
	if v, ok := stats["avgObjSize"].(int32); ok {
		exp.AvgObjSize = int64(v)
	} else if v, ok := stats["avgObjSize"].(int64); ok {
		exp.AvgObjSize = v
	}
	if v, ok := stats["storageSize"].(int32); ok {
		exp.StorageSize = int64(v)
	} else if v, ok := stats["storageSize"].(int64); ok {
		exp.StorageSize = v
	}
	exp.Capped, _ = stats["capped"].(bool)

	// $indexStats.
	pipeline := mongo.Pipeline{{{Key: "$indexStats", Value: bson.D{}}}}
	cursor, err := db.Collection(collName).Aggregate(ctx, pipeline)
	if err == nil {
		for cursor.Next(ctx) {
			var doc bson.M
			if err := cursor.Decode(&doc); err != nil {
				continue
			}
			name, _ := doc["name"].(string)
			if name == "_id_" {
				continue
			}
			var ops int64
			if accesses, ok := doc["accesses"].(bson.M); ok {
				if v, ok := accesses["ops"].(int64); ok {
					ops = v
				} else if v, ok := accesses["ops"].(int32); ok {
					ops = int64(v)
				}
			}
			unused := ops == 0
			if unused {
				exp.UnusedCount++
			}
			exp.TotalIdx++
			exp.Indexes = append(exp.Indexes, indexExplain{Name: name, Ops: ops, Unused: unused})
		}
		cursor.Close(ctx)
	}

	switch explainFormat {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(exp)
	default:
		fmt.Printf("Collection: %s.%s\n", exp.Database, exp.Collection)
		fmt.Printf("  Documents:    %d\n", exp.Documents)
		fmt.Printf("  Data size:    %s\n", humanBytes(exp.SizeBytes))
		fmt.Printf("  Storage size: %s\n", humanBytes(exp.StorageSize))
		fmt.Printf("  Avg obj size: %d bytes\n", exp.AvgObjSize)
		fmt.Printf("  Capped:       %v\n", exp.Capped)
		fmt.Printf("\nIndexes (%d total, %d unused):\n", exp.TotalIdx, exp.UnusedCount)
		for _, idx := range exp.Indexes {
			flag := ""
			if idx.Unused {
				flag = " [UNUSED]"
			}
			fmt.Printf("  %-40s %8d ops%s\n", idx.Name, idx.Ops, flag)
		}
	}
	return nil
}
