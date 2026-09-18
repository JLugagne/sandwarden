package sbx

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
)

// ListSandboxes returns every sandbox known to the daemon.
func (c *Client) ListSandboxes(ctx context.Context) ([]Sandbox, error) {
	var out []Sandbox
	if err := c.doJSON(ctx, http.MethodGet, "/sandbox", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// InspectSandbox returns the daemon's description of one sandbox.
func (c *Client) InspectSandbox(ctx context.Context, name string) (Sandbox, error) {
	var out Sandbox
	err := c.doJSON(ctx, http.MethodGet, "/sandbox/"+url.PathEscape(name), nil, &out)
	return out, err
}

// DeleteSandbox removes a sandbox. force removes it even with an open session.
func (c *Client) DeleteSandbox(ctx context.Context, name string, force bool) error {
	path := "/sandbox/" + url.PathEscape(name)
	if force {
		path += "?force=true"
	}
	return c.doJSON(ctx, http.MethodDelete, path, nil, nil)
}

// StartSandbox boots a stopped sandbox's VM.
func (c *Client) StartSandbox(ctx context.Context, name string) error {
	return c.doJSON(ctx, http.MethodPost, "/sandbox/"+url.PathEscape(name)+"/start", nil, nil)
}

// StopSandbox stops a sandbox's VM without removing it.
func (c *Client) StopSandbox(ctx context.Context, name string) error {
	return c.doJSON(ctx, http.MethodPost, "/sandbox/"+url.PathEscape(name)+"/stop", nil, nil)
}

// AddMount bind-mounts hostPath inside the sandbox at the same path.
// MountFolder bind-mounts hostPath inside the sandbox at the same path using
// the sbx CLI. The daemon's REST mounts endpoint only allow-lists the path; the
// actual bind is performed by `sbx mount`. The sandbox must be running.
func (c *Client) MountFolder(ctx context.Context, sandbox, hostPath string, readOnly bool) error {
	spec := hostPath
	if readOnly {
		// HOST:ro would parse as HOST:CTR_TARGET; the empty target selects the
		// same-path form, so read-only is HOST::ro.
		spec += "::ro"
	}
	_, err := c.runCLI(ctx, nil, nil, "mount", sandbox, spec)
	return err
}

// RemoveMount revokes a previously allowed host path.
// UnmountFolder revokes a previously mounted host path and removes its bind
// using the sbx CLI.
// UnmountFolder revokes a mounted host path: it removes the bind and the
// allow-list entry via `sbx umount`. If that fails (for example a stale
// allow-list entry left by an older allow-list-only call, which `sbx umount`
// cannot match after resolving symlinks), it falls back to removing the exact
// host path from the allow-list over REST.
// UnmountFolder revokes a mounted host path: it removes the bind and the
// allow-list entry via `sbx umount`. If that fails it tries two fallbacks, since
// runtime mounts are lost across a sandbox restart while the entry lingers:
//  1. remove the exact allow-list entry over REST;
//  2. re-create the bind with `sbx mount`, then revoke it, which clears a stale
//     entry whose bind disappeared on restart.
//
// UnmountFolder revokes a mounted host path: it removes the bind and the
// allow-list entry via `sbx umount`. Runtime mounts are lost across a sandbox
// restart while the entry lingers, which leaves the daemon claiming the path is
// "already mounted" while `umount` reports it "not bound". The fallbacks break
// that deadlock:
//  1. remove the exact allow-list entry over REST;
//  2. re-create the bind with `sbx mount` using the entry's own read-only mode,
//     then revoke it.
func (c *Client) UnmountFolder(ctx context.Context, sandbox, hostPath string, readOnly bool) error {
	_, umountErr := c.runCLI(ctx, nil, nil, "umount", sandbox, hostPath)
	if umountErr == nil {
		return nil
	}

	body := map[string]string{"host_path": hostPath}
	if err := c.doJSON(ctx, http.MethodDelete, "/sandbox/"+url.PathEscape(sandbox)+"/mounts", body, nil); err == nil {
		return nil
	}

	remountSpec := hostPath
	if readOnly {
		remountSpec += "::ro"
	}
	if _, err := c.runCLI(ctx, nil, nil, "mount", sandbox, remountSpec); err == nil {
		if _, err := c.runCLI(ctx, nil, nil, "umount", sandbox, hostPath); err == nil {
			return nil
		}
	}
	return umountErr
}

// Mounts returns the sandbox's runtime mounts by parsing `sbx inspect --json`.
// The daemon exposes no REST listing for them.
func (c *Client) Mounts(ctx context.Context, sandbox string) ([]MountInfo, error) {
	detail, err := c.InspectDetail(ctx, sandbox)
	if err != nil {
		return nil, err
	}
	mounts := detail.RuntimeMounts
	if len(mounts) == 0 {
		mounts = detail.Mounts
	}
	return mounts, nil
}

// MountFolderAt bind-mounts hostPath inside the sandbox at target. An empty
// target uses the same-path convention. The sandbox must be running.
func (c *Client) MountFolderAt(ctx context.Context, sandbox, hostPath, target string, readOnly bool) error {
	spec := hostPath
	if target != "" {
		spec += ":" + target
	}
	if readOnly {
		if target == "" {
			spec += "::ro"
		} else {
			spec += ":ro"
		}
	}
	_, err := c.runCLI(ctx, nil, nil, "mount", sandbox, spec)
	return err
}

// UnmountFolderAt revokes a bind mount created by MountFolderAt. If the direct
// umount fails it falls back to removing the allow-list entry over REST, then
// to re-creating and revoking the bind (matching UnmountFolder's deadlock
// breaking).
func (c *Client) UnmountFolderAt(ctx context.Context, sandbox, hostPath, target string) error {
	spec := hostPath
	if target != "" {
		spec += ":" + target
	}
	_, umountErr := c.runCLI(ctx, nil, nil, "umount", sandbox, spec)
	if umountErr == nil {
		return nil
	}

	body := map[string]string{"host_path": hostPath}
	if err := c.doJSON(ctx, http.MethodDelete, "/sandbox/"+url.PathEscape(sandbox)+"/mounts", body, nil); err == nil {
		return nil
	}

	if _, err := c.runCLI(ctx, nil, nil, "mount", sandbox, spec); err == nil {
		if _, err := c.runCLI(ctx, nil, nil, "umount", sandbox, spec); err == nil {
			return nil
		}
	}
	return umountErr
}

// MkdirAll creates a directory tree inside a running sandbox. It prepares
// bind-mount targets that may not exist yet and tolerates existing paths.
func (c *Client) MkdirAll(ctx context.Context, sandbox, path string) error {
	_, err := c.runCLI(ctx, nil, nil, "exec", sandbox, "mkdir", "-p", path)
	return err
}

// InspectDetail parses `sbx inspect --json`, the only source of runtime mounts,
// kit references and image metadata. The daemon exposes no REST equivalent.
func (c *Client) InspectDetail(ctx context.Context, sandbox string) (InspectDetail, error) {
	raw, err := c.runCLI(ctx, nil, nil, "inspect", sandbox, "--json")
	if err != nil {
		return InspectDetail{}, err
	}
	var detail InspectDetail
	if err := decodeCLIJSON(raw, &detail); err != nil {
		return InspectDetail{}, errors.Join(errors.New("decode inspect output"), err)
	}
	return detail, nil
}

// RunSandbox attaches to a sandbox with the sbx CLI, inheriting the current
// terminal.
func (c *Client) RunSandbox(ctx context.Context, name string, args []string) error {
	argv := []string{"run"}
	if strings.TrimSpace(name) != "" {
		argv = append(argv, "--name", name)
	}
	if len(args) > 0 {
		argv = append(argv, "--")
		argv = append(argv, args...)
	}
	cmd := exec.CommandContext(ctx, BinaryPath(), argv...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// RunCLI executes the sbx binary with inherited stdio, for the CLI
// passthrough.
func (c *Client) RunCLI(ctx context.Context, args []string) error {
	cmd := exec.CommandContext(ctx, BinaryPath(), args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
