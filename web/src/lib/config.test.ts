import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "@/api/client";
import { loadConfig, resetConfigCacheForTests, updateConfig } from "@/lib/config";

vi.mock("@/api/client", () => ({
  api: {
    getConfig: vi.fn(),
    setConfig: vi.fn(async () => undefined),
  },
}));

afterEach(() => {
  resetConfigCacheForTests();
  vi.clearAllMocks();
});

describe("updateConfig", () => {
  it("never overwrites the saved terminals when the config could not be loaded", async () => {
    vi.mocked(api.getConfig).mockRejectedValueOnce(new Error("backend not ready"));

    await updateConfig({ notifications: true }).catch(() => undefined);

    expect(api.setConfig).not.toHaveBeenCalledWith(expect.objectContaining({ terminals: { enabled: null, default: "" } }));
  });

  it("retries loading after a failed attempt", async () => {
    vi.mocked(api.getConfig)
      .mockRejectedValueOnce(new Error("backend not ready"))
      .mockResolvedValueOnce({ terminals: { enabled: ["kitty"], default: "kitty" }, notifications: false });

    await loadConfig().catch(() => undefined);
    await updateConfig({ notifications: true });

    expect(api.setConfig).toHaveBeenCalledWith({ terminals: { enabled: ["kitty"], default: "kitty" }, notifications: true });
  });
});
