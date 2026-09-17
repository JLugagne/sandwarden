package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

func validateCmd(opts *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "validate SANDBOX",
		Short: "Run sbx kit validate over a sandbox config directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			core, closeApp, err := openApp(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer closeApp()
			slug := args[0]
			if cfg, ok := core.Fleet.SandboxByName(slug); ok {
				slug = cfg.Slug
			}
			result, err := core.ValidateSandbox(cmd.Context(), slug)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), result.Output)
			if !result.OK {
				return errors.New("kit validation failed")
			}
			return nil
		},
	}
}
