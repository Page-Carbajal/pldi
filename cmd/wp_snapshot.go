package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/punchlistdigital/pldi/internal/instances"
	"github.com/punchlistdigital/pldi/internal/snapshot"
)

func newWPSnapshotCmd(app *App) *cobra.Command {
	var (
		bucket, prefix, wpRoot, out string
		timeoutStr                  string
		exclude                     []string
		force, dryRun               bool
	)

	cmd := &cobra.Command{
		Use:   "ec2:wp-snapshot <instance-id|name>",
		Short: "Snapshot wp-content and database from an EC2 WordPress instance",
		Args:  cobra.ExactArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) != 0 || app.EC2 == nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			list, err := instances.List(cmd.Context(), app.EC2, instances.ListOptions{})
			if err != nil {
				return nil, cobra.ShellCompDirectiveError
			}
			var out []string
			for _, inst := range list {
				out = append(out, inst.InstanceID)
				if inst.Name != "-" {
					out = append(out, inst.Name)
				}
			}
			return out, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := map[string]string{}
			excludeSet := cmd.Flags().Changed("exclude")
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
			if err := app.Cfg.ApplyFlags(flags, exclude, excludeSet); err != nil {
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

			return snapshot.Run(cmd.Context(), app.EC2, app.SSM, app.S3, snapshot.RunOptions{
				IDOrName: args[0],
				Bucket:   settings.Bucket,
				Prefix:   prefixVal,
				WPRoot:   settings.WPRoot,
				OutDir:   settings.Out,
				Exclude:  settings.Exclude,
				Timeout:  timeout,
				Force:    force,
				DryRun:   dryRun,
				Progress: cmd.ErrOrStderr(),
				Stdout:   cmd.OutOrStdout(),
			})
		},
	}

	cmd.Flags().StringVar(&bucket, "bucket", "", "S3 bucket for artifacts")
	cmd.Flags().StringVar(&prefix, "prefix", "", "full S3 key prefix (default: <slug>/<timestamp>)")
	cmd.Flags().StringVar(&wpRoot, "wp-root", "", "WordPress root on the instance")
	cmd.Flags().StringVar(&out, "out", "", "local output directory")
	cmd.Flags().StringArrayVar(&exclude, "exclude", nil, "wp-content subpaths to exclude (repeatable)")
	cmd.Flags().StringVar(&timeoutStr, "timeout", "", "max time for the remote command (e.g. 30m)")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing local content.zip / db.sql")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print remote script and paths without calling AWS")

	return cmd
}
