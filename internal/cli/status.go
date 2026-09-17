package cli

import (
	"fmt"

	"github.com/JLugagne/sandwarden/internal/app"
	"github.com/spf13/cobra"
)

func statusCmd(opts *Options) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "status [SANDBOX]",
		Short: "Show the file state of sandboxes",
		RunE: func(cmd *cobra.Command, args []string) error {
			core, closeApp, err := openApp(cmd.Context(), opts)
			if err != nil {
				if jsonOut {
					_ = writeJSON(cmd.OutOrStdout(), jsonStatusOutput{SchemaVersion: jsonSchemaVersion, ConfigErrors: []jsonConfigError{}, Sandboxes: []jsonStatusSandbox{}})
				}
				return err
			}
			defer closeApp()
			if jsonOut {
				return statusJSON(cmd, core, args)
			}
			if len(core.Fleet.Errors()) > 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "config errors:")
				for _, fileErr := range core.Fleet.Errors() {
					fmt.Fprintf(cmd.OutOrStdout(), "  %v\n", fileErr)
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "config directory: %s\n", core.FleetDir())
			names := args
			if len(names) == 0 {
				for _, cfg := range core.Fleet.Sandboxes() {
					names = append(names, cfg.Name())
				}
			}
			for _, name := range names {
				detail, err := core.SandboxDetail(cmd.Context(), name)
				if err != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "%s: not in daemon (%v)\n", name, err)
					continue
				}
				configState := "complete"
				if detail.Incomplete {
					configState = "incomplete"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s: %s, profiles=%v, caches=%d, skills=%d, mounts=%d, config=%s\n",
					name, detail.Sandbox.Status, detail.Sandbox.Profiles, len(detail.Caches), len(detail.Skills), len(detail.DirectMounts), configState)
				if detail.Incomplete {
					fmt.Fprintln(cmd.OutOrStdout(), "  incomplete config: CPU, memory and env were not recoverable; recreate will not restore them")
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print machine-readable JSON")
	return cmd
}

// statusJSON prints the status envelope and fails when a name is not in the daemon.
func statusJSON(cmd *cobra.Command, core *app.App, args []string) error {
	out := jsonStatusOutput{
		SchemaVersion: jsonSchemaVersion,
		ConfigDir:     core.FleetDir(),
		ConfigErrors:  jsonConfigErrors(core),
		Sandboxes:     []jsonStatusSandbox{},
	}
	names := args
	if len(names) == 0 {
		for _, cfg := range core.Fleet.Sandboxes() {
			names = append(names, cfg.Name())
		}
	}
	failed := 0
	for _, name := range names {
		cfg, _ := core.Fleet.SandboxByName(name)
		entry := jsonStatusSandbox{jsonSandbox: jsonSandboxFromConfig(name, cfg), Errors: []string{}}
		detail, err := core.SandboxDetail(cmd.Context(), name)
		if err != nil {
			failed++
			entry.Errors = append(entry.Errors, fmt.Sprintf("not in daemon: %v", err))
			out.Sandboxes = append(out.Sandboxes, entry)
			continue
		}
		entry.ID = detail.Sandbox.ID
		entry.Status = detail.Sandbox.Status
		entry.Running = detail.Sandbox.Running
		entry.Detail = &detail
		out.Sandboxes = append(out.Sandboxes, entry)
	}
	if err := writeJSON(cmd.OutOrStdout(), out); err != nil {
		return err
	}
	if failed > 0 {
		return fmt.Errorf("%d sandbox(es) not found in daemon", failed)
	}
	return nil
}
