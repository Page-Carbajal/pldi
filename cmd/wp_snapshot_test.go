package cmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

type snapTestEC2 struct{}

func (snapTestEC2) DescribeInstances(_ context.Context, params *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	inst := types.Instance{
		InstanceId: aws.String("i-abc"),
		State:      &types.InstanceState{Name: types.InstanceStateNameRunning},
		Tags:       []types.Tag{{Key: aws.String("Name"), Value: aws.String("demo")}},
	}
	return &ec2.DescribeInstancesOutput{
		Reservations: []types.Reservation{{Instances: []types.Instance{inst}}},
	}, nil
}

type snapTestSSM struct{ sent bool }

func (s *snapTestSSM) DescribeInstanceInformation(context.Context, *ssm.DescribeInstanceInformationInput, ...func(*ssm.Options)) (*ssm.DescribeInstanceInformationOutput, error) {
	return &ssm.DescribeInstanceInformationOutput{
		InstanceInformationList: []ssmtypes.InstanceInformation{{
			InstanceId: aws.String("i-abc"),
			PingStatus: ssmtypes.PingStatusOnline,
		}},
	}, nil
}
func (s *snapTestSSM) SendCommand(context.Context, *ssm.SendCommandInput, ...func(*ssm.Options)) (*ssm.SendCommandOutput, error) {
	s.sent = true
	return &ssm.SendCommandOutput{Command: &ssmtypes.Command{CommandId: aws.String("c")}}, nil
}
func (s *snapTestSSM) GetCommandInvocation(context.Context, *ssm.GetCommandInvocationInput, ...func(*ssm.Options)) (*ssm.GetCommandInvocationOutput, error) {
	return &ssm.GetCommandInvocationOutput{Status: ssmtypes.CommandInvocationStatusSuccess}, nil
}

type snapTestS3 struct{}

func (snapTestS3) GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	return &s3.GetObjectOutput{Body: io.NopCloser(strings.NewReader("x"))}, nil
}

func TestWPSnapshotMissingArgAndDryRun(t *testing.T) {
	app := testApp(t)
	ssmAPI := &snapTestSSM{}
	app.EC2 = snapTestEC2{}
	app.SSM = ssmAPI
	app.S3 = snapTestS3{}
	app.STS = &stubSTS{out: &sts.GetCallerIdentityOutput{Account: aws.String("1"), Arn: aws.String("a"), UserId: aws.String("u")}}

	root := NewRootCmd(app)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"ec2:wp-snapshot"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected missing arg error")
	}

	dir := t.TempDir()
	root = NewRootCmd(app)
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"ec2:wp-snapshot", "i-abc", "--dry-run", "--out", dir})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if ssmAPI.sent {
		t.Fatal("dry-run sent SSM")
	}
	if !strings.Contains(buf.String(), "dry-run") {
		t.Fatalf("%s", buf.String())
	}
}

func TestWPSnapshotForceOverwrite(t *testing.T) {
	app := testApp(t)
	ssmAPI := &snapTestSSM{}
	app.EC2 = snapTestEC2{}
	app.SSM = ssmAPI
	app.S3 = snapTestS3{}

	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "content.zip"), []byte("old"), 0o644)

	root := NewRootCmd(app)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"ec2:wp-snapshot", "i-abc", "--out", dir})
	if err := root.Execute(); err == nil {
		t.Fatal("expected exists error")
	}

	root = NewRootCmd(app)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"ec2:wp-snapshot", "i-abc", "--out", dir, "--force", "--prefix", "p"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !ssmAPI.sent {
		t.Fatal("expected SSM send")
	}
}
