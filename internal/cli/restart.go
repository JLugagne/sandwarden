package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func restartCmd(opts *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "restart SANDBOX...",
		Short: "Restart sandboxes and re-apply their files",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			core, closeApp, err := openApp(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer closeApp()
			for _, name := range args {
				_ = core.StopSandbox(cmd.Context(), name)
				report, err := core.StartAndConverge(cmd.Context(), name)
				if err != nil {
					return fmt.Errorf("%s: %w", name, err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s: restarted\n", name)
				report.Print(cmd.OutOrStdout())
			}
			return nil
		},
	}
}
