// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Terminal } from "@/types";
import { resetConfigCacheForTests } from "./config";
import {
  defaultTerminal,
  enabledTerminals,
  fetchTerminalPrefs,
  loadTerminalPrefs,
  saveTerminalPrefs,
} from "./terminals";

vi.mock("@/api/client", () => ({
  api: {
    getConfig: vi.fn(async () => ({
      terminals: { enabled: ["kitty"], default: "kitty" },
      notifications: true,
    })),
    setConfig: vi.fn(async () => undefined),
  },
}));

const terminals: Terminal[] = [
  { id: "terminal", name: "Terminal", binary: "/usr/bin/osascript" },
  { id: "kitty", name: "kitty", binary: "/usr/bin/kitty" },
];

afterEach(() => {
  resetConfigCacheForTests();
  vi.clearAllMocks();
});

describe("terminal helpers", () => {
  it("enables every detected terminal by default", () => {
    const prefs = { enabled: null, default: "" };
    expect(enabledTerminals(terminals, prefs)).toEqual(terminals);
    expect(defaultTerminal(terminals, prefs)?.id).toBe("terminal");
  });

  it("keeps only enabled terminals and falls back when the default is disabled", () => {
    const prefs = { enabled: ["kitty"], default: "terminal" };
    expect(enabledTerminals(terminals, prefs)).toEqual([terminals[1]]);
    expect(defaultTerminal(terminals, prefs)?.id).toBe("kitty");
  });

  it("returns no default when everything is disabled", () => {
    expect(defaultTerminal(terminals, { enabled: [], default: "kitty" })).toBeNull();
  });
});

describe("terminal preferences", () => {
  it("falls back while the backend config has not been loaded", () => {
    expect(loadTerminalPrefs()).toEqual({ enabled: null, default: "" });
  });

  it("loads the preferences from the backend config file", async () => {
    const { api } = await import("@/api/client");
    await expect(fetchTerminalPrefs()).resolves.toEqual({ enabled: ["kitty"], default: "kitty" });
    expect(vi.mocked(api.getConfig)).toHaveBeenCalled();
    expect(loadTerminalPrefs()).toEqual({ enabled: ["kitty"], default: "kitty" });
  });

  it("persists the preferences through SetConfig", async () => {
    const { api } = await import("@/api/client");
    await saveTerminalPrefs({ enabled: ["kitty"], default: "kitty" });
    expect(vi.mocked(api.setConfig)).toHaveBeenCalledWith({
      terminals: { enabled: ["kitty"], default: "kitty" },
      notifications: true,
    });
  });
});
