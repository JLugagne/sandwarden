package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func lsCmd(opts *Options) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List sandboxes and their config state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			core, closeApp, err := openApp(cmd.Context(), opts)
			if err != nil {
				if jsonOut {
					_ = writeJSON(cmd.OutOrStdout(), jsonLsOutput{SchemaVersion: jsonSchemaVersion, Sandboxes: []jsonSandbox{}, ConfigErrors: []jsonConfigError{}})
				}
				return err
			}
			defer closeApp()
			summaries, err := core.SandboxSummaries(cmd.Context())
			if err != nil {
				if jsonOut {
					out := jsonLsOutput{SchemaVersion: jsonSchemaVersion, Sandboxes: []jsonSandbox{}, ConfigErrors: jsonConfigErrors(core)}
					if writeErr := writeJSON(cmd.OutOrStdout(), out); writeErr != nil {
						return writeErr
					}
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: %v\n", err)
				}
				return err
			}
			if jsonOut {
				out := jsonLsOutput{SchemaVersion: jsonSchemaVersion, Sandboxes: make([]jsonSandbox, 0, len(summaries)), ConfigErrors: jsonConfigErrors(core)}
				for _, s := range summaries {
					cfg, _ := core.Fleet.SandboxByName(s.Name)
					entry := jsonSandboxFromConfig(s.Name, cfg)
					entry.ID = s.ID
					entry.Status = s.Status
					entry.Running = s.Running
					out.Sandboxes = append(out.Sandboxes, entry)
				}
				return writeJSON(cmd.OutOrStdout(), out)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-28s %-10s %-24s %s\n", "SANDBOX", "STATUS", "PROFILES", "CONFIG")
			for _, s := range summaries {
				configDir := "-"
				if slug, ok := configSlug(core, s.Name); ok {
					configDir = slug
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-28s %-10s %-24s %s\n", s.Name, s.Status, strings.Join(s.Profiles, ","), configDir)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print machine-readable JSON")
	return cmd
}
