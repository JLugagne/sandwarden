import { useState } from "react";
import { api } from "@/api/client";
import { errorMessage, useApiMutation } from "@/hooks/useApiMutation";
import { useToasts } from "@/components/Toaster";
import {
  Badge,
  Button,
  CheckboxField,
  Chip,
  ConfirmDialog,
  Field,
  IconFolder,
  Input,
  Panel,
  TableWrap,
  TD,
  TH,
  TRow,
} from "@/components/ui";
import type { MountInfo, SandboxDetail } from "@/types";

interface MountRow {
  key: string;
  host: string;
  target: string;
  readOnly: boolean;
  mounted: boolean;
  profiles: string[];
  optedOut: boolean;
  mountIds: number[];
}

function effectiveTarget(host: string, target?: string): string {
  return target && target !== "" ? target : host;
}

function mountKey(host: string, target?: string): string {
  return `${host}\u0000${effectiveTarget(host, target)}`;
}

/** Merges runtime mounts and profile defaults into one row per host/target. */
function buildRows(detail: SandboxDetail): MountRow[] {
  const rows = new Map<string, MountRow>();
  for (const profileMount of detail.profile_mounts ?? []) {
    const target = effectiveTarget(profileMount.host_path, profileMount.target_path);
    rows.set(mountKey(profileMount.host_path, target), {
      key: mountKey(profileMount.host_path, target),
      host: profileMount.host_path,
      target,
      readOnly: profileMount.read_only,
      mounted: profileMount.attached,
      profiles: profileMount.profile_names ?? [],
      optedOut: profileMount.opted_out,
      mountIds: profileMount.profile_mount_ids ?? [],
    });
  }
  for (const mount of detail.mounts ?? []) {
    const target = effectiveTarget(mount.host_path, mount.container_target);
    const key = mountKey(mount.host_path, target);
    const existing = rows.get(key);
    if (existing) {
      existing.mounted = true;
      continue;
    }
    rows.set(key, {
      key,
      host: mount.host_path,
      target,
      readOnly: mount.read_only ?? false,
      mounted: true,
      profiles: [],
      optedOut: false,
      mountIds: [],
    });
  }
  return [...rows.values()].sort((left, right) =>
    left.host === right.host ? left.target.localeCompare(right.target) : left.host.localeCompare(right.host),
  );
}

export function MountsTab({ name, detail }: { name: string; detail: SandboxDetail }) {
  const toast = useToasts();
  const [path, setPath] = useState("");
  const [target, setTarget] = useState("");
  const [readOnly, setReadOnly] = useState(false);
  const [pendingRemove, setPendingRemove] = useState<MountInfo | null>(null);
  const [pendingDetach, setPendingDetach] = useState<MountRow | null>(null);

  const add = useApiMutation({
    mutationFn: () =>
      api.addMount(name, { path: path.trim(), target: target.trim() || undefined, read_only: readOnly }),
    success: `Mounted ${path.trim()}`,
    onSuccess: () => {
      setPath("");
      setTarget("");
      setReadOnly(false);
    },
  });

  const remove = useApiMutation({
    mutationFn: (mount: MountInfo) => api.removeMount(name, mount.host_path, mount.container_target || undefined),
    success: "Unmounted",
    onSuccess: () => setPendingRemove(null),
  });

  const detachProfile = useApiMutation({
    mutationFn: async (row: MountRow) => {
      for (const mountId of row.mountIds) await api.detachProfileMount(name, mountId);
    },
    success: "Profile mount detached",
    onSuccess: () => setPendingDetach(null),
  });

  const applyProfile = useApiMutation({
    mutationFn: async (row: MountRow) => {
      for (const mountId of row.mountIds) await api.applyProfileMount(name, mountId);
    },
    success: "Profile mount re-enabled",
  });

  async function pickFolder() {
    try {
      const picked = await api.fsPick(path || detail.sandbox.workspace || undefined);
      if (picked.path) setPath(picked.path);
    } catch (error) {
      toast.push({ tone: "warning", title: "Folder picker unavailable", body: errorMessage(error) });
    }
  }

  const rows = buildRows(detail);

  return (
    <div className="flex flex-col gap-4">
      <Panel
        title="Mounts"
        description="Runtime bind mounts (from `sbx inspect`) plus the defaults declared by the sandbox profiles."
        bodyClassName="p-0"
      >
        {!detail.sandbox.running ? (
          <p className="border-b border-border p-4 text-xs text-warning">
            The sandbox is stopped. Start it to manage runtime mounts; bind mounts do not survive a
            restart.
          </p>
        ) : null}
        {detail.mounts_error ? (
          <p className="border-b border-border p-4 text-xs text-danger">
            Could not read runtime mounts from sbx: {detail.mounts_error}
          </p>
        ) : null}
        {rows.length === 0 ? (
          <p className="p-4 text-sm text-muted">No runtime mount and no profile default.</p>
        ) : (
          <TableWrap className="rounded-none border-0 border-b-0">
            <thead>
              <tr>
                <TH>Host path</TH>
                <TH>Sandbox target</TH>
                <TH>Mode</TH>
                <TH>Source</TH>
                <TH>State</TH>
                <TH className="w-24" />
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => (
                <TRow key={row.key}>
                  <TD className="font-mono text-xs">{row.host}</TD>
                  <TD className="font-mono text-xs">{row.target}</TD>
                  <TD>{row.readOnly ? <Badge tone="warning">ro</Badge> : <Badge>rw</Badge>}</TD>
                  <TD>
                    {row.profiles.length === 0 ? (
                      <Badge tone="neutral">manual</Badge>
                    ) : (
                      <span className="flex flex-wrap gap-1">
                        {row.profiles.map((profile) => (
                          <Badge key={profile} tone="accent">
                            {profile}
                          </Badge>
                        ))}
                      </span>
                    )}
                  </TD>
                  <TD>
                    {row.mounted ? (
                      <Badge tone="success" dot>
                        mounted
                      </Badge>
                    ) : row.optedOut ? (
                      <Badge tone="neutral">detached</Badge>
                    ) : (
                      <Badge tone="warning" dot>
                        not mounted
                      </Badge>
                    )}
                  </TD>
                  <TD className="text-right">
                    {row.profiles.length === 0 ? (
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={!detail.sandbox.running}
                        onClick={() =>
                          setPendingRemove(
                            (detail.mounts ?? []).find(
                              (mount) =>
                                mount.host_path === row.host &&
                                effectiveTarget(mount.host_path, mount.container_target) === row.target,
                            ) ?? { host_path: row.host, container_target: row.target },
                          )
                        }
                      >
                        Unmount
                      </Button>
                    ) : row.optedOut ? (
                      <Button size="sm" variant="ghost" loading={applyProfile.isPending} onClick={() => applyProfile.mutate(row)}>
                        Apply
                      </Button>
                    ) : (
                      <Button size="sm" variant="ghost" onClick={() => setPendingDetach(row)}>
                        Detach
                      </Button>
                    )}
                  </TD>
                </TRow>
              ))}
            </tbody>
          </TableWrap>
        )}
      </Panel>

      <Panel title="Declared workspaces">
        <div className="flex flex-wrap gap-2">
          <Chip>
            {detail.sandbox.workspace || "—"} <span className="text-faint">workspace</span>
          </Chip>
          {(detail.additional_workspaces ?? []).map((workspace) => (
            <Chip key={workspace.dir}>
              {workspace.dir}
              {workspace.read_only ? <span className="text-faint">ro</span> : null}
            </Chip>
          ))}
        </div>
      </Panel>

      <Panel
        title="Add a mount"
        description="The sandbox must be running; the target path is created when missing."
      >
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Host path">
            <div className="flex items-center gap-2">
              <Input
                value={path}
                onChange={(event) => setPath(event.target.value)}
                placeholder="/absolute/host/path"
                className="flex-1 font-mono text-xs"
              />
              <Button
                size="icon"
                variant="ghost"
                title="Choose folder…"
                disabled={!detail.sandbox.running}
                onClick={() => void pickFolder()}
              >
                <IconFolder className="size-3.5" />
              </Button>
            </div>
          </Field>
          <Field label="Sandbox target (optional)" hint="Defaults to the same path as the host.">
            <Input
              value={target}
              onChange={(event) => setTarget(event.target.value)}
              placeholder="/absolute/sandbox/path"
              className="font-mono text-xs"
            />
          </Field>
        </div>
        <div className="mt-3 flex items-center gap-4">
          <CheckboxField label="read only" checked={readOnly} onChange={setReadOnly} />
          <Button
            variant="primary"
            onClick={() => add.mutate()}
            loading={add.isPending}
            disabled={!path.trim() || !detail.sandbox.running}
          >
            Mount
          </Button>
        </div>
      </Panel>

      <ConfirmDialog
        open={pendingRemove !== null}
        title="Unmount this path?"
        body={<span className="font-mono text-xs">{pendingRemove?.host_path}</span>}
        confirmLabel="Unmount"
        busy={remove.isPending}
        onConfirm={() => pendingRemove && remove.mutate(pendingRemove)}
        onClose={() => setPendingRemove(null)}
      />

      <ConfirmDialog
        open={pendingDetach !== null}
        title="Detach this profile mount?"
        body={
          <span>
            The bind is released and <span className="font-mono text-xs">{pendingDetach?.host}</span> is remembered as
            detached on this sandbox; reconcile will not re-apply it. It stays a default for the profile.
          </span>
        }
        confirmLabel="Detach"
        busy={detachProfile.isPending}
        onConfirm={() => pendingDetach && detachProfile.mutate(pendingDetach)}
        onClose={() => setPendingDetach(null)}
      />
    </div>
  );
}
