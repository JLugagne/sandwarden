import { Chip, CommandLine, Description, DescriptionList, Panel } from "@/components/ui";
import { formatPort, formatTime } from "@/lib/format";
import type { SandboxDetail } from "@/types";

export function OverviewTab({ detail }: { detail: SandboxDetail }) {
  const sandbox = detail.sandbox;

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
        <div className="flex flex-col gap-2">
          <CommandLine command={sandbox.connect.run} />
          <CommandLine command={sandbox.connect.shell} />
        </div>
      </Panel>
    </div>
  );
}
