// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider } from "@/components/Toaster";
import { SettingsPage } from "./SettingsPage";

afterEach(() => {
  cleanup();
  window.localStorage.clear();
});

const cache = {
  id: 1,
  name: "go-mod",
  description: "",
  host_path: "/home/user/go/pkg/mod",
  target_path: "/home/agent/go/pkg/mod",
  read_only: false,
  auto_attach: true,
  enabled: true,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

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
    terminals: vi.fn(async () => [
      { id: "terminal", name: "Terminal", binary: "/usr/bin/osascript" },
      { id: "kitty", name: "kitty", binary: "/usr/bin/kitty" },
    ]),
    startDaemon: vi.fn(),
    createCache: vi.fn(),
    updateCache: vi.fn(),
    deleteCache: vi.fn(),
  },
}));

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/settings"]}>
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

describe("terminal panel", () => {
  it("promotes a terminal to default and persists the preference", async () => {
    renderPage();

    const kittyRow = (await screen.findByText("kitty")).closest("tr") as HTMLTableRowElement;
    expect(within(kittyRow).queryByText("default")).toBeNull();

    fireEvent.click(within(kittyRow).getByRole("button", { name: "Use by default" }));

    await waitFor(() => {
      expect(within(kittyRow).queryByText("default")).not.toBeNull();
    });
    expect(window.localStorage.getItem("sandwarden.terminals")).toContain('"default":"kitty"');
  });
});
