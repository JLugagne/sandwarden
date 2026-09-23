import type { LogEntry, PolicyLog, PolicyRule } from "@/types";

/** Proxy log split by what the user still has to decide. */
export interface TrafficView {
  /** Blocked hosts no allow or deny rule covers yet. */
  pending: LogEntry[];
  /** Blocked hosts an explicit deny rule covers. */
  denied: LogEntry[];
  /** Hosts the proxy let through, plus blocked ones an allow rule now covers. */
  allowed: LogEntry[];
}

export function trafficKey(entry: LogEntry): string {
  return `${entry.vm_name}\u0000${entry.host}`;
}

function bareHost(host: string): string {
  return host.trim().toLowerCase().replace(/:\d+$/, "");
}

/** Whether a policy resource pattern (exact, `*.domain`, `**.domain` or `*`) covers host. */
export function hostMatches(pattern: string, host: string): boolean {
  const want = pattern.trim().toLowerCase();
  const got = bareHost(host);
  if (want === "*" || want === "**") return true;
  const wildcard = /^\*{1,2}\./.exec(want);
  if (wildcard) return got.endsWith(want.slice(wildcard[0].length - 1));
  return bareHost(want) === got;
}

function appliesTo(rule: PolicyRule, sandbox: string): boolean {
  return !rule.sandbox_id || rule.sandbox_id === sandbox;
}

function decisionOf(rules: PolicyRule[], entry: LogEntry): "allow" | "deny" | null {
  let allowed = false;
  for (const rule of rules) {
    if (!appliesTo(rule, entry.vm_name)) continue;
    if (!(rule.resources ?? []).some((resource) => hostMatches(resource, entry.host))) continue;
    if (rule.decision.toLowerCase().includes("deny")) return "deny";
    if (rule.decision.toLowerCase().includes("allow")) allowed = true;
  }
  return allowed ? "allow" : null;
}

export function classifyTraffic(log: PolicyLog | undefined, rules: PolicyRule[]): TrafficView {
  const view: TrafficView = { pending: [], denied: [], allowed: [] };
  const allowed = new Map<string, LogEntry>();
  for (const entry of log?.allowed_hosts ?? []) allowed.set(trafficKey(entry), entry);
  for (const entry of log?.blocked_hosts ?? []) {
    const decision = decisionOf(rules, entry);
    if (decision === "deny") view.denied.push(entry);
    else if (decision === "allow") {
      if (!allowed.has(trafficKey(entry))) allowed.set(trafficKey(entry), entry);
    } else view.pending.push(entry);
  }
  view.allowed = [...allowed.values()];
  return view;
}

/** Hosts per sandbox, so one policy action covers a whole selection. */
export function groupBySandbox(entries: LogEntry[]): Record<string, string[]> {
  const groups: Record<string, string[]> = {};
  for (const entry of entries) {
    const hosts = (groups[entry.vm_name] ??= []);
    if (!hosts.includes(entry.host)) hosts.push(entry.host);
  }
  return groups;
}
