import { api } from "@/api/client";
import type { AppConfig, TerminalPrefs } from "@/types";

const FALLBACK_TERMINALS: TerminalPrefs = { enabled: null, default: "" };
const FALLBACK: AppConfig = { terminals: FALLBACK_TERMINALS, notifications: false };

let cache: AppConfig | null = null;
let inflight: Promise<AppConfig> | null = null;
const listeners = new Set<(config: AppConfig) => void>();

function normalize(config: AppConfig | null | undefined): AppConfig {
  if (!config || typeof config !== "object") return FALLBACK;
  const terminals = config.terminals ?? FALLBACK_TERMINALS;
  return {
    terminals: {
      enabled: Array.isArray(terminals.enabled)
        ? terminals.enabled.filter((id): id is string => typeof id === "string")
        : null,
      default: typeof terminals.default === "string" ? terminals.default : "",
    },
    notifications: Boolean(config.notifications),
  };
}

/** Last configuration known to the frontend, without waiting for the API. */
export function cachedConfig(): AppConfig {
  return cache ?? FALLBACK;
}

/** Loads the backend config file once; later callers get the cached value. */
export function loadConfig(): Promise<AppConfig> {
  if (cache) return Promise.resolve(cache);
  if (!inflight) {
    inflight = Promise.resolve()
      .then(() => api.getConfig())
      .then((config) => {
        cache = normalize(config);
        return cache;
      })
      .catch(() => FALLBACK)
      .finally(() => {
        inflight = null;
      });
  }
  return inflight;
}

/** Seeds the cache with a config already fetched through React Query. */
export function primeConfig(config: AppConfig): void {
  cache = normalize(config);
  emit();
}

/** Merges a patch into the config and persists the whole file. */
export async function updateConfig(patch: Partial<AppConfig>): Promise<void> {
  const base = cache ?? (await loadConfig());
  const next = { ...base, ...patch };
  cache = next;
  emit();
  await api.setConfig(next);
}

export function subscribeConfig(listener: (config: AppConfig) => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

/** Test helper: forget the cached configuration between test cases. */
export function resetConfigCacheForTests(): void {
  cache = null;
  inflight = null;
  listeners.clear();
}

/** Kinds of loaded configuration file the fleet reports as stale. */
export type StaleKind = "sandbox" | "profile" | "cache" | "skill-store" | "kit-store" | "config";

/** One loaded file whose content changed on disk since the last load. */
export interface StaleFile {
  kind: StaleKind;
  slug: string;
  name: string;
  file: string;
  path: string;
}

/** Staleness poll result: a global flag plus the changed files. */
export interface ConfigStaleness {
  stale: boolean;
  changed: StaleFile[];
}

export const EMPTY_STALENESS: ConfigStaleness = { stale: false, changed: [] };

/** Query key shared by the staleness poll and the reload invalidations. */
export const stalenessKey = ["config-staleness"] as const;

/** Slugs of the entities of one kind whose file changed on disk. */
export function staleSlugs(report: ConfigStaleness | undefined, kind: StaleKind): Set<string> {
  return new Set((report?.changed ?? []).filter((file) => file.kind === kind).map((file) => file.slug));
}

/** Display names of the entities of one kind whose file changed on disk. */
export function staleNames(report: ConfigStaleness | undefined, kind: StaleKind): Set<string> {
  return new Set((report?.changed ?? []).filter((file) => file.kind === kind).map((file) => file.name));
}

function emit(): void {
  const config = cachedConfig();
  for (const listener of listeners) listener(config);
}
