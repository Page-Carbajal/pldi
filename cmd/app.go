package cmd

import (
	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/punchlistdigital/pldi/internal/awsx"
	"github.com/punchlistdigital/pldi/internal/config"
)

// Version is set via ldflags at build time.
var Version = "dev"

// App holds shared dependencies for commands.
type App struct {
	Cfg *config.Loaded
	AWS aws.Config

	EC2 awsx.EC2API
	STS awsx.STSAPI
	SSM awsx.SSMAPI
	S3  awsx.S3API

	// skipAWSLoad lets tests inject clients without loading AWS config.
	skipAWSLoad bool
}

func newApp() (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	return &App{Cfg: cfg}, nil
}
