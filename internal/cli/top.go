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
	topFormat     string
	topWatch      int
	topMinElapsed float64
	topUser       string
	topDB         string
)

var topCmd = &cobra.Command{
	Use:   "top",
	Short: "Show live running operations",
	RunE:  runTop,
}

func init() {
	topCmd.Flags().StringVar(&topFormat, "format", "text", "Output format: text or json")
	topCmd.Flags().IntVar(&topWatch, "watch", 0, "Refresh interval in seconds (0 = one-shot)")
	topCmd.Flags().Float64Var(&topMinElapsed, "min-elapsed", 0, "Only show ops running longer than N seconds")
	topCmd.Flags().StringVar(&topUser, "user", "", "Filter by user")
	topCmd.Flags().StringVar(&topDB, "db", "", "Filter by database")
	rootCmd.AddCommand(topCmd)
}

type opInfo struct {
	OpID        interface{} `json:"opid"`
	User        string      `json:"user,omitempty"`
	Client      string      `json:"client,omitempty"`
	DB          string      `json:"db,omitempty"`
	Collection  string      `json:"collection,omitempty"`
	OpType      string      `json:"op_type,omitempty"`
	SecsRunning float64     `json:"secs_running"`
	Plan        string      `json:"plan_summary,omitempty"`
	Node        string      `json:"node,omitempty"`
}

func runTop(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	ctx := context.Background()

	var clients []*mongo.Client
	for _, dsn := range cfg.DSN {
		c, err := mongo.Connect(options.Client().ApplyURI(dsn).SetTimeout(10 * time.Second))
		if err != nil {
			return fmt.Errorf("connect: %w", err)
		}
		clients = append(clients, c)
		defer c.Disconnect(ctx)
	}

	for {
		ops := collectOps(ctx, clients, cfg.DSN)

		switch topFormat {
		case "json":
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			enc.Encode(ops)
		default:
			if topWatch > 0 {
				fmt.Print("\033[2J\033[H") // clear screen
			}
			fmt.Printf("%-12s %-12s %-18s %-15s %-8s %8s  %s\n", "OPID", "USER", "CLIENT", "DB.COLL", "TYPE", "SECS", "PLAN")
			for _, o := range ops {
				ns := o.DB
				if o.Collection != "" {
					ns += "." + o.Collection
				}
				fmt.Printf("%-12v %-12s %-18s %-15s %-8s %8.1f  %s\n",
					o.OpID, truncate(o.User, 12), truncate(o.Client, 18),
					truncate(ns, 15), truncate(o.OpType, 8), o.SecsRunning, truncate(o.Plan, 40))
			}
			if len(ops) == 0 {
				fmt.Println("  (no active operations)")
			}
		}

		if topWatch <= 0 {
			break
		}
		time.Sleep(time.Duration(topWatch) * time.Second)
	}

	return nil
}

func collectOps(ctx context.Context, clients []*mongo.Client, dsns []string) []opInfo {
	var all []opInfo
	for i, client := range clients {
		node := ""
		if len(dsns) > 1 {
			node = fmt.Sprintf("node-%d", i)
		}

		var result bson.M
		err := client.Database("admin").RunCommand(ctx, bson.D{
			{Key: "currentOp", Value: 1},
			{Key: "active", Value: true},
		}).Decode(&result)
		if err != nil {
			continue
		}

		inprog, _ := result["inprog"].(bson.A)
		for _, raw := range inprog {
			op, ok := raw.(bson.M)
			if !ok {
				continue
			}

			secs := 0.0
			if v, ok := op["secs_running"].(int32); ok {
				secs = float64(v)
			} else if v, ok := op["secs_running"].(int64); ok {
				secs = float64(v)
			} else if v, ok := op["microsecs_running"].(int64); ok {
				secs = float64(v) / 1e6
			}

			if secs < topMinElapsed {
				continue
			}

			user, _ := op["effectiveUsers"].(bson.A)
			userName := ""
			if len(user) > 0 {
				if u, ok := user[0].(bson.M); ok {
					userName, _ = u["user"].(string)
				}
			}

			if topUser != "" && userName != topUser {
				continue
			}

			ns, _ := op["ns"].(string)
			dbName, collName := splitNS(ns)

			if topDB != "" && dbName != topDB {
				continue
			}

			opType, _ := op["op"].(string)
			plan, _ := op["planSummary"].(string)

			clientAddr, _ := op["client"].(string)

			all = append(all, opInfo{
				OpID:        op["opid"],
				User:        userName,
				Client:      clientAddr,
				DB:          dbName,
				Collection:  collName,
				OpType:      opType,
				SecsRunning: secs,
				Plan:        plan,
				Node:        node,
			})
		}
	}

	// Sort by duration desc.
	sort.Slice(all, func(i, j int) bool {
		return all[i].SecsRunning > all[j].SecsRunning
	})

	return all
}

func splitNS(ns string) (string, string) {
	for i := 0; i < len(ns); i++ {
		if ns[i] == '.' {
			return ns[:i], ns[i+1:]
		}
	}
	return ns, ""
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
