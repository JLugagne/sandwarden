// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { api } from "@/api/client";
import { ToastProvider } from "@/components/Toaster";
import type { CacheView, ProfileView } from "@/types";
import { ProfilesPage } from "./ProfilesPage";

beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn();
});

afterEach(cleanup);

const npmCache: CacheView = {
  slug: "npm",
  dir: "npm",
  name: "npm",
  description: "",
  host_path: "/host/npm",
  target_path: "/npm",
  read_only: false,
  auto_attach: false,
  enabled: true,
};

const profile: ProfileView = {
  slug: "golang",
  name: "golang",
  description: "Go defaults",
  default: true,
  global: false,
  allow: ["*.example.com"],
  deny: [],
  mounts: [{ host_path: "/host/go", target_path: "/go", read_only: true }],
  caches: ["npm"],
  skills: [],
  sandboxes: ["box"],
};

const extraCache: CacheView = {
  slug: "pip",
  dir: "pip",
  name: "pip",
  description: "",
  host_path: "/host/pip",
  target_path: "/pip",
  read_only: false,
  auto_attach: false,
  enabled: true,
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
    validateProfile: vi.fn(async () => ({ ok: true, output: "VALID: profiles/golang" })),
    readProfileConfig: vi.fn(async () => []),
    fsPick: vi.fn(),
    configStaleness: vi.fn(async () => ({ stale: false, changed: [] })),
  },
}));

function LocationProbe() {
  const location = useLocation();
  return <output data-testid="location-search">{location.search}</output>;
}

function renderPage(entry = "/profiles") {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[entry]}>
        <LocationProbe />
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

  it("reports the validation verdict of a profile", async () => {
    vi.mocked(api.profiles).mockResolvedValue([profile]);
    renderPage();

    fireEvent.click(await screen.findByRole("button", { name: "Validate" }));

    expect(await screen.findByText("Profile is valid")).toBeDefined();
    expect(vi.mocked(api.validateProfile)).toHaveBeenCalledWith("golang");
  });

  it("opens the raw profile files from the card", async () => {
    vi.mocked(api.profiles).mockResolvedValue([profile]);
    vi.mocked(api.readProfileConfig).mockResolvedValue([
      {
        name: "spec.yaml",
        path: "/home/user/.config/sandwarden/profiles/golang/spec.yaml",
        content: "name: golang\n",
        error: "",
      },
      {
        name: "sandwarden.yaml",
        path: "/home/user/.config/sandwarden/profiles/golang/sandwarden.yaml",
        content: "default: true\n",
        error: "",
      },
    ]);
    renderPage();

    fireEvent.click(await screen.findByRole("button", { name: "View files" }));
    const dialog = await screen.findByRole("dialog");

    expect(await within(dialog).findByRole("tab", { name: /spec.yaml/ })).toBeDefined();
    expect(within(dialog).getByRole("tab", { name: /sandwarden.yaml/ })).toBeDefined();
    expect(vi.mocked(api.readProfileConfig)).toHaveBeenCalledWith("golang");
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
      expect(api.addProfileMount).toHaveBeenCalledWith("golang", {
        host_path: "/host/src",
        target_path: "",
        read_only: false,
      });
    });
  });

  it("adds a cache from Settings to the profile defaults", async () => {
    vi.mocked(api.profiles).mockResolvedValue([profile]);
    vi.mocked(api.caches).mockResolvedValue([npmCache, extraCache]);
    renderPage();

    fireEvent.click(await screen.findByRole("button", { name: "Edit" }));
    fireEvent.click(await screen.findByRole("tab", { name: /Caches/ }));

    const picker = await screen.findByRole("combobox");
    fireEvent.change(picker, { target: { value: "pip" } });
    fireEvent.click(screen.getByRole("button", { name: "Add cache" }));

    await waitFor(() => {
      expect(api.addProfileCache).toHaveBeenCalledWith("golang", "pip");
    });
  });
});

describe("profile deep link", () => {
  it("highlights the profile card linked from the search overlay and strips the parameter", async () => {
    vi.mocked(api.profiles).mockResolvedValue([profile]);
    renderPage("/profiles?profile=golang");

    const card = (await screen.findByRole("heading", { name: /golang/ })).closest("section");
    await waitFor(() => {
      expect(card?.className).toContain("ring-accent");
    });
    await waitFor(() => {
      expect(screen.getByTestId("location-search").textContent).toBe("");
    });
    expect(Element.prototype.scrollIntoView).toHaveBeenCalled();
  });

  it("leaves every profile card unhighlighted without a deep link", async () => {
    vi.mocked(api.profiles).mockResolvedValue([profile]);
    renderPage();

    const card = (await screen.findByRole("heading", { name: /golang/ })).closest("section");
    expect(card?.className).not.toContain("ring-accent");
    expect(screen.getByTestId("location-search").textContent).toBe("");
  });
});
