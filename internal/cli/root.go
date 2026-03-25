package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	rootCmd = &cobra.Command{
		Use:   "mongopulse",
		Short: "MongoDB metrics exporter for Prometheus",
		Long:  "Connects to MongoDB, polls serverStatus, rs.status, and system.profile, and exposes Prometheus metrics on /metrics.",
	}
	exitCode int
)

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}
