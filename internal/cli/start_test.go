package cli

import (
	"testing"

	"github.com/JLugagne/sandwarden/internal/fleet"
)

func TestStartConvergesSandbox(t *testing.T) {
	env := newCLIEnv(t, "box")
	fl := env.fleet(t)

	profile := mustCreateProfile(t, fl, "dev", []string{"example.com"}, fleet.ProfileApp{})
	cacheHost := t.TempDir()
	cache, err := fl.CreateCache("go-mod", fleet.CacheApp{
		Name:       "go-mod",
		HostPath:   cacheHost,
		TargetPath: "/home/agent/go/pkg/mod",
		Enabled:    boolPtr(true),
	})
	if err != nil {
		t.Fatalf("create cache: %v", err)
	}
	skill, skillHost := seedSkillStore(t, env, fl, "internal-skills", "pdf")
	mountHost := t.TempDir()
	mustCreateSandbox(t, fl, "box", fleet.SandboxApp{
		Profiles: []string{profile.Slug},
		Caches:   []string{cache.Slug},
		Skills:   []fleet.SkillRef{skill},
		Mounts:   []fleet.MountRef{{HostPath: mountHost, TargetPath: "/src", ReadOnly: true}},
	})

	stdout, _, err := env.execute(t, "start", "box")
	if err != nil {
		t.Fatalf("start: %v\n%s", err, stdout)
	}
	mustContain(t, stdout, "box: started")
	mustContain(t, stdout, "apply: 1 rule(s), 1 mount(s), 1 cache(s), 1 skill(s) mounted, 0 removed")

	if !env.daemon.called("POST", "/sandbox/box/start") {
		t.Error("expected the daemon to be asked to start box")
	}
	actions := env.daemon.appliedActions()
	if len(actions) != 1 || actions[0].Action != "allow" || actions[0].SandboxID != "box" ||
		len(actions[0].Resources) != 1 || actions[0].Resources[0] != "example.com" {
		t.Errorf("unexpected policy actions: %+v", actions)
	}
	for _, want := range []string{
		"exec box mkdir -p /src",
		"mount box " + mountHost + ":/src:ro",
		"exec box mkdir -p /home/agent/go/pkg/mod",
		"mount box " + cacheHost + ":/home/agent/go/pkg/mod",
		"exec box mkdir -p /home/agent/.agents/skills/pdf",
		"mount box " + skillHost + ":/home/agent/.agents/skills/pdf:ro",
	} {
		env.requireSbxCall(t, want)
	}
}
