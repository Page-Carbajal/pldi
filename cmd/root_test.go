package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/punchlistdigital/pldi/internal/config"
)

type stubSTS struct {
	out *sts.GetCallerIdentityOutput
	err error
}

func (s *stubSTS) GetCallerIdentity(context.Context, *sts.GetCallerIdentityInput, ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	return s.out, s.err
}

type stubEC2 struct {
	insts []types.Instance
}

func (s *stubEC2) DescribeInstances(_ context.Context, params *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	runningOnly := false
	for _, f := range params.Filters {
		if aws.ToString(f.Name) == "instance-state-name" {
			runningOnly = true
		}
	}
	var filtered []types.Instance
	for _, inst := range s.insts {
		if runningOnly && (inst.State == nil || inst.State.Name != types.InstanceStateNameRunning) {
			continue
		}
		filtered = append(filtered, inst)
	}
	return &ec2.DescribeInstancesOutput{
		Reservations: []types.Reservation{{Instances: filtered}},
	}, nil
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
}

func testApp(t *testing.T) *App {
	t.Helper()
	chdir(t, t.TempDir())
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_DEFAULT_REGION", "")
	t.Setenv("PLDI_BUCKET", "test-bucket")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	return &App{
		Cfg:         cfg,
		skipAWSLoad: true,
		STS: &stubSTS{out: &sts.GetCallerIdentityOutput{
			Account: aws.String("123"),
			Arn:     aws.String("arn:aws:iam::123:user/x"),
			UserId:  aws.String("AIDAX"),
		}},
		EC2: &stubEC2{insts: []types.Instance{
			{
				InstanceId: aws.String("i-aaa"),
				State:      &types.InstanceState{Name: types.InstanceStateNameRunning},
				Tags:       []types.Tag{{Key: aws.String("Name"), Value: aws.String("alpha")}},
			},
			{
				InstanceId: aws.String("i-bbb"),
				State:      &types.InstanceState{Name: types.InstanceStateNameRunning},
			},
		}},
	}
}

func TestStatusOKJSON(t *testing.T) {
	app := testApp(t)
	root := NewRootCmd(app)
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"status", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var payload statusJSON
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.OK {
		t.Fatalf("payload=%+v", payload)
	}
	if payload.Identity == nil || payload.Identity.Account != "123" {
		t.Fatalf("identity=%+v", payload.Identity)
	}
}

func TestStatusFailsWithoutRegion(t *testing.T) {
	app := testApp(t)
	app.Cfg.Settings.Region = ""
	root := NewRootCmd(app)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"status"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestStatusSTSFailure(t *testing.T) {
	app := testApp(t)
	app.STS = &stubSTS{err: errors.New("boom")}
	root := NewRootCmd(app)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"status"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected error")
	}
}

func TestInstancesListTableAndJSON(t *testing.T) {
	app := testApp(t)
	root := NewRootCmd(app)
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"ec2:instances", "list"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Name") || !strings.Contains(out, "i-aaa") {
		t.Fatalf("out=%q", out)
	}
	// missing Name tag renders as '-'
	if !strings.Contains(out, "-") {
		t.Fatalf("expected '-' for missing name: %q", out)
	}

	buf.Reset()
	root = NewRootCmd(app)
	root.SetOut(buf)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"ec2:instances", "list", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		Name       string `json:"name"`
		InstanceID string `json:"instanceId"`
	}
	if err := json.Unmarshal(buf.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows=%v", rows)
	}
}

func TestSilenceUsageOnRuntimeError(t *testing.T) {
	app := testApp(t)
	app.STS = &stubSTS{err: errors.New("nope")}
	root := NewRootCmd(app)
	errBuf := &bytes.Buffer{}
	root.SetOut(&bytes.Buffer{})
	root.SetErr(errBuf)
	root.SetArgs([]string{"status"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	// Cobra should not print usage when SilenceUsage is set; Execute returns the error to main.
	if strings.Contains(errBuf.String(), "Usage:") {
		t.Fatalf("usage printed: %q", errBuf.String())
	}
}

func TestUnknownFlag(t *testing.T) {
	app := testApp(t)
	root := NewRootCmd(app)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"status", "--nope"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected error")
	}
}
