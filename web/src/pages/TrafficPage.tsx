import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, useSearchParams } from "react-router-dom";
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
  PageHeader,
  Panel,
  Select,
  Spinner,
  TableWrap,
  Tabs,
  TD,
  TH,
  TRow,
} from "@/components/ui";
import type { LogEntry, PolicyRule } from "@/types";

export function TrafficPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const view = searchParams.get("view") === "rules" ? "rules" : "log";
  const [sandboxFilter, setSandboxFilter] = useState("");

  const sandboxes = useQuery({ queryKey: queryKeys.sandboxes, queryFn: api.listSandboxes });
  const traffic = useQuery({ queryKey: queryKeys.traffic, queryFn: api.traffic });
  const rules = useQuery({ queryKey: queryKeys.policyRules, queryFn: () => api.policyRules() });

  const allow = useApiMutation({
    mutationFn: (entry: LogEntry) => api.policyAction({ action: "allow", resources: [entry.host], sandbox_id: entry.vm_name }),
    success: (_, entry) => `Allowed ${entry.host} for ${entry.vm_name}`,
  });
  const deny = useApiMutation({
    mutationFn: (entry: LogEntry) => api.policyAction({ action: "deny", resources: [entry.host], sandbox_id: entry.vm_name }),
    success: (_, entry) => `Denied ${entry.host} for ${entry.vm_name}`,
  });
  const removeRule = useApiMutation({
    mutationFn: (rule: PolicyRule) =>
      api.policyAction({ action: "remove-id", id: rule.id, sandbox_id: rule.sandbox_id || undefined }),
    success: "Rule removed",
    invalidate: [queryKeys.policyRules, ["sandbox-policy"]],
  });

  const log = useMemo(() => {
    const matches = (entry: LogEntry) => !sandboxFilter || entry.vm_name === sandboxFilter;
    return {
      blocked: (traffic.data?.blocked_hosts ?? []).filter(matches),
      allowed: (traffic.data?.allowed_hosts ?? []).filter(matches),
    };
  }, [traffic.data, sandboxFilter]);

  return (
    <>
      <PageHeader
        title="Traffic"
        subtitle="Global proxy log and the rules daemon policy evaluates. Sandbox-scoped rules live in each sandbox’s Traffic tab."
      />

      <Tabs
        tabs={[
          { id: "log", label: "Log", count: (traffic.data?.blocked_hosts?.length ?? 0) + (traffic.data?.allowed_hosts?.length ?? 0) },
          { id: "rules", label: "Rules", count: rules.data?.length ?? 0 },
        ]}
        active={view}
        onChange={(next) => setSearchParams(next === "log" ? {} : { view: next }, { replace: true })}
        className="mb-4"
      />

      {view === "log" ? (
        <div className="flex flex-col gap-4">
          <Panel
            title="Proxy log"
            actions={
              <>
                <Select
                  value={sandboxFilter}
                  onChange={(event) => setSandboxFilter(event.target.value)}
                  className="w-48"
                >
                  <option value="">All sandboxes</option>
                  {(sandboxes.data ?? []).map((sandbox) => (
                    <option key={sandbox.name} value={sandbox.name}>
                      {sandbox.name}
                    </option>
                  ))}
                </Select>
                <Button size="sm" variant="ghost" onClick={() => void traffic.refetch()} loading={traffic.isFetching}>
                  Refresh
                </Button>
              </>
            }
            bodyClassName="p-0"
          >
            <div className="border-b border-border p-4">
              <h3 className="mb-2 text-xs font-semibold tracking-wide text-faint uppercase">Blocked</h3>
              <LogTable
                entries={log.blocked}
                busy={allow.isPending || deny.isPending}
                busyHost={allow.variables?.host ?? deny.variables?.host}
                onAllow={(entry) => allow.mutate(entry)}
                onDeny={(entry) => deny.mutate(entry)}
              />
            </div>
            <div className="p-4">
              <h3 className="mb-2 text-xs font-semibold tracking-wide text-faint uppercase">Allowed</h3>
              <LogTable entries={log.allowed} />
            </div>
          </Panel>
        </div>
      ) : (
        <RulesView rules={rules.data ?? []} loading={rules.isLoading} onRemove={(rule) => removeRule.mutate(rule)} busyRule={removeRule.variables?.id} />
      )}
    </>
  );
}

function LogTable({
  entries,
  onAllow,
  onDeny,
  busy,
  busyHost,
}: {
  entries: LogEntry[];
  onAllow?: (entry: LogEntry) => void;
  onDeny?: (entry: LogEntry) => void;
  busy?: boolean;
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
          <TH>Sandbox</TH>
          <TH>Proxy</TH>
          <TH>Rule</TH>
          <TH>Count</TH>
          <TH>Last seen</TH>
          {onAllow ? <TH className="w-36" /> : null}
        </tr>
      </thead>
      <tbody>
        {entries.map((entry) => (
          <TRow key={`${entry.vm_name}:${entry.host}`}>
            <TD className="font-mono text-xs">{entry.host}</TD>
            <TD>
              <Link
                to={`/sandboxes/${encodeURIComponent(entry.vm_name)}?tab=traffic`}
                className="font-mono text-xs hover:text-accent"
              >
                {entry.vm_name}
              </Link>
            </TD>
            <TD className="text-xs text-muted">{entry.proxy_type}</TD>
            <TD className="font-mono text-xs text-muted">{entry.rule}</TD>
            <TD className="text-xs">{entry.count_since}</TD>
            <TD className="text-xs text-muted">{formatTime(entry.last_seen)}</TD>
            {onAllow ? (
              <TD className="text-right">
                <span className="inline-flex gap-1">
                  <Button size="sm" variant="ghost" disabled={busy} loading={busyHost === entry.host} onClick={() => onAllow(entry)}>
                    Allow
                  </Button>
                  {onDeny ? (
                    <Button size="sm" variant="ghost" disabled={busy} onClick={() => onDeny(entry)}>
                      Deny
                    </Button>
                  ) : null}
                </span>
              </TD>
            ) : null}
          </TRow>
        ))}
      </tbody>
    </TableWrap>
  );
}

function RulesView({
  rules,
  loading,
  onRemove,
  busyRule,
}: {
  rules: PolicyRule[];
  loading: boolean;
  onRemove: (rule: PolicyRule) => void;
  busyRule?: string;
}) {
  const [action, setAction] = useState("allow");
  const [resources, setResources] = useState("");
  const [sandbox, setSandbox] = useState("");

  const sandboxes = useQuery({ queryKey: queryKeys.sandboxes, queryFn: api.listSandboxes });
  const apply = useApiMutation({
    mutationFn: () =>
      api.policyAction({
        action,
        resources: resources
          .split(/[,\n]/)
          .map((value) => value.trim())
          .filter(Boolean),
        sandbox_id: sandbox || undefined,
      }),
    success: "Policy updated",
    onSuccess: () => setResources(""),
  });

  return (
    <div className="flex flex-col gap-4">
      <Panel title="Add a rule" description="Without a sandbox the rule is global; scoped rules narrow egress for one sandbox.">
        <div className="flex flex-wrap items-center gap-2">
          <Select value={action} onChange={(event) => setAction(event.target.value)} className="w-28">
            <option value="allow">allow</option>
            <option value="deny">deny</option>
          </Select>
          <Input
            value={resources}
            onChange={(event) => setResources(event.target.value)}
            placeholder="api.example.com, *.internal.example.com"
            className="max-w-md flex-1 font-mono text-xs"
          />
          <Select value={sandbox} onChange={(event) => setSandbox(event.target.value)} className="w-48">
            <option value="">All sandboxes</option>
            {(sandboxes.data ?? []).map((item) => (
              <option key={item.name} value={item.name}>
                {item.name}
              </option>
            ))}
          </Select>
          <Button variant="primary" disabled={!resources.trim()} loading={apply.isPending} onClick={() => apply.mutate()}>
            Apply
          </Button>
        </div>
      </Panel>

      <Panel title="All policy rules" bodyClassName="p-0">
        {loading ? (
          <div className="flex justify-center p-6">
            <Spinner />
          </div>
        ) : rules.length === 0 ? (
          <p className="p-4 text-sm text-muted">No rules.</p>
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
              {rules.map((rule) => (
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
                      <Button size="sm" variant="ghost" loading={busyRule === rule.id} onClick={() => onRemove(rule)}>
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
    </div>
  );
}
