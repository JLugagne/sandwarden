import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { api } from "@/api/client";
import { useApiMutation } from "@/hooks/useApiMutation";
import { queryKeys } from "@/store/realtime";
import { formatTime } from "@/lib/format";
import { classifyTraffic, groupBySandbox, trafficKey } from "@/lib/traffic";
import { Button, Checkbox, EmptyState, Panel, Select, TableWrap, TD, TH, TRow } from "@/components/ui";
import type { LogEntry, PolicyLog, PolicyRule, ProfileView } from "@/types";

type Decision = "allow" | "deny";

const RULE_QUERIES = [queryKeys.policyRules, ["sandbox-policy"], queryKeys.traffic, queryKeys.profiles];

function plural(count: number, noun: string) {
  return `${count} ${noun}${count === 1 ? "" : "s"}`;
}

/**
 * Proxy log review: blocked hosts nobody decided on yet, with per-row and bulk
 * allow/deny either on their sandbox or into a profile. Decided hosts move to
 * the Allowed or Denied lists once the daemon reports the new rules.
 */
export function TrafficReview({
  log,
  rules,
  sandbox,
  onRefresh,
  refreshing,
}: {
  log: PolicyLog | undefined;
  rules: PolicyRule[];
  sandbox?: string;
  onRefresh?: () => void;
  refreshing?: boolean;
}) {
  const view = useMemo(() => {
    const scoped = sandbox
      ? {
          blocked_hosts: (log?.blocked_hosts ?? []).filter((entry) => entry.vm_name === sandbox),
          allowed_hosts: (log?.allowed_hosts ?? []).filter((entry) => entry.vm_name === sandbox),
        }
      : log;
    return classifyTraffic(scoped, rules);
  }, [log, rules, sandbox]);

  const profiles = useQuery({ queryKey: queryKeys.profiles, queryFn: api.profiles });
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [profile, setProfile] = useState("");

  const selection = view.pending.filter((entry) => selected.has(trafficKey(entry)));
  const allSelected = view.pending.length > 0 && selection.length === view.pending.length;

  const decide = useApiMutation({
    mutationFn: async ({ entries, decision }: { entries: LogEntry[]; decision: Decision }) => {
      const groups = groupBySandbox(entries);
      const responses = await Promise.all(
        Object.entries(groups).map(([vm, hosts]) => api.policyAction({ action: decision, resources: hosts, sandbox_id: vm })),
      );
      const failed = responses.flatMap((response) => response.results).find((result) => result.error);
      if (failed?.error) throw new Error(failed.error);
    },
    success: (_, { entries, decision }) => `${decision === "allow" ? "Allowed" : "Denied"} ${plural(entries.length, "host")}`,
    invalidate: RULE_QUERIES,
    onSuccess: (_, { entries }) => deselect(entries),
  });

  const toProfile = useApiMutation({
    mutationFn: ({ entries, decision, slug }: { entries: LogEntry[]; decision: Decision; slug: string }) =>
      api.addRules(slug, { decision, patterns: [...new Set(entries.map((entry) => entry.host))] }),
    success: (_, { entries, decision, slug }) =>
      `${decision === "allow" ? "Allowed" : "Denied"} ${plural(entries.length, "host")} in profile ${profileName(profiles.data, slug)}`,
    invalidate: RULE_QUERIES,
    onSuccess: (_, { entries }) => deselect(entries),
  });

  function deselect(entries: LogEntry[]) {
    setSelected((current) => {
      const next = new Set(current);
      for (const entry of entries) next.delete(trafficKey(entry));
      return next;
    });
  }

  function toggle(entry: LogEntry, on: boolean) {
    setSelected((current) => {
      const next = new Set(current);
      if (on) next.add(trafficKey(entry));
      else next.delete(trafficKey(entry));
      return next;
    });
  }

  const busy = decide.isPending || toProfile.isPending;
  const showSandbox = !sandbox;

  return (
    <div className="flex flex-col gap-4">
      <Panel
        title={`Pending (${view.pending.length})`}
        description="Blocked hosts no rule covers yet. Allow or deny them on their sandbox, or add them to a profile."
        actions={
          onRefresh ? (
            <Button size="sm" variant="ghost" onClick={onRefresh} loading={refreshing}>
              Refresh
            </Button>
          ) : null
        }
        bodyClassName="p-0"
      >
        {selection.length > 0 ? (
          <div
            className="flex flex-wrap items-center gap-2 border-b border-border bg-hover/40 px-4 py-2"
            role="toolbar"
            aria-label="Bulk actions"
          >
            <span className="text-xs text-muted">{selection.length} selected</span>
            <Button size="sm" variant="primary" disabled={busy} onClick={() => decide.mutate({ entries: selection, decision: "allow" })}>
              Allow selected
            </Button>
            <Button size="sm" variant="danger" disabled={busy} onClick={() => decide.mutate({ entries: selection, decision: "deny" })}>
              Deny selected
            </Button>
            <span className="mx-1 h-4 w-px bg-border" aria-hidden="true" />
            <Select
              aria-label="Profile"
              value={profile}
              onChange={(event) => setProfile(event.target.value)}
              className="w-56"
            >
              <option value="">Choose a profile…</option>
              {(profiles.data ?? []).map((item) => (
                <option key={item.slug} value={item.slug}>
                  {item.name}
                  {profileCovers(item, selection) ? " (applies here)" : ""}
                </option>
              ))}
            </Select>
            <Button
              size="sm"
              disabled={busy || !profile}
              onClick={() => toProfile.mutate({ entries: selection, decision: "allow", slug: profile })}
            >
              Allow in profile
            </Button>
            <Button
              size="sm"
              disabled={busy || !profile}
              onClick={() => toProfile.mutate({ entries: selection, decision: "deny", slug: profile })}
            >
              Deny in profile
            </Button>
          </div>
        ) : null}

        {view.pending.length === 0 ? (
          <EmptyState title="Nothing waiting for a decision." className="py-6" />
        ) : (
          <TableWrap className="border-0">
            <thead>
              <tr>
                <TH className="w-8">
                  <Checkbox
                    label={<span className="sr-only">Select all pending hosts</span>}
                    checked={allSelected}
                    onChange={(on) => setSelected(on ? new Set(view.pending.map(trafficKey)) : new Set())}
                  />
                </TH>
                <TH>Host</TH>
                {showSandbox ? <TH>Sandbox</TH> : null}
                <TH>Proxy</TH>
                <TH>Count</TH>
                <TH>Last seen</TH>
                <TH className="w-36" />
              </tr>
            </thead>
            <tbody>
              {view.pending.map((entry) => (
                <TRow key={trafficKey(entry)}>
                  <TD>
                    <Checkbox
                      label={<span className="sr-only">Select {entry.host}</span>}
                      checked={selected.has(trafficKey(entry))}
                      onChange={(on) => toggle(entry, on)}
                    />
                  </TD>
                  <TD className="font-mono text-xs">{entry.host}</TD>
                  {showSandbox ? <SandboxCell name={entry.vm_name} /> : null}
                  <TD className="text-xs text-muted">{entry.proxy_type}</TD>
                  <TD className="text-xs">{entry.count_since}</TD>
                  <TD className="text-xs text-muted">{formatTime(entry.last_seen)}</TD>
                  <TD className="text-right">
                    <span className="inline-flex gap-1">
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={busy}
                        onClick={() => decide.mutate({ entries: [entry], decision: "allow" })}
                      >
                        Allow
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={busy}
                        onClick={() => decide.mutate({ entries: [entry], decision: "deny" })}
                      >
                        Deny
                      </Button>
                    </span>
                  </TD>
                </TRow>
              ))}
            </tbody>
          </TableWrap>
        )}
      </Panel>

      <Panel title={`Allowed (${view.allowed.length})`} bodyClassName="p-0">
        <DecidedTable entries={view.allowed} showSandbox={showSandbox} />
      </Panel>
      <Panel title={`Denied (${view.denied.length})`} bodyClassName="p-0">
        <DecidedTable entries={view.denied} showSandbox={showSandbox} />
      </Panel>
    </div>
  );
}

function profileName(profiles: ProfileView[] | undefined, slug: string) {
  return profiles?.find((item) => item.slug === slug)?.name ?? slug;
}

function profileCovers(profile: ProfileView, entries: LogEntry[]) {
  if (profile.global) return true;
  const sandboxes = new Set(profile.sandboxes ?? []);
  return entries.every((entry) => sandboxes.has(entry.vm_name));
}

function SandboxCell({ name }: { name: string }) {
  return (
    <TD>
      <Link to={`/sandboxes/${encodeURIComponent(name)}?tab=traffic`} className="font-mono text-xs hover:text-accent">
        {name}
      </Link>
    </TD>
  );
}

function DecidedTable({ entries, showSandbox }: { entries: LogEntry[]; showSandbox: boolean }) {
  if (entries.length === 0) {
    return <EmptyState title="Nothing recorded." className="py-6" />;
  }
  return (
    <TableWrap className="border-0">
      <thead>
        <tr>
          <TH>Host</TH>
          {showSandbox ? <TH>Sandbox</TH> : null}
          <TH>Proxy</TH>
          <TH>Rule</TH>
          <TH>Count</TH>
          <TH>Last seen</TH>
        </tr>
      </thead>
      <tbody>
        {entries.map((entry) => (
          <TRow key={trafficKey(entry)}>
            <TD className="font-mono text-xs">{entry.host}</TD>
            {showSandbox ? <SandboxCell name={entry.vm_name} /> : null}
            <TD className="text-xs text-muted">{entry.proxy_type}</TD>
            <TD className="font-mono text-xs text-muted">{entry.rule}</TD>
            <TD className="text-xs">{entry.count_since}</TD>
            <TD className="text-xs text-muted">{formatTime(entry.last_seen)}</TD>
          </TRow>
        ))}
      </tbody>
    </TableWrap>
  );
}
