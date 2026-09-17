package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/JLugagne/sandwarden/internal/app"
	"github.com/JLugagne/sandwarden/internal/fleet"
	"github.com/spf13/cobra"
)

func doctorCmd(opts *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Scan sandbox specs for likely secret values written in clear",
		Long: "doctor checks every sandbox spec.yaml environment block against the same\n" +
			"heuristic the create path enforces. Findings report the variable name, file\n" +
			"and line only: values are never printed. It exits non-zero when findings exist.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			core, closeApp, err := openApp(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer closeApp()

			out := cmd.OutOrStdout()
			sandboxes := core.Fleet.Sandboxes()
			findings := 0
			for _, s := range sandboxes {
				lines, err := secretEnvFindings(s)
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "doctor: %s: %v\n", s.Slug, err)
					continue
				}
				for _, line := range lines {
					findings++
					fmt.Fprintln(out, line)
				}
			}
			if findings == 0 {
				fmt.Fprintf(out, "doctor: %d sandbox spec(s) scanned, no likely secrets found\n", len(sandboxes))
				return nil
			}
			fmt.Fprintf(out, "doctor: %d environment value(s) look like secrets; rotate them, store them with `sbx secret set` (or from the Secrets page), then keep only the bare KEY form in spec.yaml\n", findings)
			return fmt.Errorf("doctor: %d environment value(s) look like secrets", findings)
		},
	}
}

// secretEnvFindings lists the secret-looking environment entries of one
// sandbox spec. Only the variable name, file and line are reported.
func secretEnvFindings(s *fleet.Sandbox) ([]string, error) {
	env := s.Spec.Env()
	if len(env) == 0 {
		return nil, nil
	}
	path := filepath.Join(s.Dir, "spec.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var out []string
	for _, key := range keys {
		reason := app.SecretReason(env[key])
		if reason == "" {
			continue
		}
		where := filepath.Base(path)
		if line := keyLine(string(data), key); line > 0 {
			where = fmt.Sprintf("%s:%d", where, line)
		}
		out = append(out, fmt.Sprintf("sandbox %s: %s: %s looks like a secret (%s)", s.Name(), where, key, reason))
	}
	return out, nil
}

// keyLine returns the 1-based line of the YAML mapping entry for key, or 0
// when the key is not found.
func keyLine(data, key string) int {
	pattern := regexp.MustCompile(`^\s*` + regexp.QuoteMeta(key) + `\s*:`)
	for i, line := range strings.Split(data, "\n") {
		if pattern.MatchString(line) {
			return i + 1
		}
	}
	return 0
}
