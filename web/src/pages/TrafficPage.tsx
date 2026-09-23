import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import { api } from "@/api/client";
import { useApiMutation } from "@/hooks/useApiMutation";
import { queryKeys } from "@/store/realtime";
import { classifyTraffic } from "@/lib/traffic";
import { TrafficReview } from "@/components/TrafficReview";
import {
  Badge,
  Button,
  DecisionBadge,
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

  const removeRule = useApiMutation({
    mutationFn: (rule: PolicyRule) =>
      api.policyAction({ action: "remove-id", id: rule.id, sandbox_id: rule.sandbox_id || undefined }),
    success: "Rule removed",
    invalidate: [queryKeys.policyRules, ["sandbox-policy"]],
  });

  const log = useMemo(() => {
    if (!sandboxFilter) return traffic.data;
    const matches = (entry: LogEntry) => entry.vm_name === sandboxFilter;
    return {
      blocked_hosts: (traffic.data?.blocked_hosts ?? []).filter(matches),
      allowed_hosts: (traffic.data?.allowed_hosts ?? []).filter(matches),
    };
  }, [traffic.data, sandboxFilter]);
  const pendingCount = useMemo(() => classifyTraffic(traffic.data, rules.data ?? []).pending.length, [traffic.data, rules.data]);

  return (
    <>
      <PageHeader
        title="Traffic"
        subtitle="Global proxy log and the rules daemon policy evaluates. Sandbox-scoped rules live in each sandbox’s Traffic tab."
      />

      <Tabs
        tabs={[
          { id: "log", label: "Review", count: pendingCount },
          { id: "rules", label: "Rules", count: rules.data?.length ?? 0 },
        ]}
        active={view}
        onChange={(next) => setSearchParams(next === "log" ? {} : { view: next }, { replace: true })}
        className="mb-4"
      />

      {view === "log" ? (
        <div className="flex flex-col gap-4">
          <div className="flex items-center justify-end">
            <Select value={sandboxFilter} onChange={(event) => setSandboxFilter(event.target.value)} className="w-48" aria-label="Sandbox">
              <option value="">All sandboxes</option>
              {(sandboxes.data ?? []).map((sandbox) => (
                <option key={sandbox.name} value={sandbox.name}>
                  {sandbox.name}
                </option>
              ))}
            </Select>
          </div>
          <TrafficReview
            log={log}
            rules={rules.data ?? []}
            onRefresh={() => void traffic.refetch()}
            refreshing={traffic.isFetching}
          />
        </div>
      ) : (
        <RulesView rules={rules.data ?? []} loading={rules.isLoading} onRemove={(rule) => removeRule.mutate(rule)} busyRule={removeRule.variables?.id} />
      )}
    </>
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
