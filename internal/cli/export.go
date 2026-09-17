package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/JLugagne/sandwarden/internal/app"
	"github.com/spf13/cobra"
)

// exportCmd writes a sandbox config in a foreign declarative format.
func exportCmd(opts *Options) *cobra.Command {
	var sbxenv bool
	var output string
	cmd := &cobra.Command{
		Use:   "export [SANDBOX]",
		Short: "Export a sandbox config to a foreign declarative format",
		Long: "Export writes a sandbox's configuration in a declarative format another tool\n" +
			"reads. With --sbxenv the document is an sbxenv.yaml for `sbx env`, built from\n" +
			"the sandbox's spec.yaml and sandwarden.yaml alone: neither the daemon nor\n" +
			"`sbx env` is needed. Fields sbxenv cannot express are listed on stderr, so\n" +
			"stdout stays a clean YAML document. Name a sandbox to write it to stdout, or\n" +
			"to -o FILE; omit it to write one <slug>.sbxenv.yaml per sandbox into -o DIR.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !sbxenv {
				return errors.New("export: choose a format with --sbxenv")
			}
			core, closeApp, err := openApp(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer closeApp()
			if len(args) == 0 {
				if output == "" {
					return errors.New("export: name a sandbox, or pass -o DIR to export every sandbox")
				}
				return exportAllSbxenv(cmd, core, output)
			}
			result, err := core.ExportSbxenv(args[0])
			if err != nil {
				return err
			}
			sbxenvReport(cmd, result)
			if output == "" {
				_, err = cmd.OutOrStdout().Write(result.Document)
				return err
			}
			if err := os.WriteFile(output, result.Document, 0o644); err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "wrote "+output)
			return err
		},
	}
	cmd.Flags().BoolVar(&sbxenv, "sbxenv", false, "write an sbxenv.yaml document")
	cmd.Flags().StringVarP(&output, "output", "o", "", "write to FILE, or to DIR when every sandbox is exported")
	return cmd
}

// exportAllSbxenv writes one <slug>.sbxenv.yaml per sandbox into dir.
func exportAllSbxenv(cmd *cobra.Command, core *app.App, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	failed := 0
	for _, s := range core.Fleet.Sandboxes() {
		result, err := core.ExportSbxenv(s.Slug)
		if err != nil {
			failed++
			fmt.Fprintf(cmd.ErrOrStderr(), "export: %s: %v\n", s.Slug, err)
			continue
		}
		path := filepath.Join(dir, s.Slug+".sbxenv.yaml")
		if err := os.WriteFile(path, result.Document, 0o644); err != nil {
			failed++
			fmt.Fprintf(cmd.ErrOrStderr(), "export: %s: %v\n", s.Slug, err)
			continue
		}
		sbxenvReport(cmd, result)
		fmt.Fprintln(cmd.OutOrStdout(), "wrote "+path)
	}
	if failed > 0 {
		return fmt.Errorf("%d sandbox(es) could not be exported", failed)
	}
	return nil
}

// sbxenvReport prints the fields sbxenv cannot express on stderr.
func sbxenvReport(cmd *cobra.Command, result app.SbxenvExport) {
	for _, line := range result.Unsupported {
		fmt.Fprintf(cmd.ErrOrStderr(), "export: %s: %s\n", result.Slug, line)
	}
}
