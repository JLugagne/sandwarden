// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider } from "@/components/Toaster";
import type { SandboxSummary } from "@/types";
import { SandboxesPage } from "./SandboxesPage";

afterEach(cleanup);

function sandbox(name: string, running: boolean): SandboxSummary {
  return {
    name,
    id: name,
    status: running ? "running" : "stopped",
    running,
    workspace: `/w/${name}`,
    mount_policy_denied: false,
    profiles: [],
    run_args: "",
    connect: { run: `sbx run ${name}`, shell: `sbx exec ${name} bash` },
    cpu_percent: 0,
    memory_used_bytes: 0,
    memory_total_bytes: 0,
  };
}

const listSandboxes = vi.fn(async () => [
  sandbox("alpha", true),
  sandbox("bravo", false),
  sandbox("charlie", true),
]);

vi.mock("@/api/client", () => ({
  api: {
    listSandboxes: () => listSandboxes(),
    health: vi.fn(async () => ({
      socket: "/run/sandboxd.sock",
      sbx_binary: "/usr/bin/sbx",
      daemon_running: true,
      daemon_status: "running",
    })),
    startDaemon: vi.fn(),
    startSandbox: vi.fn(),
    stopSandbox: vi.fn(),
    deleteSandbox: vi.fn(),
  },
}));

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/sandboxes"]}>
        <ToastProvider>
          <SandboxesPage />
        </ToastProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

/** Reads the card names listed under each run-state section, in page order. */
function sectionNames(container: HTMLElement) {
  return Array.from(container.querySelectorAll("section")).map((section) => ({
    heading: section.querySelector("h2")?.textContent?.trim(),
    names: Array.from(section.querySelectorAll('[role="link"]')).map((card) =>
      card.getAttribute("aria-label")?.replace(/^Open | settings$/g, ""),
    ),
  }));
}

describe("sandboxes page", () => {
  it("groups the cards into a started and a stopped section", async () => {
    const { container } = renderPage();

    expect(await screen.findByText("Started")).toBeDefined();
    expect(sectionNames(container)).toEqual([
      { heading: "Started (2)", names: ["alpha", "charlie"] },
      { heading: "Stopped (1)", names: ["bravo"] },
    ]);
  });

  it("leaves out a section that has no sandboxes", async () => {
    listSandboxes.mockResolvedValueOnce([sandbox("alpha", true)]);
    const { container } = renderPage();

    expect(await screen.findByText("Started")).toBeDefined();
    expect(sectionNames(container)).toEqual([{ heading: "Started (1)", names: ["alpha"] }]);
    expect(screen.queryByText("Stopped")).toBeNull();
  });
});
