package instances

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type fakeEC2 struct {
	pages [][]types.Instance
	byID  map[string]types.Instance
}

func (f *fakeEC2) DescribeInstances(_ context.Context, params *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	if len(params.InstanceIds) > 0 {
		var insts []types.Instance
		for _, id := range params.InstanceIds {
			if inst, ok := f.byID[id]; ok {
				insts = append(insts, inst)
			}
		}
		return &ec2.DescribeInstancesOutput{
			Reservations: []types.Reservation{{Instances: insts}},
		}, nil
	}

	runningOnly := false
	for _, filter := range params.Filters {
		if aws.ToString(filter.Name) == "instance-state-name" {
			runningOnly = true
		}
	}

	token := aws.ToString(params.NextToken)
	idx := 0
	if token != "" {
		idx = 1
	}
	if idx >= len(f.pages) {
		return &ec2.DescribeInstancesOutput{}, nil
	}
	page := f.pages[idx]
	var filtered []types.Instance
	for _, inst := range page {
		if runningOnly && (inst.State == nil || inst.State.Name != types.InstanceStateNameRunning) {
			continue
		}
		filtered = append(filtered, inst)
	}
	out := &ec2.DescribeInstancesOutput{
		Reservations: []types.Reservation{{Instances: filtered}},
	}
	if idx+1 < len(f.pages) {
		out.NextToken = aws.String("1")
	}
	return out, nil
}

func TestListPaginationSortingAndMissingName(t *testing.T) {
	api := &fakeEC2{
		pages: [][]types.Instance{
			{
				inst("i-2", "zeta", types.InstanceStateNameRunning),
				inst("i-1", "", types.InstanceStateNameRunning),
			},
			{
				inst("i-3", "alpha", types.InstanceStateNameRunning),
				inst("i-4", "stopped", types.InstanceStateNameStopped),
			},
		},
	}
	got, err := List(context.Background(), api, ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("len=%d want 3", len(got))
	}
	if got[0].Name != "-" || got[0].InstanceID != "i-1" {
		t.Fatalf("first=%+v", got[0])
	}
	if got[1].Name != "alpha" {
		t.Fatalf("second=%+v", got[1])
	}
	if got[2].Name != "zeta" {
		t.Fatalf("third=%+v", got[2])
	}

	all, err := List(context.Background(), api, ListOptions{AllStates: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 4 {
		t.Fatalf("all len=%d", len(all))
	}
}

func TestResolve(t *testing.T) {
	api := &fakeEC2{
		byID: map[string]types.Instance{
			"i-1": inst("i-1", "one", types.InstanceStateNameRunning),
		},
		pages: [][]types.Instance{
			{
				inst("i-1", "dup", types.InstanceStateNameRunning),
				inst("i-2", "dup", types.InstanceStateNameRunning),
				inst("i-3", "solo", types.InstanceStateNameRunning),
			},
		},
	}

	got, err := Resolve(context.Background(), api, "i-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.InstanceID != "i-1" {
		t.Fatalf("%+v", got)
	}

	got, err = Resolve(context.Background(), api, "solo")
	if err != nil {
		t.Fatal(err)
	}
	if got.InstanceID != "i-3" {
		t.Fatalf("%+v", got)
	}

	if _, err := Resolve(context.Background(), api, "missing"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := Resolve(context.Background(), api, "dup"); err == nil {
		t.Fatal("expected multi match error")
	}
}

func inst(id, name string, state types.InstanceStateName) types.Instance {
	i := types.Instance{
		InstanceId: aws.String(id),
		State:      &types.InstanceState{Name: state},
	}
	if name != "" {
		i.Tags = []types.Tag{{Key: aws.String("Name"), Value: aws.String(name)}}
	}
	return i
}
