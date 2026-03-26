package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var ciInitFormat string

var ciInitCmd = &cobra.Command{
	Use:   "ci-init",
	Short: "Generate CI pipeline snippets",
	RunE:  runCIInit,
}

func init() {
	ciInitCmd.Flags().StringVar(&ciInitFormat, "format", "github", "Output format: github or gitlab")
	rootCmd.AddCommand(ciInitCmd)
}

func runCIInit(cmd *cobra.Command, args []string) error {
	switch ciInitFormat {
	case "github":
		fmt.Print(githubActions)
	case "gitlab":
		fmt.Print(gitlabCI)
	default:
		return fmt.Errorf("unknown format %q: use github or gitlab", ciInitFormat)
	}
	return nil
}

const githubActions = `# mongopulse health check — add to .github/workflows/
name: MongoDB Health Check
on:
  schedule:
    - cron: '0 6 * * *'  # Daily at 6am UTC
  workflow_dispatch:

jobs:
  health-check:
    runs-on: ubuntu-latest
    steps:
      - name: Install mongopulse
        run: |
          curl -sL https://github.com/ppiankov/mongopulse/releases/latest/download/mongopulse-linux-amd64.tar.gz | tar xz
          sudo mv mongopulse-linux-amd64 /usr/local/bin/mongopulse

      - name: Doctor check
        env:
          MONGO_DSN: ${{ secrets.MONGO_DSN }}
        run: mongopulse doctor --format json

      - name: Status check
        env:
          MONGO_DSN: ${{ secrets.MONGO_DSN }}
        run: |
          # Exit 0=healthy, 1=degraded, 2=critical
          mongopulse status --format json --unhealthy
`

const gitlabCI = `# mongopulse health check — add to .gitlab-ci.yml
mongopulse:health:
  stage: test
  image: ghcr.io/ppiankov/mongopulse:latest
  variables:
    # Set MONGO_DSN in CI/CD Variables (masked)
    MONGO_DSN: $MONGO_DSN
  script:
    - mongopulse doctor --format json
    - mongopulse status --format json --unhealthy
  rules:
    - if: $CI_PIPELINE_SOURCE == "schedule"
    - if: $CI_PIPELINE_SOURCE == "web"
  allow_failure: false
`
