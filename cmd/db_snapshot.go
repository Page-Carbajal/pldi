package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/punchlistdigital/pldi/internal/snapshot"
)

func newDBSnapshotCmd(app *App) *cobra.Command {
	var (
		bucket, prefix, wpRoot, out   string
		timeoutStr                    string
		force, dryRun, verbose, quiet bool
	)

	cmd := &cobra.Command{
		Use:               "ec2:db-snapshot <instance-id|name>",
		Short:             "Dump the database from an EC2 WordPress instance and download it",
		Long:              "Dumps the WordPress database on the instance, stages it at s3://<bucket>/<slug>/db-<timestamp>.sql, and downloads it to <out>/db.sql.",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: instanceArgCompletion(app),
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := map[string]string{}
			if cmd.Flags().Changed("bucket") {
				flags["bucket"] = bucket
			}
			if cmd.Flags().Changed("wp-root") {
				flags["wp_root"] = wpRoot
			}
			if cmd.Flags().Changed("out") {
				flags["out"] = out
			}
			if cmd.Flags().Changed("timeout") {
				flags["timeout"] = timeoutStr
			}
			if err := app.Cfg.ApplyFlags(flags, nil, false); err != nil {
				return err
			}

			settings := app.Cfg.Settings
			timeout := settings.Timeout
			if cmd.Flags().Changed("timeout") {
				d, err := time.ParseDuration(timeoutStr)
				if err != nil {
					return fmt.Errorf("parse --timeout: %w", err)
				}
				timeout = d
			}

			prefixVal := ""
			if cmd.Flags().Changed("prefix") {
				prefixVal = prefix
			}

			if verbose && quiet {
				return fmt.Errorf("cannot use --verbose and --quiet together")
			}

			return snapshot.RunDB(cmd.Context(), app.EC2, app.SSM, app.S3, snapshot.DBRunOptions{
				IDOrName: args[0],
				Bucket:   settings.Bucket,
				Prefix:   prefixVal,
				WPRoot:   settings.WPRoot,
				OutDir:   settings.Out,
				Timeout:  timeout,
				Force:    force,
				DryRun:   dryRun,
				Verbose:  verbose,
				Quiet:    quiet,
				Progress: cmd.ErrOrStderr(),
				Stdout:   cmd.OutOrStdout(),
			})
		},
	}

	cmd.Flags().StringVar(&bucket, "bucket", "", "S3 bucket for artifacts")
	cmd.Flags().StringVar(&prefix, "prefix", "", "S3 folder for the dump (default: <slug>)")
	cmd.Flags().StringVar(&wpRoot, "wp-root", "", "WordPress root on the instance")
	cmd.Flags().StringVar(&out, "out", "", "local output directory")
	cmd.Flags().StringVar(&timeoutStr, "timeout", "", "max time for the remote command (e.g. 30m)")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing local db.sql")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print remote script and paths without calling AWS")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "stream remote stdout/stderr")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "suppress progress; errors and summary only")

	return cmd
}
