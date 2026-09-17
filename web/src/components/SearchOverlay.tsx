import { useEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { useNavigate } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import { cn } from "@/lib/cn";
import { searchResultBadge, searchResultLocation, searchResultTone } from "@/lib/search";
import { Badge, IconSearch, Spinner } from "@/components/ui";
import type { SearchResult, SearchResultKind } from "@/types";

const GROUPS: { kind: SearchResultKind; label: string }[] = [
  { kind: "sandbox", label: "Sandboxes" },
  { kind: "profile", label: "Profiles" },
  { kind: "cache", label: "Caches" },
  { kind: "skill", label: "Skills" },
  { kind: "command", label: "Commands" },
  { kind: "kit", label: "Kits" },
];

const MIN_QUERY = 2;
const DEBOUNCE_MS = 150;

/** Window event dispatched by the sidebar button to open the overlay. */
export const OPEN_SEARCH_EVENT = "sandwarden:open-search";

/**
 * Global search overlay: Ctrl/Cmd+K ranks the skill, command and kit catalogs
 * through the BM25 backend. Results are grouped by kind; arrow keys move,
 * Enter opens the matching page.
 */
export function SearchOverlay() {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [debounced, setDebounced] = useState("");
  const [active, setActive] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const navigate = useNavigate();

  useEffect(() => {
    function onKey(event: KeyboardEvent) {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setOpen((value) => !value);
        return;
      }
      if (event.key === "Escape") setOpen(false);
    }
    function onOpen() {
      setOpen(true);
    }
    window.addEventListener("keydown", onKey);
    window.addEventListener(OPEN_SEARCH_EVENT, onOpen);
    return () => {
      window.removeEventListener("keydown", onKey);
      window.removeEventListener(OPEN_SEARCH_EVENT, onOpen);
    };
  }, []);

  useEffect(() => {
    if (open) {
      inputRef.current?.focus();
      return;
    }
    setQuery("");
    setDebounced("");
    setActive(0);
  }, [open]);

  useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(query.trim()), DEBOUNCE_MS);
    return () => window.clearTimeout(timer);
  }, [query]);

  const enabled = open && debounced.length >= MIN_QUERY;
  const results = useQuery({
    queryKey: ["search", debounced],
    queryFn: () => api.search(debounced),
    enabled,
    staleTime: 30_000,
    retry: false,
  });

  const grouped = useMemo(() => {
    const rows = results.data ?? [];
    return GROUPS.map((group) => ({ ...group, items: rows.filter((row) => row.kind === group.kind) })).filter(
      (group) => group.items.length > 0,
    );
  }, [results.data]);
  const flat = useMemo(() => grouped.flatMap((group) => group.items), [grouped]);

  useEffect(() => {
    setActive(0);
  }, [flat.length]);

  useEffect(() => {
    const element = listRef.current?.querySelector<HTMLElement>(`[data-index="${active}"]`);
    element?.scrollIntoView({ block: "nearest" });
  }, [active]);

  if (!open) return null;

  function close() {
    setOpen(false);
  }

  function openResult(result: SearchResult) {
    navigate(searchResultLocation(result));
    close();
  }

  function onInputKeyDown(event: React.KeyboardEvent<HTMLInputElement>) {
    if (event.key === "ArrowDown") {
      event.preventDefault();
      setActive((current) => Math.min(current + 1, flat.length - 1));
      return;
    }
    if (event.key === "ArrowUp") {
      event.preventDefault();
      setActive((current) => Math.max(current - 1, 0));
      return;
    }
    if (event.key === "Enter") {
      event.preventDefault();
      const hit = flat[active];
      if (hit) openResult(hit);
    }
  }

  return createPortal(
    <div className="fixed inset-0 z-[70] flex items-start justify-center px-4 pt-[12vh]">
      <div className="fixed inset-0 bg-overlay" onClick={close} aria-hidden="true" />
      <div
        role="dialog"
        aria-modal="true"
        aria-label="Search sandboxes, profiles, caches and catalogs"
        className="relative z-10 flex w-full max-w-xl flex-col overflow-hidden rounded-lg border border-border bg-surface shadow-lg"
      >
        <div className="flex items-center gap-2.5 border-b border-border px-3">
          <IconSearch className="shrink-0 text-faint" />
          <input
            ref={inputRef}
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            onKeyDown={onInputKeyDown}
            placeholder="Search sandboxes, profiles, caches, skills and kits…"
            aria-label="Search query"
            className="h-11 w-full bg-transparent text-sm text-fg outline-none placeholder:text-faint"
          />
          <kbd className="rounded border border-border px-1.5 py-0.5 text-2xs text-faint">esc</kbd>
        </div>

        <div ref={listRef} className="max-h-[50vh] overflow-y-auto p-1.5">
          {!enabled ? (
            <p className="px-3 py-6 text-center text-xs text-faint">
              Type at least {MIN_QUERY} characters to search.
            </p>
          ) : results.isFetching && flat.length === 0 ? (
            <div className="flex justify-center py-6">
              <Spinner />
            </div>
          ) : flat.length === 0 ? (
            <p className="px-3 py-6 text-center text-xs text-faint">No match for "{debounced}".</p>
          ) : (
            grouped.map((group) => (
              <div key={group.kind} className="mb-1 last:mb-0">
                <p className="px-2.5 pt-2 pb-1 text-2xs font-semibold tracking-wide text-faint uppercase">
                  {group.label}
                </p>
                {group.items.map((item) => {
                  const index = flat.indexOf(item);
                  return (
                    <button
                      key={`${item.kind}-${item.slug ?? item.store}-${item.name}`}
                      type="button"
                      data-index={index}
                      onMouseEnter={() => setActive(index)}
                      onClick={() => openResult(item)}
                      className={cn(
                        "flex w-full flex-col gap-0.5 rounded-sm px-2.5 py-2 text-left transition-colors",
                        index === active ? "bg-hover" : "hover:bg-hover/50",
                      )}
                    >
                      <span className="flex items-center gap-2">
                        <Badge tone={searchResultTone(item.kind)}>{searchResultBadge(item)}</Badge>
                        <span className="truncate text-sm font-medium">{item.display_name || item.name}</span>
                        <span className="ml-auto shrink-0 text-2xs text-faint">{item.store_name}</span>
                      </span>
                      {item.snippet || item.description ? (
                        <span className="line-clamp-2 text-xs text-muted">{item.snippet || item.description}</span>
                      ) : null}
                    </button>
                  );
                })}
              </div>
            ))
          )}
        </div>

        <footer className="flex items-center gap-3 border-t border-border px-3 py-2 text-2xs text-faint">
          <span>↑↓ navigate</span>
          <span>↵ open</span>
          <span>esc close</span>
        </footer>
      </div>
    </div>,
    document.body,
  );
}
