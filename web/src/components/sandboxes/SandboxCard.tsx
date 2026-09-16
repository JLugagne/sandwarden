import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { api } from "@/api/client";
import { useApiMutation } from "@/hooks/useApiMutation";
import { formatPort, formatTime, joinList } from "@/lib/format";
import {
  Badge,
  Button,
  Card,
  Chip,
  CommandLine,
  ConfirmDialog,
  IconChevronRight,
  IconPlay,
  IconStop,
  IconTrash,
  StatusBadge,
} from "@/components/ui";
import type { SandboxSummary } from "@/types";

export function SandboxCard({ sandbox }: { sandbox: SandboxSummary }) {
  const navigate = useNavigate();
  const [confirming, setConfirming] = useState(false);

  const start = useApiMutation({
    mutationFn: () => api.startSandbox(sandbox.name),
    success: `Starting ${sandbox.name}`,
  });
  const stop = useApiMutation({
    mutationFn: () => api.stopSandbox(sandbox.name),
    success: `Stopping ${sandbox.name}`,
  });
  const remove = useApiMutation({
    mutationFn: () => api.deleteSandbox(sandbox.name),
    success: `Deleted ${sandbox.name}`,
    onSuccess: () => setConfirming(false),
  });

  const busy = start.isPending || stop.isPending || remove.isPending;

  function openSettings() {
    navigate(`/sandboxes/${encodeURIComponent(sandbox.name)}`);
  }

  return (
    <>
      <Card
        role="link"
        tabIndex={0}
        aria-label={`Open ${sandbox.name} settings`}
        onClick={openSettings}
        onKeyDown={(event) => {
          if (event.key === "Enter" || event.key === " ") {
            event.preventDefault();
            openSettings();
          }
        }}
        className="flex cursor-pointer flex-col gap-3 transition-colors hover:border-accent/50"
      >
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-semibold">{sandbox.name}</span>
          <StatusBadge running={sandbox.running} status={sandbox.status} />
          {sandbox.agent ? <Badge>{sandbox.agent}</Badge> : null}
          {sandbox.daemon_profile ? <Badge tone="accent">profile {sandbox.daemon_profile}</Badge> : null}
          {sandbox.mount_policy_denied ? <Badge tone="danger">mount denied</Badge> : null}
          <div className="ml-auto flex items-center gap-1.5" onClick={(event) => event.stopPropagation()}>
            {sandbox.running ? (
              <Button size="icon" variant="outline" title="Stop" disabled={busy} onClick={() => stop.mutate()}>
                <IconStop className="size-3.5" />
              </Button>
            ) : (
              <Button size="icon" variant="outline" title="Start" disabled={busy} onClick={() => start.mutate()}>
                <IconPlay className="size-3.5" />
              </Button>
            )}
            <Button size="icon" variant="ghost" title="Delete" disabled={busy} onClick={() => setConfirming(true)}>
              <IconTrash className="size-3.5 text-danger" />
            </Button>
          </div>
        </div>

        <div className="grid gap-1 text-xs text-muted">
          <div className="truncate font-mono" title={sandbox.workspace}>
            {sandbox.workspace || "no workspace"}
          </div>
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
            <span>created {formatTime(sandbox.created_at)}</span>
            {sandbox.stopped_at ? <span>stopped {formatTime(sandbox.stopped_at)}</span> : null}
          </div>
        </div>

        {sandbox.ports && sandbox.ports.length > 0 ? (
          <div className="flex flex-wrap gap-1.5">
            {sandbox.ports.map((port, index) => (
              <Chip key={index}>{formatPort(port.host_ip, port.host_port, port.sandbox_port, port.protocol)}</Chip>
            ))}
          </div>
        ) : null}

        {sandbox.profiles && sandbox.profiles.length > 0 ? (
          <div className="flex flex-wrap items-center gap-1.5">
            <span className="text-2xs text-faint">profiles</span>
            {sandbox.profiles.map((profile) => (
              <Badge key={profile} tone="accent">
                {profile}
              </Badge>
            ))}
          </div>
        ) : null}

        <div className="flex flex-col gap-1.5" onClick={(event) => event.stopPropagation()}>
          <CommandLine command={sandbox.connect.run} />
          <CommandLine command={sandbox.connect.shell} />
        </div>

        <div className="flex items-center justify-between border-t border-border pt-3">
          <span className="text-2xs text-faint">mounts · caches · profiles · secrets · traffic · terminal</span>
          <Button variant="outline" size="sm" onClick={openSettings}>
            Settings
            <IconChevronRight className="size-3" />
          </Button>
        </div>
      </Card>

      <ConfirmDialog
        open={confirming}
        title={`Delete ${sandbox.name}?`}
        body={
          <>
            This removes the sandbox and its runtime. Assigned profiles are unapplied first.
            {sandbox.profiles && sandbox.profiles.length > 0 ? (
              <span className="mt-2 block text-xs text-faint">Profiles: {joinList(sandbox.profiles)}</span>
            ) : null}
          </>
        }
        confirmLabel="Delete"
        busy={remove.isPending}
        onConfirm={() => remove.mutate()}
        onClose={() => setConfirming(false)}
      />
    </>
  );
}
