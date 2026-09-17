package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

func applyCmd(opts *Options) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "apply [SANDBOX...]",
		Short: "Apply the files to running sandboxes",
		RunE: func(cmd *cobra.Command, args []string) error {
			core, closeApp, err := openApp(cmd.Context(), opts)
			if err != nil {
				if jsonOut {
					_ = writeJSON(cmd.OutOrStdout(), jsonApplyOutput{SchemaVersion: jsonSchemaVersion, Reports: []jsonApplyReport{}})
				}
				return err
			}
			defer closeApp()
			names := args
			if len(names) == 0 {
				for _, cfg := range core.Fleet.Sandboxes() {
					names = append(names, cfg.Name())
				}
			}
			if len(names) == 0 {
				if jsonOut {
					_ = writeJSON(cmd.OutOrStdout(), jsonApplyOutput{SchemaVersion: jsonSchemaVersion, Reports: []jsonApplyReport{}})
				}
				return errors.New("no sandbox configuration found")
			}
			out := jsonApplyOutput{SchemaVersion: jsonSchemaVersion, Reports: make([]jsonApplyReport, 0, len(names))}
			failed := 0
			for _, name := range names {
				report, err := core.Apply(cmd.Context(), name)
				if err != nil {
					failed++
					if jsonOut {
						out.Reports = append(out.Reports, jsonApplyReport{Name: name, Error: err.Error()})
						continue
					}
					fmt.Fprintf(cmd.ErrOrStderr(), "%s: %v\n", name, err)
					continue
				}
				if jsonOut {
					out.Reports = append(out.Reports, jsonApplyReport{Name: name, Report: jsonApplyResultOf(report)})
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "%s:\n", name)
					report.Print(cmd.OutOrStdout())
				}
				if len(report.Errors) > 0 {
					failed++
				}
			}
			if jsonOut {
				if err := writeJSON(cmd.OutOrStdout(), out); err != nil {
					return err
				}
			}
			if failed > 0 {
				return fmt.Errorf("%d sandbox(es) reported errors", failed)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print machine-readable JSON")
	return cmd
}
