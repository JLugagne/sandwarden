package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/JLugagne/sandwarden/internal/service"
	"github.com/spf13/cobra"
)

// setupCmd installs and inspects the optional per-user watch service.
func setupCmd(opts *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Install or inspect the optional watch autostart service",
		Long: strings.TrimSpace(`
Install sandwarden watch as a per-user service, so sandboxes started outside
the GUI keep being converged after a login. This is strictly opt-in; watch
stays usable standalone. On Linux setup writes a systemd --user unit, on macOS
a launchd agent, and it never needs root.

Pass the same --config, --socket and --db flags you normally use: they are
baked into the installed service command.`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetupInstall(cmd, opts)
		},
	}
	cmd.AddCommand(setupStatusCmd(opts), setupUninstallCmd(opts))
	return cmd
}

func setupStatusCmd(opts *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Report whether the background service is installed and running",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			status, err := service.New(nil).Status(cmd.Context())
			if err != nil {
				return err
			}
			writeServiceStatus(cmd.OutOrStdout(), status)
			return nil
		},
	}
}

func setupUninstallCmd(opts *Options) *cobra.Command {
	return &cobra.Command{
		Use:     "uninstall",
		Aliases: []string{"remove"},
		Short:   "Stop and remove the background service",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := service.New(nil).Uninstall(cmd.Context())
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if !report.Previous {
				fmt.Fprintf(out, "the sandwarden watch service is not installed (%s)\n", report.UnitPath)
				return nil
			}
			fmt.Fprintf(out, "Removed the sandwarden watch service (%s)\n", report.Platform)
			fmt.Fprintf(out, "  unit: %s\n", report.UnitPath)
			for _, command := range report.Commands {
				fmt.Fprintf(out, "  ran %s\n", command)
			}
			for _, warning := range report.Warnings {
				fmt.Fprintf(out, "  warning: %s\n", warning)
			}
			return nil
		},
	}
}

func runSetupInstall(cmd *cobra.Command, opts *Options) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve sandwarden executable: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	spec := service.Spec{Executable: exe, Args: watchArgs(opts)}
	manager := service.New(nil)
	report, err := manager.Install(cmd.Context(), spec)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	verb := "Installed"
	if report.Previous {
		verb = "Reinstalled"
	}
	fmt.Fprintf(out, "%s the sandwarden watch service (%s)\n", verb, report.Platform)
	fmt.Fprintf(out, "  unit: %s\n", report.UnitPath)
	fmt.Fprintf(out, "  exec: %s\n", strings.Join(append([]string{spec.Executable}, spec.Args...), " "))
	for _, command := range report.Commands {
		fmt.Fprintf(out, "  ran %s\n", command)
	}
	for _, warning := range report.Warnings {
		fmt.Fprintf(out, "  warning: %s\n", warning)
	}
	if status, err := manager.Status(cmd.Context()); err == nil {
		writeServiceStatus(out, status)
	}
	return nil
}

// watchArgs is the command baked into the service: `watch` plus the paths the
// user passed to setup, so the service targets the same installation.
func watchArgs(opts *Options) []string {
	args := []string{"watch"}
	for _, flag := range []struct {
		name  string
		value string
	}{
		{"--config", opts.ConfigDir},
		{"--socket", opts.Socket},
		{"--db", opts.DB},
	} {
		if value := strings.TrimSpace(flag.value); value != "" {
			args = append(args, flag.name, value)
		}
	}
	return args
}

func writeServiceStatus(out io.Writer, status service.Status) {
	fmt.Fprintf(out, "sandwarden watch service (%s)\n", status.Platform)
	fmt.Fprintf(out, "  unit:      %s\n", status.UnitPath)
	fmt.Fprintf(out, "  installed: %s\n", yesNo(status.Installed))
	fmt.Fprintf(out, "  enabled:   %s\n", yesNo(status.Enabled))
	fmt.Fprintf(out, "  running:   %s\n", yesNo(status.Running))
	if status.Detail != "" {
		fmt.Fprintf(out, "  detail:    %s\n", status.Detail)
	}
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}
