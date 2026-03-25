package snapshot

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type Status string

const (
	Healthy  Status = "healthy"
	Degraded Status = "degraded"
	Critical Status = "critical"
)

type Snapshot struct {
	Timestamp   string          `json:"timestamp"`
	Node        string          `json:"node"`
	Status      Status          `json:"status"`
	Version     string          `json:"version,omitempty"`
	Uptime      float64         `json:"uptime_seconds,omitempty"`
	Connections ConnectionsSnap `json:"connections"`
	ReplSet     *ReplSetSnap    `json:"repl_set,omitempty"`
	WiredTiger  WTSnap          `json:"wired_tiger"`
	Ops         OpsSnap         `json:"opcounters"`
	ActiveOps   int             `json:"active_ops"`
	SlowOps     int             `json:"slow_ops"`
}

type ConnectionsSnap struct {
	Current   int     `json:"current"`
	Available int     `json:"available"`
	Ratio     float64 `json:"utilization_ratio"`
}

type ReplSetSnap struct {
	Set     string       `json:"set"`
	State   string       `json:"state"`
	Members []MemberSnap `json:"members,omitempty"`
}

type MemberSnap struct {
	Name    string  `json:"name"`
	State   string  `json:"state"`
	LagSecs float64 `json:"lag_seconds,omitempty"`
}

type WTSnap struct {
	CacheUsedBytes float64 `json:"cache_used_bytes"`
	CacheMaxBytes  float64 `json:"cache_max_bytes"`
	CacheRatio     float64 `json:"cache_utilization_ratio"`
	DirtyBytes     float64 `json:"dirty_bytes"`
}

type OpsSnap struct {
	Insert  float64 `json:"insert"`
	Query   float64 `json:"query"`
	Update  float64 `json:"update"`
	Delete  float64 `json:"delete"`
	Getmore float64 `json:"getmore"`
	Command float64 `json:"command"`
}

func Take(ctx context.Context, client *mongo.Client, node string, slowThresholdSecs float64) Snapshot {
	snap := Snapshot{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Node:      node,
		Status:    Healthy,
	}

	var ss bson.M
	if err := client.Database("admin").RunCommand(ctx, bson.D{{Key: "serverStatus", Value: 1}}).Decode(&ss); err != nil {
		snap.Status = Critical
		return snap
	}

	snap.Version, _ = ss["version"].(string)
	snap.Uptime, _ = toF64(ss["uptime"])

	// Connections.
	if conns, ok := ss["connections"].(bson.M); ok {
		cur, _ := toF64(conns["current"])
		avail, _ := toF64(conns["available"])
		snap.Connections.Current = int(cur)
		snap.Connections.Available = int(avail)
		if total := cur + avail; total > 0 {
			snap.Connections.Ratio = cur / total
		}
		if snap.Connections.Ratio > 0.9 {
			snap.downgrade(Degraded)
		}
	}

	// WiredTiger.
	if wt, ok := ss["wiredTiger"].(bson.M); ok {
		if cache, ok := wt["cache"].(bson.M); ok {
			snap.WiredTiger.CacheUsedBytes, _ = toF64(cache["bytes currently in the cache"])
			snap.WiredTiger.CacheMaxBytes, _ = toF64(cache["maximum bytes configured"])
			snap.WiredTiger.DirtyBytes, _ = toF64(cache["tracked dirty bytes in the cache"])
			if snap.WiredTiger.CacheMaxBytes > 0 {
				snap.WiredTiger.CacheRatio = snap.WiredTiger.CacheUsedBytes / snap.WiredTiger.CacheMaxBytes
			}
			if snap.WiredTiger.CacheRatio > 0.8 {
				snap.downgrade(Degraded)
			}
		}
	}

	// Opcounters.
	if ops, ok := ss["opcounters"].(bson.M); ok {
		snap.Ops.Insert, _ = toF64(ops["insert"])
		snap.Ops.Query, _ = toF64(ops["query"])
		snap.Ops.Update, _ = toF64(ops["update"])
		snap.Ops.Delete, _ = toF64(ops["delete"])
		snap.Ops.Getmore, _ = toF64(ops["getmore"])
		snap.Ops.Command, _ = toF64(ops["command"])
	}

	// Replication.
	var rsStatus bson.M
	if err := client.Database("admin").RunCommand(ctx, bson.D{{Key: "replSetGetStatus", Value: 1}}).Decode(&rsStatus); err == nil {
		rs := &ReplSetSnap{}
		rs.Set, _ = rsStatus["set"].(string)
		if myState, _ := toF64(rsStatus["myState"]); myState == 1 {
			rs.State = "PRIMARY"
		} else {
			rs.State = "SECONDARY"
		}

		if members, ok := rsStatus["members"].(bson.A); ok {
			var primaryOptime time.Time
			for _, raw := range members {
				m, _ := raw.(bson.M)
				st, _ := toF64(m["state"])
				if st == 1 {
					primaryOptime, _ = m["optimeDate"].(time.Time)
				}
			}
			for _, raw := range members {
				m, _ := raw.(bson.M)
				name, _ := m["name"].(string)
				st, _ := toF64(m["state"])
				ms := MemberSnap{Name: name, State: stateStr(int(st))}
				if st == 2 && !primaryOptime.IsZero() {
					if ot, ok := m["optimeDate"].(time.Time); ok {
						ms.LagSecs = primaryOptime.Sub(ot).Seconds()
						if ms.LagSecs > 10 {
							snap.downgrade(Degraded)
						}
					}
				}
				rs.Members = append(rs.Members, ms)
			}

			hasPrimary := false
			for _, mem := range rs.Members {
				if mem.State == "PRIMARY" {
					hasPrimary = true
				}
			}
			if !hasPrimary {
				snap.downgrade(Critical)
			}
		}
		snap.ReplSet = rs
	}

	// Active/slow ops.
	var currentOp bson.M
	if err := client.Database("admin").RunCommand(ctx, bson.D{
		{Key: "currentOp", Value: 1},
		{Key: "active", Value: true},
	}).Decode(&currentOp); err == nil {
		if inprog, ok := currentOp["inprog"].(bson.A); ok {
			for _, raw := range inprog {
				op, _ := raw.(bson.M)
				snap.ActiveOps++
				secs, ok := toF64(op["secs_running"])
				if ok && secs > slowThresholdSecs {
					snap.SlowOps++
				}
			}
		}
	}

	return snap
}

func (s *Snapshot) downgrade(to Status) {
	if to == Critical {
		s.Status = Critical
	} else if to == Degraded && s.Status == Healthy {
		s.Status = Degraded
	}
}

func (s *Snapshot) IsUnhealthy() bool {
	return s.Status != Healthy
}

func stateStr(state int) string {
	switch state {
	case 1:
		return "PRIMARY"
	case 2:
		return "SECONDARY"
	case 7:
		return "ARBITER"
	case 3:
		return "RECOVERING"
	default:
		return "UNKNOWN"
	}
}

func toF64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}
