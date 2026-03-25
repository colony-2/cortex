package cmd

import (
	"context"
	"os"

	"github.com/colony-2/colony2/server/c2j/internal/runjob"
	"github.com/spf13/cobra"
)

func newExecCmd() *cobra.Command {
	opts := runjob.Options{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}

	cmd := &cobra.Command{
		Use:   "exec",
		Short: "Execute or continue an existing job through the SWF remote runtime",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runjob.Run(context.Background(), opts)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opts.JobID, "job-id", "", "Job ID to execute")
	flags.StringVar(&opts.TenantID, "tenant-id", "", "Tenant/project ID for the job")
	flags.StringVar(&opts.SWFURL, "swf-url", "", "Base URL for the SWF remote runtime")
	flags.StringVar(&opts.RecipesDir, "recipes-dir", "", "Optional directory used to resolve recipes locally for child-job starts")
	flags.DurationVar(&opts.WaitTimeout, "wait-timeout", 15_000_000_000, "How long to wait on external blocking work before exiting")
	flags.DurationVar(&opts.PollInterval, "poll-interval", 5_000_000_000, "Polling interval while waiting on external blocking work")
	flags.DurationVar(&opts.LeaseDuration, "lease-duration", 60_000_000_000, "Lease duration requested from SWF")
	flags.DurationVar(&opts.AwaitThreshold, "await-threshold", 30_000_000_000, "Await threshold before SWF reschedules instead of sleeping inline")
	flags.StringVar(&opts.WorkerID, "worker-id", "", "Optional worker ID for SWF leases")
	flags.StringVar(&opts.OnNotReady, "on-not-ready", "wait", "How to handle non-ready jobs: wait|fail|fail-on-lease|fail-on-pending-jobs|fail-on-future|fail-on-missing-capability")
	flags.BoolVar(&opts.CI, "ci", false, "Emit machine-readable input-required events instead of prompting")
	flags.StringVar(&opts.InputMode, "input-mode", "", "Input mode override: prompt|ops|fail")

	return cmd
}
