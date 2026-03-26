package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/spf13/cobra"

	"github.com/ppiankov/mongopulse/internal/config"
)

var (
	grepFormat string
	grepScope  string
)

var grepCmd = &cobra.Command{
	Use:   "grep <pattern>",
	Short: "Search across databases for patterns",
	Args:  cobra.ExactArgs(1),
	RunE:  runGrep,
}

func init() {
	grepCmd.Flags().StringVar(&grepFormat, "format", "text", "Output format: text or json")
	grepCmd.Flags().StringVar(&grepScope, "scope", "all", "Search scope: collections, indexes, or all")
	rootCmd.AddCommand(grepCmd)
}

type grepMatch struct {
	Database   string `json:"database"`
	Collection string `json:"collection,omitempty"`
	Index      string `json:"index,omitempty"`
	MatchType  string `json:"match_type"`
}

func runGrep(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	pattern, err := regexp.Compile(args[0])
	if err != nil {
		return fmt.Errorf("invalid pattern: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(cfg.DSN[0]).SetTimeout(10 * time.Second))
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer client.Disconnect(ctx)

	dbs, err := client.ListDatabaseNames(ctx, bson.D{})
	if err != nil {
		return err
	}

	var matches []grepMatch

	for _, dbName := range dbs {
		if dbName == "admin" || dbName == "local" || dbName == "config" {
			continue
		}

		db := client.Database(dbName)
		colls, err := db.ListCollectionNames(ctx, bson.D{})
		if err != nil {
			continue
		}

		for _, collName := range colls {
			if grepScope == "all" || grepScope == "collections" {
				if pattern.MatchString(collName) {
					matches = append(matches, grepMatch{
						Database:   dbName,
						Collection: collName,
						MatchType:  "collection",
					})
				}
			}

			if grepScope == "all" || grepScope == "indexes" {
				idxView := db.Collection(collName).Indexes()
				cursor, err := idxView.List(ctx)
				if err != nil {
					continue
				}
				for cursor.Next(ctx) {
					var idx bson.M
					cursor.Decode(&idx)
					name, _ := idx["name"].(string)
					if pattern.MatchString(name) {
						matches = append(matches, grepMatch{
							Database:   dbName,
							Collection: collName,
							Index:      name,
							MatchType:  "index",
						})
					}
				}
				cursor.Close(ctx)
			}
		}
	}

	if len(matches) == 0 {
		exitCode = 3
		fmt.Fprintln(os.Stderr, "No matches found")
		return nil
	}

	switch grepFormat {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(matches)
	default:
		for _, m := range matches {
			switch m.MatchType {
			case "collection":
				fmt.Printf("[collection] %s.%s\n", m.Database, m.Collection)
			case "index":
				fmt.Printf("[index]      %s.%s -> %s\n", m.Database, m.Collection, m.Index)
			}
		}
	}
	return nil
}
