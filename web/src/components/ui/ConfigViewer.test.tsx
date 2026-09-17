// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "@/api/client";
import { ConfigViewer } from "./ConfigViewer";

vi.mock("@/api/client", () => ({
  api: {
    readSandboxConfig: vi.fn(async () => []),
    readProfileConfig: vi.fn(async () => []),
  },
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

/** Matches a rendered YAML line, which is split across highlight spans. */
const line = (text: string) => (_content: string, node: Element | null) => node?.textContent === text;

function renderViewer(target = { kind: "sandbox" as const, slug: "alpha", title: "alpha" }) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <ConfigViewer open target={target} onClose={() => undefined} />
    </QueryClientProvider>,
  );
}

describe("config viewer", () => {
  it("shows both files and highlights the loaded one", async () => {
    vi.mocked(api.readSandboxConfig).mockResolvedValue([
      { name: "spec.yaml", path: "/config/sandboxes/alpha/spec.yaml", content: "name: alpha\n", error: "" },
      {
        name: "sandwarden.yaml",
        path: "/config/sandboxes/alpha/sandwarden.yaml",
        content: "sandbox: alpha\nrunArgs: --model sonnet\n",
        error: "",
      },
    ]);
    renderViewer();

    expect(await screen.findByText(line("name: alpha"))).toBeDefined();
    expect(screen.getAllByRole("tab")).toHaveLength(2);
    expect(screen.getByRole("button", { name: "Copy" })).toBeDefined();
    expect(vi.mocked(api.readSandboxConfig)).toHaveBeenCalledWith("alpha");
  });

  it("keeps a broken sidecar readable and shows its parse error", async () => {
    vi.mocked(api.readSandboxConfig).mockResolvedValue([
      { name: "spec.yaml", path: "/config/sandboxes/alpha/spec.yaml", content: "name: alpha\n", error: "" },
      {
        name: "sandwarden.yaml",
        path: "/config/sandboxes/alpha/sandwarden.yaml",
        content: "sandbox: alpha\nrunArgs: [unterminated\n",
        error: "yaml: line 2: unterminated flow sequence",
      },
    ]);
    renderViewer();

    fireEvent.click(await screen.findByRole("tab", { name: /sandwarden.yaml/ }));

    expect(await screen.findByText("yaml: line 2: unterminated flow sequence")).toBeDefined();
    expect(screen.getByText(line("runArgs: [unterminated"))).toBeDefined();
  });

  it("reports a missing file without failing the viewer", async () => {
    vi.mocked(api.readSandboxConfig).mockResolvedValue([
      { name: "spec.yaml", path: "/config/sandboxes/alpha/spec.yaml", content: "name: alpha\n", error: "" },
      {
        name: "sandwarden.yaml",
        path: "/config/sandboxes/alpha/sandwarden.yaml",
        content: "",
        error: "file not found",
      },
    ]);
    renderViewer();

    fireEvent.click(await screen.findByRole("tab", { name: /sandwarden.yaml/ }));

    expect(await screen.findByText("file not found")).toBeDefined();
    expect(screen.getByText("No content to show.")).toBeDefined();
  });

  it("surfaces a lookup failure and does not crash", async () => {
    vi.mocked(api.readSandboxConfig).mockRejectedValue(new Error("sandbox not found"));
    renderViewer();

    await waitFor(() => expect(screen.getByText("sandbox not found")).toBeDefined());
    expect(screen.queryByRole("tab")).toBeNull();
  });
});
