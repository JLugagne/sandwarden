# Sandbox environment

You are running inside a Docker Sandbox managed by sandwarden. Keep these
constraints in mind before acting.

## Network access is filtered

All outbound traffic goes through an egress proxy. Only allow-listed domains
are reachable; everything else is blocked until the user approves it from
sandwarden's Traffic page. A blocked request usually fails with a proxy error,
a TLS error, or a connection reset rather than a clear message.

**Before any operation that needs the network** (installing packages, cloning
repositories, calling an API, downloading a file), probe every domain it will
contact first:

```sh
curl -sS -o /dev/null -w '%{http_code} %{url_effective}\n' --max-time 10 https://example.com/
```

- Probe all the domains in one go, including indirect ones (package registry
  CDNs, redirect targets, auth endpoints).
- If any probe fails, stop and give the user the list of blocked domains so
  they can allow them in one pass. Do not retry the real operation, do not try
  workarounds, and do not dump long error output into the conversation.
- Only run the real operation once every probe succeeds.

## Filesystem

- The workspace is the project directory mounted from the host; changes there
  are visible to the user.
- Other host directories may be mounted read-only (shared caches, skills under
  `~/.agents/skills`, commands under `~/.agents/commands`). Do not try to write
  to them.
- This file is managed by sandwarden and mounted read-only; ask the user to
  edit it from sandwarden's Settings page.

## Secrets

Credentials are injected by the sandbox proxy. Never print, copy or commit
tokens, keys or environment variables that look like secrets.
