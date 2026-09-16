import { useState } from "react";
import { NavLink, Outlet } from "react-router-dom";
import { cn } from "@/lib/cn";
import { useConnectionStatus } from "@/app/RealtimeProvider";
import { notificationsEnabled, setNotificationsEnabled } from "@/lib/notifications";
import { Button, IconSearch } from "@/components/ui";
import { OPEN_SEARCH_EVENT, SearchOverlay } from "@/components/SearchOverlay";

const NAV = [
  { to: "/", label: "Sandboxes", end: true },
  { to: "/profiles", label: "Profiles" },
  { to: "/skills", label: "Skills" },
  { to: "/kits", label: "Kits" },
  { to: "/traffic", label: "Traffic" },
  { to: "/secrets", label: "Secrets" },
  { to: "/settings", label: "Settings" },
];

const STATUS_META = {
  online: { label: "live", className: "bg-success" },
  connecting: { label: "connecting", className: "bg-warning" },
  offline: { label: "offline", className: "bg-danger" },
} as const;

export function Layout() {
  const status = useConnectionStatus();
  const meta = STATUS_META[status];
  const [notificationsOn, setNotificationsOn] = useState(notificationsEnabled());

  return (
    <div className="flex min-h-dvh">
      <aside className="sticky top-0 flex h-dvh w-56 shrink-0 flex-col border-r border-border bg-surface/60">
        <div className="flex h-14 items-center px-4 text-[15px] font-semibold tracking-tight">
          sand<span className="text-accent">warden</span>
        </div>

        <div className="px-2 pb-1">
          <button
            type="button"
            onClick={() => window.dispatchEvent(new Event(OPEN_SEARCH_EVENT))}
            className="flex w-full items-center gap-2 rounded-sm border border-border px-2.5 py-1.5 text-xs text-muted transition-colors hover:bg-hover hover:text-fg"
          >
            <IconSearch className="size-3.5 shrink-0" />
            Search
            <kbd className="ml-auto rounded border border-border px-1 text-2xs text-faint">⌘K</kbd>
          </button>
        </div>

        <nav className="flex flex-1 flex-col gap-0.5 px-2" aria-label="Main">
          {NAV.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) =>
                cn(
                  "rounded-sm px-3 py-2 text-sm transition-colors",
                  isActive ? "bg-hover text-fg" : "text-muted hover:bg-hover/60 hover:text-fg",
                )
              }
            >
              {item.label}
            </NavLink>
          ))}
        </nav>

        <div className="flex flex-col gap-2 border-t border-border px-4 py-3">
          <span className="flex items-center gap-1.5 text-2xs text-faint" title={`events ${meta.label}`}>
            <span className={cn("size-1.5 rounded-full", meta.className)} aria-hidden="true" />
            {meta.label}
          </span>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => {
              const next = !notificationsOn;
              setNotificationsOn(next);
              setNotificationsEnabled(next);
            }}
          >
            Notifications {notificationsOn ? "on" : "off"}
          </Button>
        </div>
      </aside>

      <main className="min-w-0 flex-1 px-6 py-6">
        <div className="mx-auto max-w-6xl">
          <Outlet />
        </div>
      </main>

      <SearchOverlay />
    </div>
  );
}
