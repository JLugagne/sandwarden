package cli

import (
	"strings"

	"github.com/JLugagne/sandwarden/internal/app"
	"github.com/JLugagne/sandwarden/internal/fleet"
	"github.com/JLugagne/sandwarden/internal/sbx"
	"github.com/spf13/cobra"
)

func createCmd(opts *Options) *cobra.Command {
	var (
		agent       string
		name        string
		cpus        int
		memory      string
		daemonProf  string
		template    string
		clone       bool
		attach      bool
		runArgs     string
		workspaces  []string
		kits        []string
		kitArgs     []string
		kitArgFiles []string
		publish     []string
		envs        []string
		envFiles    []string
		skillsMode  string
		staticMCP   []string
		deny        []string
		profiles    []string
		caches      []string
		skills      []string
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a sandbox, write its config directory and apply it",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			for i := range workspaces {
				workspaces[i] = fleet.ExpandHome(workspaces[i])
			}
			for i := range kitArgFiles {
				kitArgFiles[i] = fleet.ExpandHome(kitArgFiles[i])
			}
			for i := range envFiles {
				envFiles[i] = fleet.ExpandHome(envFiles[i])
			}

			core, closeApp, err := openApp(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer closeApp()
			create := app.CreateRequest{
				Opts: sbx.CreateOptions{
					Agent:        strings.TrimSpace(agent),
					Name:         strings.TrimSpace(name),
					CPUs:         cpus,
					Memory:       strings.TrimSpace(memory),
					Profile:      strings.TrimSpace(daemonProf),
					Template:     strings.TrimSpace(template),
					Kits:         kits,
					KitArgs:      kitArgs,
					KitArgsFiles: kitArgFiles,
					Publish:      publish,
					Env:          envs,
					EnvFiles:     envFiles,
					Skills:       strings.TrimSpace(skillsMode),
					StaticMCP:    staticMCP,
					DenyNetwork:  deny,
					Clone:        clone,
					Workspaces:   workspaces,
				},
				Profiles:     profiles,
				Caches:       caches,
				RunArgs:      runArgs,
				AttachCaches: attach,
			}
			for _, raw := range skills {
				ref, err := parseSkillRef(raw)
				if err != nil {
					return err
				}
				create.Skills = append(create.Skills, ref)
			}
			return core.CreateSandbox(cmd.Context(), create, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&agent, "agent", "", "built-in agent name or sandbox kit reference")
	cmd.Flags().StringVar(&name, "name", "", "sandbox name")
	cmd.Flags().IntVar(&cpus, "cpus", 0, "number of CPUs (0 = auto)")
	cmd.Flags().StringVar(&memory, "memory", "", "memory limit (e.g. 8g)")
	cmd.Flags().StringVar(&daemonProf, "daemon-profile", "", "sandboxd governance profile")
	cmd.Flags().StringVar(&template, "template", "", "container image template")
	cmd.Flags().BoolVar(&clone, "clone", false, "clone the workspace inside the sandbox")
	cmd.Flags().BoolVar(&attach, "attach-caches", true, "attach auto-attach caches")
	cmd.Flags().StringVar(&runArgs, "run-args", "", "arguments appended after -- on connect")
	cmd.Flags().StringArrayVar(&workspaces, "workspace", nil, "workspace path, optionally :ro (repeatable)")
	cmd.Flags().StringArrayVar(&kits, "kit", nil, "mixin kit reference (repeatable)")
	cmd.Flags().StringArrayVar(&kitArgs, "kit-arg", nil, "value for an argument a kit declares, as name=value for every kit or kit.name=value for one (repeatable)")
	cmd.Flags().StringArrayVar(&kitArgFiles, "kit-args-file", nil, "file of name=value kit arguments, one per line (repeatable)")
	cmd.Flags().StringArrayVar(&publish, "publish", nil, "published port mapping (repeatable)")
	cmd.Flags().StringArrayVar(&envs, "env", nil, "environment variable KEY=VALUE, or bare KEY for the host value (repeatable); never paste secrets here")
	cmd.Flags().StringArrayVar(&envFiles, "env-file", nil, "file of environment variables, keeping secrets out of spec.yaml (repeatable)")
	cmd.Flags().StringVar(&skillsMode, "skills", "", "shared skills store mode: off, readonly or readwrite (empty = sbx default)")
	cmd.Flags().StringArrayVar(&staticMCP, "static-mcp", nil, "fixed MCP server set chosen at creation, comma-separated (repeatable; `none` allowed)")
	cmd.Flags().StringArrayVar(&deny, "deny-network", nil, "network deny pattern (repeatable)")
	cmd.Flags().StringArrayVar(&profiles, "profile", nil, "sandwarden profile slug (repeatable)")
	cmd.Flags().StringArrayVar(&caches, "cache", nil, "cache slug (repeatable)")
	cmd.Flags().StringArrayVar(&skills, "skill", nil, "skill to attach as store:kind:name (repeatable)")
	return cmd
}
