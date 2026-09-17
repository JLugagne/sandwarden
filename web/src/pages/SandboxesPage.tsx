import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "@/api/client";
import { useApiMutation } from "@/hooks/useApiMutation";
import { queryKeys } from "@/store/realtime";
import { CreateSandboxDialog } from "@/components/sandboxes/CreateSandboxDialog";
import { SandboxCard } from "@/components/sandboxes/SandboxCard";
import { Button, EmptyState, ErrorNote, IconPlus, PageHeader, Spinner } from "@/components/ui";
import type { SandboxSummary } from "@/types";

export function SandboxesPage() {
  const [creating, setCreating] = useState(false);
  const sandboxes = useQuery({ queryKey: queryKeys.sandboxes, queryFn: api.listSandboxes });
  const health = useQuery({ queryKey: queryKeys.health, queryFn: api.health, retry: 0, staleTime: 30_000 });

  const startDaemon = useApiMutation({
    mutationFn: () => api.startDaemon(),
    success: "sandboxd started",
    invalidate: [queryKeys.health, queryKeys.sandboxes],
  });

  const rows = sandboxes.data ?? [];
  const started = rows.filter((sandbox) => sandbox.running);
  const stopped = rows.filter((sandbox) => !sandbox.running);

  return (
    <>
      <PageHeader
        title="Sandboxes"
        subtitle={
          health.data ? (
            <span className="font-mono text-xs">
              daemon {health.data.socket} · cli {health.data.sbx_binary}
            </span>
          ) : undefined
        }
        actions={
          <Button variant="primary" size="md" onClick={() => setCreating(true)}>
            <IconPlus /> New sandbox
          </Button>
        }
      />

      {health.data && !health.data.daemon_running ? (
        <div className="mb-4 flex flex-wrap items-center gap-3 rounded-md border border-warning/40 bg-warning-soft px-3 py-2 text-sm">
          <span className="flex-1 text-warning">
            {health.data.daemon_status && health.data.daemon_status !== "stopped"
              ? health.data.daemon_status
              : "sandboxd is not running."}
          </span>
          <Button variant="primary" size="sm" loading={startDaemon.isPending} onClick={() => startDaemon.mutate()}>
            Start sandboxd
          </Button>
        </div>
      ) : null}

      <ErrorNote error={sandboxes.error} className="mb-4" />

      {sandboxes.isLoading ? (
        <div className="flex justify-center py-16">
          <Spinner />
        </div>
      ) : rows.length === 0 ? (
        <EmptyState
          title="No sandboxes yet"
          description="Create one to get started; every sbx create option is exposed in the form."
          action={
            <Button variant="primary" onClick={() => setCreating(true)}>
              <IconPlus /> New sandbox
            </Button>
          }
        />
      ) : (
        <div className="flex flex-col gap-6">
          <SandboxSection title="Started" sandboxes={started} />
          <SandboxSection title="Stopped" sandboxes={stopped} />
        </div>
      )}

      <CreateSandboxDialog open={creating} onClose={() => setCreating(false)} />
    </>
  );
}

/**
 * One run-state group of sandbox cards. An empty group renders nothing, so a
 * page of only-running sandboxes does not carry a bare "Stopped" heading.
 */
function SandboxSection({ title, sandboxes }: { title: string; sandboxes: SandboxSummary[] }) {
  if (sandboxes.length === 0) {
    return null;
  }
  return (
    <section>
      <h2 className="mb-2 text-xs font-semibold tracking-wide text-faint uppercase">
        {title} <span className="text-faint tabular-nums">({sandboxes.length})</span>
      </h2>
      <div className="grid gap-4 lg:grid-cols-2">
        {sandboxes.map((sandbox) => (
          <SandboxCard key={sandbox.name} sandbox={sandbox} />
        ))}
      </div>
    </section>
  );
}
