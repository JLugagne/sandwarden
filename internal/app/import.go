package app

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// Import outcome statuses, one per sandbox.
const (
	ImportCreated    = "created"
	ImportConfigured = "already configured"
	ImportFailed     = "failed"
)

// ImportResult is the outcome of importing one daemon sandbox into the config
// directory.
type ImportResult struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Incomplete bool   `json:"incomplete"`
	Error      string `json:"error,omitempty"`
}

// ImportReport is the result of one bulk import pass.
type ImportReport struct {
	Results    []ImportResult `json:"results"`
	Created    int            `json:"created"`
	Configured int            `json:"configured"`
	Failed     int            `json:"failed"`
}

// Print writes the human import report, one line per sandbox.
func (r ImportReport) Print(w io.Writer) {
	for _, res := range r.Results {
		switch res.Status {
		case ImportFailed:
			fmt.Fprintf(w, "%s: failed: %s\n", res.Name, res.Error)
		case ImportConfigured:
			fmt.Fprintf(w, "%s: already configured%s\n", res.Name, incompleteNote(res.Incomplete))
		default:
			fmt.Fprintf(w, "%s: created config%s\n", res.Name, incompleteNote(res.Incomplete))
		}
	}
	fmt.Fprintf(w, "import: %d created, %d already configured, %d failed\n", r.Created, r.Configured, r.Failed)
}

// incompleteNote explains what an imported config could not recover.
func incompleteNote(incomplete bool) string {
	if !incomplete {
		return ""
	}
	return " (incomplete: CPU, memory and env were not recoverable)"
}

// ImportSandboxes writes a config directory for the named daemon sandboxes, or
// for every daemon sandbox when names is empty. Missing configs are adopted
// from the daemon state and marked incomplete; a failing sandbox is reported
// and never aborts the batch.
func (a *App) ImportSandboxes(ctx context.Context, names []string) (ImportReport, error) {
	sandboxes, err := a.Sbx.ListSandboxes(ctx)
	if err != nil {
		return ImportReport{}, err
	}
	inDaemon := make(map[string]bool, len(sandboxes))
	for _, s := range sandboxes {
		inDaemon[s.Name] = true
	}
	order := names
	if len(order) == 0 {
		for _, s := range sandboxes {
			order = append(order, s.Name)
		}
	}
	report := ImportReport{Results: make([]ImportResult, 0, len(order))}
	if len(order) == 0 {
		return report, nil
	}
	lock, err := a.lockFleet()
	if err != nil {
		return report, err
	}
	defer func() { _ = lock.Release() }()
	for _, name := range order {
		res := ImportResult{Name: strings.TrimSpace(name)}
		switch {
		case res.Name == "":
			res.Status = ImportFailed
			res.Error = "name is required"
		case !inDaemon[res.Name]:
			res.Status = ImportFailed
			res.Error = "not in daemon"
		default:
			if cfg, ok := a.Fleet.SandboxByName(res.Name); ok {
				res.Status = ImportConfigured
				res.Incomplete = sandboxIncomplete(cfg)
			} else if cfg, err := a.ensureSandboxConfig(ctx, res.Name); err != nil {
				res.Status = ImportFailed
				res.Error = err.Error()
			} else {
				res.Status = ImportCreated
				res.Incomplete = sandboxIncomplete(cfg)
			}
		}
		switch res.Status {
		case ImportCreated:
			report.Created++
		case ImportConfigured:
			report.Configured++
		default:
			report.Failed++
		}
		report.Results = append(report.Results, res)
	}
	if report.Created > 0 {
		a.Notify(TopicSandboxes)
	}
	return report, nil
}
