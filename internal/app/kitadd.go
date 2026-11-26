package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
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
func (a *App) AttachKit(ctx context.Context, name, ref string, kitArgs []string, w io.Writer) (KitAddResult, error) {
	name = strings.TrimSpace(name)
	ref = a.resolveKitRef(ref)
	kitArgs = cleanStrings(kitArgs)
	result := KitAddResult{Sandbox: name, Ref: ref, Report: ApplyReport{Warnings: []string{}, Errors: []string{}}}
	if name == "" {
		return result, errors.New("sandbox name is required")
	}
	if ref == "" {
		return result, errors.New("kit reference is required")
	}
	for _, arg := range kitArgs {
		if !strings.Contains(arg, "=") {
			return result, fmt.Errorf("kit argument %q must be in name=value form", arg)
		}
	}
	if err := checkKitArgSecrets(kitArgs); err != nil {
		return result, err
	}
	// `sbx kit add` applies a bare name=value argument to the added kit only,
	// but a later recreate passes it through `sbx create`, where it would reach
	// every kit. Scope the persisted form to the attached kit's name.
	var persistedArgs []string
	if len(kitArgs) > 0 {
		persistedArgs = scopeKitArgs(kitArgs, a.kitArgScopeName(ctx, ref))
	}
	if err := a.withFleetLock(func() error {
		s, ok := a.Fleet.SandboxByName(name)
		if !ok {
			return fmt.Errorf("no configuration found for sandbox %q", name)
		}
		if c := s.App.Create; c != nil && slices.Contains(c.Kits, ref) {
			return fmt.Errorf("kit %s is already attached to sandbox %s", ref, name)
		}
		if err := a.checkKitAddFeatures(ctx, kitArgs); err != nil {
			return err
		}
		output, err := a.Sbx.KitAdd(ctx, name, ref, kitArgs, w)
		result.Output = output
		if err != nil {
			return err
		}
		if s.App.Create == nil {
			s.App.Create = &fleet.SandboxCreate{}
		}
		s.App.Create.Kits = append(s.App.Create.Kits, ref)
		s.App.Create.KitArgs = append(s.App.Create.KitArgs, persistedArgs...)
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

// kitArgScopeName returns the kit name sbx uses to scope an argument to one
// kit: the name declared by the kit spec, read from the local checkout when the
// reference is a directory, else through `sbx kit inspect`, falling back to the
// name derived from the reference itself.
func (a *App) kitArgScopeName(ctx context.Context, ref string) string {
	if info, err := os.Stat(ref); err == nil && info.IsDir() {
		if spec, err := parseKitSpec(ref); err == nil {
			if name := strings.TrimSpace(spec.Name); name != "" {
				return name
			}
		}
	}
	if spec, err := a.Sbx.KitInspect(ctx, ref); err == nil {
		if name := strings.TrimSpace(spec.Name); name != "" {
			return name
		}
	}
	return sbxenvKitName(ref)
}

// scopeKitArgs rewrites bare name=value arguments to kitName.name=value so a
// recreate applies them to the attached kit alone. An argument already scoped
// (a dot before the first '=') is left untouched.
func scopeKitArgs(kitArgs []string, kitName string) []string {
	kitName = strings.TrimSpace(kitName)
	out := make([]string, 0, len(kitArgs))
	for _, arg := range kitArgs {
		arg = strings.TrimSpace(arg)
		if arg == "" {
			continue
		}
		left, _, found := strings.Cut(arg, "=")
		if !found || kitName == "" || strings.Contains(left, ".") {
			out = append(out, arg)
			continue
		}
		out = append(out, kitName+"."+arg)
	}
	return out
}
