// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "@/api/client";
import { ToastProvider } from "@/components/Toaster";
import { resetConfigCacheForTests, type StaleFile } from "@/lib/config";
import { Layout } from "./Layout";

afterEach(() => {
  cleanup();
  resetConfigCacheForTests();
  vi.clearAllMocks();
});

vi.mock("@/api/client", () => ({
  api: {
    configStaleness: vi.fn(async () => ({ stale: false, changed: [] })),
    getConfig: vi.fn(async () => ({ terminals: { enabled: null, default: "" }, notifications: false })),
    reloadFleet: vi.fn(async () => [] as string[]),
    listSandboxes: vi.fn(async () => []),
    restartDaemon: vi.fn(async () => undefined),
    sbxCompatibility: vi.fn(async () => ({
      version: "v0.54.1",
      client_version: "v0.54.1",
      daemon_version: "v0.54.1",
      known: true,
      daemon_restart_required: false,
      features: [],
    })),
  },
}));

const restartNeeded = {
  version: "v0.43.0",
  client_version: "v0.54.1",
  daemon_version: "v0.43.0",
  known: true,
  daemon_restart_required: true,
  features: [],
};

function sandbox(name: string, running: boolean) {
  return { name, id: name, status: running ? "running" : "stopped", running } as never;
}

const staleFile: StaleFile = {
  kind: "sandbox",
  slug: "my-vm",
  name: "My VM",
  file: "sandwarden.yaml",
  path: "/config/sandboxes/my-vm/sandwarden.yaml",
};

function renderLayout() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <ToastProvider>
          <Layout />
        </ToastProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("daemon restart banner", () => {
  it("stays hidden while the CLI and daemon versions match", async () => {
    renderLayout();

    await screen.findByText("Search");
    expect(screen.queryByText(/sandboxd still runs/)).toBeNull();
    expect(screen.queryByRole("button", { name: "Restart daemon" })).toBeNull();
  });

  it("shows the banner and lists running sandboxes in the confirmation", async () => {
    vi.mocked(api.sbxCompatibility).mockResolvedValue(restartNeeded);
    vi.mocked(api.listSandboxes).mockResolvedValue([sandbox("web", true), sandbox("idle", false)]);
    renderLayout();

    expect(
      await screen.findByText(
        "sbx was updated to v0.54.1 but sandboxd still runs v0.43.0. Restart the daemon to finish the update — this stops running sandboxes.",
      ),
    ).toBeDefined();

    fireEvent.click(screen.getByRole("button", { name: "Restart daemon" }));

    const dialog = await screen.findByRole("dialog");
    expect(await within(dialog).findByText("web")).toBeDefined();
    expect(within(dialog).queryByText("idle")).toBeNull();

    fireEvent.click(within(dialog).getByRole("button", { name: "Restart daemon" }));

    await waitFor(() => expect(api.restartDaemon).toHaveBeenCalled());
  });
});

describe("layout staleness hint", () => {
  it("shows a global hint when a file changed on disk and clears it after reload", async () => {
    vi.mocked(api.configStaleness).mockResolvedValue({ stale: true, changed: [staleFile] });
    renderLayout();

    expect(await screen.findByText(/configuration file changed on disk/)).toBeDefined();

    vi.mocked(api.configStaleness).mockResolvedValue({ stale: false, changed: [] });
    fireEvent.click(screen.getByRole("button", { name: /Reload from disk/ }));

    await waitFor(() => {
      expect(screen.queryByText(/configuration file changed on disk/)).toBeNull();
    });
    expect(api.reloadFleet).toHaveBeenCalled();
  });

  it("stays quiet while every file matches the in-memory state", async () => {
    renderLayout();

    await screen.findByText("Search");
    expect(screen.queryByText(/changed on disk/)).toBeNull();
  });
});
