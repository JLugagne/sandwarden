package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func watchCmd(opts *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "watch",
		Short: "Run the convergence loop headlessly until interrupted",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			core, closeApp, err := openApp(ctx, opts)
			if err != nil {
				return err
			}
			defer closeApp()
			if err := core.Reconcile(ctx); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "initial reconcile: %v\n", err)
			}
			core.Start(ctx)
			fmt.Fprintln(cmd.OutOrStdout(), "watching for sandbox lifecycle events; press Ctrl-C to stop")
			<-ctx.Done()
			return nil
		},
	}
}
