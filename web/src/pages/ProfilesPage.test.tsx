// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "@/api/client";
import { ToastProvider } from "@/components/Toaster";
import { ProfilesPage } from "./ProfilesPage";

afterEach(cleanup);

const profile = {
  id: 1,
  name: "golang",
  description: "Go defaults",
  is_default: true,
  is_global: false,
  created_at: "",
  updated_at: "",
  rules: [{ id: 11, profile_id: 1, decision: "allow", pattern: "*.example.com", created_at: "" }],
  items: [],
  mounts: [
    { id: 21, profile_id: 1, host_path: "/host/go", target_path: "/go", read_only: true, created_at: "" },
  ],
  caches: [
    {
      id: 31,
      name: "npm",
      description: "",
      host_path: "/host/npm",
      target_path: "/npm",
      read_only: false,
      auto_attach: false,
      enabled: true,
      created_at: "",
      updated_at: "",
    },
  ],
  sandboxes: ["box"],
};

const extraCache = {
  id: 32,
  name: "pip",
  description: "",
  host_path: "/host/pip",
  target_path: "/pip",
  read_only: false,
  auto_attach: false,
  enabled: true,
  created_at: "",
  updated_at: "",
};

vi.mock("@/api/client", () => ({
  api: {
    profiles: vi.fn(async () => []),
    skillItems: vi.fn(async () => []),
    caches: vi.fn(async () => []),
    createProfile: vi.fn(),
    updateProfile: vi.fn(),
    deleteProfile: vi.fn(),
    addRule: vi.fn(),
    removeRule: vi.fn(),
    addProfileSkillItem: vi.fn(),
    removeProfileSkillItem: vi.fn(),
    addProfileMount: vi.fn(async () => ({})),
    removeProfileMount: vi.fn(),
    addProfileCache: vi.fn(),
    removeProfileCache: vi.fn(),
    fsPick: vi.fn(),
  },
}));

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/profiles"]}>
        <ToastProvider>
          <ProfilesPage />
        </ToastProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("profiles editor", () => {
  it("summarises the profile defaults and opens the tabbed editor", async () => {
    vi.mocked(api.profiles).mockResolvedValue([profile]);
    renderPage();

    expect(await screen.findByText("golang")).toBeDefined();
    expect(screen.getByText("mounts")).toBeDefined();
    expect(screen.getByText("caches")).toBeDefined();

    fireEvent.click(screen.getByRole("button", { name: "Edit" }));

    expect(await screen.findByRole("tab", { name: /Rules/ })).toBeDefined();
    expect(screen.getByRole("tab", { name: /Mounts/ })).toBeDefined();
    expect(screen.getByRole("tab", { name: /Caches/ })).toBeDefined();
  });

  it("shows the rules of the profile on the rules tab", async () => {
    vi.mocked(api.profiles).mockResolvedValue([profile]);
    renderPage();

    fireEvent.click(await screen.findByRole("button", { name: "Edit" }));
    fireEvent.click(await screen.findByRole("tab", { name: /Rules/ }));

    expect(screen.getByText("*.example.com")).toBeDefined();
  });

  it("adds a default mount from the mounts tab", async () => {
    vi.mocked(api.profiles).mockResolvedValue([profile]);
    renderPage();

    fireEvent.click(await screen.findByRole("button", { name: "Edit" }));
    fireEvent.click(await screen.findByRole("tab", { name: /Mounts/ }));

    expect(screen.getByText("/go")).toBeDefined();
    fireEvent.change(screen.getByLabelText("Host path"), { target: { value: "/host/src" } });
    fireEvent.click(screen.getByRole("button", { name: /Add mount/ }));

    await waitFor(() => {
      expect(api.addProfileMount).toHaveBeenCalledWith(1, {
        host_path: "/host/src",
        target_path: "",
        read_only: false,
      });
    });
  });

  it("adds a cache from Settings to the profile defaults", async () => {
    vi.mocked(api.profiles).mockResolvedValue([profile]);
    vi.mocked(api.caches).mockResolvedValue([profile.caches[0], extraCache]);
    renderPage();

    fireEvent.click(await screen.findByRole("button", { name: "Edit" }));
    fireEvent.click(await screen.findByRole("tab", { name: /Caches/ }));

    const picker = await screen.findByRole("combobox");
    fireEvent.change(picker, { target: { value: "32" } });
    fireEvent.click(screen.getByRole("button", { name: "Add cache" }));

    await waitFor(() => {
      expect(api.addProfileCache).toHaveBeenCalledWith(1, 32);
    });
  });
});
