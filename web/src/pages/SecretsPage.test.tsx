// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "@/api/client";
import { ToastProvider } from "@/components/Toaster";
import { SecretsPage } from "./SecretsPage";

afterEach(cleanup);

vi.mock("@/api/client", () => ({
  api: {
    secrets: vi.fn(async () => ({ stored: [], custom: [] })),
    listSandboxes: vi.fn(async () => []),
    importSecrets: vi.fn(async () => ({ job_id: "job-1" })),
    removeSecret: vi.fn(),
    removeRegistrySecret: vi.fn(),
    removeCustomSecret: vi.fn(),
  },
}));

function LocationProbe() {
  const location = useLocation();
  return <span data-testid="location">{location.search}</span>;
}

function renderPage(initialEntry: string) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialEntry]}>
        <ToastProvider>
          <SecretsPage />
          <LocationProbe />
        </ToastProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("secrets import scope", () => {
  it("reveals the global scope after a real import so the imported rows are visible", async () => {
    renderPage("/secrets?sandbox=box&add=import");

    fireEvent.click(screen.getByRole("button", { name: "Import + overwrite" }));

    await waitFor(() => {
      expect(screen.getByTestId("location").textContent).toContain("sandbox=global");
    });
  });

  it("leaves the scope untouched for a dry-run scan", () => {
    renderPage("/secrets?sandbox=box&add=import");

    fireEvent.click(screen.getByRole("button", { name: "Scan (dry-run)" }));

    expect(screen.getByTestId("location").textContent).toContain("sandbox=box");
  });

  it("keeps the all-scopes view unchanged after a real import", () => {
    renderPage("/secrets?add=import");

    fireEvent.click(screen.getByRole("button", { name: "Import new" }));

    expect(screen.getByTestId("location").textContent).not.toContain("sandbox=");
  });

  it("surfaces a failed import start and keeps the scope", async () => {
    vi.mocked(api.importSecrets).mockRejectedValueOnce(new Error("daemon refused"));
    renderPage("/secrets?sandbox=box&add=import");

    fireEvent.click(screen.getByRole("button", { name: "Import + overwrite" }));

    expect(await screen.findByText(/Import failed to start/)).toBeDefined();
    expect(screen.getByTestId("location").textContent).toContain("sandbox=box");
  });

  it("warns that removing a host-only registry credential also removes the all-sandboxes one", async () => {
    vi.mocked(api.secrets).mockResolvedValueOnce({
      stored: [{ scope: "(host only)", type: "registry", name: "ghcr.io", masked: "user/*****", username: "user" }],
      custom: [],
    });
    renderPage("/secrets");

    fireEvent.click(await screen.findByRole("button", { name: "Remove" }));

    expect(await screen.findByText(/removes the all-sandboxes credential/)).toBeDefined();
  });
});
