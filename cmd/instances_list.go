package cmd

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/punchlistdigital/pldi/internal/instances"
)

func newEC2InstancesCmd(app *App) *cobra.Command {
	group := &cobra.Command{
		Use:   "ec2:instances",
		Short: "EC2 instance helpers",
	}
	group.AddCommand(newInstancesListCmd(app))
	return group
}

func newInstancesListCmd(app *App) *cobra.Command {
	var allStates, asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List EC2 instances",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			list, err := instances.List(cmd.Context(), app.EC2, instances.ListOptions{AllStates: allStates})
			if err != nil {
				return err
			}
			if asJSON {
				type row struct {
					Name       string `json:"name"`
					InstanceID string `json:"instanceId"`
				}
				rows := make([]row, 0, len(list))
				for _, inst := range list {
					rows = append(rows, row{Name: inst.Name, InstanceID: inst.InstanceID})
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(rows)
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "Name\tInstance ID")
			for _, inst := range list {
				fmt.Fprintf(w, "%s\t%s\n", inst.Name, inst.InstanceID)
			}
			return w.Flush()
		},
	}
	cmd.Flags().BoolVar(&allStates, "all", false, "include instances in every state")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}
