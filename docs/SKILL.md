# mongopulse

MongoDB heartbeat monitor and Prometheus metrics exporter with query regression detection, index usage analysis, and connection exhaustion prediction.

## Install

```bash
brew install ppiankov/tap/mongopulse
```

Or:

```bash
go install github.com/ppiankov/mongopulse/cmd/mongopulse@latest
```

## Commands

### `mongopulse serve`

Start the long-running metrics exporter.

**Environment variables:**

| Variable | Default | Description |
|----------|---------|-------------|
| `MONGO_DSN` | *(required)* | MongoDB URI (comma-separated for multi-target) |
| `METRICS_PORT` | `9216` | Prometheus metrics port |
| `POLL_INTERVAL` | `5s` | Collection interval |
| `SLOW_QUERY_THRESHOLD` | `5s` | Threshold for slow query classification |
| `REGRESSION_THRESHOLD` | `2.0` | Mean multiplier to flag query regression |
| `STMT_LIMIT` | `50` | Max profiler entries per poll |

**Endpoints:**

| Path | Description |
|------|-------------|
| `/metrics` | Prometheus scrape endpoint |
| `/healthz` | Returns 200 if all targets reachable, 503 otherwise |

**Exit codes:** 0 = clean shutdown, 1 = startup failure

---

### `mongopulse status [--format json] [--unhealthy]`

One-shot cluster health snapshot.

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--format` | `text` | Output format: `text` or `json` |
| `--unhealthy` | `false` | Only show degraded/critical nodes |

**JSON output schema:**

```json
[{
  "timestamp": "2026-03-25T12:00:00Z",
  "node": "localhost:27017",
  "status": "healthy|degraded|critical",
  "version": "7.0.4",
  "uptime_seconds": 86400,
  "connections": {
    "current": 42,
    "available": 51158,
    "utilization_ratio": 0.001
  },
  "repl_set": {
    "set": "rs0",
    "state": "PRIMARY",
    "members": [{"name": "...", "state": "SECONDARY", "lag_seconds": 0.5}]
  },
  "wired_tiger": {
    "cache_used_bytes": 1073741824,
    "cache_max_bytes": 8589934592,
    "cache_utilization_ratio": 0.125,
    "dirty_bytes": 0
  },
  "opcounters": {"insert": 100, "query": 500, "update": 50, "delete": 10, "getmore": 20, "command": 1000},
  "active_ops": 3,
  "slow_ops": 0
}]
```

**Exit codes:** 0 = healthy, 1 = degraded, 2 = critical

---

### `mongopulse doctor [--format json]`

Diagnose connectivity and permissions.

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--format` | `text` | Output format: `text` or `json` |

**JSON output schema:**

```json
{
  "tool": {"name": "mongopulse", "version": "0.1.0"},
  "status": "pass|warn|fail",
  "checks": [
    {"name": "connectivity", "status": "pass", "message": "connected"},
    {"name": "server_version", "status": "pass", "message": "7.0.4"},
    {"name": "replication", "status": "warn", "message": "not a replica set member (standalone)"},
    {"name": "profiling", "status": "warn", "message": "profiling disabled (level 0)"},
    {"name": "permissions", "status": "pass", "message": "admin collection access OK"}
  ],
  "timestamp": "2026-03-25T12:00:00Z"
}
```

**Exit codes:** 0 = pass, 1 = warn, 2 = fail

---

### `mongopulse init [--format env|json]`

Print default configuration template.

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--format` | `env` | Output format: `env` or `json` |

**Exit codes:** 0 = success

---

### `mongopulse version`

Print version string.

**Exit codes:** 0 = success

## Metrics

### Core (5)
- `mongodb_up{node}` — target reachable
- `mongodb_uptime_seconds{node}` — server uptime
- `mongodb_version_info{node,version}` — server version
- `mongodb_poll_duration_seconds{node}` — collection time
- `mongodb_poll_errors_total{node}` — poll failures

### Replication (5)
- `mongodb_repl_member_state{node,member,set}` — member state
- `mongodb_repl_lag_seconds{node,member}` — replication lag
- `mongodb_repl_oplog_window_hours{node}` — oplog window
- `mongodb_repl_members_total{node,set}` — member count
- `mongodb_repl_elections_total{node}` — election events

### Connections (3)
- `mongodb_connections_current{node}`
- `mongodb_connections_available{node}`
- `mongodb_connections_created_total{node}`

### WiredTiger (6)
- `mongodb_wt_cache_bytes_used{node}`
- `mongodb_wt_cache_bytes_max{node}`
- `mongodb_wt_cache_dirty_bytes{node}`
- `mongodb_wt_cache_read_pages_total{node}`
- `mongodb_wt_cache_write_pages_total{node}`
- `mongodb_wt_cache_evictions_total{node}`

### Operations (7)
- `mongodb_opcounters_total{node,type}`
- `mongodb_active_ops{node}`
- `mongodb_slow_ops{node}`
- `mongodb_queued_ops{node,type}`
- `mongodb_longest_op_seconds{node}`
- `mongodb_cursors_open{node}`
- `mongodb_cursors_timed_out_total{node}`

### Locks (2)
- `mongodb_locks_active{node,type,mode}`
- `mongodb_locks_waiting{node,type,mode}`

### Data (9)
- `mongodb_collection_documents{node,db,collection}`
- `mongodb_collection_size_bytes{node,db,collection}`
- `mongodb_collection_indexes{node,db,collection}`
- `mongodb_collection_index_size_bytes{node,db,collection}`
- `mongodb_db_data_size_bytes{node,db}`
- `mongodb_db_storage_size_bytes{node,db}`
- `mongodb_db_index_size_bytes{node,db}`
- `mongodb_db_collections{node,db}`
- `mongodb_db_objects{node,db}`

### Network (3)
- `mongodb_network_bytes_in_total{node}`
- `mongodb_network_bytes_out_total{node}`
- `mongodb_network_requests_total{node}`

### Query Regression (5) — unique
- `mongodb_slow_queries_total{node,db,collection,op_type}`
- `mongodb_query_mean_ms{node,db,fingerprint}`
- `mongodb_query_p95_ms{node,db,fingerprint}`
- `mongodb_query_regression_total{node,db,fingerprint}`
- `mongodb_profiler_entries_total{node,db}`

### Index Usage (5) — unique
- `mongodb_index_ops_total{node,db,collection,index}`
- `mongodb_index_size_bytes{node,db,collection,index}`
- `mongodb_index_unused{node,db,collection,index}`
- `mongodb_indexes_total{node,db,collection}`
- `mongodb_indexes_unused_total{node,db,collection}`

### Topology (4) — unique
- `mongodb_topology_primary_changes_total{node,set}`
- `mongodb_topology_role{node,set}`
- `mongodb_topology_election_storm{node,set}`
- `mongodb_topology_last_election_seconds{node,set}`

### Connection Prediction (3) — unique
- `mongodb_conn_exhaustion_hours{node}`
- `mongodb_conn_utilization_ratio{node}`
- `mongodb_conn_trend_per_hour{node}`

### Sharding (8)
- `mongodb_sharding_chunks{node,shard,ns}`
- `mongodb_sharding_balancer_running{node}`
- `mongodb_sharding_jumbo_chunks{node}`
- `mongodb_sharding_collections{node}`
- `mongodb_balancer_migrations_total{node}`
- `mongodb_balancer_migration_failures_total{node}`
- `mongodb_balancer_splits_total{node}`
- `mongodb_shard_key_skew_ratio{node,ns}`

**Total: 65 Prometheus metrics.**

## What this does NOT do

1. Does not manage MongoDB clusters, replica sets, or sharded deployments
2. Does not modify data, schemas, indexes, or configuration on the target
3. Does not store time-series data — Prometheus handles storage
4. Does not replace MongoDB's built-in monitoring (mongostat, mongotop, Atlas)
5. Does not perform query optimization or automatic index tuning

## Trust Boundary

mongopulse is read-only. It issues `serverStatus`, `replSetGetStatus`, `currentOp`, `collStats`, `dbStats`, `$indexStats`, `balancerStatus`, and reads `system.profile`, `oplog.rs`, and `config` collections. It never runs DDL, DML, or administrative commands that modify state.

## Handoffs

| Output | Next tool category | Refused questions |
|--------|-------------------|-------------------|
| Prometheus metrics | Alertmanager, Grafana | "Why is this query slow?" |
| JSON status snapshot | Incident response runbook, agent workflow | "Fix the replication lag" |
| Doctor JSON | CI/CD pipeline gates | "Configure the replica set" |
| Unused index list | DBA review, index cleanup tool | "Drop this index" |

## Failure Modes

| Condition | Behavior |
|-----------|----------|
| Target unreachable | `mongodb_up=0`, retry with backoff, alert if configured |
| Not a replica set | Replication/topology collectors silently skip |
| Profiling disabled | Profiler collector silently skips |
| Not connected to mongos | Sharding collectors silently skip |
| Permission denied on system collection | Warning logged, collector skips |

## Parsing Examples

```bash
# Check if any node is down
mongopulse status --format json | jq '.[] | select(.status != "healthy")'

# Get replication lag for all secondaries
mongopulse status --format json | jq '.[].repl_set.members[] | select(.state == "SECONDARY") | {name, lag_seconds}'

# Run doctor and fail CI if any check fails
mongopulse doctor --format json | jq -e '.status == "pass"'

# List unused indexes
mongopulse status --format json | jq '.[].node' # then scrape /metrics for mongodb_index_unused == 1

# Get connection utilization
curl -s localhost:9216/metrics | grep mongodb_conn_utilization_ratio
```

---

Built with [ANCC](https://ancc.dev) conventions. Validate: `ancc validate .`
