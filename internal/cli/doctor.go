package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/ppiankov/mongopulse/internal/config"
	"github.com/ppiankov/mongopulse/internal/doctor"
)

var doctorFormat string

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose connectivity and permissions",
	RunE:  runDoctor,
}

func init() {
	doctorCmd.Flags().StringVar(&doctorFormat, "format", "text", "Output format: text or json")
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Run doctor against first target (multi-target runs one per DSN).
	for _, dsn := range cfg.DSN {
		report := doctor.Run(ctx, dsn, version)

		switch doctorFormat {
		case "json":
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(report); err != nil {
				return err
			}
		default:
			printReport(report)
		}

		switch report.Status {
		case doctor.StatusFail:
			exitCode = 2
		case doctor.StatusWarn:
			if exitCode < 1 {
				exitCode = 1
			}
		}
	}

	return nil
}

func printReport(r doctor.Report) {
	fmt.Printf("mongopulse doctor (%s)\n\n", r.Tool.Version)
	for _, c := range r.Checks {
		icon := "PASS"
		switch c.Status {
		case doctor.StatusWarn:
			icon = "WARN"
		case doctor.StatusFail:
			icon = "FAIL"
		}
		fmt.Printf("  [%s] %s: %s\n", icon, c.Name, c.Message)
	}
	fmt.Printf("\nOverall: %s\n", r.Status)
}
