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

export function bareHost(host: string): string {
  let h = host.trim().toLowerCase();
  h = h.replace(/^[a-z][a-z0-9+.-]*:\/\//, ""); // strip scheme (http://, https://)
  h = h.replace(/\/.*$/, ""); // strip trailing path
  return h.replace(/:\d+$/, ""); // strip port
}

export function sandboxMatches(a: string | undefined, b: string | undefined): boolean {
  if (!a && !b) return true;
  if (!a || !b) return false;
  const normA = a.trim().toLowerCase();
  const normB = b.trim().toLowerCase();
  if (normA === normB) return true;
  const slugA = normA.replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
  const slugB = normB.replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
  return slugA === slugB;
}

/** Whether a policy resource pattern (exact, `*.domain`, `**.domain` or `*`) covers host. */
export function hostMatches(pattern: string, host: string): boolean {
  const want = bareHost(pattern);
  const got = bareHost(host);
  if (want === "*" || want === "**") return true;
  const wildcard = /^\*{1,2}\./.exec(want);
  if (wildcard) return got.endsWith(want.slice(wildcard[0].length - 1));
  return want === got;
}

function appliesTo(rule: PolicyRule, sandbox: string): boolean {
  return !rule.sandbox_id || sandboxMatches(rule.sandbox_id, sandbox);
}

function decisionOf(rules: PolicyRule[], entry: LogEntry): "allow" | "deny" | null {
  let allowed = false;
  for (const rule of rules) {
    if (rule.status && (rule.status.toLowerCase() === "disabled" || rule.status.toLowerCase() === "inactive")) continue;
    if (!appliesTo(rule, entry.vm_name)) continue;
    if (!(rule.resources ?? []).some((resource) => hostMatches(resource, entry.host))) continue;
    if (rule.decision.toLowerCase().includes("deny")) return "deny";
    if (rule.decision.toLowerCase().includes("allow")) allowed = true;
  }
  return allowed ? "allow" : null;
}

/**
 * Splits a proxy log into pending / denied / allowed. When `sandbox` is given
 * the view is scoped to that sandbox: other sandboxes' rules are ignored while
 * global rules still apply. Without a scope every rule is considered.
 */
export function classifyTraffic(log: PolicyLog | undefined, rules: PolicyRule[], sandbox?: string): TrafficView {
  const view: TrafficView = { pending: [], denied: [], allowed: [] };
  const allowed = new Map<string, LogEntry>();
  const denied = new Map<string, LogEntry>();
  for (const entry of log?.allowed_hosts ?? []) allowed.set(trafficKey(entry), entry);
  for (const entry of log?.blocked_hosts ?? []) {
    const decision = decisionOf(rules, entry);
    if (decision === "deny") {
      denied.set(trafficKey(entry), entry);
    } else if (decision === "allow") {
      if (!allowed.has(trafficKey(entry))) allowed.set(trafficKey(entry), entry);
    } else {
      view.pending.push(entry);
    }
  }

  // Include explicit deny rules so rejected domains are visible even before or without logged proxy events
  for (const rule of rules) {
    if (rule.status && (rule.status.toLowerCase() === "disabled" || rule.status.toLowerCase() === "inactive")) continue;
    if (!rule.decision.toLowerCase().includes("deny")) continue;
    // A scoped view only surfaces rules global to every sandbox or attached to
    // the sandbox being viewed, never another sandbox's rules.
    if (sandbox && rule.sandbox_id && !sandboxMatches(rule.sandbox_id, sandbox)) continue;
    for (const res of rule.resources ?? []) {
      if (res === "*" || res === "**") continue;
      const host = bareHost(res);
      // Wildcard patterns are not concrete hosts: listing them in a Host
      // column would read as a recorded host instead of a policy pattern.
      if (!host || host.includes("*")) continue;
      const alreadyCovered = [...denied.values()].some(
        (e) => hostMatches(res, e.host) && (!rule.sandbox_id || sandboxMatches(e.vm_name, rule.sandbox_id)),
      );
      if (alreadyCovered) continue;
      const syntheticEntry: LogEntry = {
        host,
        vm_name: rule.sandbox_id || "",
        proxy_type: rule.origin || "policy",
        rule: rule.name || rule.id || "deny",
        last_seen: "",
        since: "",
        count_since: 0,
      };
      denied.set(trafficKey(syntheticEntry), syntheticEntry);
    }
  }

  view.allowed = [...allowed.values()];
  view.denied = [...denied.values()];
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
