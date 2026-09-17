package cli

import (
	"fmt"

	"github.com/JLugagne/sandwarden/internal/app"
	"github.com/spf13/cobra"
)

func runCmd(opts *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "run SANDBOX [-- ARGS...]",
		Short: "Create the sandbox from its files if needed, start, apply and attach",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			name := args[0]
			core, closeApp, err := openApp(ctx, opts)
			if err != nil {
				return err
			}
			defer closeApp()
			if _, err := core.Sbx.InspectSandbox(ctx, name); err != nil {
				cfg, ok := core.Fleet.SandboxByName(name)
				if !ok {
					return fmt.Errorf("sandbox %q does not exist and has no config directory", name)
				}
				create := app.CreateRequest{
					Opts:         core.CreateOptionsFromConfig(cfg),
					Profiles:     cfg.App.Profiles,
					Caches:       cfg.App.Caches,
					Skills:       cfg.App.Skills,
					Mounts:       cfg.App.Mounts,
					RunArgs:      cfg.App.RunArgs,
					AttachCaches: true,
				}
				create.Opts.Name = name
				if err := core.CreateSandbox(ctx, create, cmd.OutOrStdout()); err != nil {
					return err
				}
			} else {
				report, err := core.StartAndConverge(ctx, name)
				if err != nil {
					return err
				}
				report.Print(cmd.OutOrStdout())
			}
			return core.Sbx.RunSandbox(ctx, name, args[1:])
		},
	}
}
