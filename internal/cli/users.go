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

var (
	usersFormat   string
	usersUnused   bool
	usersLookback string
)

var usersCmd = &cobra.Command{
	Use:   "users [username]",
	Short: "MongoDB user and role audit",
	RunE:  runUsers,
}

func init() {
	usersCmd.Flags().StringVar(&usersFormat, "format", "text", "Output format: text or json")
	usersCmd.Flags().BoolVar(&usersUnused, "unused", false, "Show only users with no profiler activity")
	usersCmd.Flags().StringVar(&usersLookback, "lookback", "30d", "Activity lookback window")
	rootCmd.AddCommand(usersCmd)
}

type userInfo struct {
	User     string   `json:"user"`
	DB       string   `json:"db"`
	Roles    []string `json:"roles"`
	AuthMech string   `json:"auth_mechanism,omitempty"`
	Active   bool     `json:"active"`
	Queries  int      `json:"queries,omitempty"`
}

func runUsers(cmd *cobra.Command, args []string) error {
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

	// Get users from admin.system.users.
	var filter bson.D
	if len(args) > 0 {
		filter = bson.D{{Key: "user", Value: args[0]}}
	}

	cursor, err := client.Database("admin").Collection("system.users").Find(ctx, filter)
	if err != nil {
		return fmt.Errorf("list users: %w", err)
	}
	defer cursor.Close(ctx)

	// Build activity map from profiler if --unused.
	activityMap := make(map[string]int)
	if usersUnused {
		lookback, _ := parseLookback(usersLookback)
		activityMap = collectUserActivity(ctx, client, lookback)
	}

	var users []userInfo
	for cursor.Next(ctx) {
		var doc bson.M
		if err := cursor.Decode(&doc); err != nil {
			continue
		}

		u := userInfo{}
		u.User, _ = doc["user"].(string)
		u.DB, _ = doc["db"].(string)

		if roles, ok := doc["roles"].(bson.A); ok {
			for _, r := range roles {
				if rm, ok := r.(bson.M); ok {
					role, _ := rm["role"].(string)
					rdb, _ := rm["db"].(string)
					u.Roles = append(u.Roles, role+"@"+rdb)
				}
			}
		}

		if usersUnused {
			u.Queries = activityMap[u.User]
			u.Active = u.Queries > 0
			if usersUnused && u.Active {
				continue
			}
		}

		users = append(users, u)
	}

	if len(users) == 0 {
		exitCode = 3
		fmt.Fprintln(os.Stderr, "No users found")
		return nil
	}

	switch usersFormat {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(users)
	default:
		fmt.Printf("%-20s %-10s %-40s %8s\n", "USER", "DB", "ROLES", "QUERIES")
		for _, u := range users {
			roles := ""
			for i, r := range u.Roles {
				if i > 0 {
					roles += ", "
				}
				roles += r
			}
			fmt.Printf("%-20s %-10s %-40s %8d\n", u.User, u.DB, truncate(roles, 40), u.Queries)
		}
	}
	return nil
}

func collectUserActivity(ctx context.Context, client *mongo.Client, lookback time.Duration) map[string]int {
	activity := make(map[string]int)
	cutoff := time.Now().Add(-lookback)

	dbs, err := client.ListDatabaseNames(ctx, bson.D{})
	if err != nil {
		return activity
	}

	for _, dbName := range dbs {
		if dbName == "admin" || dbName == "local" || dbName == "config" {
			continue
		}

		coll := client.Database(dbName).Collection("system.profile")
		cursor, err := coll.Find(ctx, bson.D{{Key: "ts", Value: bson.D{{Key: "$gte", Value: cutoff}}}},
			options.Find().SetLimit(5000))
		if err != nil {
			continue
		}
		for cursor.Next(ctx) {
			var doc bson.M
			cursor.Decode(&doc)
			user, _ := doc["user"].(string)
			if user != "" {
				activity[user]++
			}
		}
		cursor.Close(ctx)
	}
	return activity
}
