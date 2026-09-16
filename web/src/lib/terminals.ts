import type { Terminal } from "@/types";

const STORAGE_KEY = "sandwarden.terminals";

export interface TerminalPrefs {
  /** Enabled terminal ids; null means every detected terminal is enabled. */
  enabled: string[] | null;
  /** Preferred terminal id; empty falls back to the first enabled terminal. */
  default: string;
}

const FALLBACK: TerminalPrefs = { enabled: null, default: "" };

export function loadTerminalPrefs(): TerminalPrefs {
  if (typeof window === "undefined") return FALLBACK;
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return FALLBACK;
    const parsed = JSON.parse(raw) as Partial<TerminalPrefs> | null;
    if (!parsed || typeof parsed !== "object") return FALLBACK;
    return {
      enabled: Array.isArray(parsed.enabled)
        ? parsed.enabled.filter((id): id is string => typeof id === "string")
        : null,
      default: typeof parsed.default === "string" ? parsed.default : "",
    };
  } catch {
    return FALLBACK;
  }
}

export function saveTerminalPrefs(prefs: TerminalPrefs): void {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(prefs));
  } catch {
    // Storage can be disabled or full; preferences are best-effort.
  }
}

export function enabledTerminals(terminals: Terminal[], prefs: TerminalPrefs): Terminal[] {
  if (prefs.enabled === null) return terminals;
  const allowed = new Set(prefs.enabled);
  return terminals.filter((terminal) => allowed.has(terminal.id));
}

export function defaultTerminal(terminals: Terminal[], prefs: TerminalPrefs): Terminal | null {
  const enabled = enabledTerminals(terminals, prefs);
  if (enabled.length === 0) return null;
  return enabled.find((terminal) => terminal.id === prefs.default) ?? enabled[0];
}
