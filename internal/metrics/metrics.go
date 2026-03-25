package metrics

import "github.com/prometheus/client_golang/prometheus"

type Metrics struct {
	// General server info.
	Up      *prometheus.GaugeVec
	Uptime  *prometheus.GaugeVec
	Version *prometheus.GaugeVec

	// Poll tracking.
	PollDuration *prometheus.GaugeVec
	PollErrors   *prometheus.CounterVec

	// Replication (WO-5).
	ReplMemberState  *prometheus.GaugeVec
	ReplLagSeconds   *prometheus.GaugeVec
	ReplOplogWindowH *prometheus.GaugeVec
	ReplMembersTotal *prometheus.GaugeVec
	ReplElections    *prometheus.CounterVec

	// Connections (WO-6).
	ConnCurrent   *prometheus.GaugeVec
	ConnAvailable *prometheus.GaugeVec
	ConnCreated   *prometheus.CounterVec

	// WiredTiger (WO-7).
	WTCacheBytesUsed  *prometheus.GaugeVec
	WTCacheBytesMax   *prometheus.GaugeVec
	WTCacheDirtyBytes *prometheus.GaugeVec
	WTCacheReadPages  *prometheus.CounterVec
	WTCacheWritPages  *prometheus.CounterVec
	WTCacheEvictions  *prometheus.CounterVec

	// Operations (WO-8).
	OpsTotal *prometheus.CounterVec

	// CurrentOp (WO-9).
	ActiveOps  *prometheus.GaugeVec
	SlowOps    *prometheus.GaugeVec
	QueuedOps  *prometheus.GaugeVec
	LongestOps *prometheus.GaugeVec

	// Cursors (WO-10).
	CursorsOpen      *prometheus.GaugeVec
	CursorsTimedOut  *prometheus.CounterVec
	CursorsPinned    *prometheus.GaugeVec
	CursorsNoTimeout *prometheus.GaugeVec

	// Locks (WO-11).
	LocksActive  *prometheus.GaugeVec
	LocksWaiting *prometheus.GaugeVec

	// Collections (WO-12).
	CollDocCount   *prometheus.GaugeVec
	CollSizeBytes  *prometheus.GaugeVec
	CollIndexCount *prometheus.GaugeVec
	CollIndexSize  *prometheus.GaugeVec

	// DbStats (WO-13).
	DbDataSize    *prometheus.GaugeVec
	DbStorageSize *prometheus.GaugeVec
	DbIndexSize   *prometheus.GaugeVec
	DbCollections *prometheus.GaugeVec
	DbObjects     *prometheus.GaugeVec

	// Network (WO-14).
	NetBytesIn  *prometheus.CounterVec
	NetBytesOut *prometheus.CounterVec
	NetRequests *prometheus.CounterVec

	// Profiler / query regression (WO-28).
	SlowQueriesTotal     *prometheus.CounterVec
	QueryMeanMs          *prometheus.GaugeVec
	QueryP95Ms           *prometheus.GaugeVec
	QueryRegressionTotal *prometheus.CounterVec
	ProfilerEntries      *prometheus.CounterVec

	// Index usage (WO-29).
	IndexOpsTotal      *prometheus.CounterVec
	IndexSizeBytes     *prometheus.GaugeVec
	IndexUnused        *prometheus.GaugeVec
	IndexesTotal       *prometheus.GaugeVec
	IndexesUnusedTotal *prometheus.GaugeVec

	// Topology / elections (WO-30).
	TopoPrimaryChanges   *prometheus.CounterVec
	TopoRole             *prometheus.GaugeVec
	TopoElectionStorm    *prometheus.GaugeVec
	TopoLastElectionSecs *prometheus.GaugeVec

	// Connection prediction (WO-32).
	ConnExhaustionHours *prometheus.GaugeVec
	ConnUtilization     *prometheus.GaugeVec
	ConnTrendPerHour    *prometheus.GaugeVec
}

func New(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		Up:           gauge("mongodb_up", "Whether the MongoDB target is reachable (1=up, 0=down).", "node"),
		Uptime:       gauge("mongodb_uptime_seconds", "Seconds since mongod/mongos started.", "node"),
		Version:      gauge("mongodb_version_info", "MongoDB server version.", "node", "version"),
		PollDuration: gauge("mongodb_poll_duration_seconds", "Time spent collecting metrics.", "node"),
		PollErrors:   counter("mongodb_poll_errors_total", "Total poll errors.", "node"),

		// Replication.
		ReplMemberState:  gauge("mongodb_repl_member_state", "Replica set member state.", "node", "member", "set"),
		ReplLagSeconds:   gauge("mongodb_repl_lag_seconds", "Replication lag per secondary.", "node", "member"),
		ReplOplogWindowH: gauge("mongodb_repl_oplog_window_hours", "Oplog window in hours.", "node"),
		ReplMembersTotal: gauge("mongodb_repl_members_total", "Number of replica set members.", "node", "set"),
		ReplElections:    counter("mongodb_repl_elections_total", "Election events observed.", "node"),

		// Connections.
		ConnCurrent:   gauge("mongodb_connections_current", "Current connections.", "node"),
		ConnAvailable: gauge("mongodb_connections_available", "Available connections.", "node"),
		ConnCreated:   counter("mongodb_connections_created_total", "Total connections created.", "node"),

		// WiredTiger.
		WTCacheBytesUsed:  gauge("mongodb_wt_cache_bytes_used", "WiredTiger cache bytes in use.", "node"),
		WTCacheBytesMax:   gauge("mongodb_wt_cache_bytes_max", "WiredTiger configured cache size.", "node"),
		WTCacheDirtyBytes: gauge("mongodb_wt_cache_dirty_bytes", "WiredTiger dirty cache bytes.", "node"),
		WTCacheReadPages:  counter("mongodb_wt_cache_read_pages_total", "Pages read into cache.", "node"),
		WTCacheWritPages:  counter("mongodb_wt_cache_write_pages_total", "Pages written from cache.", "node"),
		WTCacheEvictions:  counter("mongodb_wt_cache_evictions_total", "Pages evicted from cache.", "node"),

		// Operations.
		OpsTotal: counter("mongodb_opcounters_total", "Operations by type.", "node", "type"),

		// CurrentOp.
		ActiveOps:  gauge("mongodb_active_ops", "Active operations.", "node"),
		SlowOps:    gauge("mongodb_slow_ops", "Operations exceeding threshold.", "node"),
		QueuedOps:  gauge("mongodb_queued_ops", "Queued operations.", "node", "type"),
		LongestOps: gauge("mongodb_longest_op_seconds", "Duration of longest running op.", "node"),

		// Cursors.
		CursorsOpen:      gauge("mongodb_cursors_open", "Open cursors.", "node"),
		CursorsTimedOut:  counter("mongodb_cursors_timed_out_total", "Cursors timed out.", "node"),
		CursorsPinned:    gauge("mongodb_cursors_pinned", "Pinned cursors.", "node"),
		CursorsNoTimeout: gauge("mongodb_cursors_no_timeout", "Cursors with no timeout.", "node"),

		// Locks.
		LocksActive:  gauge("mongodb_locks_active", "Active locks.", "node", "type", "mode"),
		LocksWaiting: gauge("mongodb_locks_waiting", "Waiting locks.", "node", "type", "mode"),

		// Collections.
		CollDocCount:   gauge("mongodb_collection_documents", "Document count.", "node", "db", "collection"),
		CollSizeBytes:  gauge("mongodb_collection_size_bytes", "Collection data size.", "node", "db", "collection"),
		CollIndexCount: gauge("mongodb_collection_indexes", "Number of indexes.", "node", "db", "collection"),
		CollIndexSize:  gauge("mongodb_collection_index_size_bytes", "Total index size.", "node", "db", "collection"),

		// DbStats.
		DbDataSize:    gauge("mongodb_db_data_size_bytes", "Database data size.", "node", "db"),
		DbStorageSize: gauge("mongodb_db_storage_size_bytes", "Database storage size.", "node", "db"),
		DbIndexSize:   gauge("mongodb_db_index_size_bytes", "Database index size.", "node", "db"),
		DbCollections: gauge("mongodb_db_collections", "Number of collections.", "node", "db"),
		DbObjects:     gauge("mongodb_db_objects", "Number of objects.", "node", "db"),

		// Network.
		NetBytesIn:  counter("mongodb_network_bytes_in_total", "Network bytes received.", "node"),
		NetBytesOut: counter("mongodb_network_bytes_out_total", "Network bytes sent.", "node"),
		NetRequests: counter("mongodb_network_requests_total", "Network requests.", "node"),

		// Profiler / query regression.
		SlowQueriesTotal:     counter("mongodb_slow_queries_total", "Slow queries.", "node", "db", "collection", "op_type"),
		QueryMeanMs:          gauge("mongodb_query_mean_ms", "Query mean execution time.", "node", "db", "fingerprint"),
		QueryP95Ms:           gauge("mongodb_query_p95_ms", "Query p95 execution time.", "node", "db", "fingerprint"),
		QueryRegressionTotal: counter("mongodb_query_regression_total", "Query regressions detected.", "node", "db", "fingerprint"),
		ProfilerEntries:      counter("mongodb_profiler_entries_total", "Profiler entries processed.", "node", "db"),

		// Index usage.
		IndexOpsTotal:      counter("mongodb_index_ops_total", "Index operations.", "node", "db", "collection", "index"),
		IndexSizeBytes:     gauge("mongodb_index_size_bytes", "Index size.", "node", "db", "collection", "index"),
		IndexUnused:        gauge("mongodb_index_unused", "Unused index (1=unused).", "node", "db", "collection", "index"),
		IndexesTotal:       gauge("mongodb_indexes_total", "Total indexes.", "node", "db", "collection"),
		IndexesUnusedTotal: gauge("mongodb_indexes_unused_total", "Total unused indexes.", "node", "db", "collection"),

		// Topology / elections.
		TopoPrimaryChanges:   counter("mongodb_topology_primary_changes_total", "Primary changes.", "node", "set"),
		TopoRole:             gauge("mongodb_topology_role", "Node role (1=primary, 2=secondary, 3=arbiter).", "node", "set"),
		TopoElectionStorm:    gauge("mongodb_topology_election_storm", "Election storm detected (1=active).", "node", "set"),
		TopoLastElectionSecs: gauge("mongodb_topology_last_election_seconds", "Seconds since last election.", "node", "set"),

		// Connection prediction.
		ConnExhaustionHours: gauge("mongodb_conn_exhaustion_hours", "Estimated hours until connection exhaustion.", "node"),
		ConnUtilization:     gauge("mongodb_conn_utilization_ratio", "Connection utilization ratio.", "node"),
		ConnTrendPerHour:    gauge("mongodb_conn_trend_per_hour", "Connection trend per hour.", "node"),
	}

	reg.MustRegister(
		m.Up, m.Uptime, m.Version, m.PollDuration, m.PollErrors,
		m.ReplMemberState, m.ReplLagSeconds, m.ReplOplogWindowH, m.ReplMembersTotal, m.ReplElections,
		m.ConnCurrent, m.ConnAvailable, m.ConnCreated,
		m.WTCacheBytesUsed, m.WTCacheBytesMax, m.WTCacheDirtyBytes, m.WTCacheReadPages, m.WTCacheWritPages, m.WTCacheEvictions,
		m.OpsTotal,
		m.ActiveOps, m.SlowOps, m.QueuedOps, m.LongestOps,
		m.CursorsOpen, m.CursorsTimedOut, m.CursorsPinned, m.CursorsNoTimeout,
		m.LocksActive, m.LocksWaiting,
		m.CollDocCount, m.CollSizeBytes, m.CollIndexCount, m.CollIndexSize,
		m.DbDataSize, m.DbStorageSize, m.DbIndexSize, m.DbCollections, m.DbObjects,
		m.NetBytesIn, m.NetBytesOut, m.NetRequests,
		m.SlowQueriesTotal, m.QueryMeanMs, m.QueryP95Ms, m.QueryRegressionTotal, m.ProfilerEntries,
		m.IndexOpsTotal, m.IndexSizeBytes, m.IndexUnused, m.IndexesTotal, m.IndexesUnusedTotal,
		m.TopoPrimaryChanges, m.TopoRole, m.TopoElectionStorm, m.TopoLastElectionSecs,
		m.ConnExhaustionHours, m.ConnUtilization, m.ConnTrendPerHour,
	)

	return m
}

func gauge(name, help string, labels ...string) *prometheus.GaugeVec {
	return prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: name, Help: help}, labels)
}

func counter(name, help string, labels ...string) *prometheus.CounterVec {
	return prometheus.NewCounterVec(prometheus.CounterOpts{Name: name, Help: help}, labels)
}
