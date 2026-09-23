import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import { useApiMutation } from "@/hooks/useApiMutation";
import { queryKeys } from "@/store/realtime";
import { TrafficReview } from "@/components/TrafficReview";
import {
  Badge,
  Button,
  DecisionBadge,
  Input,
  Panel,
  Select,
  Spinner,
  TableWrap,
  TD,
  TH,
  TRow,
} from "@/components/ui";
import type { PolicyRule } from "@/types";

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


  const ruleRows = rules.data ?? [];

  return (
    <div className="flex flex-col gap-4">
      <TrafficReview
        log={traffic.data}
        rules={ruleRows}
        sandbox={name}
        onRefresh={() => void traffic.refetch()}
        refreshing={traffic.isFetching}
      />

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

    </div>
  );
}

