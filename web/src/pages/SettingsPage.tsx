import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import { useApiMutation } from "@/hooks/useApiMutation";
import { queryKeys } from "@/store/realtime";
import { useConnectionStatus } from "@/app/RealtimeProvider";
import { requestNotificationPermission } from "@/components/Toaster";
import {
  Badge,
  Button,
  CheckboxField,
  ConfirmDialog,
  Description,
  DescriptionList,
  EmptyState,
  Field,
  IconPlus,
  Input,
  Modal,
  PageHeader,
  Panel,
  Spinner,
  TableWrap,
  TD,
  TH,
  TRow,
} from "@/components/ui";
import type { CacheInput, CacheMount } from "@/types";

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
      host_path: "~/.cache/go-build",
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
  const notificationsEnabled = "Notification" in window && Notification.permission === "granted";

  const caches = useQuery({ queryKey: queryKeys.caches, queryFn: api.caches });
  const [editing, setEditing] = useState<{ mode: "create" } | { mode: "edit"; cache: CacheMount } | null>(null);
  const [deleting, setDeleting] = useState<CacheMount | null>(null);

  const create = useApiMutation({
    mutationFn: (input: CacheInput) => api.createCache(input),
    success: (cache) => `Cache ${cache.name} created`,
    onSuccess: () => setEditing(null),
  });
  const update = useApiMutation({
    mutationFn: ({ id, input }: { id: number; input: CacheInput }) => api.updateCache(id, input),
    success: (cache) => `Cache ${cache.name} updated`,
    onSuccess: () => setEditing(null),
  });
  const remove = useApiMutation({
    mutationFn: (id: number) => api.deleteCache(id),
    success: "Cache deleted",
    onSuccess: () => setDeleting(null),
  });

  const rows = caches.data ?? [];

  return (
    <>
      <PageHeader
        title="Settings"
        subtitle="Connection, shared cache directories and notifications."
      />

      <div className="flex flex-col gap-4">
        <Panel title="Connection">
          {health.isLoading ? (
            <Spinner />
          ) : health.error ? (
            <p className="text-sm text-danger">
              {health.error instanceof Error ? health.error.message : String(health.error)}
            </p>
          ) : health.data ? (
            <DescriptionList>
              <Description label="Realtime">
                <Badge tone={status === "online" ? "success" : status === "connecting" ? "warning" : "danger"} dot>
                  {status}
                </Badge>
              </Description>
              <Description label="sandboxd socket">
                <span className="font-mono text-xs">{health.data.socket}</span>
              </Description>
              <Description label="sbx CLI">
                <span className="font-mono text-xs">{health.data.sbx_binary}</span>
              </Description>
            </DescriptionList>
          ) : null}
        </Panel>

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
                  <TRow key={cache.id}>
                    <TD className="font-medium">{cache.name}</TD>
                    <TD className="font-mono text-xs">{cache.host_path}</TD>
                    <TD className="font-mono text-xs">{cache.target_path}</TD>
                    <TD>{cache.read_only ? <Badge tone="warning">ro</Badge> : <Badge>rw</Badge>}</TD>
                    <TD>
                      <span className="flex flex-wrap gap-1.5">
                        {cache.auto_attach ? <Badge tone="accent">auto-attach</Badge> : null}
                        {cache.enabled ? null : <Badge tone="danger">disabled</Badge>}
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

        <Panel title="Notifications" description="Browser notifications for newly blocked hosts.">
          <div className="flex items-center gap-3">
            <Button onClick={requestNotificationPermission} disabled={notificationsEnabled}>
              Enable notifications
            </Button>
            <span className="text-xs text-muted">
              {notificationsEnabled ? "Enabled for this browser." : "Not enabled yet."}
            </span>
          </div>
        </Panel>
      </div>

      <CacheDialog
        open={editing !== null}
        cache={editing?.mode === "edit" ? editing.cache : null}
        busy={create.isPending || update.isPending}
        onClose={() => setEditing(null)}
        onSubmit={(input) => {
          if (editing?.mode === "edit") update.mutate({ id: editing.cache.id, input });
          else create.mutate(input);
        }}
      />

      <ConfirmDialog
        open={deleting !== null}
        title={`Delete cache ${deleting?.name ?? ""}?`}
        body="The definition is removed. Existing bind mounts stay until the sandbox restarts or you detach them."
        confirmLabel="Delete"
        busy={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting.id)}
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
  cache: CacheMount | null;
  busy: boolean;
  onSubmit: (input: CacheInput) => void;
  onClose: () => void;
}) {
  const [input, setInput] = useState<CacheInput>(EMPTY_CACHE);
  const [loadedId, setLoadedId] = useState<number | "new" | null>(null);

  const target = cache ? cache.id : ("new" as const);
  if (open && loadedId !== target) {
    setLoadedId(target);
    setInput(
      cache
        ? {
            name: cache.name,
            description: cache.description,
            host_path: cache.host_path,
            target_path: cache.target_path,
            read_only: cache.read_only,
            auto_attach: cache.auto_attach,
            enabled: cache.enabled,
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
              placeholder="~/.cache/go-build"
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
