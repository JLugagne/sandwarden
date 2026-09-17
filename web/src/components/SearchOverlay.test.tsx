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
            kind: "sandbox",
            store: "",
            store_name: "",
            name: "frontend-box",
            display_name: "Frontend Box",
            slug: "frontend-box",
            description: "UI work sandbox",
            agent: "claude",
            score: 4.4,
          },
          {
            kind: "profile",
            store: "",
            store_name: "",
            name: "Net Allow",
            slug: "net-allow",
            description: "corporate egress allowlist",
            score: 4.1,
          },
          {
            kind: "cache",
            store: "",
            store_name: "",
            name: "Go module cache",
            slug: "go-mod",
            description: "shared downloads",
            score: 3.7,
          },
          {
            kind: "skill",
            store: "anthropics",
            store_name: "anthropics",
            name: "kubernetes-deploy",
            description: "Deploy workloads",
            score: 3.2,
          },
          {
            kind: "kit",
            store: "sbx-kits-contrib",
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

async function openWithQuery(query: string) {
  fireEvent.keyDown(window, { key: "k", ctrlKey: true });
  const input = await screen.findByLabelText("Search query");
  fireEvent.change(input, { target: { value: query } });
  return input;
}

describe("search overlay", () => {
  it("opens on Ctrl+K and lists grouped results", async () => {
    renderOverlay();
    expect(screen.queryByLabelText("Search query")).toBeNull();

    fireEvent.keyDown(window, { key: "k", ctrlKey: true });
    const input = await screen.findByLabelText("Search query");
    expect(screen.getByText(/Type at least 2 characters/)).toBeDefined();

    fireEvent.change(input, { target: { value: "front" } });

    expect(await screen.findByText("Frontend Box")).toBeDefined();
    expect(screen.getByText("Net Allow")).toBeDefined();
    expect(screen.getByText("Go module cache")).toBeDefined();
    expect(screen.getByText("kubernetes-deploy")).toBeDefined();
    expect(screen.getByText("code-server (web VS Code)")).toBeDefined();
    expect(screen.getByText("Sandboxes")).toBeDefined();
    expect(screen.getByText("Profiles")).toBeDefined();
    expect(screen.getByText("Caches")).toBeDefined();
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
    const input = await openWithQuery("code");
    await screen.findByText("code-server (web VS Code)");

    for (let step = 0; step < 4; step += 1) {
      fireEvent.keyDown(input, { key: "ArrowDown" });
    }
    fireEvent.keyDown(input, { key: "Enter" });

    await waitFor(() =>
      expect(screen.getByTestId("location").textContent).toBe("/kits?store=sbx-kits-contrib&item=code-server"),
    );
    expect(screen.queryByLabelText("Search query")).toBeNull();
  });

  it("opens the sandbox detail page for a sandbox hit", async () => {
    renderOverlay();
    const input = await openWithQuery("front");
    await screen.findByText("Frontend Box");

    fireEvent.keyDown(input, { key: "Enter" });

    await waitFor(() => expect(screen.getByTestId("location").textContent).toBe("/sandboxes/frontend-box"));
  });

  it("opens the profiles page for a profile hit", async () => {
    renderOverlay();
    const input = await openWithQuery("net");
    await screen.findByText("Net Allow");

    fireEvent.keyDown(input, { key: "ArrowDown" });
    fireEvent.keyDown(input, { key: "Enter" });

    await waitFor(() => expect(screen.getByTestId("location").textContent).toBe("/profiles?profile=net-allow"));
  });

  it("opens the settings caches section for a cache hit", async () => {
    renderOverlay();
    const input = await openWithQuery("go");
    await screen.findByText("Go module cache");

    fireEvent.keyDown(input, { key: "ArrowDown" });
    fireEvent.keyDown(input, { key: "ArrowDown" });
    fireEvent.keyDown(input, { key: "Enter" });

    await waitFor(() => expect(screen.getByTestId("location").textContent).toBe("/settings?cache=go-mod"));
  });
});
