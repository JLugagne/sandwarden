import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "@/api/client";
import { queryKeys } from "@/store/realtime";
import { CreateSandboxDialog } from "@/components/sandboxes/CreateSandboxDialog";
import { SandboxCard } from "@/components/sandboxes/SandboxCard";
import { Button, EmptyState, ErrorNote, IconPlus, PageHeader, Spinner } from "@/components/ui";

export function SandboxesPage() {
  const [creating, setCreating] = useState(false);
  const sandboxes = useQuery({ queryKey: queryKeys.sandboxes, queryFn: api.listSandboxes });
  const health = useQuery({ queryKey: queryKeys.health, queryFn: api.health, retry: 0, staleTime: 30_000 });

  const rows = sandboxes.data ?? [];

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
        <div className="grid gap-4 lg:grid-cols-2">
          {rows.map((sandbox) => (
            <SandboxCard key={sandbox.name} sandbox={sandbox} />
          ))}
        </div>
      )}

      <CreateSandboxDialog open={creating} onClose={() => setCreating(false)} />
    </>
  );
}
