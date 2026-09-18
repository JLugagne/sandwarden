// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider } from "@/components/Toaster";
import { KitsPage } from "./KitsPage";

afterEach(cleanup);

const item = {
  store: "sbx-kits-contrib",
  store_name: "sbx-kits-contrib",
  kind: "mixin",
  name: "code-server",
  display_name: "code-server (web VS Code)",
  description: "Runs code-server on port 8080.",
  version: "1.0.0",
  image: "",
  requires_agent: "claude",
  rel_path: "code-server",
  ref: "/home/dev/.local/share/sandwarden/kit-stores/sbx-kits-contrib/code-server",
  spec: {
    schemaVersion: "2",
    kind: "mixin",
    name: "code-server",
    description: "Runs code-server on port 8080.",
    requires: { agent: "claude" },
    ports: [{ container: 8080, protocol: "tcp", name: "code-server" }],
    permissions: { network: { allow: ["code-server.dev"] } },
    sandbox: {},
    environment: {},
    setup: {},
    arguments: {},
  },
};

vi.mock("@/api/client", () => ({
  api: {
    kitStores: vi.fn(async () => [
      {
        slug: "sbx-kits-contrib",
        name: "sbx-kits-contrib",
        description: "Community kits",
        url: "https://github.com/docker/sbx-kits-contrib",
        ref: "",
        auth: "",
        path: "/data/kits/contrib",
        synced_at: "2026-09-16T16:00:00Z",
        error: "",
      },
    ]),
    kitItems: vi.fn(async () => [item]),
    templates: vi.fn(async () => [
      {
        id: "abc123",
        repository: "docker.io/docker/sandbox-templates",
        tag: "opencode-docker",
        flavor: "opencode-docker",
        created_at: "2026-09-15T22:07:00Z",
        size: 1839288240,
      },
    ]),
    createKitStore: vi.fn(),
    updateKitStore: vi.fn(),
    deleteKitStore: vi.fn(),
    refreshKitStore: vi.fn(),
    validateKit: vi.fn(async () => ({ ok: true, output: "VALID: . (directory)" })),
    removeTemplate: vi.fn(),
  },
}));

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/kits"]}>
        <ToastProvider>
          <KitsPage />
        </ToastProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("kits page", () => {
  it("lists discovered kits and template images", async () => {
    renderPage();

    expect(await screen.findByText("code-server (web VS Code)")).toBeDefined();
    expect(screen.getByText("docker.io/docker/sandbox-templates")).toBeDefined();
  });

  it("shows the create-time reference and specs in the details modal", async () => {
    renderPage();

    fireEvent.click(await screen.findByRole("button", { name: "Details" }));

    expect(await screen.findByText(/kit-stores\/sbx-kits-contrib\/code-server/)).toBeDefined();
    expect(screen.getByText("code-server.dev")).toBeDefined();
    expect(screen.getByText("8080/tcp")).toBeDefined();
    expect(screen.getAllByText("Runs code-server on port 8080.").length).toBeGreaterThan(0);
  });

  it("reports the validation verdict", async () => {
    const { api } = await import("@/api/client");
    renderPage();

    fireEvent.click(await screen.findByRole("button", { name: "Validate" }));

    expect(await screen.findByText("Kit is valid")).toBeDefined();
    expect(vi.mocked(api.validateKit)).toHaveBeenCalledWith("sbx-kits-contrib", "code-server");
  });
});
