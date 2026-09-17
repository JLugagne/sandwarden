import { useEffect, useState } from "react";
import { api } from "@/api/client";
import { useApiMutation } from "@/hooks/useApiMutation";
import { Button, Chip, CommandLine, Description, DescriptionList, Field, Input, Panel } from "@/components/ui";
import { formatPort, formatTime } from "@/lib/format";
import type { SandboxDetail } from "@/types";

export function OverviewTab({ name, detail }: { name: string; detail: SandboxDetail }) {
  const sandbox = detail.sandbox;
  const [runArgs, setRunArgs] = useState(sandbox.run_args);
  useEffect(() => setRunArgs(sandbox.run_args), [sandbox.run_args]);

  const saveRunArgs = useApiMutation({
    mutationFn: (args: string) => api.setSandboxRunArgs(name, args),
    success: "Run arguments updated",
  });

  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <Panel title="Runtime">
        <DescriptionList>
          <Description label="Workspace">
            <span className="font-mono text-xs">{sandbox.workspace || "—"}</span>
          </Description>
          <Description label="Additional workspaces">
            {detail.additional_workspaces && detail.additional_workspaces.length > 0 ? (
              <span className="flex flex-wrap gap-1.5">
                {detail.additional_workspaces.map((workspace) => (
                  <Chip key={workspace.dir}>
                    {workspace.dir}
                    {workspace.read_only ? <span className="text-faint">ro</span> : null}
                  </Chip>
                ))}
              </span>
            ) : (
              <span className="text-faint">—</span>
            )}
          </Description>
          <Description label="Published ports">
            {sandbox.ports && sandbox.ports.length > 0 ? (
              <span className="flex flex-wrap gap-1.5">
                {sandbox.ports.map((port, index) => (
                  <Chip key={index}>{formatPort(port.host_ip, port.host_port, port.sandbox_port, port.protocol)}</Chip>
                ))}
              </span>
            ) : (
              <span className="text-faint">—</span>
            )}
          </Description>
          <Description label="Created">{formatTime(sandbox.created_at)}</Description>
          <Description label="Stopped">{sandbox.stopped_at ? formatTime(sandbox.stopped_at) : "—"}</Description>
          <Description label="Image">
            <span className="font-mono text-xs">{detail.image || "—"}</span>
          </Description>
          <Description label="Kits">
            {detail.kits && detail.kits.length > 0 ? (
              <span className="flex flex-wrap gap-1.5">
                {detail.kits.map((kit) => (
                  <Chip key={kit}>{kit}</Chip>
                ))}
              </span>
            ) : (
              <span className="text-faint">none</span>
            )}
          </Description>
          <Description label="sbx profile">{sandbox.daemon_profile ?? "—"}</Description>
          <Description label="Mount policy">
            {sandbox.mount_policy_denied ? (
              <span className="text-danger">denied</span>
            ) : (
              <span className="text-muted">allowed</span>
            )}
          </Description>
        </DescriptionList>
      </Panel>

      <Panel title="Connect" description="Run these on the host, not inside the sandbox.">
        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-2">
            <CommandLine command={sandbox.connect.run} openDir={sandbox.workspace} />
            <CommandLine command={sandbox.connect.shell} openDir={sandbox.workspace} />
          </div>
          <Field label="Custom run arguments" hint="Appended after -- to the run command above.">
            <div className="flex gap-2">
              <Input
                value={runArgs}
                onChange={(event) => setRunArgs(event.target.value)}
                placeholder="--auto"
                className="font-mono"
              />
              <Button
                size="sm"
                variant="outline"
                disabled={runArgs === sandbox.run_args}
                loading={saveRunArgs.isPending}
                onClick={() => saveRunArgs.mutate(runArgs)}
              >
                Save
              </Button>
            </div>
          </Field>
        </div>
      </Panel>
    </div>
  );
}
