import { useEffect, useId, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import { useApiMutation } from "@/hooks/useApiMutation";
import { queryKeys } from "@/store/realtime";
import { cn } from "@/lib/cn";
import { defaultTerminal, enabledTerminals, fetchTerminalPrefs, loadTerminalPrefs, type TerminalPrefs } from "@/lib/terminals";
import { IconChevronDown, IconTerminal } from "./Icons";

const SEGMENT =
  "inline-flex h-6 cursor-pointer items-center gap-1.5 border border-border bg-canvas px-2 text-2xs text-muted transition-colors hover:border-border-strong hover:text-fg disabled:cursor-not-allowed disabled:opacity-50";

/**
 * Split button that opens a command in a terminal emulator. The main segment
 * launches the default terminal immediately; the chevron lists the enabled
 * ones for a one-off choice. Terminals are picked in Settings.
 */
export function OpenInTerminalButton({
  dir,
  command,
  className,
}: {
  dir: string;
  command: string;
  className?: string;
}) {
  const [menuOpen, setMenuOpen] = useState(false);
  const [prefs, setPrefs] = useState<TerminalPrefs>(loadTerminalPrefs);
  const anchorRef = useRef<HTMLDivElement>(null);
  const popoverRef = useRef<HTMLDivElement>(null);
  const anchorName = `--anchor-${useId().replace(/[^a-zA-Z0-9_-]/g, "")}`;

  const terminals = useQuery({ queryKey: queryKeys.terminals, queryFn: api.terminals, staleTime: 60_000 });

  useEffect(() => {
    let active = true;
    void fetchTerminalPrefs().then((loaded) => {
      if (active) setPrefs(loaded);
    });
    return () => {
      active = false;
    };
  }, []);
  const available = useMemo(() => enabledTerminals(terminals.data ?? [], prefs), [terminals.data, prefs]);
  const preferred = useMemo(() => defaultTerminal(terminals.data ?? [], prefs), [terminals.data, prefs]);

  const launch = useApiMutation({
    mutationFn: (terminalId: string) => api.openInTerminal(terminalId, dir, command),
    onSuccess: () => setMenuOpen(false),
  });

  useEffect(() => {
    if (!menuOpen) return;
    const onPointerDown = (event: PointerEvent) => {
      const target = event.target as Node | null;
      if (!target) return;
      if (anchorRef.current?.contains(target) || popoverRef.current?.contains(target)) return;
      setMenuOpen(false);
    };
    document.addEventListener("pointerdown", onPointerDown);
    return () => document.removeEventListener("pointerdown", onPointerDown);
  }, [menuOpen]);

  const loading = terminals.isLoading;
  const noTerminal = !loading && available.length === 0;

  return (
    <div
      ref={anchorRef}
      className={cn("anchor-trigger inline-flex", className)}
      style={{ "--anchor": anchorName } as React.CSSProperties}
    >
      <button
        type="button"
        disabled={loading || noTerminal || launch.isPending}
        title={
          noTerminal
            ? "No terminal emulator enabled — pick one in Settings"
            : `Open in ${preferred?.name ?? "terminal"}`
        }
        onClick={() => preferred && launch.mutate(preferred.id)}
        className={cn(SEGMENT, "rounded-l-sm", available.length > 1 ? "border-r-0" : "rounded-r-sm")}
      >
        <IconTerminal className="size-3" />
        Open
      </button>
      {available.length > 1 ? (
        <button
          type="button"
          disabled={loading || launch.isPending}
          aria-haspopup="menu"
          aria-expanded={menuOpen}
          aria-label="Choose a terminal"
          title="Choose a terminal"
          onClick={() => setMenuOpen((current) => !current)}
          className={cn(SEGMENT, "rounded-r-sm px-1")}
        >
          <IconChevronDown className="size-3" />
        </button>
      ) : null}

      {menuOpen
        ? createPortal(
            <div
              ref={popoverRef}
              role="menu"
              aria-label="Terminals"
              className="anchor-popover z-[70] flex flex-col overflow-hidden rounded-md border border-border bg-surface p-1 shadow-lg"
              style={{ "--anchor": anchorName, width: "13rem" } as React.CSSProperties}
            >
              {available.map((terminal) => (
                <button
                  key={terminal.id}
                  type="button"
                  role="menuitem"
                  onClick={() => launch.mutate(terminal.id)}
                  className="flex cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-left transition-colors hover:bg-hover"
                >
                  <span className="min-w-0 flex-1 truncate text-sm text-fg">{terminal.name}</span>
                  {terminal.id === preferred?.id ? (
                    <span className="shrink-0 text-2xs text-faint">default</span>
                  ) : null}
                </button>
              ))}
            </div>,
            document.body,
          )
        : null}
    </div>
  );
}
