import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { api } from "@/api/client";
import { useApiMutation } from "@/hooks/useApiMutation";
import { useConfigStaleness } from "@/hooks/useConfigStaleness";
import { cn } from "@/lib/cn";
import { staleNames } from "@/lib/config";
import { formatBytes, formatPort, formatTime, joinList } from "@/lib/format";
import { StaleBadge } from "@/components/StaleBadge";
import {
  Badge,
  Button,
  Card,
  CheckboxField,
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
  const [purgeConfig, setPurgeConfig] = useState(false);
  const staleness = useConfigStaleness();
  const changedOnDisk = staleNames(staleness.data, "sandbox").has(sandbox.name);

  const start = useApiMutation({
    mutationFn: () => api.startSandbox(sandbox.name),
    success: `Starting ${sandbox.name}`,
  });
  const stop = useApiMutation({
    mutationFn: () => api.stopSandbox(sandbox.name),
    success: `Stopping ${sandbox.name}`,
  });
  const remove = useApiMutation({
    mutationFn: (purge: boolean) => api.deleteSandbox(sandbox.name, true, purge),
    success: `Deleted ${sandbox.name}`,
    onSuccess: () => setConfirming(false),
  });

  const busy = start.isPending || stop.isPending || remove.isPending;

  // Resource indicators stay visible whatever the state so a stopped sandbox
  // is recognisable at a glance; they grey out until live samples exist.
  const hasSample = sandbox.memory_total_bytes > 0;
  const statsDisabled = !sandbox.running || !hasSample;
  const cpuTitle = !sandbox.running
    ? "Sandbox is stopped"
    : hasSample
      ? `${sandbox.cpu_percent.toFixed(1)}% of the sandbox CPUs`
      : "Waiting for the first sample";
  const memoryTitle = !sandbox.running
    ? "Sandbox is stopped"
    : hasSample
      ? "Memory used by the sandbox"
      : "Waiting for the first sample";

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
          {changedOnDisk ? <StaleBadge /> : null}
          {sandbox.agent ? <Badge>{sandbox.agent}</Badge> : null}
          {sandbox.daemon_profile ? <Badge tone="accent">profile {sandbox.daemon_profile}</Badge> : null}
          {sandbox.mount_policy_denied ? <Badge tone="danger">mount denied</Badge> : null}
          {sandbox.incomplete ? (
            <span title="Imported without its original create parameters: CPU, memory and env cannot be restored by a recreate.">
              <Badge tone="warning">incomplete</Badge>
            </span>
          ) : null}
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

        <div className="flex flex-wrap items-center gap-x-4 gap-y-1.5">
          <ResourceMeter
            label="CPU"
            value={sandbox.cpu_percent}
            max={100}
            disabled={statsDisabled}
            display={hasSample ? `${Math.round(sandbox.cpu_percent)}%` : "—"}
            title={cpuTitle}
          />
          <ResourceMeter
            label="MEM"
            value={sandbox.memory_used_bytes}
            max={sandbox.memory_total_bytes}
            disabled={statsDisabled}
            display={
              hasSample
                ? `${formatBytes(sandbox.memory_used_bytes)} / ${formatBytes(sandbox.memory_total_bytes)}`
                : "—"
            }
            title={memoryTitle}
            warnFrom={85}
          />
        </div>

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
          <CommandLine command={sandbox.connect.run} openDir={sandbox.workspace} />
          <CommandLine command={sandbox.connect.shell} openDir={sandbox.workspace} />
        </div>

        {sandbox.ports && sandbox.ports.length > 0 ? (
          <div className="flex flex-wrap gap-1.5">
            {sandbox.ports.map((port, index) => (
              <Chip key={index}>{formatPort(port.host_ip, port.host_port, port.sandbox_port, port.protocol)}</Chip>
            ))}
          </div>
        ) : null}

        <div className="mt-auto flex items-center justify-end border-t border-border pt-3">
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
            <span className="block">
              This removes the sandbox and its runtime. Open sessions are disconnected and assigned
              profiles are unapplied first.
            </span>
            <CheckboxField
              label="also delete the config directory"
              hint="Without it the sandbox files are kept and the sandbox can be recreated later."
              checked={purgeConfig}
              onChange={setPurgeConfig}
            />
            {sandbox.profiles && sandbox.profiles.length > 0 ? (
              <span className="mt-2 block text-xs text-faint">Profiles: {joinList(sandbox.profiles)}</span>
            ) : null}
          </>
        }
        confirmLabel="Delete"
        busy={remove.isPending}
        onConfirm={() => remove.mutate(purgeConfig)}
        onClose={() => {
          setConfirming(false);
          setPurgeConfig(false);
        }}
      />
    </>
  );
}

/** Compact usage bar used for the CPU and memory indicators of a sandbox. */
function ResourceMeter({
  label,
  value,
  max,
  display,
  title,
  warnFrom,
  disabled = false,
}: {
  label: string;
  value: number;
  max: number;
  display: string;
  title: string;
  warnFrom?: number;
  disabled?: boolean;
}) {
  const percent = max > 0 ? Math.min(100, Math.max(0, (value / max) * 100)) : 0;
  const tone = disabled
    ? "bg-border-strong"
    : warnFrom !== undefined && percent >= warnFrom
      ? "bg-warning"
      : "bg-accent";
  return (
    <span className={cn("inline-flex items-center gap-1.5", disabled && "opacity-60")} title={title}>
      <span className="text-2xs font-medium text-faint">{label}</span>
      <span className="h-1.5 w-20 overflow-hidden rounded-full bg-hover">
        <span
          data-meter-fill
          className={cn("block h-full rounded-full transition-[width] duration-500", tone)}
          style={{ width: `${disabled ? 0 : percent}%` }}
        />
      </span>
      <span className={cn("text-2xs tabular-nums", disabled ? "text-faint" : "text-muted")}>{display}</span>
    </span>
  );
}
