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
	whoFormat   string
	whoLookback string
)

var whoCmd = &cobra.Command{
	Use:   "who <db.collection>",
	Short: "Show which clients query a collection",
	Args:  cobra.ExactArgs(1),
	RunE:  runWho,
}

func init() {
	whoCmd.Flags().StringVar(&whoFormat, "format", "text", "Output format: text or json")
	whoCmd.Flags().StringVar(&whoLookback, "lookback", "7d", "Time window (e.g. 24h, 7d)")
	rootCmd.AddCommand(whoCmd)
}

type clientInfo struct {
	Client   string    `json:"client"`
	AppName  string    `json:"app_name,omitempty"`
	Reads    int       `json:"reads"`
	Writes   int       `json:"writes"`
	LastSeen time.Time `json:"last_seen"`
}

func runWho(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	dbName, collName := splitNS(args[0])
	if collName == "" {
		return fmt.Errorf("usage: mongopulse who <db.collection>")
	}

	lookback, err := parseLookback(whoLookback)
	if err != nil {
		return fmt.Errorf("invalid lookback: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(cfg.DSN[0]).SetTimeout(10 * time.Second))
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer client.Disconnect(ctx)

	// Check profiling.
	var profileResult bson.M
	if err := client.Database(dbName).RunCommand(ctx, bson.D{{Key: "profile", Value: -1}}).Decode(&profileResult); err != nil {
		return fmt.Errorf("cannot check profiling: %w", err)
	}
	level, _ := profileResult["was"].(int32)
	if level == 0 {
		fmt.Fprintf(os.Stderr, "Warning: profiling is disabled on %s. Enable with db.setProfilingLevel(1)\n", dbName)
		exitCode = 3
		return nil
	}

	cutoff := time.Now().Add(-lookback)
	targetNS := dbName + "." + collName

	coll := client.Database(dbName).Collection("system.profile")
	filter := bson.D{
		{Key: "ns", Value: targetNS},
		{Key: "ts", Value: bson.D{{Key: "$gte", Value: cutoff}}},
	}
	cursor, err := coll.Find(ctx, filter, options.Find().SetLimit(5000))
	if err != nil {
		return fmt.Errorf("query system.profile: %w", err)
	}
	defer cursor.Close(ctx)

	clientMap := make(map[string]*clientInfo)

	for cursor.Next(ctx) {
		var doc bson.M
		if err := cursor.Decode(&doc); err != nil {
			continue
		}

		clientAddr, _ := doc["client"].(string)
		appName, _ := doc["appName"].(string)
		opType, _ := doc["op"].(string)
		ts, _ := doc["ts"].(time.Time)

		key := clientAddr
		if appName != "" {
			key = appName
		}
		if key == "" {
			key = "(unknown)"
		}

		ci, exists := clientMap[key]
		if !exists {
			ci = &clientInfo{Client: clientAddr, AppName: appName}
			clientMap[key] = ci
		}

		switch opType {
		case "query", "getmore", "command":
			ci.Reads++
		case "insert", "update", "remove":
			ci.Writes++
		default:
			ci.Reads++
		}

		if ts.After(ci.LastSeen) {
			ci.LastSeen = ts
		}
	}

	if len(clientMap) == 0 {
		fmt.Fprintf(os.Stderr, "No profiler data found for %s in last %s\n", targetNS, whoLookback)
		exitCode = 3
		return nil
	}

	var clients []clientInfo
	for _, ci := range clientMap {
		clients = append(clients, *ci)
	}
	sort.Slice(clients, func(i, j int) bool {
		return (clients[i].Reads + clients[i].Writes) > (clients[j].Reads + clients[j].Writes)
	})

	switch whoFormat {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(clients)
	default:
		fmt.Printf("%-25s %-20s %8s %8s  %s\n", "CLIENT", "APP_NAME", "READS", "WRITES", "LAST_SEEN")
		for _, c := range clients {
			fmt.Printf("%-25s %-20s %8d %8d  %s\n",
				truncate(c.Client, 25), truncate(c.AppName, 20),
				c.Reads, c.Writes, c.LastSeen.Format("2006-01-02 15:04"))
		}
	}
	return nil
}

func parseLookback(s string) (time.Duration, error) {
	// Support "7d" shorthand.
	if len(s) > 1 && s[len(s)-1] == 'd' {
		var days int
		if _, err := fmt.Sscanf(s, "%dd", &days); err == nil {
			return time.Duration(days) * 24 * time.Hour, nil
		}
	}
	return time.ParseDuration(s)
}
