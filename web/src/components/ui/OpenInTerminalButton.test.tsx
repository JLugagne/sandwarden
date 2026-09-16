// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider } from "@/components/Toaster";
import { api } from "@/api/client";
import { CommandLine } from "./CopyButton";

vi.mock("@/api/client", () => ({
  api: {
    terminals: vi.fn(async () => [
      { id: "terminal", name: "Terminal", binary: "/usr/bin/osascript" },
      { id: "kitty", name: "kitty", binary: "/usr/bin/kitty" },
    ]),
    openInTerminal: vi.fn(async () => undefined),
  },
}));

afterEach(() => {
  cleanup();
  window.localStorage.clear();
  vi.clearAllMocks();
});

function renderCommandLine(openDir?: string) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <ToastProvider>
        <CommandLine command="sbx run --name box" openDir={openDir} />
      </ToastProvider>
    </QueryClientProvider>,
  );
}

describe("open in terminal", () => {
  it("launches the default terminal in the workspace", async () => {
    renderCommandLine("/srv/work");

    const open = (await screen.findByRole("button", { name: "Open" })) as HTMLButtonElement;
    await waitFor(() => expect(open.disabled).toBe(false));
    fireEvent.click(open);

    await waitFor(() =>
      expect(vi.mocked(api.openInTerminal)).toHaveBeenCalledWith("terminal", "/srv/work", "sbx run --name box"),
    );
  });

  it("offers the other enabled terminals in a dropdown", async () => {
    renderCommandLine("/srv/work");

    fireEvent.click(await screen.findByRole("button", { name: "Choose a terminal" }));
    fireEvent.click(await screen.findByRole("menuitem", { name: /kitty/ }));

    await waitFor(() =>
      expect(vi.mocked(api.openInTerminal)).toHaveBeenCalledWith("kitty", "/srv/work", "sbx run --name box"),
    );
  });

  it("still opens a terminal for sandboxes without a workspace", async () => {
    renderCommandLine("");

    const open = (await screen.findByRole("button", { name: "Open" })) as HTMLButtonElement;
    await waitFor(() => expect(open.disabled).toBe(false));
    fireEvent.click(open);

    await waitFor(() =>
      expect(vi.mocked(api.openInTerminal)).toHaveBeenCalledWith("terminal", "", "sbx run --name box"),
    );
  });

  it("renders no open button when the directory prop is omitted", () => {
    renderCommandLine();

    expect(screen.queryByRole("button", { name: "Open" })).toBeNull();
    expect(screen.getByRole("button", { name: "Copy" })).toBeTruthy();
  });
});
