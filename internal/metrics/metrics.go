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
}

func New(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		Up: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "mongodb_up",
			Help: "Whether the MongoDB target is reachable (1 = up, 0 = down).",
		}, []string{"node"}),
		Uptime: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "mongodb_uptime_seconds",
			Help: "Seconds since mongod/mongos started.",
		}, []string{"node"}),
		Version: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "mongodb_version_info",
			Help: "MongoDB server version as a label.",
		}, []string{"node", "version"}),

		PollDuration: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "mongodb_poll_duration_seconds",
			Help: "Time spent collecting metrics from the target.",
		}, []string{"node"}),
		PollErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "mongodb_poll_errors_total",
			Help: "Total number of poll errors per target.",
		}, []string{"node"}),
	}

	reg.MustRegister(m.Up, m.Uptime, m.Version, m.PollDuration, m.PollErrors)
	return m
}
