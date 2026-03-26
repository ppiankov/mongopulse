[![CI](https://github.com/ppiankov/mongopulse/actions/workflows/ci.yml/badge.svg)](https://github.com/ppiankov/mongopulse/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

# mongopulse

A heartbeat monitor for MongoDB — polls serverStatus, rs.status, and system.profile, exposes Prometheus metrics on `/metrics`.

## What mongopulse is

- A lightweight sidecar that connects to MongoDB and exposes 65 Prometheus-compatible metrics
- A poll-based exporter covering connections, replication, WiredTiger, opcounters, cursors, locks, collections, databases, network, sharding, and profiler data
- The only MongoDB exporter with query regression detection, unused index detection, election storm analysis, and connection exhaustion prediction
- Compatible with MongoDB 5.0+ (standalone, replica set, and sharded clusters via mongos)
- Ships with a Grafana dashboard and a Helm chart with ServiceMonitor + PrometheusRule
- Multi-target: monitor multiple MongoDB instances from a single process
- Zero config beyond a DSN — sensible defaults for everything

## What mongopulse is NOT

- Not a replacement for `mongodb_exporter` — mongopulse computes operational signals (query regressions, unused indexes, election storms, connection forecasting) that raw counters miss
- Not a query profiler or optimizer — it fingerprints slow queries and detects regressions, but won't explain or rewrite your queries
- Not a cluster manager — it reads serverStatus and system collections, it never writes to MongoDB or modifies settings
- Not an alerting engine on its own — built-in alerts go to Telegram/webhook, but pair it with Alertmanager for production thresholds
- Not a replacement for MongoDB Atlas monitoring — it's for self-hosted or VM-based deployments where you own the observability stack

## Philosophy

Observe, don't interfere. mongopulse opens a read-only window into MongoDB's own server status and system collections. It adds no extensions, modifies no data, and uses minimal resources. The metrics tell you what's happening; the predictions tell you what's coming; you decide what to do about it.

## MongoDB prerequisites

### Required: a monitoring user

Create a dedicated user with read access:

```javascript
db.createUser({
  user: "mongopulse",
  pwd: "your-secure-password",
  roles: [
    { role: "clusterMonitor", db: "admin" },
    { role: "read", db: "local" }
  ]
});
```

### Commands and collections used

| Source | Collector | Notes |
|--------|-----------|-------|
| `serverStatus` | Connections, WiredTiger, opcounters, cursors, locks, network | Core server health |
| `replSetGetStatus` | Replication, topology | Member states, lag, elections |
| `currentOp` | Active operations | Slow ops, longest running |
| `collStats` | Collections | Document count, sizes, indexes |
| `dbStats` | Databases | Data/storage/index sizes |
| `$indexStats` | Index usage | Per-index ops, unused detection |
| `system.profile` | Profiler | Query fingerprinting, regression |
| `oplog.rs` | Oplog window | Time window estimation |
| `config.chunks` | Sharding | Chunk distribution, skew |
| `config.changelog` | Balancer | Migration/split activity |
| `balancerStatus` | Balancer state | Running/stopped |

## Quick start

```bash
# Install
brew install ppiankov/tap/mongopulse

# Or build from source
make build

# Run
export MONGO_DSN="mongodb://mongopulse:password@localhost:27017"
mongopulse serve

# Docker
docker build -t mongopulse:dev .
docker run -e MONGO_DSN="mongodb://mongopulse:password@localhost:27017" -p 9216:9216 mongopulse:dev
```

Metrics at `http://localhost:9216/metrics`, health check at `/healthz`.

### Helm (Kubernetes)

```bash
helm upgrade --install mongopulse charts/mongopulse/ \
  --set mongoDSN="mongodb://mongopulse:password@mongodb:27017" \
  --set serviceMonitor.enabled=true \
  -n mongopulse-system
```

## Commands

| Command | Description | Exit codes |
|---------|-------------|------------|
| `mongopulse serve` | Start the metrics exporter | 0=clean, 1=failure |
| `mongopulse status [--format json] [--unhealthy]` | One-shot cluster health snapshot | 0=healthy, 1=degraded, 2=critical |
| `mongopulse doctor [--format json]` | Diagnose connectivity and permissions | 0=pass, 1=warn, 2=fail |
| `mongopulse init [--format env\|json]` | Print default configuration template | 0=success |
| `mongopulse version` | Print version | 0=success |

## Configuration

All configuration is via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `MONGO_DSN` or `DATABASE_URL` | *(required)* | MongoDB URI (comma-separated for multi-target) |
| `METRICS_PORT` | `9216` | Port for the HTTP metrics server |
| `POLL_INTERVAL` | `5s` | How often to collect metrics |
| `SLOW_QUERY_THRESHOLD` | `5s` | Duration after which a query is counted as slow |
| `REGRESSION_THRESHOLD` | `2.0` | Mean time multiplier to flag a query as regressed |
| `STMT_LIMIT` | `50` | Max profiler entries to process per poll |
| `TELEGRAM_BOT_TOKEN` | *(disabled)* | Telegram bot token for alerts |
| `TELEGRAM_CHAT_ID` | *(disabled)* | Telegram chat ID for alerts |
| `ALERT_WEBHOOK_URL` | *(disabled)* | Slack or generic webhook URL for alerts |
| `ALERT_COOLDOWN` | `5m` | Minimum interval between repeated alerts of same type |
| `GRAFANA_URL` | *(disabled)* | Grafana base URL for anomaly annotations |
| `GRAFANA_TOKEN` | *(disabled)* | Grafana service account token |
| `GRAFANA_DASHBOARD_UID` | *(optional)* | Scope annotations to a specific dashboard |

## What makes mongopulse unique

Four capabilities no other MongoDB exporter provides:

**Query regression detection** — Fingerprints queries from `system.profile` by shape, tracks mean/p95 execution time per fingerprint across polls, and flags regressions when the mean exceeds a configurable threshold multiplier over baseline.

**Unused index detection** — Runs `$indexStats` on all user collections, tracks per-index operation counts, and flags indexes with zero ops since server start. Surfaces `mongodb_index_unused=1` for agents and dashboards to act on.

**Election storm detection** — Stateful collector that tracks primary role across polls, detects primary changes, counts elections in a rolling window, and flags storms (>3 elections in 10 minutes).

**Connection exhaustion prediction** — Samples connection counts over time, computes a linear trend, and estimates hours until `maxIncomingConnections` is reached. Agents can trigger pre-emptive scaling or pool tuning based on `mongodb_conn_exhaustion_hours`.

## Architecture

```
cmd/mongopulse/main.go               CLI entry point (delegates to internal/cli)
internal/
  cli/                                Cobra commands: serve, status, doctor, init, version
  config/                             Environment-based configuration
  engine/                             Multi-target connection engine with retry
  collector/                          Poll loop + 15 collectors
    replication.go                    rs.status, oplog window
    connections.go                    serverStatus.connections
    wiredtiger.go                     serverStatus.wiredTiger.cache
    opcounters.go                     serverStatus.opcounters
    currentop.go                      currentOp (active, slow, longest)
    cursors.go                        serverStatus.metrics.cursor
    locks.go                          serverStatus.locks
    collections.go                    collStats per collection
    dbstats.go                        dbStats per database
    network.go                        serverStatus.network
    profiler.go                       system.profile + query regression (unique)
    indexusage.go                     $indexStats + unused detection (unique)
    topology.go                       Election storm detection (unique)
    connpredict.go                    Connection exhaustion prediction (unique)
    sharding.go                       Chunks, balancer, migrations, skew
  metrics/                            65 Prometheus metric definitions
  snapshot/                           Point-in-time health snapshot for status command
  doctor/                             Connectivity and permission diagnostics (ANCC)
  alerter/                            Telegram + webhook, 10 typed alerts, per-type cooldown
  annotator/                          Grafana anomaly annotations
  retry/                              Exponential backoff
  server/                             HTTP server (/metrics, /healthz)
  testutil/                           Test helpers (testcontainers MongoDB)
charts/mongopulse/                    Helm chart with ServiceMonitor + PrometheusRule
docs/
  SKILL.md                            ANCC interface declaration
  grafana-dashboard.json              Importable Grafana dashboard (20 panels)
```

## Known limitations

- Query fingerprinting requires profiling enabled (level 1 or 2) on target databases
- Sharding and balancer collectors only active when connected to mongos
- Connection exhaustion prediction requires 2+ poll cycles to compute trend
- Election storm detection resets on process restart (state is in-memory)
- Index usage stats reset on MongoDB server restart

## Roadmap

- [x] Core scaffold and CLI (serve, status, doctor, init, version)
- [x] 15 collectors (65 Prometheus metrics)
- [x] Query regression detection (system.profile fingerprinting)
- [x] Unused index detection ($indexStats)
- [x] Election storm detection (stateful)
- [x] Connection exhaustion prediction (stateful)
- [x] Multi-target support
- [x] Built-in alerting (Telegram, webhook)
- [x] Grafana dashboard and annotations
- [x] Helm chart with ServiceMonitor + PrometheusRule
- [x] ANCC compliance (SKILL.md, doctor JSON, exit codes)
- [x] Integration tests (testcontainers-go)
- [ ] Chainwatch runbook integration
- [ ] MCP server mode

## License

[MIT](LICENSE)
