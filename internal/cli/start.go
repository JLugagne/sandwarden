package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func startCmd(opts *Options) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "start SANDBOX...",
		Short: "Start sandboxes and apply their files",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			core, closeApp, err := openApp(cmd.Context(), opts)
			if err != nil {
				if jsonOut {
					_ = writeJSON(cmd.OutOrStdout(), jsonStartOutput{SchemaVersion: jsonSchemaVersion, Sandboxes: []jsonStartResult{}})
				}
				return err
			}
			defer closeApp()
			out := jsonStartOutput{SchemaVersion: jsonSchemaVersion, Sandboxes: make([]jsonStartResult, 0, len(args))}
			for _, name := range args {
				report, err := core.StartAndConverge(cmd.Context(), name)
				if err != nil {
					if jsonOut {
						out.Sandboxes = append(out.Sandboxes, jsonStartResult{Name: name, Error: err.Error()})
						if writeErr := writeJSON(cmd.OutOrStdout(), out); writeErr != nil {
							return writeErr
						}
					}
					return fmt.Errorf("%s: %w", name, err)
				}
				if jsonOut {
					out.Sandboxes = append(out.Sandboxes, jsonStartResult{Name: name, Started: true, Report: jsonApplyResultOf(report)})
					continue
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s: started\n", name)
				report.Print(cmd.OutOrStdout())
			}
			if jsonOut {
				return writeJSON(cmd.OutOrStdout(), out)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print machine-readable JSON")
	return cmd
}
