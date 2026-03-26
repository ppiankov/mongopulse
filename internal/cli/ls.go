package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/spf13/cobra"

	"github.com/ppiankov/mongopulse/internal/config"
)

var (
	lsFormat        string
	lsSort          string
	lsIncludeSystem bool
)

var lsCmd = &cobra.Command{
	Use:   "ls [database]",
	Short: "List databases or collections",
	RunE:  runLs,
}

func init() {
	lsCmd.Flags().StringVar(&lsFormat, "format", "text", "Output format: text or json")
	lsCmd.Flags().StringVar(&lsSort, "sort", "name", "Sort by: name, size, or count")
	lsCmd.Flags().BoolVar(&lsIncludeSystem, "include-system", false, "Include system databases")
	rootCmd.AddCommand(lsCmd)
}

type dbInfo struct {
	Name        string `json:"name"`
	SizeBytes   int64  `json:"size_bytes"`
	Collections int    `json:"collections"`
}

type collInfo struct {
	Name      string `json:"name"`
	Documents int64  `json:"documents"`
	SizeBytes int64  `json:"size_bytes"`
	Indexes   int    `json:"indexes"`
	Capped    bool   `json:"capped"`
}

func runLs(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(cfg.DSN[0]).SetTimeout(10 * time.Second))
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer client.Disconnect(ctx)

	if len(args) > 0 {
		return listCollections(ctx, client, args[0])
	}
	return listDatabases(ctx, client)
}

func listDatabases(ctx context.Context, client *mongo.Client) error {
	result, err := client.ListDatabases(ctx, bson.D{})
	if err != nil {
		return err
	}

	var dbs []dbInfo
	for _, db := range result.Databases {
		if !lsIncludeSystem && isSystemDB(db.Name) {
			continue
		}
		colls, _ := client.Database(db.Name).ListCollectionNames(ctx, bson.D{})
		dbs = append(dbs, dbInfo{
			Name:        db.Name,
			SizeBytes:   db.SizeOnDisk,
			Collections: len(colls),
		})
	}

	sortDBs(dbs)

	switch lsFormat {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(dbs)
	default:
		fmt.Printf("%-30s %12s %8s\n", "DATABASE", "SIZE", "COLLS")
		for _, d := range dbs {
			fmt.Printf("%-30s %12s %8d\n", d.Name, humanBytes(d.SizeBytes), d.Collections)
		}
	}
	return nil
}

func listCollections(ctx context.Context, client *mongo.Client, dbName string) error {
	db := client.Database(dbName)
	colls, err := db.ListCollectionNames(ctx, bson.D{})
	if err != nil {
		return err
	}

	var infos []collInfo
	for _, name := range colls {
		var result bson.M
		err := db.RunCommand(ctx, bson.D{{Key: "collStats", Value: name}}).Decode(&result)
		if err != nil {
			continue
		}
		ci := collInfo{Name: name}
		if v, ok := result["count"].(int32); ok {
			ci.Documents = int64(v)
		} else if v, ok := result["count"].(int64); ok {
			ci.Documents = v
		}
		if v, ok := result["size"].(int32); ok {
			ci.SizeBytes = int64(v)
		} else if v, ok := result["size"].(int64); ok {
			ci.SizeBytes = v
		}
		if v, ok := result["nindexes"].(int32); ok {
			ci.Indexes = int(v)
		}
		ci.Capped, _ = result["capped"].(bool)
		infos = append(infos, ci)
	}

	sortColls(infos)

	switch lsFormat {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(infos)
	default:
		fmt.Printf("%-40s %12s %10s %6s %6s\n", "COLLECTION", "DOCS", "SIZE", "IDX", "CAPPED")
		for _, c := range infos {
			capped := ""
			if c.Capped {
				capped = "yes"
			}
			fmt.Printf("%-40s %12d %10s %6d %6s\n", c.Name, c.Documents, humanBytes(c.SizeBytes), c.Indexes, capped)
		}
	}
	return nil
}

func isSystemDB(name string) bool {
	return name == "admin" || name == "local" || name == "config"
}

func sortDBs(dbs []dbInfo) {
	sort.Slice(dbs, func(i, j int) bool {
		switch lsSort {
		case "size":
			return dbs[i].SizeBytes > dbs[j].SizeBytes
		case "count":
			return dbs[i].Collections > dbs[j].Collections
		default:
			return dbs[i].Name < dbs[j].Name
		}
	})
}

func sortColls(colls []collInfo) {
	sort.Slice(colls, func(i, j int) bool {
		switch lsSort {
		case "size":
			return colls[i].SizeBytes > colls[j].SizeBytes
		case "count":
			return colls[i].Documents > colls[j].Documents
		default:
			return colls[i].Name < colls[j].Name
		}
	})
}

func humanBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}
