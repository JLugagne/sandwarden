// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "@/api/client";
import { ToastProvider } from "@/components/Toaster";
import type { KitItemView } from "@/types";
import { CreateSandboxDialog } from "./CreateSandboxDialog";

afterEach(cleanup);

const agentKitRef = "git+https://github.com/acme/kits#dir=go-agent";
const linterKitRef = "git+https://github.com/acme/kits#dir=go-linter";
const formatterKitRef = "git+https://github.com/acme/kits#dir=go-formatter";

const kitItem = (overrides: Partial<KitItemView>): KitItemView => ({
  id: 1,
  store_id: 1,
  store_name: "acme/kits",
  kind: "mixin",
  name: "kit",
  display_name: "",
  description: "",
  version: "",
  image: "",
  requires_agent: "",
  rel_path: "",
  ref: "",
  spec: { schemaVersion: "1", kind: "mixin", name: "kit" },
  ...overrides,
});

const kitItems: KitItemView[] = [
  kitItem({ id: 1, kind: "sandbox", name: "go-agent", display_name: "Go Agent Kit", ref: agentKitRef }),
  kitItem({ id: 2, kind: "mixin", name: "go-linter", display_name: "Go Linter", ref: linterKitRef }),
  kitItem({ id: 3, kind: "mixin", name: "go-formatter", display_name: "Go Formatter", ref: formatterKitRef }),
];

vi.mock("@/api/client", () => ({
  api: {
    templates: vi.fn(async () => []),
    kitItems: vi.fn(async () => []),
    fsPick: vi.fn(async () => ({ path: "/picked/folder" })),
    createSandbox: vi.fn(async () => ({ job_id: "job-1" })),
  },
}));

function renderDialog() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <ToastProvider>
          <CreateSandboxDialog open onClose={() => {}} />
        </ToastProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("new sandbox dialog", () => {
  it("splits the options across tabs instead of one long form", async () => {
    renderDialog();

    for (const label of ["General", "Resources", "Network", "Options"]) {
      expect(screen.getByRole("tab", { name: new RegExp(label) })).toBeDefined();
    }
    expect(screen.getByLabelText(/^Agent/)).toBeDefined();
    expect(screen.getByLabelText(/^Name \(optional\)/)).toBeDefined();
    expect(screen.getByLabelText(/^Template \(optional\)/)).toBeDefined();
    expect(screen.getByLabelText(/^Policy profile \(optional\)/)).toBeDefined();
    expect(screen.getByPlaceholderText("/absolute/host/path")).toBeDefined();
    expect(screen.getByLabelText(/^Kit mixins/)).toBeDefined();

    fireEvent.click(screen.getByRole("tab", { name: /Resources/ }));
    expect(screen.getByLabelText(/^CPUs/)).toBeDefined();
    expect(screen.getByLabelText(/^Memory/)).toBeDefined();

    fireEvent.click(screen.getByRole("tab", { name: /Network/ }));
    expect(screen.getByLabelText(/^Published ports/)).toBeDefined();
    expect(screen.getByLabelText(/^Denied network/)).toBeDefined();

    fireEvent.click(screen.getByRole("tab", { name: /Options/ }));
    expect(screen.getByLabelText(/^Environment/)).toBeDefined();
    expect(screen.getByText("Attach shared caches")).toBeDefined();
  });

  it("offers the sandbox kits of the repositories as agents", async () => {
    vi.mocked(api.kitItems).mockResolvedValue(kitItems);
    renderDialog();

    fireEvent.click(await screen.findByLabelText(/^Agent/));
    fireEvent.click(await screen.findByRole("option", { name: /Go Agent Kit/ }));

    expect(screen.getByLabelText(/^Agent/).textContent).toContain("Go Agent Kit");

    fireEvent.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(api.createSandbox).toHaveBeenCalledWith(expect.objectContaining({ agent: agentKitRef }));
    });
  });

  it("selects more than one kit mixin from the repositories", async () => {
    vi.mocked(api.kitItems).mockResolvedValue(kitItems);
    renderDialog();

    fireEvent.click(await screen.findByLabelText("Go Linter"));
    fireEvent.click(screen.getByLabelText("Go Formatter"));

    // Toggling off removes the reference again.
    fireEvent.click(screen.getByLabelText("Go Linter"));
    expect(screen.getByText(/1 selected/)).toBeDefined();
    fireEvent.click(screen.getByLabelText("Go Linter"));

    fireEvent.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(api.createSandbox).toHaveBeenCalledWith(
        expect.objectContaining({ kits: [formatterKitRef, linterKitRef] }),
      );
    });
  });

  it("submits values collected from every tab and streams in the progress tab", async () => {
    renderDialog();

    fireEvent.change(screen.getByLabelText(/^Name \(optional\)/), { target: { value: "my-sandbox" } });
    fireEvent.change(screen.getByPlaceholderText("/absolute/host/path"), { target: { value: "/host/project" } });

    fireEvent.click(screen.getByRole("tab", { name: /Resources/ }));
    fireEvent.change(screen.getByLabelText(/^CPUs/), { target: { value: "4" } });

    fireEvent.click(screen.getByRole("tab", { name: /Network/ }));
    fireEvent.change(screen.getByLabelText(/^Published ports/), { target: { value: "8080:80" } });

    fireEvent.click(screen.getByRole("tab", { name: /Options/ }));
    fireEvent.change(screen.getByLabelText(/^Environment/), { target: { value: "NODE_ENV=development" } });

    fireEvent.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(api.createSandbox).toHaveBeenCalledWith(
        expect.objectContaining({
          agent: "claude",
          name: "my-sandbox",
          workspaces: [{ path: "/host/project", read_only: false }],
          cpus: 4,
          publish: ["8080:80"],
          env: ["NODE_ENV=development"],
          attach_caches: true,
        }),
      );
    });

    const progress = await screen.findByRole("tab", { name: /Progress/ });
    expect(progress.getAttribute("aria-selected")).toBe("true");
    expect(screen.getByText("Waiting for output…")).toBeDefined();
  });

  it("adds and removes workspace rows from General", () => {
    renderDialog();

    fireEvent.click(screen.getByRole("button", { name: /Add workspace/ }));
    expect(screen.getAllByPlaceholderText("/absolute/host/path")).toHaveLength(2);

    fireEvent.click(screen.getAllByTitle("Remove workspace")[1]);
    expect(screen.getAllByPlaceholderText("/absolute/host/path")).toHaveLength(1);
  });
});
