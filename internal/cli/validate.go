package cli

import (
	"errors"
	"fmt"

	"github.com/JLugagne/sandwarden/internal/fleet"
	"github.com/spf13/cobra"
)

func validateCmd(opts *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "validate SANDBOX",
		Short: "Validate a sandbox config and report sbx compatibility problems",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			core, closeApp, err := openApp(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer closeApp()
			slug := args[0]
			var cfg *fleet.Sandbox
			if found, ok := core.Fleet.SandboxByName(slug); ok {
				cfg = found
				slug = found.Slug
			} else if found, ok := core.Fleet.Sandbox(slug); ok {
				cfg = found
			}
			result, err := core.ValidateSandbox(cmd.Context(), slug)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintln(out, result.Output)
			incompatible := 0
			if cfg != nil {
				for _, message := range core.SandboxCompatibility(cmd.Context(), cfg) {
					incompatible++
					fmt.Fprintf(out, "incompatible: %s\n", message)
				}
			}
			if !result.OK {
				return errors.New("kit validation failed")
			}
			if incompatible > 0 {
				return fmt.Errorf("%d feature(s) need a newer sbx", incompatible)
			}
			return nil
		},
	}
}
