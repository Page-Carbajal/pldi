package cmd

import (
	"github.com/spf13/cobra"

	"github.com/punchlistdigital/pldi/internal/instances"
)

// instanceArgCompletion completes the first positional argument with running
// instance IDs and Name tags.
func instanceArgCompletion(app *App) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
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
	}
}
