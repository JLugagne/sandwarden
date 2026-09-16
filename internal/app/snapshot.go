package app

import (
	"context"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/JLugagne/sandwarden/internal/sbx"
	"github.com/JLugagne/sandwarden/internal/store"
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
	// CPUPercent is the sampled CPU usage, 0-100 across the sandbox CPUs.
	CPUPercent float64 `json:"cpu_percent"`
	// MemoryUsed/MemoryTotal are the sampled memory figures in bytes.
	MemoryUsed  int64 `json:"memory_used_bytes"`
	MemoryTotal int64 `json:"memory_total_bytes"`
}

// SandboxDetail is the per-sandbox projection pushed on TopicSandbox.
type SandboxDetail struct {
	Sandbox              SandboxSummary        `json:"sandbox"`
	Profiles             []store.Profile       `json:"profiles"`
	Mounts               []sbx.MountInfo       `json:"mounts"`
	MountsError          string                `json:"mounts_error,omitempty"`
	Image                string                `json:"image,omitempty"`
	ImageDigest          string                `json:"image_digest,omitempty"`
	Kits                 []string              `json:"kits,omitempty"`
	Secrets              []sbx.Secret          `json:"secrets"`
	CustomSecrets        []sbx.CustomSecret    `json:"custom_secrets"`
	PolicyRules          []sbx.PolicyRule      `json:"policy_rules"`
	Caches               []SandboxCache        `json:"caches"`
	ProfileMounts        []SandboxProfileMount `json:"profile_mounts"`
	AdditionalWorkspaces []sbx.WorkspaceMount  `json:"additional_workspaces,omitempty"`
	Skills               []SandboxSkill        `json:"skills"`
}

// SandboxSummaries builds the list projection of every sandbox.
func (a *App) SandboxSummaries(ctx context.Context) ([]SandboxSummary, error) {
	sandboxes, err := a.Sbx.ListSandboxes(ctx)
	if err != nil {
		return nil, err
	}
	assignments, err := a.Store.AllAssignments(ctx)
	if err != nil {
		return nil, err
	}
	profiles, err := a.Store.ListProfiles(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]store.Profile, len(profiles))
	for _, p := range profiles {
		byID[p.ID] = p
	}
	out := make([]SandboxSummary, 0, len(sandboxes))
	for _, s := range sandboxes {
		summary := buildSummary(s, assignments[s.Name], byID)
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
	profiles, err := a.Store.ListProfilesForSandbox(ctx, name)
	if err != nil {
		return SandboxDetail{}, err
	}
	byID := make(map[int64]store.Profile, len(profiles))
	ids := make([]int64, 0, len(profiles))
	for _, p := range profiles {
		byID[p.ID] = p
		ids = append(ids, p.ID)
	}
	summary := buildSummary(info, ids, byID)
	a.stats.apply(&summary)
	detail := SandboxDetail{
		Sandbox:              summary,
		Profiles:             profiles,
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
	if profileMounts, err := a.profileMountsForSandbox(ctx, name, detail.Mounts); err == nil {
		detail.ProfileMounts = profileMounts
	}
	if skills := a.sandboxSkills(ctx, name, detail.Mounts); len(skills) > 0 {
		detail.Skills = skills
	}
	return detail, nil
}

func buildSummary(s sbx.Sandbox, assigned []int64, byID map[int64]store.Profile) SandboxSummary {
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
		Profiles:          []string{},
		Connect:           connectInfo(s.Name),
	}
	if s.Profile != nil {
		sum.DaemonProfile = *s.Profile
	}
	for _, id := range assigned {
		if p, ok := byID[id]; ok {
			sum.Profiles = append(sum.Profiles, p.Name)
		}
	}
	return sum
}

func connectInfo(name string) ConnectInfo {
	return ConnectInfo{Run: "sbx run --name " + name, Shell: "sbx exec -it " + name + " bash"}
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
		if data, err := a.Store.ListCacheMounts(ctx); err == nil {
			a.Hub.Publish(EventFor(topic, data))
		}
	case TopicSkills:
		if data, err := a.ListSkillStores(ctx); err == nil {
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
type SandboxCache struct {
	store.CacheMount
	Attached bool     `json:"attached"`
	Direct   bool     `json:"direct"`
	Profiles []string `json:"profiles"`
	OptedOut bool     `json:"opted_out"`
}

func cacheMounted(mounts []sbx.MountInfo, c store.CacheMount) bool {
	return mountPresent(mounts, c.HostPath, c.TargetPath)
}

// sandboxCaches projects the caches of one sandbox: direct assignments plus
// profile defaults, with live attachment and opt-out state. The desired set
// feeds the reconcile pass; this one feeds the UI.
func (a *App) sandboxCaches(ctx context.Context, sandbox string, mounts []sbx.MountInfo) []SandboxCache {
	direct, err := a.Store.ListCachesForSandbox(ctx, sandbox)
	if err != nil {
		return nil
	}
	profiled, err := a.Store.ProfileCachesForSandbox(ctx, sandbox)
	if err != nil {
		return nil
	}
	optOuts, err := a.Store.ProfileOptOuts(ctx, sandbox)
	if err != nil {
		return nil
	}
	merged := map[int64]*SandboxCache{}
	var order []int64
	row := func(c store.CacheMount) *SandboxCache {
		if existing, ok := merged[c.ID]; ok {
			return existing
		}
		created := &SandboxCache{CacheMount: c, Attached: cacheMounted(mounts, c)}
		merged[c.ID] = created
		order = append(order, c.ID)
		return created
	}
	for _, c := range profiled {
		entry := row(c.CacheMount)
		if !slices.Contains(entry.Profiles, c.ProfileName) {
			entry.Profiles = append(entry.Profiles, c.ProfileName)
		}
	}
	for _, c := range direct {
		row(c).Direct = true
	}
	out := make([]SandboxCache, 0, len(order))
	for _, id := range order {
		entry := merged[id]
		entry.OptedOut = !entry.Direct && optOuts[store.ProfileOptOutKey(store.OptOutCache, id)]
		sort.Strings(entry.Profiles)
		out = append(out, *entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
