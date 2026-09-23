// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { api } from "@/api/client";
import { ToastProvider } from "@/components/Toaster";
import { resetConfigCacheForTests } from "@/lib/config";
import { SettingsPage } from "./SettingsPage";
import type { AppConfig } from "@/types";

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

const DEFAULT_AGENTS = "# Sandbox environment\n";

function agentsDoc(content: string, isDefault: boolean) {
  return {
    path: "/home/user/.config/sandwarden/agents/AGENTS.md",
    target: "/home/agent/.agents/AGENTS.md",
    content,
    default: isDefault,
  };
}

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
    notify: vi.fn(async () => undefined),
    notificationStatus: vi.fn(async () => ({ available: true, authorized: true })),
    requestNotificationAuthorization: vi.fn(async () => ({ available: true, authorized: true })),
    agentsFile: vi.fn(async () => agentsDoc(DEFAULT_AGENTS, true)),
    saveAgentsFile: vi.fn(async (content: string) => agentsDoc(content, false)),
    resetAgentsFile: vi.fn(async () => agentsDoc(DEFAULT_AGENTS, true)),
  },
}));

function LocationProbe() {
  const location = useLocation();
  return <output data-testid="location-search">{location.search}</output>;
}

function renderPage(entry = "/settings", queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })) {
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
    renderPage("/settings?tab=caches");

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
    renderPage("/settings?tab=files");

    expect(await screen.findByText("/home/user/.config/sandwarden")).toBeDefined();

    fireEvent.click(screen.getByRole("button", { name: /Reload from disk/ }));

    await waitFor(() => {
      expect(api.reloadFleet).toHaveBeenCalled();
    });
    expect(await screen.findByText("Configuration reloaded")).toBeDefined();
  });

  it("lists the per-file errors returned by the reload", async () => {
    vi.mocked(api.reloadFleet).mockResolvedValueOnce(["profiles/broken.yaml: invalid YAML"]);
    renderPage("/settings?tab=files");

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
    renderPage("/settings?tab=caches");

    expect(await screen.findByText("changed on disk")).toBeDefined();
    fireEvent.click(screen.getByRole("tab", { name: "Files" }));

    vi.mocked(api.configStaleness).mockResolvedValue({ stale: false, changed: [] });
    fireEvent.click(screen.getByRole("button", { name: /Reload from disk/ }));
    await waitFor(() => expect(api.reloadFleet).toHaveBeenCalled());
    fireEvent.click(screen.getByRole("tab", { name: "Caches" }));

    expect(await screen.findByText("go-mod")).toBeDefined();
    await waitFor(() => {
      expect(screen.queryByText("changed on disk")).toBeNull();
    });
  });
});

describe("terminal panel", () => {
  it("promotes a terminal to default and persists the preference through the config file", async () => {
    renderPage("/settings?tab=terminals");

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

describe("terminal preferences", () => {
  function kittyCheckbox() {
    const row = screen.getByText("kitty").closest("tr") as HTMLTableRowElement;
    return within(row).getByRole("checkbox") as HTMLInputElement;
  }

  it("keeps a disabled terminal unchecked after leaving and reopening the page", async () => {
    let saved: AppConfig = { terminals: { enabled: null, default: "" }, notifications: false };
    vi.mocked(api.getConfig).mockImplementation((async () => saved) as unknown as typeof api.getConfig);
    vi.mocked(api.setConfig).mockImplementation((async (next: AppConfig) => {
      saved = next;
    }) as unknown as typeof api.setConfig);
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const first = renderPage("/settings?tab=terminals", queryClient);
    await screen.findByText("kitty");
    fireEvent.click(kittyCheckbox());
    await waitFor(() => expect(api.setConfig).toHaveBeenCalled());
    first.unmount();

    renderPage("/settings?tab=terminals", queryClient);
    await screen.findByText("kitty");

    await waitFor(() => expect(kittyCheckbox().checked).toBe(false), { timeout: 2000 });
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(kittyCheckbox().checked).toBe(false);
  });

  it("reverts and reports a terminal change the backend refused to save", async () => {
    vi.mocked(api.setConfig).mockRejectedValueOnce(new Error("another sandwarden instance is applying changes, retry"));
    renderPage("/settings?tab=terminals");
    await screen.findByText("kitty");

    fireEvent.click(kittyCheckbox());

    await waitFor(() => expect(kittyCheckbox().checked).toBe(true), { timeout: 2000 });
    expect(await screen.findByText(/another sandwarden instance is applying changes/, {}, { timeout: 2000 })).toBeDefined();
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
      expect(screen.getByTestId("location-search").textContent).toBe("?tab=caches");
    });
    expect(Element.prototype.scrollIntoView).toHaveBeenCalled();
  });

  it("leaves every cache row unhighlighted without a deep link", async () => {
    renderPage("/settings?tab=caches");

    const row = (await screen.findByText("go-mod")).closest("tr") as HTMLTableRowElement;
    expect(row.className).not.toContain("ring-accent");
    expect(screen.getByTestId("location-search").textContent).toBe("?tab=caches");
  });
});

describe("settings tabs", () => {
  it("opens on General and switches tabs through the URL", async () => {
    renderPage();

    expect(await screen.findByText("Connection")).toBeDefined();
    expect(screen.queryByText("Shared caches")).toBeNull();

    fireEvent.click(screen.getByRole("tab", { name: "Caches" }));

    expect(await screen.findByText("Shared caches")).toBeDefined();
    expect(screen.getByTestId("location-search").textContent).toBe("?tab=caches");
  });
});

describe("agent instructions", () => {
  it("saves an edited AGENTS.md", async () => {
    renderPage("/settings?tab=agents");

    const editor = (await screen.findByRole("textbox", { name: "AGENTS.md content" })) as HTMLTextAreaElement;
    await waitFor(() => expect(editor.value).toBe(DEFAULT_AGENTS));
    fireEvent.change(editor, { target: { value: "# Custom\n" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(api.saveAgentsFile).toHaveBeenCalledWith("# Custom\n"));
    expect(await screen.findByText("customized")).toBeDefined();
  });

  it("resets AGENTS.md to the built-in template after confirmation", async () => {
    vi.mocked(api.agentsFile).mockResolvedValueOnce(agentsDoc("# Custom\n", false));
    renderPage("/settings?tab=agents");

    fireEvent.click(await screen.findByRole("button", { name: "Reset to default" }));
    fireEvent.click(await screen.findByRole("button", { name: "Reset" }));

    await waitFor(() => expect(api.resetAgentsFile).toHaveBeenCalled());
    const editor = screen.getByRole("textbox", { name: "AGENTS.md content" }) as HTMLTextAreaElement;
    await waitFor(() => expect(editor.value).toBe(DEFAULT_AGENTS));
  });
});

describe("notifications", () => {
  it("asks for authorization when enabled and explains why they cannot be sent", async () => {
    const unavailable = { available: false, authorized: false, reason: "notifications require a valid bundle identifier" };
    vi.mocked(api.notificationStatus).mockResolvedValue(unavailable);
    vi.mocked(api.requestNotificationAuthorization).mockResolvedValueOnce(unavailable);
    renderPage();
    await waitFor(() => expect(api.getConfig).toHaveBeenCalled());
    await new Promise((resolve) => setTimeout(resolve, 20));

    fireEvent.click(await screen.findByRole("checkbox", { name: "enable desktop notifications" }, { timeout: 2000 }));

    await waitFor(() => expect(api.requestNotificationAuthorization).toHaveBeenCalled(), { timeout: 2000 });
    expect(await screen.findByText(/Unavailable: notifications require a valid bundle identifier/, {}, { timeout: 2000 })).toBeDefined();
  });
});
