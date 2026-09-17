package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// importCmd adopts daemon sandboxes that have no config directory: it writes a
// minimal sidecar per sandbox from the daemon state. CPU, memory and
// environment are not recoverable from the daemon, so adopted configs are
// marked incomplete.
func importCmd(opts *Options) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "import [SANDBOX...]",
		Short: "Adopt daemon sandboxes into the config directory",
		Long: "Import writes a config directory for every daemon sandbox that has none,\n" +
			"or only for the named ones. Sandboxes adopted without their original create\n" +
			"parameters are marked incomplete: recreate will not restore CPU, memory or\n" +
			"environment.",
		RunE: func(cmd *cobra.Command, args []string) error {
			core, closeApp, err := openApp(cmd.Context(), opts)
			if err != nil {
				if jsonOut {
					_ = writeJSON(cmd.OutOrStdout(), jsonImportOutput{SchemaVersion: jsonSchemaVersion, Results: []jsonImportResult{}})
				}
				return err
			}
			defer closeApp()
			report, err := core.ImportSandboxes(cmd.Context(), args)
			if err != nil {
				if jsonOut {
					_ = writeJSON(cmd.OutOrStdout(), jsonImportOutput{SchemaVersion: jsonSchemaVersion, Results: []jsonImportResult{}})
				}
				return err
			}
			if jsonOut {
				if err := writeJSON(cmd.OutOrStdout(), jsonImportReport(report)); err != nil {
					return err
				}
			} else {
				report.Print(cmd.OutOrStdout())
			}
			if report.Failed > 0 {
				return fmt.Errorf("%d sandbox(es) could not be imported", report.Failed)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print machine-readable JSON")
	return cmd
}
