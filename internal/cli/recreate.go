package cli

import "github.com/spf13/cobra"

func recreateCmd(opts *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "recreate SANDBOX",
		Short: "Delete and recreate a sandbox from its files",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			core, closeApp, err := openApp(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer closeApp()
			return core.RecreateSandbox(cmd.Context(), args[0], cmd.OutOrStdout())
		},
	}
}
