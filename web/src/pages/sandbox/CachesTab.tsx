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
import { useToasts } from "@/components/Toaster";
import type { SandboxCache, SandboxDetail } from "@/types";

export function CachesTab({ name, detail }: { name: string; detail: SandboxDetail }) {
  const caches = useQuery({ queryKey: queryKeys.caches, queryFn: api.caches });
  const [selected, setSelected] = useState("");
  const [detaching, setDetaching] = useState<SandboxCache | null>(null);
  const toast = useToasts();

  const attach = useApiMutation({
    mutationFn: (cacheSlug: string) => api.attachCache(name, cacheSlug),
    success: "Cache attached",
    onSuccess: () => setSelected(""),
  });
  const detach = useApiMutation({
    mutationFn: async (cache: SandboxCache) => {
      if (cache.direct) await api.detachCache(name, cache.slug);
      if ((cache.profiles ?? []).length > 0) await api.detachProfileCache(name, cache.slug);
    },
    success: "Cache detached",
    onSuccess: () => setDetaching(null),
  });
  const reapply = useApiMutation({
    mutationFn: () => api.reapplyCaches(name),
    onSuccess: (result) => {
      if (result.errors && result.errors.length > 0) {
        toast.push({ tone: "danger", title: "Cache re-apply failed", body: result.errors.join("; ") });
        return undefined;
      }
      return result.applied > 0 ? `Re-mounted ${result.applied} cache(s)` : "All caches already mounted";
    },
  });
  const applyProfile = useApiMutation({
    mutationFn: (cacheSlug: string) => api.applyProfileCache(name, cacheSlug),
    success: "Profile cache re-enabled",
  });

  const assigned = detail.caches ?? [];
  const assignedSlugs = new Set(assigned.map((cache) => cache.slug));
  const available = (caches.data ?? []).filter(
    (cache) => !assignedSlugs.has(cache.slug) && cache.enabled !== false,
  );
  const drift = assigned.filter((cache) => cache.enabled !== false && !cache.attached && !cache.opted_out);

  return (
    <div className="flex flex-col gap-4">
      <Panel
        title="Shared caches"
        description="Host directories bind-mounted into this sandbox (Go module/build caches, npm, pnpm, yarn). The same host directory can be shared by several sandboxes, and profiles can attach caches by default."
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
                <TH>Source</TH>
                <TH>State</TH>
                <TH className="w-24" />
              </tr>
            </thead>
            <tbody>
              {assigned.map((cache) => {
                const profiles = cache.profiles ?? [];
                return (
                  <TRow key={cache.slug}>
                    <TD className="font-medium">
                      {cache.name}
                      {cache.auto_attach !== false ? <Badge className="ml-2">auto</Badge> : null}
                    </TD>
                    <TD className="font-mono text-xs">
                      {cache.host_path}
                      <span className="text-faint"> → </span>
                      {cache.target_path}
                    </TD>
                    <TD>{cache.read_only ? <Badge tone="warning">ro</Badge> : <Badge>rw</Badge>}</TD>
                    <TD>
                      <span className="flex flex-wrap gap-1">
                        {cache.direct ? <Badge tone="neutral">direct</Badge> : null}
                        {profiles.map((profile) => (
                          <Badge key={profile} tone="accent">
                            {profile}
                          </Badge>
                        ))}
                      </span>
                    </TD>
                    <TD>
                      {cache.enabled === false ? (
                        <Badge tone="neutral">disabled</Badge>
                      ) : cache.attached ? (
                        <Badge tone="success" dot>
                          mounted
                        </Badge>
                      ) : cache.opted_out ? (
                        <Badge tone="neutral">detached</Badge>
                      ) : (
                        <Badge tone="warning" dot>
                          not mounted
                        </Badge>
                      )}
                    </TD>
                    <TD className="text-right">
                      {cache.opted_out && !cache.direct ? (
                        <Button
                          size="sm"
                          variant="ghost"
                          loading={applyProfile.isPending}
                          onClick={() => applyProfile.mutate(cache.slug)}
                        >
                          Apply
                        </Button>
                      ) : (
                        <Button size="sm" variant="ghost" onClick={() => setDetaching(cache)}>
                          Detach
                        </Button>
                      )}
                    </TD>
                  </TRow>
                );
              })}
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
                <option key={cache.slug} value={cache.slug}>
                  {cache.name} ({cache.host_path})
                </option>
              ))}
            </Select>
            <Button
              variant="primary"
              disabled={!selected || !detail.sandbox.running}
              loading={attach.isPending}
              onClick={() => attach.mutate(selected)}
            >
              Attach
            </Button>
          </div>
        )}
      </Panel>

      <ConfirmDialog
        open={detaching !== null}
        title={`Detach ${detaching?.name ?? "cache"}?`}
        body="Direct attachments are removed, profile defaults are marked detached on this sandbox and the bind is released when the sandbox is running."
        confirmLabel="Detach"
        busy={detach.isPending}
        onConfirm={() => detaching && detach.mutate(detaching)}
        onClose={() => setDetaching(null)}
      />
    </div>
  );
}
