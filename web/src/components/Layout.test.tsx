// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
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
  },
}));

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
