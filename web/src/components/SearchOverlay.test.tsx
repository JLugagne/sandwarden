// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import type { SearchResult } from "@/types";
import { OPEN_SEARCH_EVENT, SearchOverlay } from "./SearchOverlay";

beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn();
});

afterEach(cleanup);

vi.mock("@/api/client", () => ({
  api: {
    search: vi.fn(
      async () =>
        [
          {
            kind: "skill",
            id: 11,
            store_id: 1,
            store_name: "anthropics",
            name: "kubernetes-deploy",
            description: "Deploy workloads",
            score: 3.2,
          },
          {
            kind: "kit",
            id: 7,
            store_id: 2,
            store_name: "sbx-kits-contrib",
            name: "code-server",
            display_name: "code-server (web VS Code)",
            description: "Runs code-server",
            kit_kind: "mixin",
            score: 2.1,
          },
        ] satisfies SearchResult[],
    ),
  },
}));

function Location() {
  const location = useLocation();
  return <span data-testid="location">{`${location.pathname}${location.search}`}</span>;
}

function renderOverlay() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={["/"]}>
        <SearchOverlay />
        <Routes>
          <Route path="*" element={<Location />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("search overlay", () => {
  it("opens on Ctrl+K and lists grouped results", async () => {
    renderOverlay();
    expect(screen.queryByLabelText("Search query")).toBeNull();

    fireEvent.keyDown(window, { key: "k", ctrlKey: true });
    const input = await screen.findByLabelText("Search query");
    expect(screen.getByText(/Type at least 2 characters/)).toBeDefined();

    fireEvent.change(input, { target: { value: "kube" } });

    expect(await screen.findByText("kubernetes-deploy")).toBeDefined();
    expect(screen.getByText("code-server (web VS Code)")).toBeDefined();
    expect(screen.getByText("Skills")).toBeDefined();
    expect(screen.getByText("Kits")).toBeDefined();
  });

  it("opens from the sidebar event", async () => {
    renderOverlay();
    window.dispatchEvent(new Event(OPEN_SEARCH_EVENT));
    expect(await screen.findByLabelText("Search query")).toBeDefined();
  });

  it("navigates to the kits page when a kit result is opened", async () => {
    renderOverlay();
    fireEvent.keyDown(window, { key: "k", ctrlKey: true });
    const input = await screen.findByLabelText("Search query");
    fireEvent.change(input, { target: { value: "code" } });
    await screen.findByText("code-server (web VS Code)");

    fireEvent.keyDown(input, { key: "ArrowDown" });
    fireEvent.keyDown(input, { key: "Enter" });

    await waitFor(() => expect(screen.getByTestId("location").textContent).toBe("/kits?item=7"));
    expect(screen.queryByLabelText("Search query")).toBeNull();
  });
});
