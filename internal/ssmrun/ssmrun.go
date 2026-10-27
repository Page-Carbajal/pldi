package ssmrun

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"

	"github.com/punchlistdigital/pldi/internal/awsx"
)

const stepPrefix = "pldi:step "

// Result is the outcome of a remote command.
type Result struct {
	CommandID     string
	Stdout        string
	Stderr        string
	Status        string
	Steps         []string
	Duration      time.Duration
	ExitCode      int32
	StatusDetails string
}

// Options controls SendAndWait.
type Options struct {
	Timeout      time.Duration
	PollEvery    time.Duration
	Progress     io.Writer // optional; receives overwriting progress lines
	DocumentName string
}

// EnsureOnline fails unless the instance is SSM-managed with PingStatus Online.
func EnsureOnline(ctx context.Context, api awsx.SSMAPI, instanceID string) error {
	out, err := api.DescribeInstanceInformation(ctx, &ssm.DescribeInstanceInformationInput{
		Filters: []types.InstanceInformationStringFilter{{
			Key:    aws.String("InstanceIds"),
			Values: []string{instanceID},
		}},
	})
	if err != nil {
		return fmt.Errorf("ssm describe instance information: %w", err)
	}
	if len(out.InstanceInformationList) == 0 {
		return fmt.Errorf("instance %s is not SSM-managed or not reporting; cannot run commands", instanceID)
	}
	info := out.InstanceInformationList[0]
	if info.PingStatus != types.PingStatusOnline {
		return fmt.Errorf("instance %s SSM PingStatus is %s (want Online)", instanceID, info.PingStatus)
	}
	return nil
}

// SendAndWait sends AWS-RunShellScript and polls until terminal status or timeout.
func SendAndWait(ctx context.Context, api awsx.SSMAPI, instanceID, script string, opts Options) (Result, error) {
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Minute
	}
	if opts.PollEvery <= 0 {
		opts.PollEvery = 2 * time.Second
	}
	if opts.DocumentName == "" {
		opts.DocumentName = "AWS-RunShellScript"
	}

	if err := EnsureOnline(ctx, api, instanceID); err != nil {
		return Result{}, err
	}

	timeoutSecs := int64(opts.Timeout.Seconds())
	if timeoutSecs < 30 {
		timeoutSecs = 30
	}

	start := time.Now()
	sendOut, err := api.SendCommand(ctx, &ssm.SendCommandInput{
		DocumentName: aws.String(opts.DocumentName),
		InstanceIds:  []string{instanceID},
		Parameters: map[string][]string{
			"commands":         {script},
			"executionTimeout": {fmt.Sprintf("%d", timeoutSecs)},
		},
		TimeoutSeconds: aws.Int32(int32(timeoutSecs)),
	})
	if err != nil {
		return Result{}, fmt.Errorf("ssm send command: %w", err)
	}
	commandID := aws.ToString(sendOut.Command.CommandId)

	deadline := start.Add(opts.Timeout)
	var lastSteps []string
	for {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		if time.Now().After(deadline) {
			return Result{CommandID: commandID, Steps: lastSteps}, fmt.Errorf("ssm command %s timed out after %s", commandID, opts.Timeout)
		}

		inv, err := api.GetCommandInvocation(ctx, &ssm.GetCommandInvocationInput{
			CommandId:  aws.String(commandID),
			InstanceId: aws.String(instanceID),
		})
		if err != nil {
			var notFound *types.InvocationDoesNotExist
			if errors.As(err, &notFound) || strings.Contains(err.Error(), "InvocationDoesNotExist") {
				select {
				case <-ctx.Done():
					return Result{}, ctx.Err()
				case <-time.After(opts.PollEvery):
				}
				continue
			}
			return Result{}, fmt.Errorf("ssm get command invocation: %w", err)
		}

		stdout := aws.ToString(inv.StandardOutputContent)
		stderr := aws.ToString(inv.StandardErrorContent)
		steps := ParseSteps(stdout)
		lastSteps = steps
		if opts.Progress != nil {
			step := "waiting"
			if len(steps) > 0 {
				step = steps[len(steps)-1]
			}
			fmt.Fprintf(opts.Progress, "\rpldi: %s (status=%s)          ", step, inv.Status)
		}

		status := string(inv.Status)
		switch inv.Status {
		case types.CommandInvocationStatusSuccess:
			if opts.Progress != nil {
				fmt.Fprint(opts.Progress, "\n")
			}
			return Result{
				CommandID:     commandID,
				Stdout:        stdout,
				Stderr:        stderr,
				Status:        status,
				Steps:         steps,
				Duration:      time.Since(start),
				ExitCode:      inv.ResponseCode,
				StatusDetails: aws.ToString(inv.StatusDetails),
			}, nil
		case types.CommandInvocationStatusCancelled,
			types.CommandInvocationStatusTimedOut,
			types.CommandInvocationStatusFailed,
			types.CommandInvocationStatusCancelling:
			if opts.Progress != nil {
				fmt.Fprint(opts.Progress, "\n")
			}
			res := Result{
				CommandID:     commandID,
				Stdout:        stdout,
				Stderr:        stderr,
				Status:        status,
				Steps:         steps,
				Duration:      time.Since(start),
				ExitCode:      inv.ResponseCode,
				StatusDetails: aws.ToString(inv.StatusDetails),
			}
			msg := stderr
			if msg == "" {
				msg = aws.ToString(inv.StatusDetails)
			}
			return res, fmt.Errorf("ssm command %s %s: %s", commandID, status, strings.TrimSpace(msg))
		}

		select {
		case <-ctx.Done():
			return Result{}, ctx.Err()
		case <-time.After(opts.PollEvery):
		}
	}
}

// ParseSteps extracts pldi:step markers from remote stdout.
func ParseSteps(stdout string) []string {
	var steps []string
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, stepPrefix) {
			steps = append(steps, strings.TrimSpace(strings.TrimPrefix(line, stepPrefix)))
		}
	}
	return steps
}
