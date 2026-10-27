package cmd

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/spf13/cobra"

	"github.com/punchlistdigital/pldi/internal/awsx"
)

// Execute runs the root command.
func Execute() error {
	app, err := newApp()
	if err != nil {
		return err
	}
	return NewRootCmd(app).Execute()
}

// NewRootCmd builds the command tree for app.
func NewRootCmd(app *App) *cobra.Command {
	var profileFlag, regionFlag string

	root := &cobra.Command{
		Use:           "pldi",
		Short:         "Pull live WordPress sites from Bitnami EC2 via SSM",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       Version,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			flags := map[string]string{}
			if cmd.Flags().Changed("profile") {
				flags["profile"] = profileFlag
			}
			if cmd.Flags().Changed("region") {
				flags["region"] = regionFlag
			}
			if err := app.Cfg.ApplyFlags(flags, nil, false); err != nil {
				return err
			}
			if app.skipAWSLoad {
				return nil
			}
			cfg, err := awsx.LoadAWS(cmd.Context(), app.Cfg.Settings.Profile, app.Cfg.Settings.Region)
			if err != nil {
				return fmt.Errorf("load aws config: %w", err)
			}
			app.AWS = cfg
			if app.EC2 == nil {
				app.EC2 = ec2.NewFromConfig(cfg)
			}
			if app.STS == nil {
				app.STS = sts.NewFromConfig(cfg)
			}
			if app.SSM == nil {
				app.SSM = ssm.NewFromConfig(cfg)
			}
			if app.S3 == nil {
				app.S3 = s3.NewFromConfig(cfg)
			}
			return nil
		},
	}

	root.SetContext(context.Background())
	root.PersistentFlags().StringVar(&profileFlag, "profile", "", "AWS profile")
	root.PersistentFlags().StringVar(&regionFlag, "region", "", "AWS region")

	root.AddCommand(newStatusCmd(app))
	root.AddCommand(newEC2InstancesCmd(app))
	root.AddCommand(newWPSnapshotCmd(app))

	return root
}
