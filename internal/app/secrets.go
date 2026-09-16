package app

import (
	"context"
	"io"

	"github.com/JLugagne/sandwarden/internal/sbx"
)

// ListSecrets returns the stored secret inventory. scope "" lists every scope,
// "global" lists global-only, and any other value is a sandbox name.
func (a *App) ListSecrets(ctx context.Context, scope string) (sbx.SecretList, error) {
	all, err := a.Sbx.ListSecrets(ctx)
	if err != nil {
		return sbx.SecretList{}, err
	}
	if scope == "" {
		return all, nil
	}
	want := scope
	if scope == "global" {
		want = ""
	}
	var out sbx.SecretList
	for _, s := range all.Stored {
		if s.Scope == want {
			out.Stored = append(out.Stored, s)
		}
	}
	for _, s := range all.Custom {
		if s.Scope == want {
			out.Custom = append(out.Custom, s)
		}
	}
	return out, nil
}

// SetServiceSecret stores a service secret and notifies viewers.
func (a *App) SetServiceSecret(ctx context.Context, spec sbx.ServiceSecretSpec) error {
	if err := a.Sbx.SetServiceSecret(ctx, spec); err != nil {
		return err
	}
	a.notifySecretScope(spec.Scope)
	return nil
}

// SetRegistrySecret stores a registry credential and notifies viewers.
func (a *App) SetRegistrySecret(ctx context.Context, spec sbx.RegistrySecretSpec) error {
	if err := a.Sbx.SetRegistrySecret(ctx, spec); err != nil {
		return err
	}
	a.notifySecretScope(spec.Scope)
	return nil
}

// SetCustomSecret stores a custom proxy-injected secret and notifies viewers.
func (a *App) SetCustomSecret(ctx context.Context, spec sbx.CustomSecretSpec) error {
	if err := a.Sbx.SetCustomSecret(ctx, spec); err != nil {
		return err
	}
	a.notifySecretScope(spec.Scope)
	return nil
}

// RemoveSecret deletes a service secret and notifies viewers.
func (a *App) RemoveSecret(ctx context.Context, scope, service string) error {
	if err := a.Sbx.RemoveSecret(ctx, scope, service); err != nil {
		return err
	}
	a.notifySecretScope(scope)
	return nil
}

// RemoveCustomSecret deletes a custom secret by placeholder and notifies viewers.
func (a *App) RemoveCustomSecret(ctx context.Context, scope, placeholder string) error {
	if err := a.Sbx.RemoveCustomSecret(ctx, scope, placeholder); err != nil {
		return err
	}
	a.notifySecretScope(scope)
	return nil
}

// ImportSecrets runs the host-environment import, streaming output to stream.
func (a *App) ImportSecrets(ctx context.Context, service string, all, dryRun, force bool, stream io.Writer) error {
	if err := a.Sbx.ImportSecrets(ctx, service, all, dryRun, force, stream); err != nil {
		return err
	}
	if !dryRun {
		a.Notify(TopicSecrets)
	}
	return nil
}

// RemoveRegistrySecret deletes registry pull credentials for one host and
// notifies viewers.
func (a *App) RemoveRegistrySecret(ctx context.Context, scope, host string) error {
	if err := a.Sbx.RemoveRegistrySecret(ctx, scope, host); err != nil {
		return err
	}
	a.notifySecretScope(scope)
	return nil
}

// notifySecretScope refreshes the secret inventory and, for sandbox-scoped
// entries, the owning sandbox's detail view.
func (a *App) notifySecretScope(scope string) {
	a.Notify(TopicSecrets)
	if scope != "" && scope != sbx.SecretScopeHostOnly {
		a.Notify(TopicSandbox(scope))
	}
}
