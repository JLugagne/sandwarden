import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { api } from "@/api/client";
import { useApiMutation } from "@/hooks/useApiMutation";
import { queryKeys } from "@/store/realtime";
import {
  Badge,
  Button,
  ConfirmDialog,
  EmptyState,
  Panel,
  Select,
  Spinner,
  TableWrap,
  TD,
  TH,
  TRow,
} from "@/components/ui";
import type { SandboxCache, SandboxDetail } from "@/types";

export function CachesTab({ name, detail }: { name: string; detail: SandboxDetail }) {
  const caches = useQuery({ queryKey: queryKeys.caches, queryFn: api.caches });
  const [selected, setSelected] = useState("");
  const [detaching, setDetaching] = useState<SandboxCache | null>(null);

  const attach = useApiMutation({
    mutationFn: (cacheId: number) => api.attachCache(name, cacheId),
    success: "Cache attached",
    onSuccess: () => setSelected(""),
  });
  const detach = useApiMutation({
    mutationFn: (cacheId: number) => api.detachCache(name, cacheId),
    success: "Cache detached",
    onSuccess: () => setDetaching(null),
  });
  const reapply = useApiMutation({
    mutationFn: () => api.reapplyCaches(name),
    onSuccess: (result) => {
      if (result.errors && result.errors.length > 0) {
        return undefined;
      }
      return result.applied > 0 ? `Re-mounted ${result.applied} cache(s)` : "All caches already mounted";
    },
  });

  const assigned = detail.caches ?? [];
  const assignedIds = new Set(assigned.map((cache) => cache.id));
  const available = (caches.data ?? []).filter((cache) => !assignedIds.has(cache.id) && cache.enabled);
  const drift = assigned.filter((cache) => cache.enabled && !cache.attached);

  return (
    <div className="flex flex-col gap-4">
      <Panel
        title="Shared caches"
        description="Host directories bind-mounted into this sandbox (Go module/build caches, npm, pnpm, yarn). The same host directory can be shared by several sandboxes."
        actions={
          <>
            <Button
              size="sm"
              variant="ghost"
              disabled={!detail.sandbox.running}
              loading={reapply.isPending}
              onClick={() => reapply.mutate()}
            >
              Reapply
            </Button>
            <Link to="/settings" className="text-xs text-accent hover:underline">
              Configure caches
            </Link>
          </>
        }
        bodyClassName="p-0"
      >
        {!detail.sandbox.running ? (
          <p className="border-b border-border p-4 text-xs text-warning">
            The sandbox is stopped. Start it to attach caches; desired caches are re-applied automatically on start.
          </p>
        ) : drift.length > 0 ? (
          <p className="border-b border-border p-4 text-xs text-warning">
            {drift.length} cache(s) are configured but not mounted in this sandbox. Use Reapply.
          </p>
        ) : null}

        {assigned.length === 0 ? (
          <p className="p-4 text-sm text-muted">No cache attached to this sandbox.</p>
        ) : (
          <TableWrap className="border-0">
            <thead>
              <tr>
                <TH>Cache</TH>
                <TH>Host → sandbox</TH>
                <TH>Mode</TH>
                <TH>State</TH>
                <TH className="w-24" />
              </tr>
            </thead>
            <tbody>
              {assigned.map((cache) => (
                <TRow key={cache.id}>
                  <TD className="font-medium">
                    {cache.name}
                    {cache.auto_attach ? <Badge className="ml-2">auto</Badge> : null}
                  </TD>
                  <TD className="font-mono text-xs">
                    {cache.host_path}
                    <span className="text-faint"> → </span>
                    {cache.target_path}
                  </TD>
                  <TD>{cache.read_only ? <Badge tone="warning">ro</Badge> : <Badge>rw</Badge>}</TD>
                  <TD>
                    {!cache.enabled ? (
                      <Badge tone="neutral">disabled</Badge>
                    ) : cache.attached ? (
                      <Badge tone="success" dot>
                        mounted
                      </Badge>
                    ) : (
                      <Badge tone="warning" dot>
                        not mounted
                      </Badge>
                    )}
                  </TD>
                  <TD className="text-right">
                    <Button size="sm" variant="ghost" onClick={() => setDetaching(cache)}>
                      Detach
                    </Button>
                  </TD>
                </TRow>
              ))}
            </tbody>
          </TableWrap>
        )}
      </Panel>

      <Panel title="Attach a cache" description="Caches are defined in Settings and can be shared across sandboxes.">
        {caches.isLoading ? (
          <Spinner />
        ) : available.length === 0 ? (
          <EmptyState
            title="No cache available to attach."
            description="Create one in Settings first, or all configured caches are already assigned."
          />
        ) : (
          <div className="flex items-center gap-2">
            <Select value={selected} onChange={(event) => setSelected(event.target.value)} className="max-w-xs">
              <option value="">Choose a cache…</option>
              {available.map((cache) => (
                <option key={cache.id} value={cache.id}>
                  {cache.name} ({cache.host_path})
                </option>
              ))}
            </Select>
            <Button
              variant="primary"
              disabled={!selected || !detail.sandbox.running}
              loading={attach.isPending}
              onClick={() => attach.mutate(Number(selected))}
            >
              Attach
            </Button>
          </div>
        )}
      </Panel>

      <ConfirmDialog
        open={detaching !== null}
        title={`Detach ${detaching?.name ?? "cache"}?`}
        body="The desired state is removed and the bind is released if the sandbox is running."
        confirmLabel="Detach"
        busy={detach.isPending}
        onConfirm={() => detaching && detach.mutate(detaching.id)}
        onClose={() => setDetaching(null)}
      />
    </div>
  );
}
