package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/ppiankov/mongopulse/internal/engine"
)

type Server struct {
	srv    *http.Server
	engine *engine.Engine
}

func New(port int, reg *prometheus.Registry, eng *engine.Engine) *Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("/healthz", healthz(eng))

	return &Server{
		srv: &http.Server{
			Addr:    fmt.Sprintf(":%d", port),
			Handler: mux,
		},
		engine: eng,
	}
}

func (s *Server) ListenAndServe() error {
	slog.Info("serving metrics", "addr", s.srv.Addr, "path", "/metrics")
	return s.srv.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

func healthz(eng *engine.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		targets := eng.Targets()
		if len(targets) == 0 {
			http.Error(w, "no targets", http.StatusServiceUnavailable)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		for _, t := range targets {
			if err := t.Client.Ping(ctx, nil); err != nil {
				http.Error(w, fmt.Sprintf("unhealthy: %s", t.Node), http.StatusServiceUnavailable)
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	}
}
