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

var snapshotOutput string

var snapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Save cluster state for offline analysis",
	RunE:  runSnapshot,
}

func init() {
	snapshotCmd.Flags().StringVarP(&snapshotOutput, "output", "o", "", "Output file (default: stdout)")
	rootCmd.AddCommand(snapshotCmd)
}

type clusterSnapshot struct {
	Timestamp   string         `json:"timestamp"`
	Version     string         `json:"version,omitempty"`
	Uptime      float64        `json:"uptime_seconds,omitempty"`
	Databases   []dbSnapshot   `json:"databases"`
	Users       []userSnapshot `json:"users,omitempty"`
	Replication *replSnapshot  `json:"replication,omitempty"`
}

type dbSnapshot struct {
	Name        string         `json:"name"`
	SizeBytes   int64          `json:"size_bytes"`
	Collections []collSnapshot `json:"collections"`
}

type collSnapshot struct {
	Name      string        `json:"name"`
	Documents int64         `json:"documents"`
	SizeBytes int64         `json:"size_bytes"`
	Indexes   []idxSnapshot `json:"indexes"`
}

type idxSnapshot struct {
	Name string `json:"name"`
	Key  bson.M `json:"key"`
}

type userSnapshot struct {
	User  string   `json:"user"`
	DB    string   `json:"db"`
	Roles []string `json:"roles"`
}

type replSnapshot struct {
	Set     string   `json:"set"`
	Members []string `json:"members"`
}

func runSnapshot(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(cfg.DSN[0]).SetTimeout(10 * time.Second))
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer client.Disconnect(ctx)

	snap := clusterSnapshot{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	// Server info.
	var ss bson.M
	if err := client.Database("admin").RunCommand(ctx, bson.D{{Key: "serverStatus", Value: 1}}).Decode(&ss); err == nil {
		snap.Version, _ = ss["version"].(string)
		if v, ok := ss["uptime"].(float64); ok {
			snap.Uptime = v
		}
	}

	// Databases and collections.
	dbResult, err := client.ListDatabases(ctx, bson.D{})
	if err == nil {
		for _, dbMeta := range dbResult.Databases {
			if dbMeta.Name == "admin" || dbMeta.Name == "local" || dbMeta.Name == "config" {
				continue
			}
			dbs := dbSnapshot{Name: dbMeta.Name, SizeBytes: dbMeta.SizeOnDisk}
			db := client.Database(dbMeta.Name)
			colls, _ := db.ListCollectionNames(ctx, bson.D{})
			for _, collName := range colls {
				cs := collSnapshot{Name: collName}
				var stats bson.M
				if err := db.RunCommand(ctx, bson.D{{Key: "collStats", Value: collName}}).Decode(&stats); err == nil {
					if v, ok := stats["count"].(int32); ok {
						cs.Documents = int64(v)
					}
					if v, ok := stats["size"].(int32); ok {
						cs.SizeBytes = int64(v)
					}
				}
				// Indexes.
				idxCursor, err := db.Collection(collName).Indexes().List(ctx)
				if err == nil {
					for idxCursor.Next(ctx) {
						var idx bson.M
						idxCursor.Decode(&idx)
						name, _ := idx["name"].(string)
						key, _ := idx["key"].(bson.M)
						cs.Indexes = append(cs.Indexes, idxSnapshot{Name: name, Key: key})
					}
					idxCursor.Close(ctx)
				}
				dbs.Collections = append(dbs.Collections, cs)
			}
			snap.Databases = append(snap.Databases, dbs)
		}
	}

	// Users.
	userCursor, err := client.Database("admin").Collection("system.users").Find(ctx, bson.D{})
	if err == nil {
		for userCursor.Next(ctx) {
			var doc bson.M
			userCursor.Decode(&doc)
			u := userSnapshot{}
			u.User, _ = doc["user"].(string)
			u.DB, _ = doc["db"].(string)
			if roles, ok := doc["roles"].(bson.A); ok {
				for _, r := range roles {
					if rm, ok := r.(bson.M); ok {
						role, _ := rm["role"].(string)
						u.Roles = append(u.Roles, role)
					}
				}
			}
			snap.Users = append(snap.Users, u)
		}
		userCursor.Close(ctx)
	}

	// Replication.
	var rsStatus bson.M
	if err := client.Database("admin").RunCommand(ctx, bson.D{{Key: "replSetGetStatus", Value: 1}}).Decode(&rsStatus); err == nil {
		rs := &replSnapshot{}
		rs.Set, _ = rsStatus["set"].(string)
		if members, ok := rsStatus["members"].(bson.A); ok {
			for _, raw := range members {
				m, _ := raw.(bson.M)
				name, _ := m["name"].(string)
				rs.Members = append(rs.Members, name)
			}
		}
		snap.Replication = rs
	}

	// Output.
	var out *os.File
	if snapshotOutput != "" {
		out, err = os.Create(snapshotOutput)
		if err != nil {
			return err
		}
		defer out.Close()
	} else {
		out = os.Stdout
	}

	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(snap)
}
