import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import { useApiMutation } from "@/hooks/useApiMutation";
import { queryKeys } from "@/store/realtime";
import { formatTime } from "@/lib/format";
import {
  Badge,
  Button,
  DecisionBadge,
  EmptyState,
  Input,
  Panel,
  Select,
  Spinner,
  TableWrap,
  TD,
  TH,
  TRow,
} from "@/components/ui";
import type { LogEntry, PolicyRule } from "@/types";

export function TrafficTab({ name }: { name: string }) {
  const rules = useQuery({ queryKey: queryKeys.sandboxPolicy(name), queryFn: () => api.sandboxPolicy(name) });
  const traffic = useQuery({ queryKey: queryKeys.traffic, queryFn: api.traffic });

  const [decision, setDecision] = useState("allow");
  const [host, setHost] = useState("");

  const apply = useApiMutation({
    mutationFn: () => api.sandboxPolicyAction(name, decision, [host.trim()]),
    success: `Rule applied to ${name}`,
    onSuccess: () => setHost(""),
  });
  const removeRule = useApiMutation({
    mutationFn: (rule: PolicyRule) =>
      api.policyAction({ action: "remove-id", id: rule.id, sandbox_id: rule.sandbox_id || undefined }),
    success: "Rule removed",
    invalidate: [queryKeys.sandboxPolicy(name), queryKeys.policyRules],
  });
  const allowHost = useApiMutation({
    mutationFn: (entry: LogEntry) => api.sandboxPolicyAction(name, "allow", [entry.host]),
    success: (_, entry) => `Allowed ${entry.host} in ${name}`,
  });

  const log = useMemo(
    () => ({
      blocked: (traffic.data?.blocked_hosts ?? []).filter((entry) => entry.vm_name === name),
      allowed: (traffic.data?.allowed_hosts ?? []).filter((entry) => entry.vm_name === name),
    }),
    [traffic.data, name],
  );

  const ruleRows = rules.data ?? [];

  return (
    <div className="flex flex-col gap-4">
      <Panel
        title="Add a rule for this sandbox"
        description="Scoped rules only affect this sandbox; they cannot widen global policy."
      >
        <div className="flex flex-wrap items-center gap-2">
          <Select value={decision} onChange={(event) => setDecision(event.target.value)} className="w-28">
            <option value="allow">allow</option>
            <option value="deny">deny</option>
          </Select>
          <Input
            value={host}
            onChange={(event) => setHost(event.target.value)}
            placeholder="api.example.com or *.example.com"
            className="max-w-md flex-1 font-mono text-xs"
            onKeyDown={(event) => {
              if (event.key === "Enter" && host.trim()) apply.mutate();
            }}
          />
          <Button variant="primary" disabled={!host.trim()} loading={apply.isPending} onClick={() => apply.mutate()}>
            Apply
          </Button>
        </div>
      </Panel>

      <Panel
        title="Rules applying here"
        description="Global rules plus rules scoped to this sandbox, as resolved by the daemon."
        bodyClassName="p-0"
      >
        {rules.isLoading ? (
          <div className="flex justify-center p-6">
            <Spinner />
          </div>
        ) : ruleRows.length === 0 ? (
          <p className="p-4 text-sm text-muted">No rules apply to this sandbox.</p>
        ) : (
          <TableWrap className="border-0">
            <thead>
              <tr>
                <TH>Decision</TH>
                <TH>Resources</TH>
                <TH>Scope</TH>
                <TH>Origin</TH>
                <TH>Status</TH>
                <TH className="w-24" />
              </tr>
            </thead>
            <tbody>
              {ruleRows.map((rule) => (
                <TRow key={rule.id}>
                  <TD>
                    <DecisionBadge decision={rule.decision} />
                  </TD>
                  <TD className="font-mono text-xs">{(rule.resources ?? []).join(", ")}</TD>
                  <TD>
                    <span className="flex gap-1.5">
                      <Badge tone={rule.scope === "global" ? "accent" : "neutral"}>{rule.scope || "local"}</Badge>
                      {rule.applies_to ? <Badge>{rule.applies_to}</Badge> : null}
                    </span>
                  </TD>
                  <TD className="text-xs text-muted">{rule.origin || "—"}</TD>
                  <TD className="text-xs text-muted">{rule.status || "—"}</TD>
                  <TD className="text-right">
                    {rule.editable ? (
                      <Button
                        size="sm"
                        variant="ghost"
                        loading={removeRule.isPending && removeRule.variables?.id === rule.id}
                        onClick={() => removeRule.mutate(rule)}
                      >
                        Remove
                      </Button>
                    ) : null}
                  </TD>
                </TRow>
              ))}
            </tbody>
          </TableWrap>
        )}
      </Panel>

      <Panel
        title="Proxy log"
        description="Traffic seen by the egress proxy for this sandbox."
        actions={
          <Button size="sm" variant="ghost" onClick={() => void traffic.refetch()} loading={traffic.isFetching}>
            Refresh
          </Button>
        }
        bodyClassName="p-0"
      >
        <div className="border-b border-border p-4">
          <h3 className="mb-2 text-xs font-semibold tracking-wide text-faint uppercase">Blocked</h3>
          <LogTable entries={log.blocked} onAllow={(entry) => allowHost.mutate(entry)} busyHost={allowHost.variables?.host} />
        </div>
        <div className="p-4">
          <h3 className="mb-2 text-xs font-semibold tracking-wide text-faint uppercase">Allowed</h3>
          <LogTable entries={log.allowed} />
        </div>
      </Panel>
    </div>
  );
}

function LogTable({
  entries,
  onAllow,
  busyHost,
}: {
  entries: LogEntry[];
  onAllow?: (entry: LogEntry) => void;
  busyHost?: string;
}) {
  if (entries.length === 0) {
    return <EmptyState title="Nothing recorded." className="py-6" />;
  }
  return (
    <TableWrap className="border-0">
      <thead>
        <tr>
          <TH>Host</TH>
          <TH>Proxy</TH>
          <TH>Rule</TH>
          <TH>Count</TH>
          <TH>Last seen</TH>
          {onAllow ? <TH className="w-20" /> : null}
        </tr>
      </thead>
      <tbody>
        {entries.map((entry) => (
          <TRow key={entry.host}>
            <TD className="font-mono text-xs">{entry.host}</TD>
            <TD className="text-xs text-muted">{entry.proxy_type}</TD>
            <TD className="font-mono text-xs text-muted">{entry.rule}</TD>
            <TD className="text-xs">{entry.count_since}</TD>
            <TD className="text-xs text-muted">{formatTime(entry.last_seen)}</TD>
            {onAllow ? (
              <TD className="text-right">
                <Button
                  size="sm"
                  variant="ghost"
                  loading={busyHost === entry.host}
                  onClick={() => onAllow(entry)}
                >
                  Allow
                </Button>
              </TD>
            ) : null}
          </TRow>
        ))}
      </tbody>
    </TableWrap>
  );
}
