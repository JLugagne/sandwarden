package app

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/JLugagne/sbx-ui/internal/sbx"
	"github.com/JLugagne/sbx-ui/internal/store"
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
}

// SandboxDetail is the per-sandbox projection pushed on TopicSandbox.
type SandboxDetail struct {
	Sandbox              SandboxSummary       `json:"sandbox"`
	Profiles             []store.Profile      `json:"profiles"`
	Mounts               []sbx.MountInfo      `json:"mounts"`
	Secrets              []sbx.Secret         `json:"secrets"`
	CustomSecrets        []sbx.CustomSecret   `json:"custom_secrets"`
	PolicyRules          []sbx.PolicyRule     `json:"policy_rules"`
	Caches               []SandboxCache       `json:"caches"`
	AdditionalWorkspaces []sbx.WorkspaceMount `json:"additional_workspaces,omitempty"`
	Skills               []SandboxSkill       `json:"skills"`
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
		out = append(out, buildSummary(s, assignments[s.Name], byID))
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
	detail := SandboxDetail{
		Sandbox:              buildSummary(info, ids, byID),
		Profiles:             profiles,
		AdditionalWorkspaces: info.AdditionalWorkspaces,
	}
	if mounts, err := a.Sbx.Mounts(ctx, name); err == nil {
		detail.Mounts = mounts
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
	if caches, err := a.Store.ListCachesForSandbox(ctx, name); err == nil {
		detail.Caches = buildSandboxCaches(caches, detail.Mounts)
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
	Attached bool `json:"attached"`
}

// buildSandboxCaches marks each configured cache as attached when a matching
// bind mount is live in the sandbox.
func buildSandboxCaches(caches []store.CacheMount, mounts []sbx.MountInfo) []SandboxCache {
	out := make([]SandboxCache, 0, len(caches))
	for _, c := range caches {
		out = append(out, SandboxCache{CacheMount: c, Attached: cacheMounted(mounts, c)})
	}
	return out
}

func cacheMounted(mounts []sbx.MountInfo, c store.CacheMount) bool {
	for _, m := range mounts {
		if m.HostPath != c.HostPath {
			continue
		}
		target := m.Target
		if target == "" {
			target = m.HostPath
		}
		if target == c.TargetPath {
			return true
		}
	}
	return false
}
