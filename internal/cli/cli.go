// Package cli implements the headless sandwarden commands: the same binary
// runs the desktop GUI without a subcommand and a sbx-like lifecycle with one.
package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JLugagne/sandwarden/internal/app"
	"github.com/JLugagne/sandwarden/internal/fleet"
	"github.com/JLugagne/sandwarden/internal/sbx"
	"github.com/JLugagne/sandwarden/internal/store"
	"github.com/spf13/cobra"
)

// Options are the shared paths of every command.
type Options struct {
	Socket    string
	DB        string
	ConfigDir string
	Version   string
}

// New builds the command tree. Root launches runGUI when no subcommand is
// given.
func New(opts *Options, runGUI func(*Options) error) *cobra.Command {
	root := &cobra.Command{
		Use:          "sandwarden",
		Short:        "Manage Docker Sandboxes declaratively",
		Long:         "sandwarden drives the sandboxd daemon and the sbx CLI from a directory of kit files.\nRun without a command to open the desktop application.",
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		RunE:         func(cmd *cobra.Command, args []string) error { return runGUI(opts) },
	}
	root.PersistentFlags().StringVar(&opts.Socket, "socket", "", "sandboxd unix socket path")
	root.PersistentFlags().StringVar(&opts.DB, "db", "", "path to the SQLite index (default: XDG state dir)")
	root.PersistentFlags().StringVar(&opts.ConfigDir, "config", "", "config directory (default: $SANDWARDEN_CONFIG_DIR or XDG)")
	root.AddCommand(
		lsCmd(opts),
		startCmd(opts),
		stopCmd(opts),
		restartCmd(opts),
		rmCmd(opts),
		applyCmd(opts),
		createCmd(opts),
		runCmd(opts),
		recreateCmd(opts),
		statusCmd(opts),
		validateCmd(opts),
		doctorCmd(opts),
		watchCmd(opts),
		sbxCmd(opts),
		importCmd(opts),
		exportCmd(opts),
		kitCmd(opts),
		setupCmd(opts),
	)
	return root
}

// DefaultDBPath resolves the SQLite index path: $XDG_STATE_HOME or
// ~/.local/state.
func DefaultDBPath() string {
	if root := strings.TrimSpace(os.Getenv("XDG_STATE_HOME")); root != "" {
		return filepath.Join(root, "sandwarden", "sandwarden.db")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "sandwarden", "sandwarden.db")
	}
	return filepath.Join(home, ".local", "state", "sandwarden", "sandwarden.db")
}

// openApp wires the headless services. It is a variable so tests can wrap it.
var openApp = func(ctx context.Context, opts *Options) (*app.App, func(), error) {
	dbPath := strings.TrimSpace(opts.DB)
	if dbPath == "" {
		dbPath = DefaultDBPath()
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, nil, err
	}
	st, err := store.Open(dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open index: %w", err)
	}
	fl, err := fleet.Open(opts.ConfigDir)
	if err != nil {
		_ = st.Close()
		return nil, nil, fmt.Errorf("open config: %w", err)
	}
	core := app.New(sbx.New(opts.Socket), st, fl)
	return core, func() { _ = st.Close() }, nil
}

// configSlug finds the directory slug of a sandbox by daemon name.
func configSlug(core *app.App, name string) (string, bool) {
	cfg, ok := core.Fleet.SandboxByName(name)
	if !ok {
		return "", false
	}
	return cfg.Slug, true
}

// parseSkillRef reads a store:kind:name skill reference.
func parseSkillRef(raw string) (fleet.SkillRef, error) {
	parts := strings.Split(strings.TrimSpace(raw), ":")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return fleet.SkillRef{}, fmt.Errorf("invalid skill reference %q, expected store:kind:name", raw)
	}
	return fleet.SkillRef{Store: parts[0], Kind: parts[1], Name: parts[2]}, nil
}
