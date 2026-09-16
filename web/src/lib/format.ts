/** Format an RFC3339 timestamp for display, tolerating empty/invalid input. */
export function formatTime(value?: string | null): string {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  const diff = Date.now() - date.getTime();
  const minute = 60_000;
  const hour = 60 * minute;
  const day = 24 * hour;
  if (diff >= 0 && diff < minute) return "just now";
  if (diff >= 0 && diff < hour) return `${Math.floor(diff / minute)}m ago`;
  if (diff >= 0 && diff < day) return `${Math.floor(diff / hour)}h ago`;
  if (diff >= 0 && diff < 7 * day) return `${Math.floor(diff / day)}d ago`;
  return date.toLocaleString();
}

/** Render a port mapping as host → sandbox. */
export function formatPort(hostIP: string, hostPort: number, sandboxPort: number, protocol: string): string {
  const ip = hostIP && hostIP !== "0.0.0.0" ? `${hostIP}:` : "";
  return `${ip}${hostPort} → ${sandboxPort}/${protocol}`;
}

/** Split a comma-separated input into trimmed, non-empty entries. */
export function splitList(value: string): string[] {
  return value
    .split(/[,\n]/)
    .map((part) => part.trim())
    .filter(Boolean);
}

/** Join a nullable list for display. */
export function joinList(values?: string[] | null, separator = ", "): string {
  return (values ?? []).join(separator);
}
