package cli

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
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
	slowFormat      string
	slowLookback    string
	slowMinDuration string
	slowSortBy      string
	slowShowExample bool
	slowTop         int
)

var slowCmd = &cobra.Command{
	Use:   "slow",
	Short: "Slow query digest from system.profile",
	RunE:  runSlow,
}

func init() {
	slowCmd.Flags().StringVar(&slowFormat, "format", "text", "Output format: text or json")
	slowCmd.Flags().StringVar(&slowLookback, "lookback", "24h", "Time window")
	slowCmd.Flags().StringVar(&slowMinDuration, "min-duration", "0s", "Minimum query duration to include")
	slowCmd.Flags().StringVar(&slowSortBy, "sort", "duration", "Sort by: duration, count, or docs")
	slowCmd.Flags().BoolVar(&slowShowExample, "show-example", false, "Show one example query per pattern")
	slowCmd.Flags().IntVar(&slowTop, "top", 20, "Number of patterns to show")
	rootCmd.AddCommand(slowCmd)
}

type queryPattern struct {
	Fingerprint string  `json:"fingerprint"`
	DB          string  `json:"db"`
	Collection  string  `json:"collection"`
	OpType      string  `json:"op_type"`
	Count       int     `json:"count"`
	TotalMs     float64 `json:"total_ms"`
	MeanMs      float64 `json:"mean_ms"`
	P50Ms       float64 `json:"p50_ms"`
	P95Ms       float64 `json:"p95_ms"`
	P99Ms       float64 `json:"p99_ms"`
	AvgDocsExam float64 `json:"avg_docs_examined"`
	AvgKeysExam float64 `json:"avg_keys_examined"`
	Example     string  `json:"example,omitempty"`
	samples     []float64
	totalDocs   float64
	totalKeys   float64
}

func runSlow(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	lookback, err := time.ParseDuration(slowLookback)
	if err != nil {
		return fmt.Errorf("invalid lookback: %w", err)
	}
	minDur, err := time.ParseDuration(slowMinDuration)
	if err != nil {
		return fmt.Errorf("invalid min-duration: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(cfg.DSN[0]).SetTimeout(10 * time.Second))
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer client.Disconnect(ctx)

	patterns := collectSlowPatterns(ctx, client, lookback, minDur)

	sortPatterns(patterns)
	if len(patterns) > slowTop {
		patterns = patterns[:slowTop]
	}

	// Compute percentiles.
	for i := range patterns {
		p := &patterns[i]
		p.MeanMs = p.TotalMs / float64(p.Count)
		p.P50Ms = percentileSlow(p.samples, 50)
		p.P95Ms = percentileSlow(p.samples, 95)
		p.P99Ms = percentileSlow(p.samples, 99)
		if p.Count > 0 {
			p.AvgDocsExam = p.totalDocs / float64(p.Count)
			p.AvgKeysExam = p.totalKeys / float64(p.Count)
		}
	}

	switch slowFormat {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(patterns)
	default:
		if len(patterns) == 0 {
			fmt.Println("No slow queries found. Is profiling enabled?")
			return nil
		}
		fmt.Printf("%-16s %-8s %-20s %6s %10s %8s %8s %8s\n",
			"FINGERPRINT", "TYPE", "COLLECTION", "COUNT", "TOTAL_MS", "P50", "P95", "P99")
		for _, p := range patterns {
			coll := p.DB + "." + p.Collection
			fmt.Printf("%-16s %-8s %-20s %6d %10.0f %8.0f %8.0f %8.0f\n",
				p.Fingerprint[:16], p.OpType, truncate(coll, 20),
				p.Count, p.TotalMs, p.P50Ms, p.P95Ms, p.P99Ms)
			if slowShowExample && p.Example != "" {
				fmt.Printf("  example: %s\n", truncate(p.Example, 120))
			}
		}
	}
	return nil
}

func collectSlowPatterns(ctx context.Context, client *mongo.Client, lookback, minDur time.Duration) []queryPattern {
	dbs, err := client.ListDatabaseNames(ctx, bson.D{})
	if err != nil {
		return nil
	}

	patternMap := make(map[string]*queryPattern)
	cutoff := time.Now().Add(-lookback)
	minMs := minDur.Seconds() * 1000

	for _, dbName := range dbs {
		if dbName == "admin" || dbName == "local" || dbName == "config" {
			continue
		}

		// Check profiling level.
		var profileResult bson.M
		if err := client.Database(dbName).RunCommand(ctx, bson.D{{Key: "profile", Value: -1}}).Decode(&profileResult); err != nil {
			continue
		}
		level, _ := profileResult["was"].(int32)
		if level == 0 {
			continue
		}

		coll := client.Database(dbName).Collection("system.profile")
		filter := bson.D{{Key: "ts", Value: bson.D{{Key: "$gte", Value: cutoff}}}}
		cursor, err := coll.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "millis", Value: -1}}).SetLimit(1000))
		if err != nil {
			continue
		}

		for cursor.Next(ctx) {
			var doc bson.M
			if err := cursor.Decode(&doc); err != nil {
				continue
			}

			millis := 0.0
			if v, ok := doc["millis"].(int32); ok {
				millis = float64(v)
			} else if v, ok := doc["millis"].(int64); ok {
				millis = float64(v)
			}

			if millis < minMs {
				continue
			}

			opType, _ := doc["op"].(string)
			ns, _ := doc["ns"].(string)
			_, collName := splitNS(ns)

			fp := fingerprintQuery(doc)
			key := dbName + ":" + fp

			p, exists := patternMap[key]
			if !exists {
				p = &queryPattern{
					Fingerprint: fp,
					DB:          dbName,
					Collection:  collName,
					OpType:      opType,
				}
				patternMap[key] = p
			}

			p.Count++
			p.TotalMs += millis
			p.samples = append(p.samples, millis)

			if v, ok := doc["docsExamined"].(int32); ok {
				p.totalDocs += float64(v)
			} else if v, ok := doc["docsExamined"].(int64); ok {
				p.totalDocs += float64(v)
			}
			if v, ok := doc["keysExamined"].(int32); ok {
				p.totalKeys += float64(v)
			} else if v, ok := doc["keysExamined"].(int64); ok {
				p.totalKeys += float64(v)
			}

			if slowShowExample && p.Example == "" {
				if cmd, ok := doc["command"].(bson.M); ok {
					b, _ := json.Marshal(cmd)
					p.Example = string(b)
				}
			}
		}
		cursor.Close(ctx)
	}

	var result []queryPattern
	for _, p := range patternMap {
		result = append(result, *p)
	}
	return result
}

func fingerprintQuery(doc bson.M) string {
	cmd, ok := doc["command"].(bson.M)
	if !ok {
		cmd = doc
	}
	shape := extractShape(cmd)
	b, _ := json.Marshal(shape)
	h := sha256.Sum256(b)
	return fmt.Sprintf("%x", h[:8])
}

func extractShape(m bson.M) map[string]interface{} {
	out := make(map[string]interface{})
	for k, v := range m {
		if sub, ok := v.(bson.M); ok {
			out[k] = extractShape(sub)
		} else {
			out[k] = 1
		}
	}
	return out
}

func sortPatterns(patterns []queryPattern) {
	sort.Slice(patterns, func(i, j int) bool {
		switch slowSortBy {
		case "count":
			return patterns[i].Count > patterns[j].Count
		case "docs":
			return patterns[i].totalDocs > patterns[j].totalDocs
		default:
			return patterns[i].TotalMs > patterns[j].TotalMs
		}
	})
}

func percentileSlow(samples []float64, pct float64) float64 {
	if len(samples) == 0 {
		return 0
	}
	sorted := make([]float64, len(samples))
	copy(sorted, samples)
	sort.Float64s(sorted)
	idx := (pct / 100.0) * float64(len(sorted)-1)
	lower := int(math.Floor(idx))
	upper := int(math.Ceil(idx))
	if lower == upper || upper >= len(sorted) {
		return sorted[lower]
	}
	frac := idx - float64(lower)
	return sorted[lower]*(1-frac) + sorted[upper]*frac
}
