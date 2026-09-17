package cli

import (
	"github.com/JLugagne/sandwarden/internal/sbx"
	"github.com/spf13/cobra"
)

func sbxCmd(opts *Options) *cobra.Command {
	return &cobra.Command{
		Use:                "sbx ARGS...",
		Short:              "Run the sbx binary unchanged",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := sbx.New(opts.Socket)
			return client.RunCLI(cmd.Context(), args)
		},
	}
}
