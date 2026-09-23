import type { QueryClient } from "@tanstack/react-query";
import { cacheView, type FleetCache } from "@/api/client";
import type { EventEnvelope, SandboxSummary } from "@/types";

export const queryKeys = {
  health: ["health"] as const,
  sandboxes: ["sandboxes"] as const,
  sandboxDetails: ["sandbox"] as const,
  sandbox: (name: string) => ["sandbox", name] as const,
  sandboxPolicy: (name: string) => ["sandbox-policy", name] as const,
  sandboxConfigDir: (slug: string) => ["sandbox-config-dir", slug] as const,
  fleetDir: ["fleet-dir"] as const,
  agentsFile: ["agents-file"] as const,
  notificationStatus: ["notification-status"] as const,
  profiles: ["profiles"] as const,
  secrets: ["secrets"] as const,
  traffic: ["traffic"] as const,
  policyRules: ["policy-rules"] as const,
  caches: ["caches"] as const,
  skillStores: ["skill-stores"] as const,
  skillItems: ["skill-items"] as const,
  kitStores: ["kit-stores"] as const,
  kitItems: ["kit-items"] as const,
  templates: ["templates"] as const,
  version: ["version"] as const,
  updates: ["updates"] as const,
  terminals: ["terminals"] as const,
  config: ["config"] as const,
};

/**
 * applyEvent folds one realtime envelope into the query cache:
 *   - snapshot topics replace their query data directly
 *   - `sandboxes` additionally prunes detail queries for deleted sandboxes
 *   - `policy-rules` invalidates per-sandbox rule queries (server-filtered)
 * `blocked` and `jobs:*` are handled outside the cache by their consumers.
 */
export function applyEvent(client: QueryClient, event: EventEnvelope): void {
  const { topic, data } = event;

  if (topic === "sandboxes") {
    if (!Array.isArray(data)) return;
    const list = data as SandboxSummary[];
    const names = new Set(list.map((sandbox) => sandbox.name));
    client.setQueryData(queryKeys.sandboxes, list);
    for (const query of client.getQueryCache().findAll({ queryKey: ["sandbox"] })) {
      const key = query.queryKey;
      if (key.length === 2 && typeof key[1] === "string" && !names.has(key[1])) {
        client.removeQueries({ queryKey: key });
      }
    }
    return;
  }

  if (topic.startsWith("sandbox:")) {
    const name = topic.slice("sandbox:".length);
    if (name) client.setQueryData(queryKeys.sandbox(name), data);
    return;
  }

  switch (topic) {
    case "profiles":
      client.setQueryData(queryKeys.profiles, data);
      return;
    case "secrets":
      client.setQueryData(queryKeys.secrets, data);
      return;
    case "traffic":
      client.setQueryData(queryKeys.traffic, data);
      return;
    case "policy-rules":
      client.setQueryData(queryKeys.policyRules, data);
      client.invalidateQueries({ queryKey: ["sandbox-policy"] });
      return;
    case "caches":
      if (!Array.isArray(data)) return;
      client.setQueryData(queryKeys.caches, (data as FleetCache[]).map(cacheView));
      return;
    case "skills":
      client.setQueryData(queryKeys.skillStores, data);
      client.invalidateQueries({ queryKey: queryKeys.skillItems });
      client.invalidateQueries({ queryKey: queryKeys.profiles });
      return;
    default:
      return;
  }
}
