import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import { useApiMutation } from "@/hooks/useApiMutation";
import { queryKeys } from "@/store/realtime";
import {
  Badge,
  Button,
  ConfirmDialog,
  EmptyState,
  Field,
  IconPlus,
  Input,
  Modal,
  PageHeader,
  Panel,
  Spinner,
  TableWrap,
  TD,
  TH,
  TRow,
} from "@/components/ui";
import type { SkillItem, SkillStore, SkillStoreInput } from "@/types";

const EMPTY_STORE: SkillStoreInput = { name: "", description: "", url: "", ref: "" };

export function SkillsPage() {
  const stores = useQuery({ queryKey: queryKeys.skillStores, queryFn: api.skillStores });
  const items = useQuery({ queryKey: queryKeys.skillItems, queryFn: () => api.skillItems() });
  const [editing, setEditing] = useState<{ mode: "create" } | { mode: "edit"; store: SkillStore } | null>(
    null,
  );
  const [deleting, setDeleting] = useState<SkillStore | null>(null);

  const create = useApiMutation({
    mutationFn: (body: SkillStoreInput) => api.createSkillStore(body),
    success: (store) => (store.error ? undefined : `Store ${store.name} added`),
    onSuccess: () => setEditing(null),
  });
  const update = useApiMutation({
    mutationFn: ({ id, body }: { id: number; body: SkillStoreInput }) => api.updateSkillStore(id, body),
    success: (store) => (store.error ? undefined : `Store ${store.name} updated`),
    onSuccess: () => setEditing(null),
  });
  const remove = useApiMutation({
    mutationFn: (id: number) => api.deleteSkillStore(id),
    success: "Store deleted",
    onSuccess: () => setDeleting(null),
  });
  const refresh = useApiMutation({
    mutationFn: (id: number) => api.refreshSkillStore(id),
    success: (store) => (store.error ? undefined : `Store ${store.name} refreshed`),
  });

  const rows = stores.data ?? [];
  const catalog = items.data ?? [];

  return (
    <>
      <PageHeader
        title="Skills"
        subtitle="Git stores in the Anthropic plugin format. Skills and commands are picked per profile and mounted read-only at ~/.agents in each sandbox."
      />

      <div className="flex flex-col gap-4">
        <Panel
          title="Skill stores"
          description="Each store is cloned into the app data directory and refreshed on demand. Plugin manifests are read when present, otherwise skills/ and commands/ directories are scanned."
          actions={
            <Button variant="primary" onClick={() => setEditing({ mode: "create" })}>
              <IconPlus /> Add store
            </Button>
          }
        >
          {stores.isLoading ? (
            <div className="flex justify-center py-6">
              <Spinner />
            </div>
          ) : rows.length === 0 ? (
            <EmptyState
              title="No skill store yet"
              description="Add a git repository in the Anthropic plugin format, then pick its skills and commands from a profile or a sandbox."
              action={
                <Button variant="primary" onClick={() => setEditing({ mode: "create" })}>
                  <IconPlus /> Add store
                </Button>
              }
            />
          ) : (
            <p className="text-sm text-muted">
              {rows.length} store(s), {catalog.length} item(s) discovered.
            </p>
          )}
        </Panel>

        {rows.map((store) => (
          <StorePanel
            key={store.id}
            store={store}
            items={catalog.filter((item) => item.store_id === store.id)}
            refreshing={refresh.isPending && refresh.variables === store.id}
            onRefresh={() => refresh.mutate(store.id)}
            onEdit={() => setEditing({ mode: "edit", store })}
            onDelete={() => setDeleting(store)}
          />
        ))}
      </div>

      <StoreDialog
        open={editing !== null}
        store={editing?.mode === "edit" ? editing.store : null}
        busy={create.isPending || update.isPending}
        onClose={() => setEditing(null)}
        onSubmit={(body) => {
          if (editing?.mode === "edit") update.mutate({ id: editing.store.id, body });
          else create.mutate(body);
        }}
      />

      <ConfirmDialog
        open={deleting !== null}
        title={`Delete store ${deleting?.name ?? ""}?`}
        body="Its catalog, the profile and sandbox selections pointing at it, and its checkout are removed. Running sandboxes unmount the items."
        confirmLabel="Delete"
        busy={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting.id)}
        onClose={() => setDeleting(null)}
      />
    </>
  );
}

function StorePanel({
  store,
  items,
  refreshing,
  onRefresh,
  onEdit,
  onDelete,
}: {
  store: SkillStore;
  items: SkillItem[];
  refreshing: boolean;
  onRefresh: () => void;
  onEdit: () => void;
  onDelete: () => void;
}) {
  const skills = items.filter((item) => item.kind === "skill").length;
  const commands = items.filter((item) => item.kind === "command").length;

  return (
    <Panel
      title={
        <span className="flex flex-wrap items-center gap-2">
          {store.name}
          {store.error ? <Badge tone="danger">sync failed</Badge> : null}
          {store.ref ? <Badge tone="accent">{store.ref}</Badge> : null}
        </span>
      }
      description={store.description || undefined}
      actions={
        <>
          <Button size="sm" variant="ghost" loading={refreshing} onClick={onRefresh}>
            Refresh
          </Button>
          <Button size="sm" variant="ghost" onClick={onEdit}>
            Edit
          </Button>
          <Button size="sm" variant="ghost" onClick={onDelete}>
            <span className="text-danger">Delete</span>
          </Button>
        </>
      }
      bodyClassName="p-0"
    >
      <div className="flex flex-col gap-1 border-b border-border p-4 text-xs text-muted">
        <span className="font-mono">{store.url}</span>
        <span>
          {store.synced_at ? `Synced ${store.synced_at}` : "Never synced"} · {skills} skill(s),{" "}
          {commands} command(s)
        </span>
        {store.error ? <span className="text-danger">{store.error}</span> : null}
      </div>

      {items.length === 0 ? (
        <p className="p-4 text-sm text-muted">
          No skill or command discovered. Check the repository layout, then Refresh.
        </p>
      ) : (
        <TableWrap className="border-0">
          <thead>
            <tr>
              <TH>Kind</TH>
              <TH>Name</TH>
              <TH>Plugin</TH>
              <TH>Description</TH>
            </tr>
          </thead>
          <tbody>
            {items.map((item) => (
              <TRow key={item.id}>
                <TD>
                  <KindBadge kind={item.kind} />
                </TD>
                <TD className="font-medium">{item.name}</TD>
                <TD className="text-muted">{item.plugin || "—"}</TD>
                <TD className="text-muted">{item.description || "—"}</TD>
              </TRow>
            ))}
          </tbody>
        </TableWrap>
      )}
    </Panel>
  );
}

export function KindBadge({ kind }: { kind: string }) {
  return kind === "command" ? <Badge tone="accent">command</Badge> : <Badge>skill</Badge>;
}

function StoreDialog({
  open,
  store,
  busy,
  onSubmit,
  onClose,
}: {
  open: boolean;
  store: SkillStore | null;
  busy: boolean;
  onSubmit: (body: SkillStoreInput) => void;
  onClose: () => void;
}) {
  const [input, setInput] = useState<SkillStoreInput>(EMPTY_STORE);
  const [loadedId, setLoadedId] = useState<number | "new" | null>(null);

  const target = store ? store.id : ("new" as const);
  if (open && loadedId !== target) {
    setLoadedId(target);
    setInput(
      store
        ? { name: store.name, description: store.description, url: store.url, ref: store.ref }
        : EMPTY_STORE,
    );
  }

  function update<K extends keyof SkillStoreInput>(key: K, value: SkillStoreInput[K]) {
    setInput((current) => ({ ...current, [key]: value }));
  }

  const valid = input.name.trim() !== "" && input.url.trim() !== "";

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={store ? `Edit store ${store.name}` : "Add a skill store"}
      description="A git repository using the Anthropic plugin format: either a marketplace with plugins, or plain skills/ and commands/ directories. Private repositories need credentials embedded in the URL."
      size="md"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" disabled={!valid} loading={busy} onClick={() => onSubmit(input)}>
            {store ? "Save" : "Add"}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Name">
            <Input
              value={input.name}
              onChange={(event) => update("name", event.target.value)}
              placeholder="anthropics"
            />
          </Field>
          <Field label="Description">
            <Input
              value={input.description}
              onChange={(event) => update("description", event.target.value)}
              placeholder="What this store provides"
            />
          </Field>
        </div>
        <Field label="Git URL">
          <Input
            value={input.url}
            onChange={(event) => update("url", event.target.value)}
            placeholder="https://github.com/anthropics/skills"
            className="font-mono text-xs"
          />
        </Field>
        <Field label="Branch or tag" hint="Leave empty to follow the default branch.">
          <Input
            value={input.ref}
            onChange={(event) => update("ref", event.target.value)}
            placeholder="main"
            className="font-mono text-xs"
          />
        </Field>
      </div>
    </Modal>
  );
}
