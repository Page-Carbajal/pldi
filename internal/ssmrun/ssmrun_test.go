package ssmrun

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

type fakeSSM struct {
	ping        types.PingStatus
	infoEmpty   bool
	sendID      string
	invocations []*ssm.GetCommandInvocationOutput
	invErrs     []error
	getCalls    int
}

func (f *fakeSSM) DescribeInstanceInformation(context.Context, *ssm.DescribeInstanceInformationInput, ...func(*ssm.Options)) (*ssm.DescribeInstanceInformationOutput, error) {
	if f.infoEmpty {
		return &ssm.DescribeInstanceInformationOutput{}, nil
	}
	status := f.ping
	if status == "" {
		status = types.PingStatusOnline
	}
	return &ssm.DescribeInstanceInformationOutput{
		InstanceInformationList: []types.InstanceInformation{{
			InstanceId: aws.String("i-1"),
			PingStatus: status,
		}},
	}, nil
}

func (f *fakeSSM) SendCommand(context.Context, *ssm.SendCommandInput, ...func(*ssm.Options)) (*ssm.SendCommandOutput, error) {
	id := f.sendID
	if id == "" {
		id = "cmd-1"
	}
	return &ssm.SendCommandOutput{
		Command: &types.Command{CommandId: aws.String(id)},
	}, nil
}

func (f *fakeSSM) GetCommandInvocation(context.Context, *ssm.GetCommandInvocationInput, ...func(*ssm.Options)) (*ssm.GetCommandInvocationOutput, error) {
	i := f.getCalls
	f.getCalls++
	if i < len(f.invErrs) && f.invErrs[i] != nil {
		return nil, f.invErrs[i]
	}
	if i >= len(f.invocations) {
		return f.invocations[len(f.invocations)-1], nil
	}
	return f.invocations[i], nil
}

func TestEnsureOnline(t *testing.T) {
	if err := EnsureOnline(context.Background(), &fakeSSM{ping: types.PingStatusOnline}, "i-1"); err != nil {
		t.Fatal(err)
	}
	err := EnsureOnline(context.Background(), &fakeSSM{ping: types.PingStatusConnectionLost}, "i-1")
	if err == nil || !strings.Contains(err.Error(), "PingStatus") {
		t.Fatalf("err=%v", err)
	}
	err = EnsureOnline(context.Background(), &fakeSSM{infoEmpty: true}, "i-1")
	if err == nil || !strings.Contains(err.Error(), "not SSM-managed") {
		t.Fatalf("err=%v", err)
	}
}

func TestSendAndWaitSuccessAndSteps(t *testing.T) {
	api := &fakeSSM{
		invocations: []*ssm.GetCommandInvocationOutput{
			{
				Status:                types.CommandInvocationStatusInProgress,
				StandardOutputContent: aws.String("pldi:step dump\n"),
				StandardErrorContent:  aws.String(""),
			},
			{
				Status:                types.CommandInvocationStatusSuccess,
				StandardOutputContent: aws.String("pldi:step dump\npldi:step zip\npldi:step upload\n"),
				ResponseCode:          0,
			},
		},
	}
	res, err := SendAndWait(context.Background(), api, "i-1", "echo hi", Options{PollEvery: time.Millisecond, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != string(types.CommandInvocationStatusSuccess) {
		t.Fatalf("%+v", res)
	}
	if len(res.Steps) != 3 || res.Steps[2] != "upload" {
		t.Fatalf("steps=%v", res.Steps)
	}
}

func TestSendAndWaitFailureIncludesStderr(t *testing.T) {
	api := &fakeSSM{
		invocations: []*ssm.GetCommandInvocationOutput{{
			Status:               types.CommandInvocationStatusFailed,
			StandardErrorContent: aws.String("boom remote"),
			StatusDetails:        aws.String("Failed"),
			ResponseCode:         1,
		}},
	}
	_, err := SendAndWait(context.Background(), api, "i-1", "false", Options{PollEvery: time.Millisecond, Timeout: time.Second})
	if err == nil || !strings.Contains(err.Error(), "boom remote") {
		t.Fatalf("err=%v", err)
	}
}

func TestSendAndWaitTimeout(t *testing.T) {
	api := &fakeSSM{
		invocations: []*ssm.GetCommandInvocationOutput{{
			Status: types.CommandInvocationStatusInProgress,
		}},
	}
	_, err := SendAndWait(context.Background(), api, "i-1", "sleep", Options{PollEvery: time.Millisecond, Timeout: 20 * time.Millisecond})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err=%v", err)
	}
}

func TestSendAndWaitInvocationNotReadyThenSuccess(t *testing.T) {
	api := &fakeSSM{
		invErrs: []error{errors.New("InvocationDoesNotExist: not ready"), nil},
		invocations: []*ssm.GetCommandInvocationOutput{
			nil,
			{
				Status:                types.CommandInvocationStatusSuccess,
				StandardOutputContent: aws.String("pldi:step done\n"),
			},
		},
	}
	res, err := SendAndWait(context.Background(), api, "i-1", "echo", Options{PollEvery: time.Millisecond, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Steps) != 1 {
		t.Fatalf("steps=%v", res.Steps)
	}
}

func TestParseSteps(t *testing.T) {
	got := ParseSteps("noise\npldi:step dump\npldi:step zip\n")
	if len(got) != 2 || got[0] != "dump" || got[1] != "zip" {
		t.Fatalf("%v", got)
	}
}
