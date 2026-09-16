import { Fragment, useEffect, useId, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { cn } from "@/lib/cn";
import { Badge, type BadgeTone } from "./Badge";
import { CONTROL_CLASS } from "./Field";
import { IconCheck, IconChevronDown, IconChevronRight, IconSearch } from "./Icons";

export interface MenuSelectTag {
  label: string;
  tone?: BadgeTone;
}

export interface MenuSelectOption {
  value: string;
  label: string;
  description?: string;
  tag?: MenuSelectTag;
  keywords?: string;
}

export interface MenuSelectGroup {
  value: string;
  label: string;
  description?: string;
  keywords?: string;
  options: MenuSelectOption[];
}

interface Row {
  group: MenuSelectGroup;
  option: MenuSelectOption;
}

export function MenuSelect({
  groups,
  value = "",
  onChange,
  placeholder = "Choose…",
  searchPlaceholder = "Search…",
  emptyLabel = "Nothing to choose from.",
  disabled = false,
  className,
  id,
}: {
  groups: MenuSelectGroup[];
  value?: string;
  onChange: (value: string) => void;
  placeholder?: string;
  searchPlaceholder?: string;
  emptyLabel?: string;
  disabled?: boolean;
  className?: string;
  id?: string;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [activeGroup, setActiveGroup] = useState<string | null>(null);
  const [activeIndex, setActiveIndex] = useState(-1);

  const anchorRef = useRef<HTMLButtonElement>(null);
  const popoverRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const anchorName = `--anchor-${useId().replace(/[^a-zA-Z0-9_-]/g, "")}`;

  const selected = useMemo(
    () => groups.flatMap((group) => group.options).find((option) => option.value === value) ?? null,
    [groups, value],
  );

  const searching = query.trim() !== "";
  const activeGroupData = useMemo(
    () => groups.find((group) => group.value === activeGroup) ?? null,
    [groups, activeGroup],
  );

  const searchRows = useMemo<Row[]>(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return [];
    const rows: Row[] = [];
    for (const group of groups) {
      const groupMatches = `${group.label} ${group.keywords ?? ""}`.toLowerCase().includes(needle);
      for (const option of group.options) {
        const haystack = `${group.label} ${group.keywords ?? ""} ${option.label} ${option.keywords ?? ""}`.toLowerCase();
        if (groupMatches || haystack.includes(needle)) rows.push({ group, option });
      }
    }
    return rows;
  }, [groups, query]);

  const optionRows = useMemo<Row[]>(() => {
    if (searching) return searchRows;
    if (!activeGroupData) return [];
    return activeGroupData.options.map((option) => ({ group: activeGroupData, option }));
  }, [searching, searchRows, activeGroupData]);

  const groupRows = !searching && !activeGroup ? groups : null;
  const rowCount = groupRows ? groupRows.length : optionRows.length;
  const singleGroup = groups.length === 1 ? groups[0].value : null;

  useEffect(() => {
    if (!open) return;
    const onPointerDown = (event: PointerEvent) => {
      const target = event.target as Node | null;
      if (!target) return;
      if (anchorRef.current?.contains(target) || popoverRef.current?.contains(target)) return;
      setOpen(false);
    };
    document.addEventListener("pointerdown", onPointerDown);
    return () => document.removeEventListener("pointerdown", onPointerDown);
  }, [open]);

  useEffect(() => {
    if (activeIndex < 0) return;
    const element = listRef.current?.querySelector<HTMLElement>(`[data-row-index="${activeIndex}"]`);
    element?.scrollIntoView?.({ block: "nearest" });
  }, [activeIndex]);

  function close() {
    setOpen(false);
    setQuery("");
    setActiveGroup(null);
    setActiveIndex(-1);
  }

  function enterGroup(group: string) {
    setActiveGroup(group);
    setActiveIndex(-1);
    inputRef.current?.focus();
  }

  function choose(option: MenuSelectOption) {
    onChange(option.value);
    close();
    anchorRef.current?.focus();
  }

  function onKeyDown(event: React.KeyboardEvent<HTMLDivElement>) {
    if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      close();
      anchorRef.current?.focus();
      return;
    }
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      if (rowCount === 0) return;
      event.preventDefault();
      const step = event.key === "ArrowDown" ? 1 : -1;
      setActiveIndex((current) => {
        if (current < 0) return step > 0 ? 0 : rowCount - 1;
        return (current + step + rowCount) % rowCount;
      });
      return;
    }
    if (event.key === "Enter" && activeIndex >= 0) {
      if (groupRows) {
        const group = groupRows[activeIndex];
        if (group) {
          event.preventDefault();
          enterGroup(group.value);
        }
        return;
      }
      const row = optionRows[activeIndex];
      if (row) {
        event.preventDefault();
        choose(row.option);
      }
      return;
    }
    if (event.key === "Backspace" && activeGroup && query === "" && groups.length > 1) {
      const target = event.target as HTMLInputElement;
      if (target.tagName === "INPUT" && target.value === "") {
        event.preventDefault();
        setActiveGroup(null);
        setActiveIndex(-1);
      }
    }
  }

  function optionRow(row: Row, index: number) {
    const isSelected = row.option.value === value;
    return (
      <button
        key={`${row.group.value}:${row.option.value}`}
        type="button"
        role="option"
        aria-selected={isSelected}
        data-row-index={index}
        onClick={() => choose(row.option)}
        onMouseEnter={() => setActiveIndex(index)}
        className={cn(
          "flex w-full items-start gap-2 rounded-sm px-2 py-1.5 text-left transition-colors",
          activeIndex === index ? "bg-hover" : "hover:bg-hover",
        )}
      >
        <span className="min-w-0 flex-1">
          <span className="flex items-center gap-1.5">
            <span className="truncate text-sm text-fg">{row.option.label}</span>
            {row.option.tag ? <Badge tone={row.option.tag.tone}>{row.option.tag.label}</Badge> : null}
          </span>
          {row.option.description ? (
            <span className="mt-0.5 block truncate text-2xs text-faint">{row.option.description}</span>
          ) : null}
        </span>
        {isSelected ? <IconCheck className="mt-1 size-3.5 shrink-0 text-accent" /> : null}
      </button>
    );
  }

  let list: React.ReactNode;
  if (groups.length === 0) {
    list = <p className="px-2 py-3 text-sm text-muted">{emptyLabel}</p>;
  } else if (searching) {
    list =
      searchRows.length === 0 ? (
        <p className="px-2 py-3 text-sm text-muted">No match for “{query.trim()}”.</p>
      ) : (
        searchRows.map((row, index) => (
          <Fragment key={`${row.group.value}:${row.option.value}`}>
            {index === 0 || searchRows[index - 1].group.value !== row.group.value ? (
              <div className="px-2 pt-2.5 pb-1 text-2xs font-semibold tracking-wide text-faint uppercase">
                {row.group.label}
              </div>
            ) : null}
            {optionRow(row, index)}
          </Fragment>
        ))
      );
  } else if (activeGroupData) {
    list = (
      <>
        <div className="sticky top-0 z-10 -mx-1 -mt-1 mb-1 flex items-center gap-1.5 border-b border-border bg-surface px-2 py-1.5">
          {groups.length > 1 ? (
            <button
              type="button"
              aria-label="Back to repositories"
              onClick={() => {
                setActiveGroup(null);
                setActiveIndex(-1);
                inputRef.current?.focus();
              }}
              className="rounded-sm p-0.5 text-faint transition-colors hover:bg-hover hover:text-fg"
            >
              <IconChevronRight className="size-3.5 rotate-180" />
            </button>
          ) : null}
          <span className="min-w-0 truncate text-xs font-semibold">{activeGroupData.label}</span>
          <span className="ml-auto shrink-0 text-2xs text-faint">
            {activeGroupData.options.length} item(s)
          </span>
        </div>
        {optionRows.length === 0 ? (
          <p className="px-2 py-3 text-sm text-muted">{emptyLabel}</p>
        ) : (
          optionRows.map((row, index) => optionRow(row, index))
        )}
      </>
    );
  } else {
    list = groups.map((group, index) => (
      <button
        key={group.value}
        type="button"
        role="option"
        aria-selected={false}
        data-row-index={index}
        onClick={() => enterGroup(group.value)}
        onMouseEnter={() => setActiveIndex(index)}
        className={cn(
          "flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left transition-colors",
          activeIndex === index ? "bg-hover" : "hover:bg-hover",
        )}
      >
        <span className="min-w-0 flex-1">
          <span className="block truncate text-sm text-fg">{group.label}</span>
          {group.description ? (
            <span className="block truncate text-2xs text-faint">{group.description}</span>
          ) : null}
        </span>
        <span className="shrink-0 text-2xs text-faint">{group.options.length}</span>
        <IconChevronRight className="size-3.5 shrink-0 text-faint" />
      </button>
    ));
  }

  return (
    <>
      <button
        ref={anchorRef}
        id={id}
        type="button"
        role="combobox"
        disabled={disabled}
        aria-haspopup="listbox"
        aria-expanded={open}
        onClick={() => {
          if (open) {
            close();
            return;
          }
          setQuery("");
          setActiveIndex(-1);
          setActiveGroup(singleGroup);
          setOpen(true);
        }}
        onKeyDown={(event) => {
          if (!open && (event.key === "ArrowDown" || event.key === "Enter" || event.key === " ")) {
            event.preventDefault();
            setOpen(true);
          }
        }}
        className={cn(
          CONTROL_CLASS,
          "anchor-trigger flex h-8 cursor-pointer items-center gap-1.5 pr-2 text-left disabled:cursor-not-allowed",
          selected ? "text-fg" : "text-faint",
          className,
        )}
        style={{ "--anchor": anchorName } as React.CSSProperties}
      >
        <span className="min-w-0 flex-1 truncate">{selected ? selected.label : placeholder}</span>
        <IconChevronDown className="size-3.5 shrink-0 text-faint" />
      </button>

      {open
        ? createPortal(
            <div
              ref={popoverRef}
              onKeyDown={onKeyDown}
              className="anchor-popover z-[70] flex flex-col overflow-hidden rounded-md border border-border bg-surface shadow-lg"
              style={{ "--anchor": anchorName } as React.CSSProperties}
            >
              <div className="flex items-center gap-2 border-b border-border px-2 py-1.5">
                <IconSearch className="size-3.5 shrink-0 text-faint" />
                <input
                  ref={inputRef}
                  autoFocus
                  value={query}
                  onChange={(event) => {
                    const next = event.target.value;
                    setQuery(next);
                    setActiveGroup(next.trim() ? null : singleGroup);
                    setActiveIndex(-1);
                  }}
                  placeholder={searchPlaceholder}
                  aria-label={searchPlaceholder}
                  className="h-7 min-w-0 flex-1 bg-transparent text-sm text-fg outline-none placeholder:text-faint"
                />
              </div>
              <div ref={listRef} role="listbox" aria-label={placeholder} className="min-h-0 flex-1 overflow-y-auto p-1">
                {list}
              </div>
            </div>,
            document.body,
          )
        : null}
    </>
  );
}
