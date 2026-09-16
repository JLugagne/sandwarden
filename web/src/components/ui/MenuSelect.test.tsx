// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MenuSelect, type MenuSelectGroup } from "./MenuSelect";

afterEach(cleanup);

const groups: MenuSelectGroup[] = [
  {
    value: "sbx-kits-contrib",
    label: "sbx-kits-contrib",
    options: [
      { value: "ref:aider", label: "Aider", tag: { label: "sandbox" } },
      { value: "ref:amp", label: "Amp", tag: { label: "mixin" }, keywords: "amp agent" },
    ],
  },
  {
    value: "other-repo",
    label: "other-repo",
    options: [
      { value: "ref:tools", label: "Toolbox", tag: { label: "mixin" }, keywords: "toolbox helpers" },
    ],
  },
];

function openMenu(props: Partial<React.ComponentProps<typeof MenuSelect>> = {}) {
  const onChange = vi.fn();
  render(
    <MenuSelect
      groups={groups}
      onChange={onChange}
      placeholder="Add a kit from your repositories…"
      searchPlaceholder="Search kits…"
      {...props}
    />,
  );
  fireEvent.click(screen.getByRole("combobox"));
  return { onChange, search: screen.getByLabelText("Search kits…") };
}

describe("MenuSelect", () => {
  it("drills down from repositories to their items and reports the choice", () => {
    const { onChange } = openMenu();

    expect(screen.getByText("sbx-kits-contrib")).toBeTruthy();
    expect(screen.getByText("other-repo")).toBeTruthy();
    expect(screen.queryByText("Aider")).toBeNull();

    fireEvent.click(screen.getByText("sbx-kits-contrib"));
    expect(screen.getByText("Aider")).toBeTruthy();
    expect(screen.queryByText("Toolbox")).toBeNull();

    fireEvent.click(screen.getByText("Aider"));
    expect(onChange).toHaveBeenCalledWith("ref:aider");
    expect(screen.queryByText("Aider")).toBeNull();
  });

  it("searches across every repository", () => {
    const { search } = openMenu();

    fireEvent.change(search, { target: { value: "toolbox" } });

    expect(screen.getByText("Toolbox")).toBeTruthy();
    expect(screen.getByText("other-repo")).toBeTruthy();
    expect(screen.queryByText("Aider")).toBeNull();
  });

  it("reports no match for an unknown query", () => {
    const { search } = openMenu();

    fireEvent.change(search, { target: { value: "zzz" } });

    expect(screen.getByText(/No match for/)).toBeTruthy();
  });

  it("selects with the keyboard and goes back to the repository list", () => {
    const { onChange, search } = openMenu();

    fireEvent.keyDown(search, { key: "ArrowDown" });
    fireEvent.keyDown(search, { key: "Enter" });
    expect(screen.getByText("Aider")).toBeTruthy();

    fireEvent.keyDown(search, { key: "Backspace" });
    expect(screen.getByText("other-repo")).toBeTruthy();

    fireEvent.keyDown(search, { key: "ArrowDown" });
    fireEvent.keyDown(search, { key: "Enter" });
    fireEvent.keyDown(search, { key: "ArrowDown" });
    fireEvent.keyDown(search, { key: "Enter" });
    expect(onChange).toHaveBeenCalledWith("ref:aider");
  });

  it("closes on Escape and on an outside click", () => {
    const { search } = openMenu();

    fireEvent.keyDown(search, { key: "Escape" });
    expect(screen.queryByLabelText("Search kits…")).toBeNull();

    fireEvent.click(screen.getByRole("combobox"));
    expect(screen.getByLabelText("Search kits…")).toBeTruthy();
    fireEvent.pointerDown(document.body);
    expect(screen.queryByLabelText("Search kits…")).toBeNull();
  });

  it("shows the current selection on the trigger", () => {
    render(
      <MenuSelect
        groups={groups}
        value="ref:amp"
        onChange={() => {}}
        placeholder="Add a kit from your repositories…"
      />,
    );
    expect(screen.getByRole("combobox").textContent).toContain("Amp");
  });

  it("opens directly on the items when there is a single repository", () => {
    render(
      <MenuSelect
        groups={[groups[1]]}
        onChange={() => {}}
        placeholder="Add a kit from your repositories…"
        searchPlaceholder="Search kits…"
      />,
    );

    fireEvent.click(screen.getByRole("combobox"));

    expect(screen.getByText("Toolbox")).toBeTruthy();
    expect(screen.queryByLabelText("Back to repositories")).toBeNull();
  });

  it("anchors each popover to its own trigger with CSS anchor positioning", () => {
    render(
      <div>
        <MenuSelect groups={groups} onChange={() => {}} placeholder="First menu" searchPlaceholder="Search kits…" />
        <MenuSelect groups={groups} onChange={() => {}} placeholder="Second menu" searchPlaceholder="Search skills…" />
      </div>,
    );

    const triggers = screen.getAllByRole("combobox");
    const firstAnchor = triggers[0].style.getPropertyValue("--anchor");
    const secondAnchor = triggers[1].style.getPropertyValue("--anchor");
    expect(firstAnchor).toMatch(/^--anchor-/);
    expect(secondAnchor).not.toBe(firstAnchor);
    expect(triggers[0].classList.contains("anchor-trigger")).toBe(true);

    fireEvent.click(triggers[0]);

    const popover = screen.getByRole("listbox").parentElement;
    expect(popover?.classList.contains("anchor-popover")).toBe(true);
    expect(popover?.style.getPropertyValue("--anchor")).toBe(firstAnchor);
  });
});
