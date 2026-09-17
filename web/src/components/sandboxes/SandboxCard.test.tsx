// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "@/api/client";
import { ToastProvider } from "@/components/Toaster";
import type { SandboxSummary } from "@/types";
import { SandboxCard } from "./SandboxCard";

afterEach(cleanup);

vi.mock("@/api/client", () => ({
  api: {
    startSandbox: vi.fn(),
    stopSandbox: vi.fn(),
    deleteSandbox: vi.fn(async () => undefined),
    configStaleness: vi.fn(async () => ({ stale: false, changed: [] })),
  },
}));

function sandbox(overrides: Partial<SandboxSummary> = {}): SandboxSummary {
  return {
    name: "box",
    id: "1",
    status: "running",
    running: true,
    workspace: "/w",
    mount_policy_denied: false,
    incomplete: false,
    profiles: [],
    run_args: "",
    connect: { run: "sbx run box", shell: "sbx exec box bash" },
    cpu_percent: 0,
    memory_used_bytes: 0,
    memory_total_bytes: 0,
    ...overrides,
  };
}

function renderCard(summary: SandboxSummary) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <ToastProvider>
          <SandboxCard sandbox={summary} />
        </ToastProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("sandbox card resource indicators", () => {
  it("shows the CPU and memory meters once the sandbox has a sample", () => {
    const { container } = renderCard(
      sandbox({
        cpu_percent: 42.4,
        memory_used_bytes: 1536 * 1024 * 1024,
        memory_total_bytes: 4 * 1024 * 1024 * 1024,
      }),
    );

    expect(screen.getByText("CPU")).toBeDefined();
    expect(screen.getByText("42%")).toBeDefined();
    expect(screen.getByText("MEM")).toBeDefined();
    expect(screen.getByText("1.5 GiB / 4.0 GiB")).toBeDefined();

    const fills = container.querySelectorAll<HTMLElement>("[data-meter-fill]");
    expect(fills).toHaveLength(2);
    expect(parseFloat(fills[0].style.width)).toBeCloseTo(42.4);
  });

  it("shows the meters disabled for a stopped sandbox", () => {
    const { container } = renderCard(
      sandbox({
        running: false,
        status: "stopped",
        cpu_percent: 12,
        memory_used_bytes: 1024,
        memory_total_bytes: 4096,
      }),
    );

    expect(screen.getByText("CPU")).toBeDefined();
    expect(screen.getByText("12%")).toBeDefined();
    expect(screen.getByText("MEM")).toBeDefined();
    expect(screen.getByText("1.0 KiB / 4.0 KiB")).toBeDefined();

    const fills = container.querySelectorAll<HTMLElement>("[data-meter-fill]");
    expect(fills).toHaveLength(2);
    for (const fill of fills) {
      expect(fill.style.width).toBe("0%");
      expect(fill.className).toContain("bg-border-strong");
    }
  });

  it("shows disabled placeholder meters until the first sample lands", () => {
    const { container } = renderCard(sandbox());

    expect(screen.getAllByText("—")).toHaveLength(2);

    const fills = container.querySelectorAll<HTMLElement>("[data-meter-fill]");
    expect(fills).toHaveLength(2);
    for (const fill of fills) {
      expect(fill.style.width).toBe("0%");
    }
  });
});

describe("sandbox card incomplete config", () => {
  it("badges an incomplete config directory", () => {
    renderCard(sandbox({ incomplete: true }));

    expect(screen.getByText("incomplete")).toBeDefined();
  });

  it("hides the badge when the config is complete", async () => {
    renderCard(sandbox());

    await screen.findByText("CPU");
    expect(screen.queryByText("incomplete")).toBeNull();
  });
});

describe("sandbox card deletion", () => {
  it("forces the deletion after the user confirms it", async () => {
    renderCard(sandbox());

    fireEvent.click(screen.getByTitle("Delete"));
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Delete" }));

    await waitFor(() => {
      expect(api.deleteSandbox).toHaveBeenCalledWith("box", true, false);
    });
  });

  it("also purges the config directory when asked", async () => {
    renderCard(sandbox());

    fireEvent.click(screen.getByTitle("Delete"));
    const dialog = screen.getByRole("dialog");
    fireEvent.click(within(dialog).getByRole("checkbox", { name: "also delete the config directory" }));
    fireEvent.click(within(dialog).getByRole("button", { name: "Delete" }));

    await waitFor(() => {
      expect(api.deleteSandbox).toHaveBeenCalledWith("box", true, true);
    });
  });
});

describe("sandbox card staleness", () => {
  it("badges a sandbox whose config file changed on disk", async () => {
    vi.mocked(api.configStaleness).mockResolvedValue({
      stale: true,
      changed: [
        {
          kind: "sandbox",
          slug: "box",
          name: "box",
          file: "sandwarden.yaml",
          path: "/config/sandboxes/box/sandwarden.yaml",
        },
      ],
    });

    renderCard(sandbox());

    expect(await screen.findByText("changed on disk")).toBeDefined();
  });

  it("hides the badge when the fleet is not stale", async () => {
    renderCard(sandbox());

    await screen.findByText("CPU");
    expect(screen.queryByText("changed on disk")).toBeNull();
  });
});
