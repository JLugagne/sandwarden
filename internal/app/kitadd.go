package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/JLugagne/sandwarden/internal/fleet"
)

// KitAddResult is the outcome of attaching a mixin kit to an existing
// sandbox: the sbx CLI output plus the sidecar convergence pass that follows.
type KitAddResult struct {
	Sandbox string      `json:"sandbox"`
	Ref     string      `json:"ref"`
	Output  string      `json:"output,omitempty"`
	Report  ApplyReport `json:"report"`
}

// Print writes the human summary of one attach. The sbx output itself is
// streamed to the writer the caller passed to AttachKit, so it is not repeated
// here.
func (r KitAddResult) Print(w io.Writer) {
	fmt.Fprintf(w, "attached kit %s to sandbox %s\n", r.Ref, r.Sandbox)
	r.Report.Print(w)
}

// AttachKit appends a mixin kit to an existing sandbox. `sbx kit add`
// recreates the container with the kit added to its kit list, preserving
// kit-owned volumes (agent session state) and --clone workspaces, unlike a
// recreate. The reference is then recorded in the sidecar's create.kits, so a
// later recreate keeps it, and the sidecar (rules, mounts, caches, skills) is
// re-applied; its ApplyReport is returned.
//
// Ordering: sbx runs first and the sidecar is only written once it succeeded,
// so a refusal — for example a sandbox created before sbx's recreate-aware
// labels — never leaves a phantom kit in create.kits. A sidecar write failure
// after a successful sbx run is reported explicitly: the kit is live but
// unrecorded.
func (a *App) AttachKit(ctx context.Context, name, ref string, w io.Writer) (KitAddResult, error) {
	name = strings.TrimSpace(name)
	ref = strings.TrimSpace(ref)
	result := KitAddResult{Sandbox: name, Ref: ref, Report: ApplyReport{Warnings: []string{}, Errors: []string{}}}
	if name == "" {
		return result, errors.New("sandbox name is required")
	}
	if ref == "" {
		return result, errors.New("kit reference is required")
	}
	if err := a.withFleetLock(func() error {
		s, ok := a.Fleet.SandboxByName(name)
		if !ok {
			return fmt.Errorf("no configuration found for sandbox %q", name)
		}
		if c := s.App.Create; c != nil && slices.Contains(c.Kits, ref) {
			return fmt.Errorf("kit %s is already attached to sandbox %s", ref, name)
		}
		output, err := a.Sbx.KitAdd(ctx, name, ref, w)
		result.Output = output
		if err != nil {
			return err
		}
		if s.App.Create == nil {
			s.App.Create = &fleet.SandboxCreate{}
		}
		s.App.Create.Kits = append(s.App.Create.Kits, ref)
		if err := a.Fleet.SaveSandbox(s); err != nil {
			return fmt.Errorf("sbx added the kit but recording it in the sidecar failed: %w", err)
		}
		report, applyErr := a.applyLocked(ctx, name)
		result.Report = report
		if applyErr != nil {
			result.Report.Errors = append(result.Report.Errors, applyErr.Error())
		}
		return nil
	}); err != nil {
		return result, err
	}
	a.Notify(TopicSandboxes)
	a.Notify(TopicSandbox(name))
	return result, nil
}
