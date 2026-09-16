package sbx

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// PolicyAction is one mutation in a POST /policy/network/rules request.
//
// Action is allow, deny, http-allow, http-deny, remove-resource or remove-id.
// SandboxID scopes the rule to a single sandbox; empty means global.
type PolicyAction struct {
	Action    string   `json:"action"`
	Resources []string `json:"resources,omitempty"`
	SandboxID string   `json:"sandbox_id,omitempty"`
	ID        string   `json:"id,omitempty"`
}

// PolicyRuleRef identifies a rule created, matched or removed by a mutation.
type PolicyRuleRef struct {
	Resource string `json:"resource"`
	RuleID   string `json:"rule_id"`
	RuleName string `json:"rule_name"`
}

// PolicyActionResult is the daemon's per-action outcome. Error is set (and the
// action failed) when the daemon rejected that action.
type PolicyActionResult struct {
	Action    string          `json:"action"`
	Resources []string        `json:"resources,omitempty"`
	SandboxID string          `json:"sandbox_id,omitempty"`
	Error     string          `json:"error,omitempty"`
	Created   []PolicyRuleRef `json:"created,omitempty"`
	Existing  []PolicyRuleRef `json:"existing,omitempty"`
	Removed   []PolicyRuleRef `json:"removed,omitempty"`
}

// Failed reports whether the daemon rejected this action.
func (r PolicyActionResult) Failed() bool { return r.Error != "" }

type policyModifyResponse struct {
	Results []PolicyActionResult `json:"results"`
}

// ModifyPolicy applies one or more rule mutations and returns each outcome.
func (c *Client) ModifyPolicy(ctx context.Context, actions ...PolicyAction) ([]PolicyActionResult, error) {
	if len(actions) == 0 {
		return nil, errors.New("no policy actions given")
	}
	var resp policyModifyResponse
	body := map[string]any{"actions": actions}
	if err := c.doJSON(ctx, http.MethodPost, "/policy/network/rules", body, &resp); err != nil {
		return nil, err
	}
	return resp.Results, nil
}

// ListPolicyRules returns the daemon's rules. sandbox empty lists every rule;
// a sandbox name filters to the rules that apply to it.
func (c *Client) ListPolicyRules(ctx context.Context, sandbox string) ([]PolicyRule, error) {
	q := url.Values{}
	q.Set("type", "all")
	if sandbox != "" {
		q.Set("sandbox", sandbox)
	}
	var resp struct {
		Rules []PolicyRule `json:"rules"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/policy/network/rules?"+q.Encode(), nil, &resp); err != nil {
		return nil, err
	}
	return resp.Rules, nil
}

// PolicyLog returns the proxy's allowed/blocked host log.
func (c *Client) PolicyLog(ctx context.Context) (PolicyLog, error) {
	var out PolicyLog
	err := c.doJSON(ctx, http.MethodGet, "/policy/network/log", nil, &out)
	return out, err
}

// NotFound reports whether the daemon rejected the action because the target
// rule no longer exists.
func (r PolicyActionResult) NotFound() bool {
	return r.Failed() && strings.Contains(r.Error, "not found")
}
