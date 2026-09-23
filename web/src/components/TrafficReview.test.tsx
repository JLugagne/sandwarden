// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "@/api/client";
import { ToastProvider } from "@/components/Toaster";
import { TrafficReview } from "./TrafficReview";
import type { LogEntry, PolicyLog, PolicyRule } from "@/types";

vi.mock("@/api/client", () => ({
  api: {
    profiles: vi.fn(async () => [
      { slug: "web-dev", name: "Web dev", description: "", default: false, global: false, allow: [], deny: [], mounts: [], caches: [], skills: [], sandboxes: ["box"] },
    ]),
    policyAction: vi.fn(async () => ({ results: [{ action: "allow" }] })),
    addRules: vi.fn(async () => undefined),
  },
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function entry(host: string, vm = "box"): LogEntry {
  return { host, vm_name: vm, proxy_type: "https", rule: "", last_seen: "", since: "", count_since: 1 };
}

const log: PolicyLog = {
  blocked_hosts: [entry("a.test"), entry("b.test"), entry("c.test", "box-2"), entry("known.test")],
  allowed_hosts: [entry("seen.test")],
};

const rules: PolicyRule[] = [
  {
    id: "r1",
    name: "",
    policy_id: "",
    scope: "local",
    applies_to: "",
    sandbox_id: "box",
    resource_type: "domain",
    decision: "allow",
    resources: ["known.test"],
    origin: "",
    status: "",
    editable: true,
  },
];

function renderReview() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <ToastProvider>
          <TrafficReview log={log} rules={rules} />
        </ToastProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("TrafficReview", () => {
  it("lists only undecided hosts as pending and covered ones as allowed", () => {
    renderReview();

    expect(screen.getByText("Pending (3)")).toBeDefined();
    expect(screen.getByText("Allowed (2)")).toBeDefined();
    expect(screen.queryByRole("checkbox", { name: "Select known.test" })).toBeNull();
  });

  it("allows a selection with one policy action per sandbox", async () => {
    renderReview();

    fireEvent.click(screen.getByRole("checkbox", { name: "Select all pending hosts" }));
    fireEvent.click(within(screen.getByRole("toolbar")).getByRole("button", { name: "Allow selected" }));

    await waitFor(() => expect(api.policyAction).toHaveBeenCalledTimes(2));
    expect(api.policyAction).toHaveBeenCalledWith({ action: "allow", resources: ["a.test", "b.test"], sandbox_id: "box" });
    expect(api.policyAction).toHaveBeenCalledWith({ action: "allow", resources: ["c.test"], sandbox_id: "box-2" });
  });

  it("adds the selected hosts to a profile in one call", async () => {
    renderReview();

    fireEvent.click(screen.getByRole("checkbox", { name: "Select a.test" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Select b.test" }));
    const toolbar = screen.getByRole("toolbar");
    await within(toolbar).findByRole("option", { name: /Web dev \(applies here\)/ });
    fireEvent.change(within(toolbar).getByRole("combobox", { name: "Profile" }), { target: { value: "web-dev" } });
    fireEvent.click(within(toolbar).getByRole("button", { name: "Deny in profile" }));

    await waitFor(() => expect(api.addRules).toHaveBeenCalledWith("web-dev", { decision: "deny", patterns: ["a.test", "b.test"] }));
  });

  it("denies a single host from its row", async () => {
    renderReview();

    const row = screen.getByText("c.test").closest("tr") as HTMLTableRowElement;
    fireEvent.click(within(row).getByRole("button", { name: "Deny" }));

    await waitFor(() => expect(api.policyAction).toHaveBeenCalledWith({ action: "deny", resources: ["c.test"], sandbox_id: "box-2" }));
  });
});
