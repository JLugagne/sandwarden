import { cachedConfig, loadConfig, updateConfig } from "@/lib/config";
import type { Terminal } from "@/types";

export interface TerminalPrefs {
  /** Enabled terminal ids; null means every detected terminal is enabled. */
  enabled: string[] | null;
  /** Preferred terminal id; empty falls back to the first enabled terminal. */
  default: string;
}

/** Synchronous view of the terminal preferences cached from the backend. */
export function loadTerminalPrefs(): TerminalPrefs {
  return cachedConfig().terminals;
}

/** Loads the backend configuration and returns the terminal preferences. */
export async function fetchTerminalPrefs(): Promise<TerminalPrefs> {
  return (await loadConfig()).terminals;
}

/** Persists the terminal preferences to the backend config file. */
export async function saveTerminalPrefs(prefs: TerminalPrefs): Promise<void> {
  await updateConfig({ terminals: prefs });
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
