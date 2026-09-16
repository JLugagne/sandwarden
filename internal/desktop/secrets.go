package desktop

import "github.com/JLugagne/sandwarden/internal/sbx"

// ListSecrets returns stored and custom secrets for one scope.
func (d *Desktop) ListSecrets(scope string) (sbx.SecretList, error) {
	return d.app.ListSecrets(d.root, scope)
}

// SetServiceSecret stores a service credential.
func (d *Desktop) SetServiceSecret(req ServiceSecretRequest) error {
	return d.app.SetServiceSecret(d.root, sbx.ServiceSecretSpec{
		Service:   req.Service,
		Scope:     normalizeSecretScope(req.Scope),
		Value:     req.Value,
		Ref:       req.Ref,
		Command:   req.Command,
		Refresh:   req.Refresh,
		Overwrite: req.Overwrite,
	})
}

// SetRegistrySecret stores a registry credential.
func (d *Desktop) SetRegistrySecret(req RegistrySecretRequest) error {
	return d.app.SetRegistrySecret(d.root, sbx.RegistrySecretSpec{
		Host:      req.Host,
		Username:  req.Username,
		Password:  req.Password,
		Scope:     normalizeSecretScope(req.Scope),
		Overwrite: req.Overwrite,
	})
}

// SetCustomSecret stores a placeholder secret bound to hosts.
func (d *Desktop) SetCustomSecret(req CustomSecretRequest) error {
	return d.app.SetCustomSecret(d.root, sbx.CustomSecretSpec{
		Hosts:       cleanStrings(req.Hosts),
		Env:         req.Env,
		Value:       req.Value,
		Ref:         req.Ref,
		Command:     req.Command,
		Placeholder: req.Placeholder,
		Refresh:     req.Refresh,
		Scope:       normalizeSecretScope(req.Scope),
		Overwrite:   req.Overwrite,
	})
}

// RemoveSecret deletes a stored service secret.
func (d *Desktop) RemoveSecret(scope, name string) error {
	return d.app.RemoveSecret(d.root, normalizeSecretScope(scope), name)
}

// RemoveCustomSecret deletes a custom secret by placeholder.
func (d *Desktop) RemoveCustomSecret(scope, placeholder string) error {
	return d.app.RemoveCustomSecret(d.root, normalizeSecretScope(scope), placeholder)
}

// ImportSecrets starts a bulk import as a job, returning the job id whose
// output streams over hub events.
func (d *Desktop) ImportSecrets(req ImportSecretsRequest) (string, error) {
	return d.app.StartImportJob(req.Service, req.All, req.DryRun, req.Force, req.JobID), nil
}

// RemoveRegistrySecret deletes registry pull credentials for one host.
func (d *Desktop) RemoveRegistrySecret(scope, host string) error {
	return d.app.RemoveRegistrySecret(d.root, normalizeSecretScope(scope), host)
}
