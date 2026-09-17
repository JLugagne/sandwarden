package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func rmCmd(opts *Options) *cobra.Command {
	var force, purge bool
	cmd := &cobra.Command{
		Use:   "rm SANDBOX...",
		Short: "Remove sandboxes; the config directory is kept unless --purge",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			core, closeApp, err := openApp(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer closeApp()
			for _, name := range args {
				if err := core.DeleteSandbox(cmd.Context(), name, force, purge); err != nil {
					return fmt.Errorf("%s: %w", name, err)
				}
				kept := "config kept"
				if purge {
					kept = "config purged"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s: removed (%s)\n", name, kept)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "force removal past the daemon's checks")
	cmd.Flags().BoolVar(&purge, "purge", false, "also delete the sandbox config directory")
	return cmd
}
