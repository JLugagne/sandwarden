// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import type { Terminal } from "@/types";
import { defaultTerminal, enabledTerminals, loadTerminalPrefs, saveTerminalPrefs } from "./terminals";

const terminals: Terminal[] = [
  { id: "terminal", name: "Terminal", binary: "/usr/bin/osascript" },
  { id: "kitty", name: "kitty", binary: "/usr/bin/kitty" },
];

afterEach(() => window.localStorage.clear());

describe("terminal preferences", () => {
  it("enables every detected terminal by default", () => {
    const prefs = loadTerminalPrefs();
    expect(prefs.enabled).toBeNull();
    expect(enabledTerminals(terminals, prefs)).toEqual(terminals);
    expect(defaultTerminal(terminals, prefs)?.id).toBe("terminal");
  });

  it("round-trips saved preferences", () => {
    saveTerminalPrefs({ enabled: ["kitty"], default: "kitty" });
    expect(loadTerminalPrefs()).toEqual({ enabled: ["kitty"], default: "kitty" });
  });

  it("falls back when the stored value is malformed", () => {
    window.localStorage.setItem("sandwarden.terminals", "{not json");
    expect(loadTerminalPrefs()).toEqual({ enabled: null, default: "" });
  });

  it("filters non-string entries", () => {
    window.localStorage.setItem("sandwarden.terminals", JSON.stringify({ enabled: ["kitty", 42], default: 7 }));
    const prefs = loadTerminalPrefs();
    expect(prefs.enabled).toEqual(["kitty"]);
    expect(prefs.default).toBe("");
  });

  it("keeps only enabled terminals and falls back when the default is disabled", () => {
    const prefs = { enabled: ["kitty"], default: "terminal" };
    expect(enabledTerminals(terminals, prefs)).toEqual([terminals[1]]);
    expect(defaultTerminal(terminals, prefs)?.id).toBe("kitty");
  });

  it("returns no default when everything is disabled", () => {
    const prefs = { enabled: [], default: "kitty" };
    expect(defaultTerminal(terminals, prefs)).toBeNull();
  });
});
