package fleet

// Clone returns a deep copy of the spec.
func (s Spec) Clone() Spec {
	c := s
	if s.Requires != nil {
		r := *s.Requires
		c.Requires = &r
	}
	if s.Sandbox != nil {
		sb := *s.Sandbox
		sb.Entrypoint = append([]string(nil), s.Sandbox.Entrypoint...)
		if s.Sandbox.Command != nil {
			cmd := *s.Sandbox.Command
			sb.Command = &cmd
		}
		if s.Sandbox.Resources != nil {
			res := *s.Sandbox.Resources
			sb.Resources = &res
		}
		c.Sandbox = &sb
	}
	if s.Environment != nil {
		env := SpecEnv{Variables: make(map[string]string, len(s.Environment.Variables))}
		for k, v := range s.Environment.Variables {
			env.Variables[k] = v
		}
		c.Environment = &env
	}
	c.Ports = append([]SpecPort(nil), s.Ports...)
	if s.Permissions != nil {
		p := *s.Permissions
		if s.Permissions.Network != nil {
			n := *s.Permissions.Network
			n.Allow = append([]string(nil), s.Permissions.Network.Allow...)
			n.Deny = append([]string(nil), s.Permissions.Network.Deny...)
			p.Network = &n
		}
		c.Permissions = &p
	}
	if s.Setup != nil {
		setup := *s.Setup
		setup.Install = append([]SpecCommand(nil), s.Setup.Install...)
		setup.Startup = append([]SpecCommand(nil), s.Setup.Startup...)
		setup.Files = append([]SpecFile(nil), s.Setup.Files...)
		c.Setup = &setup
	}
	c.Credentials = append([]any(nil), s.Credentials...)
	if s.Arguments != nil {
		args := make(map[string]any, len(s.Arguments))
		for k, v := range s.Arguments {
			args[k] = v
		}
		c.Arguments = args
	}
	return c
}

// Clone returns a deep copy of the sidecar.
func (a SandboxApp) Clone() SandboxApp {
	c := a
	if a.Create != nil {
		create := *a.Create
		create.Workspaces = append([]string(nil), a.Create.Workspaces...)
		create.Publish = append([]string(nil), a.Create.Publish...)
		create.Kits = append([]string(nil), a.Create.Kits...)
		c.Create = &create
	}
	c.Profiles = append([]string(nil), a.Profiles...)
	c.Caches = append([]string(nil), a.Caches...)
	c.Skills = append([]SkillRef(nil), a.Skills...)
	c.Mounts = append([]MountRef(nil), a.Mounts...)
	if a.OptOuts != nil {
		out := *a.OptOuts
		out.Mounts = append([]string(nil), a.OptOuts.Mounts...)
		out.Caches = append([]string(nil), a.OptOuts.Caches...)
		c.OptOuts = &out
	}
	return c
}

// Clone returns a deep copy of the sidecar.
func (a ProfileApp) Clone() ProfileApp {
	c := a
	c.Mounts = append([]MountRef(nil), a.Mounts...)
	c.Caches = append([]string(nil), a.Caches...)
	c.Skills = append([]SkillRef(nil), a.Skills...)
	return c
}

// Clone returns a deep copy of the cache definition.
func (a CacheApp) Clone() CacheApp {
	c := a
	if a.AutoAttach != nil {
		v := *a.AutoAttach
		c.AutoAttach = &v
	}
	if a.Enabled != nil {
		v := *a.Enabled
		c.Enabled = &v
	}
	return c
}

// Clone returns a deep copy of the registration.
func (r *StoreReg) Clone() *StoreReg {
	if r == nil {
		return nil
	}
	c := *r
	return &c
}

// Clone returns a deep copy of the configuration.
func (c AppConfig) Clone() AppConfig {
	out := c
	if c.Terminals.Enabled != nil {
		enabled := append([]string(nil), (*c.Terminals.Enabled)...)
		out.Terminals.Enabled = &enabled
	}
	return out
}
