package app

import (
	"context"

	"github.com/JLugagne/sandwarden/internal/fleet"
	"github.com/JLugagne/sandwarden/internal/sbx"
)

// SandboxCompatibility lists the create-block features of one sandbox the
// installed sbx cannot run. Messages are empty when the version is unknown, so
// a dev build is never reported as incompatible. The version lookup is cached
// on the sbx client.
func (a *App) SandboxCompatibility(ctx context.Context, s *fleet.Sandbox) []string {
	if a == nil || a.Sbx == nil || s == nil || s.App.Create == nil {
		return nil
	}
	c := s.App.Create
	opts := sbx.CreateOptions{
		Workspaces:   c.Workspaces,
		KitArgs:      c.KitArgs,
		KitArgsFiles: c.KitArgsFiles,
		EnvFiles:     c.EnvFiles,
		Skills:       c.Skills,
		StaticMCP:    c.StaticMCP,
	}
	required := opts.RequiredFeatures()
	if len(required) == 0 {
		return nil
	}
	v, _ := a.Sbx.Version(ctx)
	return sbx.Messages(v, sbx.Incompatible(v, required))
}

// checkCreateFeatures refuses a create whose options need a newer sbx than the
// installed one. It fails before any destructive sbx call; a version lookup
// failure counts as unknown and allows the create.
func (a *App) checkCreateFeatures(ctx context.Context, opts sbx.CreateOptions) error {
	required := opts.RequiredFeatures()
	if len(required) == 0 {
		return nil
	}
	v, _ := a.Sbx.Version(ctx)
	return sbx.FormatIncompatible(v, required)
}

// checkKitAddFeatures refuses `sbx kit add` when its flags need a newer sbx
// than the installed one. A version lookup failure counts as unknown and
// allows the attach.
func (a *App) checkKitAddFeatures(ctx context.Context, kitArgs []string) error {
	required := sbx.KitAddFeatures(kitArgs)
	if len(required) == 0 {
		return nil
	}
	v, _ := a.Sbx.Version(ctx)
	return sbx.FormatIncompatible(v, required)
}
