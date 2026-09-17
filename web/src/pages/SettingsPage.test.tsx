// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { api } from "@/api/client";
import { ToastProvider } from "@/components/Toaster";
import { resetConfigCacheForTests } from "@/lib/config";
import { SettingsPage } from "./SettingsPage";

beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn();
});

afterEach(() => {
  cleanup();
  resetConfigCacheForTests();
  vi.clearAllMocks();
});

const cache = {
  slug: "go-mod",
  dir: "go-mod",
  name: "go-mod",
  description: "",
  host_path: "/home/user/go/pkg/mod",
  target_path: "/home/agent/go/pkg/mod",
  read_only: false,
  auto_attach: true,
  enabled: true,
};

const terminalRows = [
  { id: "terminal", name: "Terminal", binary: "/usr/bin/osascript" },
  { id: "kitty", name: "kitty", binary: "/usr/bin/kitty" },
];

vi.mock("@/api/client", () => ({
  api: {
    health: vi.fn(async () => ({
      ok: true,
      socket: "/run/sandboxd.sock",
      sbx_binary: "sbx",
      daemon_running: true,
      daemon_status: "running",
    })),
    versionInfo: vi.fn(async () => ({ version: "dev", latest_version: "", update_available: false, release_url: "" })),
    checkUpdates: vi.fn(async () => ({ version: "dev", latest_version: "", update_available: false, release_url: "" })),
    caches: vi.fn(async () => [cache]),
    terminals: vi.fn(async () => terminalRows),
    getConfig: vi.fn(async () => ({ terminals: { enabled: null, default: "" }, notifications: false })),
    setConfig: vi.fn(async () => undefined),
    fleetDir: vi.fn(async () => "/home/user/.config/sandwarden"),
    reloadFleet: vi.fn(async () => [] as string[]),
    startDaemon: vi.fn(),
    createCache: vi.fn(),
    updateCache: vi.fn(),
    deleteCache: vi.fn(),
    configStaleness: vi.fn(async () => ({ stale: false, changed: [] })),
  },
}));

function LocationProbe() {
  const location = useLocation();
  return <output data-testid="location-search">{location.search}</output>;
}

function renderPage(entry = "/settings") {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[entry]}>
        <LocationProbe />
        <ToastProvider>
          <SettingsPage />
        </ToastProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("cache dialog", () => {
  it("discards cancelled edits when the same cache is reopened", async () => {
    renderPage();

    fireEvent.click(await screen.findByRole("button", { name: "Edit" }));
    fireEvent.change(await screen.findByDisplayValue("go-mod"), { target: { value: "renamed" } });
    expect(screen.getByDisplayValue("renamed")).toBeDefined();

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    fireEvent.click(await screen.findByRole("button", { name: "Edit" }));

    await waitFor(() => {
      expect(screen.getByDisplayValue("go-mod")).toBeDefined();
    });
    expect(screen.queryByDisplayValue("renamed")).toBeNull();
  });
});

describe("configuration files", () => {
  it("shows the fleet directory and reloads every file from disk", async () => {
    renderPage();

    expect(await screen.findByText("/home/user/.config/sandwarden")).toBeDefined();

    fireEvent.click(screen.getByRole("button", { name: /Reload from disk/ }));

    await waitFor(() => {
      expect(api.reloadFleet).toHaveBeenCalled();
    });
    expect(await screen.findByText("Configuration reloaded")).toBeDefined();
  });

  it("lists the per-file errors returned by the reload", async () => {
    vi.mocked(api.reloadFleet).mockResolvedValueOnce(["profiles/broken.yaml: invalid YAML"]);
    renderPage();

    fireEvent.click(await screen.findByRole("button", { name: /Reload from disk/ }));

    expect(await screen.findByText("profiles/broken.yaml: invalid YAML")).toBeDefined();
  });

  it("badges a cache changed on disk and clears it after reload", async () => {
    vi.mocked(api.configStaleness).mockResolvedValue({
      stale: true,
      changed: [
        {
          kind: "cache",
          slug: "go-mod",
          name: "go-mod",
          file: "sandwarden.yaml",
          path: "/home/user/.config/sandwarden/caches/go-mod/sandwarden.yaml",
        },
      ],
    });
    renderPage();

    expect(await screen.findByText("changed on disk")).toBeDefined();

    vi.mocked(api.configStaleness).mockResolvedValue({ stale: false, changed: [] });
    fireEvent.click(screen.getByRole("button", { name: /Reload from disk/ }));

    await waitFor(() => {
      expect(screen.queryByText("changed on disk")).toBeNull();
    });
  });
});

describe("terminal panel", () => {
  it("promotes a terminal to default and persists the preference through the config file", async () => {
    renderPage();

    const kittyRow = (await screen.findByText("kitty")).closest("tr") as HTMLTableRowElement;
    expect(within(kittyRow).queryByText("default")).toBeNull();

    fireEvent.click(within(kittyRow).getByRole("button", { name: "Use by default" }));

    await waitFor(() => {
      expect(within(kittyRow).queryByText("default")).not.toBeNull();
    });
    await waitFor(() => {
      expect(api.setConfig).toHaveBeenCalledWith({
        terminals: { enabled: ["terminal", "kitty"], default: "kitty" },
        notifications: false,
      });
    });
  });
});

describe("cache deep link", () => {
  it("highlights the cache row linked from the search overlay and strips the parameter", async () => {
    renderPage("/settings?cache=go-mod");

    const row = (await screen.findByText("go-mod")).closest("tr") as HTMLTableRowElement;
    await waitFor(() => {
      expect(row.className).toContain("ring-accent");
    });
    await waitFor(() => {
      expect(screen.getByTestId("location-search").textContent).toBe("");
    });
    expect(Element.prototype.scrollIntoView).toHaveBeenCalled();
  });

  it("leaves every cache row unhighlighted without a deep link", async () => {
    renderPage();

    const row = (await screen.findByText("go-mod")).closest("tr") as HTMLTableRowElement;
    expect(row.className).not.toContain("ring-accent");
    expect(screen.getByTestId("location-search").textContent).toBe("");
  });
});
