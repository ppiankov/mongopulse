package cli

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/cobra"

	"github.com/ppiankov/mongopulse/internal/config"
	"github.com/ppiankov/mongopulse/internal/engine"
	"github.com/ppiankov/mongopulse/internal/metrics"
	"github.com/ppiankov/mongopulse/internal/server"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the metrics exporter",
	RunE:  runServe,
}

func init() {
	rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	reg := prometheus.NewRegistry()
	m := metrics.New(reg)
	eng := engine.New(cfg, m)

	if err := eng.Connect(ctx); err != nil {
		return fmt.Errorf("engine: %w", err)
	}

	ctx, cancel = signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go eng.Run(ctx)

	srv := server.New(cfg.MetricsPort, reg, eng)

	go func() {
		<-ctx.Done()
		log.Println("shutting down...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		srv.Shutdown(shutdownCtx)
		eng.Close(shutdownCtx)
	}()

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return fmt.Errorf("server: %w", err)
	}

	return nil
}
