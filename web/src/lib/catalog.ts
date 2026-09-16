import type { MenuSelectGroup, MenuSelectOption } from "@/components/ui";
import type { KitItemView, SkillItem } from "@/types";

function groupByStore<T>(
  items: T[],
  store: (item: T) => string,
  option: (item: T) => MenuSelectOption,
): MenuSelectGroup[] {
  const stores = new Map<string, MenuSelectOption[]>();
  for (const item of items) {
    const key = store(item) || "—";
    const options = stores.get(key) ?? [];
    options.push(option(item));
    stores.set(key, options);
  }
  return [...stores.entries()]
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([label, options]) => ({
      value: label,
      label,
      keywords: label,
      options: options.sort((left, right) => left.label.localeCompare(right.label)),
    }));
}

export function skillMenuGroups(items: SkillItem[]): MenuSelectGroup[] {
  return groupByStore(
    items,
    (item) => item.store_name,
    (item) => ({
      value: String(item.id),
      label: item.kind === "command" ? `/${item.name}` : item.name,
      description: item.description || undefined,
      tag: { label: item.kind === "command" ? "command" : "skill", tone: item.kind === "command" ? "accent" : undefined },
      keywords: `${item.name} ${item.plugin} ${item.description}`,
    }),
  );
}

export function kitMenuGroups(items: KitItemView[]): MenuSelectGroup[] {
  return groupByStore(
    items,
    (item) => item.store_name,
    (item) => ({
      value: item.ref,
      label: item.display_name || item.name,
      description: item.description || undefined,
      tag: { label: item.kind },
      keywords: `${item.name} ${item.kind} ${item.description} ${item.image}`,
    }),
  );
}

/**
 * Agent picker options: the built-in agents plus every sandbox kit discovered
 * in the repositories, since `sbx create` accepts either as its positional
 * argument.
 */
export function agentMenuGroups(agents: string[], items: KitItemView[]): MenuSelectGroup[] {
  const options: MenuSelectOption[] = agents.map((agent) => ({
    value: agent,
    label: agent,
    keywords: agent,
  }));
  for (const item of items) {
    if (item.kind !== "sandbox") continue;
    options.push({
      value: item.ref,
      label: item.display_name || item.name,
      description: [item.store_name, item.version ? `v${item.version}` : ""].filter(Boolean).join(" · ") || undefined,
      tag: { label: "agent kit", tone: "accent" },
      keywords: `${item.name} ${item.display_name} ${item.store_name} ${item.description}`,
    });
  }
  return [{ value: "agents", label: "Agents", keywords: "agent kit", options }];
}
