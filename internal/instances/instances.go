package instances

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/punchlistdigital/pldi/internal/awsx"
)

// Instance is a summarized EC2 instance.
type Instance struct {
	Name       string
	InstanceID string
	State      string
}

// ListOptions controls listing.
type ListOptions struct {
	AllStates bool
}

// List returns instances, optionally restricted to running, sorted by name.
func List(ctx context.Context, api awsx.EC2API, opts ListOptions) ([]Instance, error) {
	input := &ec2.DescribeInstancesInput{}
	if !opts.AllStates {
		input.Filters = []types.Filter{{
			Name:   aws.String("instance-state-name"),
			Values: []string{string(types.InstanceStateNameRunning)},
		}}
	}

	var out []Instance
	paginator := ec2.NewDescribeInstancesPaginator(api, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, res := range page.Reservations {
			for _, inst := range res.Instances {
				out = append(out, summarize(inst))
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// Resolve finds an instance by ID or exact case-sensitive Name tag.
func Resolve(ctx context.Context, api awsx.EC2API, idOrName string) (Instance, error) {
	if strings.HasPrefix(idOrName, "i-") {
		out, err := api.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
			InstanceIds: []string{idOrName},
		})
		if err != nil {
			return Instance{}, err
		}
		var found []Instance
		for _, res := range out.Reservations {
			for _, inst := range res.Instances {
				found = append(found, summarize(inst))
			}
		}
		if len(found) == 0 {
			return Instance{}, fmt.Errorf("no instance with id %q", idOrName)
		}
		return found[0], nil
	}

	all, err := List(ctx, api, ListOptions{AllStates: true})
	if err != nil {
		return Instance{}, err
	}
	var matches []Instance
	for _, inst := range all {
		if inst.Name == idOrName {
			matches = append(matches, inst)
		}
	}
	switch len(matches) {
	case 0:
		return Instance{}, fmt.Errorf("no instance with Name tag %q", idOrName)
	case 1:
		return matches[0], nil
	default:
		var b strings.Builder
		fmt.Fprintf(&b, "multiple instances with Name tag %q:", idOrName)
		for _, m := range matches {
			fmt.Fprintf(&b, "\n  %s  %s", m.Name, m.InstanceID)
		}
		return Instance{}, fmt.Errorf("%s", b.String())
	}
}

func summarize(inst types.Instance) Instance {
	name := "-"
	for _, tag := range inst.Tags {
		if aws.ToString(tag.Key) == "Name" {
			if v := aws.ToString(tag.Value); v != "" {
				name = v
			}
			break
		}
	}
	state := ""
	if inst.State != nil {
		state = string(inst.State.Name)
	}
	return Instance{
		Name:       name,
		InstanceID: aws.ToString(inst.InstanceId),
		State:      state,
	}
}
