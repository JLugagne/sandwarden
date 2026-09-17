// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "@/api/client";
import { ToastProvider } from "@/components/Toaster";
import { jobHub } from "@/store/jobs";
import type { SandboxDetail } from "@/types";
import { SandboxDetailPage } from "./SandboxDetailPage";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const detail: SandboxDetail = {
  sandbox: {
    name: "alpha",
    id: "alpha",
    agent: "claude",
    status: "running",
    running: true,
    workspace: "/w/alpha",
    mount_policy_denied: false,
    incomplete: false,
    profiles: [],
    run_args: "",
    connect: { run: "sbx run alpha", shell: "sbx exec alpha bash" },
    cpu_percent: 0,
    memory_used_bytes: 0,
    memory_total_bytes: 0,
  },
  incomplete: false,
  profiles: [],
  mounts: [],
  kits: [],
  secrets: [],
  custom_secrets: [],
  policy_rules: [],
  caches: [],
  profile_mounts: [],
  skills: [],
  additional_workspaces: [],
};

vi.mock("@/api/client", () => ({
  api: {
    sandbox: vi.fn(async () => detail),
    sandboxConfigDir: vi.fn(async () => "/home/user/.config/sandwarden/sandboxes/alpha"),
    readSandboxConfig: vi.fn(async () => []),
    applySandbox: vi.fn(async (_name: string, jobId: string) => ({ job_id: jobId })),
    recreateSandbox: vi.fn(async (_name: string, jobId: string) => ({ job_id: jobId })),
    attachKit: vi.fn(async (_name: string, ref: string) => ({
      sandbox: "alpha",
      ref,
      report: {
        rules_applied: 0,
        mounts_applied: 0,
        caches_applied: 0,
        skills_applied: 0,
        skills_removed: 0,
        warnings: [],
        errors: [],
      },
    })),
    validateSandbox: vi.fn(async () => ({ ok: true, output: "VALID: sandboxes/alpha" })),
    startSandbox: vi.fn(async () => undefined),
    stopSandbox: vi.fn(async () => undefined),
    deleteSandbox: vi.fn(async () => undefined),
    cancelJob: vi.fn(async () => undefined),
    setSandboxRunArgs: vi.fn(async () => undefined),
    completeSandboxConfig: vi.fn(async () => undefined),
    exec: vi.fn(async (_name: string, _command: string, jobId: string) => ({ job_id: jobId })),
    configStaleness: vi.fn(async () => ({ stale: false, changed: [] })),
  },
}));

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/sandboxes/alpha"]}>
        <ToastProvider>
          <Routes>
            <Route path="/sandboxes/:name" element={<SandboxDetailPage />} />
          </Routes>
        </ToastProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("sandbox config actions", () => {
  it("shows the config directory with a copy affordance", async () => {
    renderPage();

    expect(await screen.findByText("/home/user/.config/sandwarden/sandboxes/alpha")).toBeDefined();
    expect(screen.getAllByRole("button", { name: "Copy" }).length).toBeGreaterThan(0);
  });

  it("hides the config directory when the lookup fails", async () => {
    vi.mocked(api.sandboxConfigDir).mockRejectedValueOnce(new Error("no config directory"));
    renderPage();

    expect(await screen.findByRole("button", { name: /Apply/ })).toBeDefined();
    await waitFor(() => {
      expect(screen.queryByText("Config")).toBeNull();
    });
  });

  it("shows the config files of the sandbox", async () => {
    vi.mocked(api.readSandboxConfig).mockResolvedValue([
      {
        name: "spec.yaml",
        path: "/home/user/.config/sandwarden/sandboxes/alpha/spec.yaml",
        content: "name: alpha\n",
        error: "",
      },
    ]);
    renderPage();

    fireEvent.click(await screen.findByRole("button", { name: "View files" }));
    const dialog = await screen.findByRole("dialog");

    expect(await within(dialog).findByRole("tab", { name: /spec.yaml/ })).toBeDefined();
    expect(within(dialog).getByRole("button", { name: "Copy" })).toBeDefined();
    expect(vi.mocked(api.readSandboxConfig)).toHaveBeenCalledWith("alpha");
  });

  it("renders a broken sidecar's parse error without crashing the page", async () => {
    vi.mocked(api.readSandboxConfig).mockResolvedValue([
      {
        name: "spec.yaml",
        path: "/home/user/.config/sandwarden/sandboxes/alpha/spec.yaml",
        content: "name: alpha\n",
        error: "",
      },
      {
        name: "sandwarden.yaml",
        path: "/home/user/.config/sandwarden/sandboxes/alpha/sandwarden.yaml",
        content: "sandbox: alpha\nrunArgs: [unterminated\n",
        error: "yaml: line 2: unterminated flow sequence",
      },
    ]);
    renderPage();

    fireEvent.click(await screen.findByRole("button", { name: "View files" }));
    const dialog = await screen.findByRole("dialog");
    fireEvent.click(await within(dialog).findByRole("tab", { name: /sandwarden.yaml/ }));

    expect(await within(dialog).findByText("yaml: line 2: unterminated flow sequence")).toBeDefined();
    // The detail page still renders around the viewer.
    expect(within(dialog).getByText("alpha configuration")).toBeDefined();
    expect(screen.getByRole("button", { name: /Apply/ })).toBeDefined();
  });

  it("starts an apply job and reports its completion", async () => {
    renderPage();

    fireEvent.click(await screen.findByRole("button", { name: /Apply/ }));
    await waitFor(() => {
      expect(api.applySandbox).toHaveBeenCalledWith("alpha", expect.any(String));
    });

    const jobId = vi.mocked(api.applySandbox).mock.calls[0][1];
    act(() => jobHub.dispatch(jobId, { kind: "done" }));

    expect(await screen.findByText("Sandbox alpha applied")).toBeDefined();
  });

  it("reports the validation verdict", async () => {
    renderPage();

    fireEvent.click(await screen.findByRole("button", { name: "Validate" }));

    expect(await screen.findByText("Sandbox is valid")).toBeDefined();
    expect(vi.mocked(api.validateSandbox)).toHaveBeenCalledWith("alpha");
  });

  it("warns before recreating the sandbox from its files", async () => {
    renderPage();

    fireEvent.click(await screen.findByRole("button", { name: /Recreate/ }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText(/everything inside the sandbox/i)).toBeDefined();
    expect(within(dialog).getByText(/from scratch/i)).toBeDefined();
    expect(within(dialog).getByText(/container state/i)).toBeDefined();

    fireEvent.click(within(dialog).getByRole("button", { name: "Recreate" }));

    await waitFor(() => {
      expect(api.recreateSandbox).toHaveBeenCalledWith("alpha", expect.any(String));
    });
  });

  it("attaches a kit without recreating the sandbox", async () => {
    renderPage();

    fireEvent.click(await screen.findByRole("button", { name: /Attach kit/ }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText(/kit-owned volumes/i)).toBeDefined();
    expect(within(dialog).getByText(/create\.kits/)).toBeDefined();
    expect(within(dialog).getByText(/Unlike Recreate/i)).toBeDefined();

    const confirm = within(dialog).getByRole("button", { name: "Attach kit" });
    expect(confirm.hasAttribute("disabled")).toBe(true);
    fireEvent.change(within(dialog).getByLabelText(/Kit reference/), {
      target: { value: "ghcr.io/acme/mcp-postgres:1.0" },
    });
    fireEvent.click(confirm);

    await waitFor(() => {
      expect(api.attachKit).toHaveBeenCalledWith("alpha", "ghcr.io/acme/mcp-postgres:1.0");
    });
  });
});

function incompleteDetail(): SandboxDetail {
  return {
    ...detail,
    incomplete: true,
    sandbox: { ...detail.sandbox, incomplete: true },
  };
}

describe("incomplete sandbox config", () => {
  it("surfaces the missing create parameters and warns before recreating", async () => {
    vi.mocked(api.sandbox).mockResolvedValueOnce(incompleteDetail());
    renderPage();

    expect(await screen.findByText("Incomplete configuration")).toBeDefined();
    expect(screen.getByText("incomplete config")).toBeDefined();
    expect(screen.getByText(/CPU, memory and environment settings were not recoverable/)).toBeDefined();

    fireEvent.click(screen.getByRole("button", { name: /Recreate/ }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText(/will not restore them/)).toBeDefined();
  });

  it("records create parameters to complete the config", async () => {
    vi.mocked(api.sandbox).mockResolvedValueOnce(incompleteDetail());
    renderPage();

    fireEvent.click(await screen.findByRole("button", { name: "Complete this config" }));
    const dialog = await screen.findByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText(/^CPUs/), { target: { value: "4" } });
    fireEvent.change(within(dialog).getByLabelText(/^Memory/), { target: { value: "8g" } });
    fireEvent.change(within(dialog).getByLabelText(/^Environment/), { target: { value: "A=B\nC=D" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(api.completeSandboxConfig).toHaveBeenCalledWith("alpha", {
        cpus: 4,
        memory: "8g",
        env: ["A=B", "C=D"],
      });
    });
  });
});

describe("sandbox deletion", () => {
  it("forces the deletion after the user confirms it", async () => {
    renderPage();

    fireEvent.click(await screen.findByRole("button", { name: "Delete" }));
    const dialog = await screen.findByRole("dialog");
    fireEvent.click(within(dialog).getByRole("button", { name: "Delete" }));

    await waitFor(() => {
      expect(api.deleteSandbox).toHaveBeenCalledWith("alpha", true, false);
    });
  });
});
