import type { BadgeTone } from "@/components/ui";
import type { SearchResult, SearchResultKind } from "@/types";

/**
 * Route of one search hit. Catalog items deep link into their page with the
 * store and item query parameters, config entities into the sandbox detail,
 * profiles page and settings caches section.
 */
export function searchResultLocation(result: SearchResult): string {
  switch (result.kind) {
    case "sandbox":
      return `/sandboxes/${encodeURIComponent(result.name)}`;
    case "profile":
      return `/profiles?profile=${encodeURIComponent(result.slug || result.name)}`;
    case "cache":
      return `/settings?cache=${encodeURIComponent(result.slug || result.name)}`;
    case "kit":
      return `/kits?store=${encodeURIComponent(result.store)}&item=${encodeURIComponent(result.name)}`;
    default:
      return `/skills?store=${encodeURIComponent(result.store)}&item=${encodeURIComponent(result.name)}`;
  }
}

/** Badge label of one hit: the kit kind, the sandbox agent, or the kind. */
export function searchResultBadge(result: SearchResult): string {
  if (result.kind === "kit") return result.kit_kind || "kit";
  if (result.kind === "sandbox") return result.agent || "sandbox";
  return result.kind;
}

const TONES: Partial<Record<SearchResultKind, BadgeTone>> = {
  sandbox: "success",
  profile: "accent",
  cache: "warning",
  kit: "accent",
};

export function searchResultTone(kind: SearchResultKind): BadgeTone {
  return TONES[kind] ?? "neutral";
}
