import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/api/client";
import { errorMessage, useApiMutation } from "@/hooks/useApiMutation";
import { useToasts } from "@/components/Toaster";
import { useConfigStaleness } from "@/hooks/useConfigStaleness";
import { queryKeys } from "@/store/realtime";
import { useConnectionStatus } from "@/app/RealtimeProvider";
import { primeConfig, stalenessKey, staleSlugs } from "@/lib/config";
import { cn } from "@/lib/cn";
import { StaleBadge } from "@/components/StaleBadge";
import { fetchNotificationsEnabled, notificationsEnabled, setNotificationsEnabled } from "@/lib/notifications";
import {
  defaultTerminal,
  enabledTerminals,
  loadTerminalPrefs,
  saveTerminalPrefs,
  type TerminalPrefs,
} from "@/lib/terminals";
import {
  Badge,
  Button,
  Checkbox,
  CheckboxField,
  CommandLine,
  ConfirmDialog,
  CopyButton,
  Description,
  DescriptionList,
  EmptyState,
  Field,
  IconPlus,
  IconRefresh,
  Input,
  Modal,
  PageHeader,
  Panel,
  Spinner,
  TableWrap,
  Tabs,
  TD,
  TextArea,
  TH,
  TRow,
} from "@/components/ui";
import type { CacheInput, CacheView, NotificationStatus } from "@/types";

const GO_BUILD_CACHE = navigator.userAgent.includes("Mac") ? "~/Library/Caches/go-build" : "~/.cache/go-build";

const PRESETS: Array<{ label: string; input: CacheInput }> = [
  {
    label: "Go modules",
    input: {
      name: "go-mod",
      description: "Go module cache (GOMODCACHE) shared across sandboxes",
      host_path: "~/go/pkg/mod",
      target_path: "/home/agent/go/pkg/mod",
      read_only: false,
      auto_attach: true,
      enabled: true,
    },
  },
  {
    label: "Go build",
    input: {
      name: "go-build",
      description: "Go build cache (GOCACHE)",
      host_path: GO_BUILD_CACHE,
      target_path: "/home/agent/.cache/go-build",
      read_only: false,
      auto_attach: true,
      enabled: true,
    },
  },
  {
    label: "npm",
    input: {
      name: "npm",
      description: "npm cache",
      host_path: "~/.npm",
      target_path: "/home/agent/.npm",
      read_only: false,
      auto_attach: true,
      enabled: true,
    },
  },
  {
    label: "pnpm",
    input: {
      name: "pnpm-store",
      description: "pnpm content-addressable store",
      host_path: "~/.local/share/pnpm/store",
      target_path: "/home/agent/.local/share/pnpm/store",
      read_only: false,
      auto_attach: true,
      enabled: true,
    },
  },
  {
    label: "yarn",
    input: {
      name: "yarn",
      description: "Yarn classic cache",
      host_path: "~/.cache/yarn",
      target_path: "/home/agent/.cache/yarn",
      read_only: false,
      auto_attach: true,
      enabled: true,
    },
  },
];

const EMPTY_CACHE: CacheInput = {
  name: "",
  description: "",
  host_path: "",
  target_path: "",
  read_only: false,
  auto_attach: true,
  enabled: true,
};

export function SettingsPage() {
  const health = useQuery({ queryKey: queryKeys.health, queryFn: api.health, retry: 0 });
  const status = useConnectionStatus();
  const [notificationsOn, setNotificationsOn] = useState(notificationsEnabled());

  const caches = useQuery({ queryKey: queryKeys.caches, queryFn: api.caches });
  const staleness = useConfigStaleness();
  const staleCaches = staleSlugs(staleness.data, "cache");
  const [editing, setEditing] = useState<{ mode: "create" } | { mode: "edit"; cache: CacheView } | null>(null);
  const [deleting, setDeleting] = useState<CacheView | null>(null);
  const [searchParams, setSearchParams] = useSearchParams();
  const [highlight, setHighlight] = useState<string | null>(null);
  const tab = settingsTab(searchParams.get("tab"), searchParams.has("cache"));
  function setTab(next: string) {
    setSearchParams(
      (current) => {
        const params = new URLSearchParams(current);
        if (next === "general") params.delete("tab");
        else params.set("tab", next);
        return params;
      },
      { replace: true },
    );
  }

  const fleetDir = useQuery({ queryKey: queryKeys.fleetDir, queryFn: api.fleetDir, retry: 0 });
  const [reloadErrors, setReloadErrors] = useState<string[]>([]);
  const reload = useApiMutation({
    mutationFn: () => api.reloadFleet(),
    success: (errors) => (errors.length === 0 ? "Configuration reloaded" : undefined),
    invalidate: [
      stalenessKey,
      queryKeys.sandboxes,
      queryKeys.sandboxDetails,
      queryKeys.profiles,
      queryKeys.caches,
      queryKeys.skillStores,
      queryKeys.skillItems,
      queryKeys.kitStores,
      queryKeys.kitItems,
      queryKeys.config,
    ],
    onSuccess: (errors) => setReloadErrors(errors),
  });

  const toast = useToasts();
  const terminals = useQuery({ queryKey: queryKeys.terminals, queryFn: api.terminals, staleTime: 60_000 });
  const [terminalPrefs, setTerminalPrefs] = useState<TerminalPrefs>(loadTerminalPrefs);
  const config = useQuery({ queryKey: queryKeys.config, queryFn: api.getConfig, staleTime: 0, refetchOnMount: "always" });

  // The backend config file is the source of truth for both preferences. Cached
  // query data can predate a save made since, so only a fresh read is applied.
  useEffect(() => {
    if (!config.data || config.isFetching) return;
    primeConfig(config.data);
    setTerminalPrefs(config.data.terminals);
    setNotificationsOn(config.data.notifications);
  }, [config.data, config.isFetching]);

  useEffect(() => {
    let active = true;
    void fetchNotificationsEnabled().then((enabled) => {
      if (active) setNotificationsOn(enabled);
    });
    return () => {
      active = false;
    };
  }, []);

  const terminalRows = terminals.data ?? [];
  const enabledIds = new Set(enabledTerminals(terminalRows, terminalPrefs).map((terminal) => terminal.id));
  const currentDefault = defaultTerminal(terminalRows, terminalPrefs);

  function persistConfig(save: Promise<unknown>, revert: () => void) {
    save.catch((error) => {
      revert();
      toast.push({ tone: "danger", title: "Settings not saved", body: errorMessage(error) });
    });
  }

  const notificationStatus = useQuery({
    queryKey: queryKeys.notificationStatus,
    queryFn: api.notificationStatus,
    retry: 0,
  });
  const queryClient = useQueryClient();

  async function toggleNotifications(value: boolean) {
    setNotificationsOn(value);
    if (value) {
      try {
        queryClient.setQueryData(queryKeys.notificationStatus, await api.requestNotificationAuthorization());
      } catch (error) {
        void notificationStatus.refetch();
        toast.push({ tone: "warning", title: "Notifications unavailable", body: errorMessage(error) });
      }
    }
    persistConfig(setNotificationsEnabled(value), () => setNotificationsOn(!value));
  }

  const testNotification = useApiMutation({
    mutationFn: () => api.notify("sandwarden", "Desktop notifications are working."),
    success: "Test notification sent",
  });

  function updateTerminalPrefs(next: TerminalPrefs) {
    const previous = terminalPrefs;
    setTerminalPrefs(next);
    persistConfig(saveTerminalPrefs(next), () => setTerminalPrefs(previous));
  }

  function toggleTerminal(id: string, enabled: boolean) {
    const base = terminalPrefs.enabled ?? terminalRows.map((terminal) => terminal.id);
    const next = enabled ? Array.from(new Set([...base, id])) : base.filter((value) => value !== id);
    updateTerminalPrefs({ enabled: next, default: terminalPrefs.default });
  }

  function makeDefaultTerminal(id: string) {
    const enabled = terminalPrefs.enabled ?? terminalRows.map((terminal) => terminal.id);
    updateTerminalPrefs({ enabled, default: id });
  }

  const startDaemon = useApiMutation({
    mutationFn: () => api.startDaemon(),
    success: "sandboxd started",
    invalidate: [queryKeys.health, queryKeys.sandboxes],
  });

  const version = useQuery({ queryKey: queryKeys.version, queryFn: api.versionInfo, retry: 0, staleTime: Infinity });
  const updates = useQuery({
    queryKey: queryKeys.updates,
    queryFn: () => api.checkUpdates(false),
    retry: 0,
    staleTime: 300_000,
  });
  const checkUpdates = useApiMutation({
    mutationFn: () => api.checkUpdates(true),
    success: (info) =>
      !info.latest_version
        ? "No stable release published yet"
        : info.update_available
          ? `Version ${info.latest_version} is available`
          : "sandwarden is up to date",
    invalidate: [queryKeys.updates],
  });

  const create = useApiMutation({
    mutationFn: (input: CacheInput) => api.createCache(input),
    success: (cache) => `Cache ${cache.name} created`,
    onSuccess: () => setEditing(null),
  });
  const update = useApiMutation({
    mutationFn: ({ slug, input }: { slug: string; input: CacheInput }) => api.updateCache(slug, input),
    success: (cache) => `Cache ${cache.name} updated`,
    onSuccess: () => setEditing(null),
  });
  const remove = useApiMutation({
    mutationFn: (slug: string) => api.deleteCache(slug),
    success: "Cache deleted",
    onSuccess: () => setDeleting(null),
  });

  const rows = caches.data ?? [];

  // Deep link from the global search overlay: `/settings?cache=<slug>`
  // highlights the matching row, scrolls the caches panel into view and strips
  // the parameter.
  useEffect(() => {
    const slug = searchParams.get("cache");
    if (!slug) return;
    setHighlight(slug);
    setSearchParams(
      (current) => {
        const next = new URLSearchParams(current);
        next.delete("cache");
        next.set("tab", "caches");
        return next;
      },
      { replace: true },
    );
  }, [searchParams, setSearchParams]);

  useEffect(() => {
    if (highlight === null || rows.length === 0) return;
    if (!rows.some((cache) => cache.slug === highlight)) return;
    document.getElementById("shared-caches")?.scrollIntoView({ behavior: "smooth", block: "start" });
    const timer = window.setTimeout(() => setHighlight(null), 2500);
    return () => window.clearTimeout(timer);
  }, [highlight, rows]);

  return (
    <>
      <PageHeader
        title="Settings"
        subtitle="Connection, agent instructions, shared caches, terminals and configuration files."
      />

      <Tabs tabs={SETTINGS_TABS} active={tab} onChange={setTab} className="mb-4" />

      <div className="flex flex-col gap-4">
        {tab === "general" ? (
          <>
        <Panel title="Connection">
          {health.isLoading ? (
            <Spinner />
          ) : health.error ? (
            <p className="text-sm text-danger">
              {health.error instanceof Error ? health.error.message : String(health.error)}
            </p>
          ) : health.data ? (
            <div className="flex flex-col gap-3">
              <DescriptionList>
                <Description label="Realtime">
                  <Badge tone={status === "online" ? "success" : status === "connecting" ? "warning" : "danger"} dot>
                    {status}
                  </Badge>
                </Description>
                <Description label="sandboxd">
                  <Badge tone={health.data.daemon_running ? "success" : "danger"} dot>
                    {health.data.daemon_running ? "running" : "stopped"}
                  </Badge>
                </Description>
                <Description label="sandboxd socket">
                  <span className="font-mono text-xs">{health.data.socket}</span>
                </Description>
                <Description label="sbx CLI">
                  <span className="font-mono text-xs">{health.data.sbx_binary}</span>
                </Description>
              </DescriptionList>
              {!health.data.daemon_running ? (
                <div className="flex flex-wrap items-center gap-3">
                  <Button variant="primary" loading={startDaemon.isPending} onClick={() => startDaemon.mutate()}>
                    Start sandboxd
                  </Button>
                  <span className="text-xs text-muted">
                    {health.data.daemon_status && health.data.daemon_status !== "stopped"
                      ? health.data.daemon_status
                      : "sandboxd is not running; start it to manage sandboxes."}
                  </span>
                </div>
              ) : null}
            </div>
          ) : null}
        </Panel>

        <Panel title="Notifications" description="Native desktop notifications for newly blocked hosts.">
          <div className="flex items-center gap-3">
            <CheckboxField
              label="enable desktop notifications"
              checked={notificationsOn}
              onChange={(value) => toggleNotifications(value)}
            />
            <span className="text-xs text-muted">{notificationSummary(notificationsOn, notificationStatus.data)}</span>
            {notificationsOn && notificationStatus.data?.authorized ? (
              <Button size="sm" variant="ghost" loading={testNotification.isPending} onClick={() => testNotification.mutate()}>
                Send a test
              </Button>
            ) : null}
          </div>
        </Panel>

        <Panel
          title="About"
          description="Version and update channel. Stable releases are published for Linux (amd64) and macOS (Apple silicon); the unstable pre-release follows the main branch."
        >
          {version.isLoading ? (
            <Spinner />
          ) : version.data ? (
            <div className="flex flex-col gap-3">
              <DescriptionList>
                <Description label="Version">
                  <span className="font-mono text-xs">{version.data.version}</span>
                </Description>
                <Description label="Latest stable">
                  {updates.isLoading ? (
                    <span className="text-xs text-muted">checking…</span>
                  ) : updates.error ? (
                    <span className="text-xs text-muted">unknown</span>
                  ) : updates.data?.latest_version ? (
                    <span className="font-mono text-xs">{updates.data.latest_version}</span>
                  ) : (
                    <span className="text-xs text-muted">none published yet</span>
                  )}
                </Description>
              </DescriptionList>
              {updates.data?.update_available ? (
                <div className="flex flex-col gap-2 rounded-md border border-accent/40 bg-accent-soft px-3 py-2 text-sm">
                  <span>sandwarden {updates.data.latest_version} is available.</span>
                  <CommandLine command="curl -fsSL https://raw.githubusercontent.com/JLugagne/sandwarden/main/install.sh | bash" />
                  {updates.data.release_url ? (
                    <a href={updates.data.release_url} className="text-xs text-accent hover:underline">
                      Release notes
                    </a>
                  ) : null}
                </div>
              ) : null}
              {updates.error ? (
                <p className="text-xs text-muted">
                  Update check failed: {updates.error instanceof Error ? updates.error.message : String(updates.error)}
                </p>
              ) : null}
              <div>
                <Button loading={checkUpdates.isPending} onClick={() => checkUpdates.mutate()}>
                  Check for updates
                </Button>
              </div>
            </div>
          ) : null}
        </Panel>
          </>
        ) : null}

        {tab === "agents" ? <AgentsFilePanel /> : null}

        {tab === "caches" ? (
        <div id="shared-caches">
          <Panel
            title="Shared caches"
            description="Host directories bind-mounted into sandboxes and shareable across them. Paths support ~ for your home directory. Auto-attach caches are mounted on every new sandbox and re-applied on start."
            actions={
              <Button variant="primary" onClick={() => setEditing({ mode: "create" })}>
                <IconPlus /> Add cache
              </Button>
            }
            bodyClassName="p-0"
          >
            {caches.isLoading ? (
              <div className="flex justify-center p-6">
                <Spinner />
              </div>
            ) : rows.length === 0 ? (
              <EmptyState
                title="No shared cache configured."
                description="Add the Go module cache, the Go build cache, or an npm/pnpm/yarn cache to reuse downloads across sandboxes."
                action={
                  <Button variant="primary" onClick={() => setEditing({ mode: "create" })}>
                    <IconPlus /> Add cache
                  </Button>
                }
                className="rounded-none border-0"
              />
            ) : (
              <TableWrap className="border-0">
                <thead>
                  <tr>
                    <TH>Cache</TH>
                    <TH>Host directory</TH>
                    <TH>Sandbox directory</TH>
                    <TH>Mode</TH>
                    <TH>Flags</TH>
                    <TH className="w-32" />
                  </tr>
                </thead>
                <tbody>
                  {rows.map((cache) => (
                    <TRow
                      key={cache.slug}
                      className={cn(highlight === cache.slug && "bg-accent-soft ring-2 ring-accent")}
                    >
                      <TD className="font-medium">
                        <span className="inline-flex items-center gap-2">
                          {cache.name}
                          {staleCaches.has(cache.slug) ? <StaleBadge /> : null}
                        </span>
                      </TD>
                      <TD className="font-mono text-xs">{cache.host_path}</TD>
                      <TD className="font-mono text-xs">{cache.target_path}</TD>
                      <TD>{cache.read_only ? <Badge tone="warning">ro</Badge> : <Badge>rw</Badge>}</TD>
                      <TD>
                        <span className="flex flex-wrap gap-1.5">
                          {cache.auto_attach !== false ? <Badge tone="accent">auto-attach</Badge> : null}
                          {cache.enabled === false ? <Badge tone="danger">disabled</Badge> : null}
                        </span>
                      </TD>
                      <TD className="text-right">
                        <span className="inline-flex gap-1">
                          <Button size="sm" variant="ghost" onClick={() => setEditing({ mode: "edit", cache })}>
                            Edit
                          </Button>
                          <Button size="sm" variant="ghost" onClick={() => setDeleting(cache)}>
                            <span className="text-danger">Delete</span>
                          </Button>
                        </span>
                      </TD>
                    </TRow>
                  ))}
                </tbody>
              </TableWrap>
            )}
          </Panel>
        </div>
        ) : null}

        {tab === "terminals" ? (
        <Panel
          title="Terminals"
          description="Terminal emulators found on this host. The Open buttons next to sandbox commands launch the default one in the sandbox workspace."
          actions={
            <Button loading={terminals.isFetching} onClick={() => void terminals.refetch()}>
              <IconRefresh /> Rescan
            </Button>
          }
          bodyClassName="p-0"
        >
          {terminals.isLoading ? (
            <div className="flex justify-center p-6">
              <Spinner />
            </div>
          ) : terminalRows.length === 0 ? (
            <EmptyState
              title="No terminal emulator found."
              description="Install one (GNOME Terminal, Konsole, iTerm2, kitty, …) on this machine and rescan."
              action={
                <Button loading={terminals.isFetching} onClick={() => void terminals.refetch()}>
                  <IconRefresh /> Rescan
                </Button>
              }
              className="rounded-none border-0"
            />
          ) : (
            <TableWrap className="border-0">
              <thead>
                <tr>
                  <TH>Terminal</TH>
                  <TH>Binary</TH>
                  <TH>Default</TH>
                  <TH>Enabled</TH>
                </tr>
              </thead>
              <tbody>
                {terminalRows.map((terminal) => {
                  const isEnabled = enabledIds.has(terminal.id);
                  const isDefault = currentDefault?.id === terminal.id;
                  return (
                    <TRow key={terminal.id}>
                      <TD className="font-medium">{terminal.name}</TD>
                      <TD className="font-mono text-xs">{terminal.binary}</TD>
                      <TD>
                        {isDefault ? (
                          <Badge tone="accent">default</Badge>
                        ) : (
                          <Button
                            size="sm"
                            variant="ghost"
                            disabled={!isEnabled}
                            onClick={() => makeDefaultTerminal(terminal.id)}
                          >
                            Use by default
                          </Button>
                        )}
                      </TD>
                      <TD>
                        <Checkbox
                          label="enabled"
                          checked={isEnabled}
                          onChange={(value) => toggleTerminal(terminal.id, value)}
                        />
                      </TD>
                    </TRow>
                  );
                })}
              </tbody>
            </TableWrap>
          )}
        </Panel>
        ) : null}

        {tab === "files" ? (
        <Panel
          title="Configuration files"
          description="Sandbox, profile, cache, skill and kit definitions live as files under this directory. Reloading re-reads every file from disk and reports the ones that failed."
          actions={
            <Button loading={reload.isPending} onClick={() => reload.mutate()}>
              <IconRefresh /> Reload from disk
            </Button>
          }
        >
          <div className="flex flex-col gap-3">
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-xs text-faint">Directory</span>
              {fleetDir.isLoading ? (
                <Spinner />
              ) : fleetDir.data ? (
                <>
                  <code className="font-mono text-xs">{fleetDir.data}</code>
                  <CopyButton value={fleetDir.data} />
                </>
              ) : (
                <span className="text-sm text-danger">
                  {fleetDir.error instanceof Error ? fleetDir.error.message : "configuration directory unavailable"}
                </span>
              )}
            </div>
            {reloadErrors.length > 0 ? (
              <div className="flex flex-col gap-1 rounded-md border border-danger/40 bg-danger-soft px-3 py-2 text-sm text-danger">
                <span>Some files could not be reloaded:</span>
                <ul className="list-disc pl-5 font-mono text-xs">
                  {reloadErrors.map((error) => (
                    <li key={error}>{error}</li>
                  ))}
                </ul>
              </div>
            ) : null}
          </div>
        </Panel>
        ) : null}
      </div>

      <CacheDialog
        open={editing !== null}
        cache={editing?.mode === "edit" ? editing.cache : null}
        busy={create.isPending || update.isPending}
        onClose={() => setEditing(null)}
        onSubmit={(input) => {
          if (editing?.mode === "edit") update.mutate({ slug: editing.cache.slug, input });
          else create.mutate(input);
        }}
      />

      <ConfirmDialog
        open={deleting !== null}
        title={`Delete cache ${deleting?.name ?? ""}?`}
        body="The definition is removed. Existing bind mounts stay until the sandbox restarts or you detach them."
        confirmLabel="Delete"
        busy={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting.slug)}
        onClose={() => setDeleting(null)}
      />
    </>
  );
}

function CacheDialog({
  open,
  cache,
  busy,
  onSubmit,
  onClose,
}: {
  open: boolean;
  cache: CacheView | null;
  busy: boolean;
  onSubmit: (input: CacheInput) => void;
  onClose: () => void;
}) {
  const [input, setInput] = useState<CacheInput>(EMPTY_CACHE);
  const [loadedSlug, setLoadedSlug] = useState<string | "new" | null>(null);

  const target = cache ? cache.slug : ("new" as const);
  if (!open && loadedSlug !== null) {
    setLoadedSlug(null);
  }
  if (open && loadedSlug !== target) {
    setLoadedSlug(target);
    setInput(
      cache
        ? {
            name: cache.name,
            description: cache.description,
            host_path: cache.host_path,
            target_path: cache.target_path,
            read_only: cache.read_only,
            auto_attach: cache.auto_attach ?? true,
            enabled: cache.enabled ?? true,
          }
        : EMPTY_CACHE,
    );
  }

  function update<K extends keyof CacheInput>(key: K, value: CacheInput[K]) {
    setInput((current) => ({ ...current, [key]: value }));
  }

  const valid = input.name.trim() !== "" && input.host_path.trim() !== "" && input.target_path.trim() !== "";

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={cache ? `Edit cache ${cache.name}` : "Add a shared cache"}
      description="Host path supports ~ (expanded on the host). Sandbox path is where the agent toolchain expects the cache."
      size="md"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" disabled={!valid} loading={busy} onClick={() => onSubmit(input)}>
            {cache ? "Save" : "Create"}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        {!cache ? (
          <div className="flex flex-wrap items-center gap-1.5">
            <span className="text-xs text-faint">Presets:</span>
            {PRESETS.map((preset) => (
              <Button key={preset.label} size="sm" variant="ghost" onClick={() => setInput(preset.input)}>
                {preset.label}
              </Button>
            ))}
          </div>
        ) : null}

        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Name">
            <Input value={input.name} onChange={(event) => update("name", event.target.value)} placeholder="go-mod" />
          </Field>
          <Field label="Description">
            <Input
              value={input.description}
              onChange={(event) => update("description", event.target.value)}
              placeholder="What uses this cache"
            />
          </Field>
        </div>

        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Host directory" hint="On the Docker host, e.g. ~/.npm or your GOMODCACHE.">
            <Input
              value={input.host_path}
              onChange={(event) => update("host_path", event.target.value)}
              placeholder={GO_BUILD_CACHE}
              className="font-mono text-xs"
            />
          </Field>
          <Field label="Sandbox directory" hint="Usually under /home/agent.">
            <Input
              value={input.target_path}
              onChange={(event) => update("target_path", event.target.value)}
              placeholder="/home/agent/.cache/go-build"
              className="font-mono text-xs"
            />
          </Field>
        </div>

        <div className="flex flex-wrap gap-5">
          <CheckboxField
            label="auto-attach to new sandboxes"
            hint="Mounted on create and re-applied after a restart."
            checked={input.auto_attach}
            onChange={(value) => update("auto_attach", value)}
          />
          <CheckboxField
            label="enabled"
            checked={input.enabled}
            onChange={(value) => update("enabled", value)}
          />
          <CheckboxField
            label="read only"
            hint="Caches are normally writable."
            checked={input.read_only}
            onChange={(value) => update("read_only", value)}
          />
        </div>
      </div>
    </Modal>
  );
}

const SETTINGS_TABS = [
  { id: "general", label: "General" },
  { id: "agents", label: "Agents" },
  { id: "caches", label: "Caches" },
  { id: "terminals", label: "Terminals" },
  { id: "files", label: "Files" },
];

function settingsTab(requested: string | null, cacheLink: boolean): string {
  if (cacheLink) return "caches";
  return SETTINGS_TABS.some((item) => item.id === requested) ? (requested as string) : "general";
}

function notificationSummary(enabled: boolean, status: NotificationStatus | undefined): string {
  if (!enabled) return "Not enabled.";
  if (!status) return "Checking…";
  if (!status.available) return `Unavailable: ${status.reason ?? "the platform backend did not start"}.`;
  if (!status.authorized) return status.reason ?? "Not authorized by the system.";
  return "Sent by the desktop app.";
}

function AgentsFilePanel() {
  const queryClient = useQueryClient();
  const doc = useQuery({ queryKey: queryKeys.agentsFile, queryFn: api.agentsFile });
  const [draft, setDraft] = useState<string | null>(null);
  const [confirmReset, setConfirmReset] = useState(false);

  const content = draft ?? doc.data?.content ?? "";
  const dirty = draft !== null && draft !== doc.data?.content;

  const save = useApiMutation({
    mutationFn: (next: string) => api.saveAgentsFile(next),
    success: "AGENTS.md saved",
    onSuccess: (saved) => {
      queryClient.setQueryData(queryKeys.agentsFile, saved);
      setDraft(null);
    },
  });
  const reset = useApiMutation({
    mutationFn: () => api.resetAgentsFile(),
    success: "AGENTS.md reset to the default",
    onSuccess: (saved) => {
      queryClient.setQueryData(queryKeys.agentsFile, saved);
      setDraft(null);
      setConfirmReset(false);
    },
  });

  return (
    <Panel
      title="Agent instructions"
      description="One AGENTS.md shared by every sandbox, mounted read-only. Saving updates running sandboxes immediately."
      actions={
        doc.data ? <Badge tone={doc.data.default && !dirty ? "neutral" : "accent"}>{doc.data.default && !dirty ? "default" : "customized"}</Badge> : null
      }
    >
      {doc.isLoading ? (
        <Spinner />
      ) : doc.error ? (
        <p className="text-sm text-danger">{errorMessage(doc.error)}</p>
      ) : (
        <div className="flex flex-col gap-3">
          <DescriptionList>
            <Description label="Host file">
              <span className="font-mono text-xs">{doc.data?.path}</span>
            </Description>
            <Description label="In sandboxes">
              <span className="font-mono text-xs">{doc.data?.target}</span>
            </Description>
          </DescriptionList>
          <TextArea
            aria-label="AGENTS.md content"
            value={content}
            onChange={(event) => setDraft(event.target.value)}
            spellCheck={false}
            className="min-h-96 font-mono text-xs"
          />
          <div className="flex flex-wrap items-center gap-2">
            <Button variant="primary" disabled={!dirty} loading={save.isPending} onClick={() => save.mutate(content)}>
              Save
            </Button>
            <Button variant="ghost" disabled={!dirty} onClick={() => setDraft(null)}>
              Discard changes
            </Button>
            <Button variant="ghost" disabled={doc.data?.default && !dirty} onClick={() => setConfirmReset(true)}>
              Reset to default
            </Button>
          </div>
        </div>
      )}
      <ConfirmDialog
        open={confirmReset}
        title="Reset AGENTS.md?"
        body="Your edits are replaced by the built-in template in every sandbox."
        confirmLabel="Reset"
        busy={reset.isPending}
        onConfirm={() => reset.mutate()}
        onClose={() => setConfirmReset(false)}
      />
    </Panel>
  );
}
