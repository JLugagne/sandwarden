package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// kitCmd groups the mixin-kit operations.
func kitCmd(opts *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "kit",
		Short: "Manage the mixin kits attached to a sandbox",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(kitAddCmd(opts))
	return cmd
}

func kitAddCmd(opts *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "add SANDBOX REF",
		Short: "Attach a mixin kit to an existing sandbox",
		Long: "Attach a mixin kit to an existing sandbox without recreating it from scratch.\n" +
			"sbx recreates the container with the reference appended to the sandbox's kit list,\n" +
			"preserving kit-owned volumes (agent session state) and --clone workspaces. The\n" +
			"reference is recorded in the sandbox's create.kits, so a later recreate keeps it,\n" +
			"and the sidecar (rules, mounts, caches, skills) is re-applied afterwards.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			core, closeApp, err := openApp(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer closeApp()
			result, err := core.AttachKit(cmd.Context(), args[0], args[1], cmd.OutOrStdout())
			if err != nil {
				return err
			}
			result.Print(cmd.OutOrStdout())
			if len(result.Report.Errors) > 0 {
				return fmt.Errorf("%d error(s) during apply", len(result.Report.Errors))
			}
			return nil
		},
	}
}
