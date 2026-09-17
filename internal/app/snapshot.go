package app

import (
	"context"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/JLugagne/sandwarden/internal/fleet"
	"github.com/JLugagne/sandwarden/internal/sbx"
)

// ConnectInfo carries the CLI commands that attach to a sandbox's agent.
type ConnectInfo struct {
	Run   string `json:"run"`
	Shell string `json:"shell"`
}

// SandboxSummary is the list projection of a sandbox.
type SandboxSummary struct {
	Name              string              `json:"name"`
	ID                string              `json:"id"`
	Agent             string              `json:"agent,omitempty"`
	Status            string              `json:"status"`
	Running           bool                `json:"running"`
	Workspace         string              `json:"workspace"`
	DaemonProfile     string              `json:"daemon_profile,omitempty"`
	CreatedAt         *time.Time          `json:"created_at,omitempty"`
	StoppedAt         *time.Time          `json:"stopped_at,omitempty"`
	Ports             []sbx.PublishedPort `json:"ports,omitempty"`
	MountPolicyDenied bool                `json:"mount_policy_denied"`
	Profiles          []string            `json:"profiles"`
	Connect           ConnectInfo         `json:"connect"`
	// RunArgs is appended after `--` to the connect run command, if set.
	RunArgs string `json:"run_args"`
	// CPUPercent is the sampled CPU usage, 0-100 across the sandbox CPUs.
	CPUPercent float64 `json:"cpu_percent"`
	// MemoryUsed/MemoryTotal are the sampled memory figures in bytes.
	MemoryUsed  int64 `json:"memory_used_bytes"`
	MemoryTotal int64 `json:"memory_total_bytes"`
	// Incomplete reports a config directory adopted from the daemon without its original create parameters.
	Incomplete bool `json:"incomplete"`
}

// SandboxDetail is the per-sandbox projection pushed on TopicSandbox.
type SandboxDetail struct {
	Sandbox     SandboxSummary  `json:"sandbox"`
	Profiles    []ProfileRef    `json:"profiles"`
	Mounts      []sbx.MountInfo `json:"mounts"`
	MountsError string          `json:"mounts_error,omitempty"`
	// Incomplete reports a config directory adopted from the daemon without its original create parameters.
	Incomplete           bool                  `json:"incomplete"`
	Image                string                `json:"image,omitempty"`
	ImageDigest          string                `json:"image_digest,omitempty"`
	Kits                 []string              `json:"kits,omitempty"`
	Secrets              []sbx.Secret          `json:"secrets"`
	CustomSecrets        []sbx.CustomSecret    `json:"custom_secrets"`
	PolicyRules          []sbx.PolicyRule      `json:"policy_rules"`
	Caches               []SandboxCache        `json:"caches"`
	ProfileMounts        []SandboxProfileMount `json:"profile_mounts"`
	DirectMounts         []SandboxDirectMount  `json:"direct_mounts"`
	AdditionalWorkspaces []sbx.WorkspaceMount  `json:"additional_workspaces,omitempty"`
	Skills               []SandboxSkill        `json:"skills"`
}

// SandboxSummaries builds the list projection of every sandbox.
func (a *App) SandboxSummaries(ctx context.Context) ([]SandboxSummary, error) {
	sandboxes, err := a.Sbx.ListSandboxes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]SandboxSummary, 0, len(sandboxes))
	for _, s := range sandboxes {
		var cfg *fleet.Sandbox
		if c, ok := a.Fleet.SandboxByName(s.Name); ok {
			cfg = c
		}
		summary := buildSummary(s, a.profileNames(cfg), runArgsOf(cfg))
		summary.Incomplete = sandboxIncomplete(cfg)
		a.stats.apply(&summary)
		out = append(out, summary)
	}
	return out, nil
}

// SandboxDetail builds the detail projection of one sandbox.
func (a *App) SandboxDetail(ctx context.Context, name string) (SandboxDetail, error) {
	info, err := a.Sbx.InspectSandbox(ctx, name)
	if err != nil {
		return SandboxDetail{}, err
	}
	var cfg *fleet.Sandbox
	if c, ok := a.Fleet.SandboxByName(name); ok {
		cfg = c
	}
	summary := buildSummary(info, a.profileNames(cfg), runArgsOf(cfg))
	summary.Incomplete = sandboxIncomplete(cfg)
	a.stats.apply(&summary)
	detail := SandboxDetail{
		Sandbox:              summary,
		Incomplete:           summary.Incomplete,
		Profiles:             a.profileRefs(cfg),
		AdditionalWorkspaces: info.AdditionalWorkspaces,
	}
	if inspected, err := a.Sbx.InspectDetail(ctx, name); err == nil {
		detail.Mounts = inspected.RuntimeMounts
		if len(detail.Mounts) == 0 {
			detail.Mounts = inspected.Mounts
		}
		detail.Image = inspected.Image
		detail.ImageDigest = inspected.ImageDigest
		detail.Kits = inspected.Kits
	} else {
		detail.MountsError = err.Error()
	}
	if list, err := a.Sbx.ListSecrets(ctx); err == nil {
		for _, s := range list.Stored {
			if s.Scope == name {
				detail.Secrets = append(detail.Secrets, s)
			}
		}
		for _, s := range list.Custom {
			if s.Scope == name {
				detail.CustomSecrets = append(detail.CustomSecrets, s)
			}
		}
	}
	if rules, err := a.Sbx.ListPolicyRules(ctx, name); err == nil {
		detail.PolicyRules = rules
	}
	detail.Caches = a.sandboxCaches(ctx, name, detail.Mounts)
	if mounts, err := a.profileMountsForSandbox(ctx, name, detail.Mounts); err == nil {
		detail.ProfileMounts = mounts
	}
	detail.DirectMounts = a.sandboxMounts(cfg, detail.Mounts)
	if skills := a.sandboxSkills(ctx, name, detail.Mounts); len(skills) > 0 {
		detail.Skills = skills
	}
	return detail, nil
}

func buildSummary(s sbx.Sandbox, profileNames []string, runArgs string) SandboxSummary {
	sum := SandboxSummary{
		Name:              s.Name,
		ID:                s.ID,
		Agent:             s.AgentName(),
		Status:            s.Status,
		Running:           s.Running(),
		Workspace:         s.Workspace,
		CreatedAt:         s.CreatedAt,
		StoppedAt:         s.StoppedAt,
		Ports:             s.Ports,
		MountPolicyDenied: s.MountPolicyDenied != nil && *s.MountPolicyDenied,
		Profiles:          append([]string{}, profileNames...),
		RunArgs:           runArgs,
		Connect:           connectInfo(s.Name, runArgs),
	}
	if s.Profile != nil {
		sum.DaemonProfile = *s.Profile
	}
	return sum
}

func connectInfo(name, runArgs string) ConnectInfo {
	run := "sbx run --name " + name
	if runArgs != "" {
		run += " -- " + runArgs
	}
	return ConnectInfo{Run: run, Shell: "sbx exec -it " + name + " bash"}
}

// Notify schedules a coalesced push of the topic's current snapshot.
func (a *App) Notify(topic Topic) {
	a.notifier.notify(topic)
}

// publishTopic builds and broadcasts the snapshot for one topic.
// publishTopic builds and broadcasts the snapshot for one topic.
func (a *App) publishTopic(topic Topic) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if name, ok := sandboxNameFromTopic(topic); ok {
		if data, err := a.SandboxDetail(ctx, name); err == nil {
			a.Hub.Publish(EventFor(topic, data))
		}
		return
	}
	switch topic {
	case TopicSandboxes:
		if data, err := a.SandboxSummaries(ctx); err == nil {
			a.Hub.Publish(EventFor(topic, data))
		}
	case TopicProfiles:
		if data, err := a.ListProfiles(ctx); err == nil {
			a.Hub.Publish(EventFor(topic, data))
		}
	case TopicSecrets:
		if data, err := a.Sbx.ListSecrets(ctx); err == nil {
			a.Hub.Publish(EventFor(topic, data))
		}
	case TopicTraffic:
		if data, err := a.Traffic(ctx); err == nil {
			a.Hub.Publish(EventFor(topic, data))
		}
	case TopicPolicyRules:
		if data, err := a.Sbx.ListPolicyRules(ctx, ""); err == nil {
			a.Hub.Publish(EventFor(topic, data))
		}
	case TopicCaches:
		if data, err := a.ListCaches(ctx); err == nil {
			a.Hub.Publish(EventFor(topic, data))
		}
	case TopicSkills:
		if data, err := a.ListSkillStores(ctx); err == nil {
			a.Hub.Publish(EventFor(topic, data))
		}
	case TopicKits:
		if data, err := a.ListKitStores(ctx); err == nil {
			a.Hub.Publish(EventFor(topic, data))
		}
	}
}

func sandboxNameFromTopic(topic Topic) (string, bool) {
	const prefix = "sandbox:"
	s := string(topic)
	if !strings.HasPrefix(s, prefix) {
		return "", false
	}
	return strings.TrimPrefix(s, prefix), true
}

// notifier coalesces rapid topic notifications into one push per topic per
// window, so bursts of daemon events do not fan out into repeated snapshots.
type notifier struct {
	mu      sync.Mutex
	pending map[Topic]bool
	timer   *time.Timer
	flush   func(Topic)
}

func newNotifier(flush func(Topic)) *notifier {
	return &notifier{pending: make(map[Topic]bool), flush: flush}
}

func (n *notifier) notify(topic Topic) {
	n.mu.Lock()
	n.pending[topic] = true
	if n.timer == nil {
		n.timer = time.AfterFunc(200*time.Millisecond, n.drain)
	}
	n.mu.Unlock()
}

func (n *notifier) drain() {
	n.mu.Lock()
	topics := make([]Topic, 0, len(n.pending))
	for topic := range n.pending {
		topics = append(topics, topic)
	}
	n.pending = make(map[Topic]bool)
	n.timer = nil
	n.mu.Unlock()
	for _, topic := range topics {
		n.flush(topic)
	}
}

// SandboxCache is a configured cache plus its live attachment state for one
// sandbox.
// SandboxCache is a configured cache plus its live attachment state for one
// sandbox.
type SandboxCache struct {
	Slug string `json:"slug"`
	fleet.CacheApp
	Attached bool     `json:"attached"`
	Direct   bool     `json:"direct"`
	Profiles []string `json:"profiles"`
	OptedOut bool     `json:"opted_out"`
}

// sandboxCaches projects the caches of one sandbox: direct assignments plus
// profile defaults, with live attachment and opt-out state. The desired set
// feeds the reconcile pass; this one feeds the UI.
func (a *App) sandboxCaches(ctx context.Context, name string, mounts []sbx.MountInfo) []SandboxCache {
	s, ok := a.Fleet.SandboxByName(name)
	if !ok {
		return nil
	}
	optedOut := map[string]bool{}
	if s.App.OptOuts != nil {
		for _, slug := range s.App.OptOuts.Caches {
			optedOut[slug] = true
		}
	}
	merged := map[string]*SandboxCache{}
	var order []string
	row := func(c fleet.Cache) *SandboxCache {
		if existing, ok := merged[c.Slug]; ok {
			return existing
		}
		created := &SandboxCache{Slug: c.Slug, CacheApp: c.App, Attached: cacheMounted(mounts, c)}
		merged[c.Slug] = created
		order = append(order, c.Slug)
		return created
	}
	for _, p := range a.profilesForSandbox(s) {
		for _, slug := range p.App.Caches {
			c, ok := a.Fleet.Cache(slug)
			if !ok {
				continue
			}
			entry := row(*c)
			if label := p.Label(); !slices.Contains(entry.Profiles, label) {
				entry.Profiles = append(entry.Profiles, label)
			}
		}
	}
	for _, slug := range s.App.Caches {
		if c, ok := a.Fleet.Cache(slug); ok {
			row(*c).Direct = true
		}
	}
	out := make([]SandboxCache, 0, len(order))
	for _, slug := range order {
		entry := merged[slug]
		entry.OptedOut = !entry.Direct && optedOut[slug]
		sort.Strings(entry.Profiles)
		out = append(out, *entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// sandboxMounts is the DirectMounts tab projection: the sandbox's own
// declared mounts with their live attachment state.
func (a *App) sandboxMounts(cfg *fleet.Sandbox, live []sbx.MountInfo) []SandboxDirectMount {
	if cfg == nil {
		return nil
	}
	out := make([]SandboxDirectMount, 0, len(cfg.App.Mounts))
	for _, m := range cfg.App.Mounts {
		out = append(out, SandboxDirectMount{
			MountRef: m,
			Attached: mountPresent(live, m.HostPath, m.EffectiveTarget()),
		})
	}
	return out
}

// SandboxDirectMount is a sandbox-owned declared mount with its live state.
type SandboxDirectMount struct {
	fleet.MountRef
	Attached bool `json:"attached"`
}
