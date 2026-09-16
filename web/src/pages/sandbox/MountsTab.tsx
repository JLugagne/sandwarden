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

export function MountsTab({ name, detail }: { name: string; detail: SandboxDetail }) {
  const toast = useToasts();
  const [path, setPath] = useState("");
  const [target, setTarget] = useState("");
  const [readOnly, setReadOnly] = useState(false);
  const [pendingRemove, setPendingRemove] = useState<MountInfo | null>(null);

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

  async function pickFolder() {
    try {
      const picked = await api.fsPick(path || detail.sandbox.workspace || undefined);
      setPath(picked.path);
    } catch (error) {
      toast.push({ tone: "warning", title: "Folder picker unavailable", body: errorMessage(error) });
    }
  }

  const mounts = detail.mounts ?? [];

  return (
    <div className="flex flex-col gap-4">
      <Panel
        title="Runtime mounts"
        description="Bind mounts active in the running sandbox (from `sbx inspect`)."
        bodyClassName="p-0"
      >
        {mounts.length === 0 ? (
          <p className="p-4 text-sm text-muted">No runtime mounts.</p>
        ) : (
          <TableWrap className="rounded-none border-0 border-b-0">
            <thead>
              <tr>
                <TH>Host path</TH>
                <TH>Sandbox target</TH>
                <TH>Mode</TH>
                <TH className="w-20" />
              </tr>
            </thead>
            <tbody>
              {mounts.map((mount) => (
                <TRow key={`${mount.host_path}:${mount.container_target ?? ""}`}>
                  <TD className="font-mono text-xs">{mount.host_path}</TD>
                  <TD className="font-mono text-xs">{mount.container_target || mount.host_path}</TD>
                  <TD>{mount.read_only ? <Badge tone="warning">ro</Badge> : <Badge>rw</Badge>}</TD>
                  <TD className="text-right">
                    <Button size="sm" variant="ghost" onClick={() => setPendingRemove(mount)}>
                      Unmount
                    </Button>
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
              <Button size="icon" variant="ghost" title="Choose folder…" onClick={() => void pickFolder()}>
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
          <Button variant="primary" onClick={() => add.mutate()} loading={add.isPending} disabled={!path.trim()}>
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
    </div>
  );
}
