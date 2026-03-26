# Changelog

All notable changes to this project will be documented in this file.

## [0.1.0] - 2026-03-26

### Added

- Core CLI with five commands: `serve`, `status`, `doctor`, `init`, `version`
- 15 collectors producing 65 Prometheus metrics
- Replication monitoring: member state, lag per secondary, oplog window, election events
- Connection monitoring with exhaustion prediction (stateful forecasting)
- WiredTiger cache monitoring: bytes, dirty, eviction, read/write pages
- Operations: opcounters, currentOp (active/slow/longest), cursors, locks
- Data collectors: per-collection stats, per-database stats, network I/O
- Query regression detection via system.profile fingerprinting (unique)
- Unused index detection via $indexStats (unique)
- Election storm detection with stateful pattern analysis (unique)
- Sharding collectors: chunks per shard, balancer status, migration activity, shard key skew
- Built-in alerting: Telegram + webhook, 10 typed alerts with per-type cooldown
- Grafana anomaly annotations on spikes
- Multi-target support via comma-separated MONGO_DSN
- `status --unhealthy` filter and `--format json` on all commands
- ANCC-compliant `doctor` with 3-level exit codes (0=pass, 1=warn, 2=fail)
- Retry logic with exponential backoff for connectivity
- Helm chart with Deployment, Service, ServiceMonitor, PrometheusRule (7 alert rules)
- Grafana dashboard with 20 panels
- Dockerfile (distroless), CI workflow, release workflow with Homebrew tap automation
- ANCC-compliant SKILL.md with full interface declaration
- Integration tests with testcontainers-go against real MongoDB
