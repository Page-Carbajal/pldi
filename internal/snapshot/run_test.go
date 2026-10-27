package snapshot

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

type snapEC2 struct{}

func (snapEC2) DescribeInstances(_ context.Context, params *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	inst := types.Instance{
		InstanceId: aws.String("i-123"),
		State:      &types.InstanceState{Name: types.InstanceStateNameRunning},
		Tags:       []types.Tag{{Key: aws.String("Name"), Value: aws.String("My Site")}},
	}
	if len(params.InstanceIds) > 0 && params.InstanceIds[0] != "i-123" {
		return &ec2.DescribeInstancesOutput{}, nil
	}
	return &ec2.DescribeInstancesOutput{
		Reservations: []types.Reservation{{Instances: []types.Instance{inst}}},
	}, nil
}

type snapSSM struct {
	sent bool
}

func (s *snapSSM) DescribeInstanceInformation(context.Context, *ssm.DescribeInstanceInformationInput, ...func(*ssm.Options)) (*ssm.DescribeInstanceInformationOutput, error) {
	return &ssm.DescribeInstanceInformationOutput{
		InstanceInformationList: []ssmtypes.InstanceInformation{{
			InstanceId: aws.String("i-123"),
			PingStatus: ssmtypes.PingStatusOnline,
		}},
	}, nil
}

func (s *snapSSM) SendCommand(context.Context, *ssm.SendCommandInput, ...func(*ssm.Options)) (*ssm.SendCommandOutput, error) {
	s.sent = true
	return &ssm.SendCommandOutput{Command: &ssmtypes.Command{CommandId: aws.String("c-1")}}, nil
}

func (s *snapSSM) GetCommandInvocation(context.Context, *ssm.GetCommandInvocationInput, ...func(*ssm.Options)) (*ssm.GetCommandInvocationOutput, error) {
	return &ssm.GetCommandInvocationOutput{
		Status:                ssmtypes.CommandInvocationStatusSuccess,
		StandardOutputContent: aws.String("pldi:step done\n"),
	}, nil
}

type snapS3 struct {
	gets int
}

func (s *snapS3) GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	s.gets++
	return &s3.GetObjectOutput{Body: io.NopCloser(strings.NewReader("data"))}, nil
}

func TestRunDryRunNoAWSSideEffects(t *testing.T) {
	dir := t.TempDir()
	ssmAPI := &snapSSM{}
	s3api := &snapS3{}
	out := &bytes.Buffer{}
	err := Run(context.Background(), snapEC2{}, ssmAPI, s3api, RunOptions{
		IDOrName: "i-123",
		Bucket:   "b",
		WPRoot:   "/bitnami/wordpress",
		OutDir:   dir,
		DryRun:   true,
		Stdout:   out,
		Now:      func() time.Time { return time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if ssmAPI.sent || s3api.gets != 0 {
		t.Fatal("dry-run called SSM or S3")
	}
	if !strings.Contains(out.String(), "my-site/2026-01-02-150405") {
		t.Fatalf("prefix missing: %s", out.String())
	}
}

func TestRunRefusesExistingFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "content.zip"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Run(context.Background(), snapEC2{}, &snapSSM{}, &snapS3{}, RunOptions{
		IDOrName: "i-123",
		Bucket:   "b",
		WPRoot:   "/bitnami/wordpress",
		OutDir:   dir,
	})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err=%v", err)
	}
}

func TestRunForceAndDownload(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "db.sql"), []byte("old"), 0o644)
	ssmAPI := &snapSSM{}
	s3api := &snapS3{}
	out := &bytes.Buffer{}
	err := Run(context.Background(), snapEC2{}, ssmAPI, s3api, RunOptions{
		IDOrName: "i-123",
		Bucket:   "b",
		Prefix:   "custom/prefix",
		WPRoot:   "/bitnami/wordpress",
		OutDir:   dir,
		Force:    true,
		Timeout:  time.Minute,
		Stdout:   out,
		Progress: io.Discard,
		Now:      time.Now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ssmAPI.sent || s3api.gets != 2 {
		t.Fatalf("sent=%v gets=%d", ssmAPI.sent, s3api.gets)
	}
	if !strings.Contains(out.String(), "custom/prefix") {
		t.Fatalf("%s", out.String())
	}
}

func TestRunSSMFailure(t *testing.T) {
	dir := t.TempDir()
	ssmAPI := &failSSM{}
	err := Run(context.Background(), snapEC2{}, ssmAPI, &snapS3{}, RunOptions{
		IDOrName: "i-123",
		Bucket:   "b",
		WPRoot:   "/bitnami/wordpress",
		OutDir:   dir,
		Timeout:  time.Minute,
		Progress: io.Discard,
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

type failSSM struct{}

func (failSSM) DescribeInstanceInformation(context.Context, *ssm.DescribeInstanceInformationInput, ...func(*ssm.Options)) (*ssm.DescribeInstanceInformationOutput, error) {
	return nil, errors.New("offline")
}

func (failSSM) SendCommand(context.Context, *ssm.SendCommandInput, ...func(*ssm.Options)) (*ssm.SendCommandOutput, error) {
	return nil, errors.New("no")
}

func (failSSM) GetCommandInvocation(context.Context, *ssm.GetCommandInvocationInput, ...func(*ssm.Options)) (*ssm.GetCommandInvocationOutput, error) {
	return nil, errors.New("no")
}
