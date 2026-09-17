package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func stopCmd(opts *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "stop SANDBOX...",
		Short: "Stop sandboxes",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			core, closeApp, err := openApp(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer closeApp()
			for _, name := range args {
				if err := core.StopSandbox(cmd.Context(), name); err != nil {
					return fmt.Errorf("%s: %w", name, err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s: stopped\n", name)
			}
			return nil
		},
	}
}
