package desktop

import (
	"errors"
	"strings"

	"github.com/JLugagne/sbx-ui/internal/sbx"
)

// Traffic returns the cross-sandbox proxy log.
func (d *Desktop) Traffic() (sbx.PolicyLog, error) {
	return d.app.Traffic(d.root)
}

// SandboxTraffic returns the proxy log filtered to one sandbox.
func (d *Desktop) SandboxTraffic(name string) (sbx.PolicyLog, error) {
	return d.app.SandboxTraffic(d.root, name)
}

// PolicyRules returns network policy rules, optionally filtered by sandbox.
func (d *Desktop) PolicyRules(sandbox string) ([]sbx.PolicyRule, error) {
	return d.app.Sbx.ListPolicyRules(d.root, sandbox)
}

// SandboxPolicy returns the rules that apply to one sandbox.
func (d *Desktop) SandboxPolicy(name string) ([]sbx.PolicyRule, error) {
	return d.app.Sbx.ListPolicyRules(d.root, name)
}

// PolicyAction applies a global policy mutation.
func (d *Desktop) PolicyAction(req PolicyActionRequest) ([]sbx.PolicyActionResult, error) {
	if !policyActions[req.Action] {
		return nil, errors.New("unknown policy action")
	}
	resources := cleanStrings(req.Resources)
	if req.Action != "remove-id" && len(resources) == 0 {
		return nil, errors.New("at least one resource is required")
	}
	if req.Action == "remove-id" && strings.TrimSpace(req.ID) == "" {
		return nil, errors.New("id is required")
	}
	return d.app.ApplyPolicy(d.root, sbx.PolicyAction{
		Action:    req.Action,
		Resources: resources,
		SandboxID: strings.TrimSpace(req.SandboxID),
		ID:        strings.TrimSpace(req.ID),
	})
}

// SandboxPolicyAction applies an allow/deny mutation scoped to one sandbox.
func (d *Desktop) SandboxPolicyAction(name, action string, resources []string) ([]sbx.PolicyActionResult, error) {
	switch action {
	case "allow", "deny", "http-allow", "http-deny":
	default:
		return nil, errors.New("action must be allow or deny")
	}
	cleaned := cleanStrings(resources)
	if len(cleaned) == 0 {
		return nil, errors.New("at least one resource is required")
	}
	return d.app.ApplyPolicy(d.root, sbx.PolicyAction{
		Action:    action,
		Resources: cleaned,
		SandboxID: name,
	})
}

var policyActions = map[string]bool{
	"allow":           true,
	"deny":            true,
	"http-allow":      true,
	"http-deny":       true,
	"remove-resource": true,
	"remove-id":       true,
}
