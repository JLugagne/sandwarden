import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import { api } from "@/api/client";
import { useApiMutation } from "@/hooks/useApiMutation";
import { queryKeys } from "@/store/realtime";
import { formatTime } from "@/lib/format";
import { useToasts } from "@/components/Toaster";
import {
  Badge,
  Button,
  CheckboxField,
  Chip,
  CommandLine,
  ConfirmDialog,
  Description,
  DescriptionList,
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
import type { KitItemView, KitStore, KitStoreInput, Template } from "@/types";

const EMPTY_STORE: KitStoreInput = { name: "", description: "", url: "", ref: "", auth: "" };

export function KitsPage() {
  const stores = useQuery({ queryKey: queryKeys.kitStores, queryFn: api.kitStores });
  const items = useQuery({ queryKey: queryKeys.kitItems, queryFn: () => api.kitItems() });
  const templates = useQuery({ queryKey: queryKeys.templates, queryFn: api.templates });
  const [editing, setEditing] = useState<{ mode: "create" } | { mode: "edit"; store: KitStore } | null>(null);
  const [deleting, setDeleting] = useState<KitStore | null>(null);
  const [inspecting, setInspecting] = useState<KitItemView | null>(null);
  const [removingTemplate, setRemovingTemplate] = useState<Template | null>(null);
  const [searchParams, setSearchParams] = useSearchParams();
  const [pendingItem, setPendingItem] = useState<{ store: string; name: string } | null>(null);

  const create = useApiMutation({
    mutationFn: (body: KitStoreInput) => api.createKitStore(body),
    success: (store) => (store.error ? undefined : `Repository ${store.name} added`),
    invalidate: [queryKeys.kitStores, queryKeys.kitItems],
    onSuccess: () => setEditing(null),
  });
  const update = useApiMutation({
    mutationFn: ({ slug, body }: { slug: string; body: KitStoreInput }) => api.updateKitStore(slug, body),
    success: (store) => (store.error ? undefined : `Repository ${store.name} updated`),
    invalidate: [queryKeys.kitStores, queryKeys.kitItems],
    onSuccess: () => setEditing(null),
  });
  const remove = useApiMutation({
    mutationFn: (slug: string) => api.deleteKitStore(slug),
    success: "Repository deleted",
    invalidate: [queryKeys.kitStores, queryKeys.kitItems],
    onSuccess: () => setDeleting(null),
  });
  const refresh = useApiMutation({
    mutationFn: (slug: string) => api.refreshKitStore(slug),
    success: (store) => (store.error ? undefined : `Repository ${store.name} refreshed`),
    invalidate: [queryKeys.kitStores, queryKeys.kitItems],
  });
  const removeTemplate = useApiMutation({
    mutationFn: (ref: string) => api.removeTemplate(ref),
    success: "Template removed",
    invalidate: [queryKeys.templates],
    onSuccess: () => setRemovingTemplate(null),
  });

  const rows = stores.data ?? [];
  const catalog = items.data ?? [];
  const templateRows = templates.data ?? [];

  // Deep link from the global search overlay: `/kits?store=<slug>&item=<name>`
  // opens the details dialog once the catalog is loaded.
  useEffect(() => {
    const store = searchParams.get("store");
    const raw = searchParams.get("item");
    if (!raw) return;
    setPendingItem({ store: store ?? "", name: raw });
    setSearchParams(
      (current) => {
        const next = new URLSearchParams(current);
        next.delete("store");
        next.delete("item");
        return next;
      },
      { replace: true },
    );
  }, [searchParams, setSearchParams]);

  useEffect(() => {
    if (pendingItem === null) return;
    const match = catalog.find((item) => item.name === pendingItem.name && item.store === pendingItem.store);
    if (!match) return;
    setInspecting(match);
    setPendingItem(null);
  }, [pendingItem, catalog]);

  return (
    <>
      <PageHeader
        title="Kits"
        subtitle="Kit artifacts are declarative YAML specs that define a sandbox agent or extend one (mixin) with credentials, network policy, env vars, startup commands and files. Register a repository, then reference its kits from the create form with --kit."
      />

      <div className="flex flex-col gap-4">
        <Panel
          title="Kit repositories"
          description="Each repository is cloned into the app data directory; every directory holding a spec.yaml is inspected through `sbx kit inspect` and listed below."
          actions={
            <Button variant="primary" onClick={() => setEditing({ mode: "create" })}>
              <IconPlus /> Add repository
            </Button>
          }
        >
          {stores.isLoading ? (
            <div className="flex justify-center py-6">
              <Spinner />
            </div>
          ) : rows.length === 0 ? (
            <EmptyState
              title="No kit repository yet"
              description="Add a repository such as https://github.com/docker/sbx-kits-contrib to browse its kits and reference them at sandbox creation."
              action={
                <Button variant="primary" onClick={() => setEditing({ mode: "create" })}>
                  <IconPlus /> Add repository
                </Button>
              }
            />
          ) : (
            <p className="text-sm text-muted">
              {rows.length} repository(ies), {catalog.length} kit(s) discovered.
            </p>
          )}
        </Panel>

        {rows.map((store) => (
          <StorePanel
            key={store.slug}
            store={store}
            items={catalog.filter((item) => item.store === store.slug)}
            refreshing={refresh.isPending && refresh.variables === store.slug}
            onRefresh={() => refresh.mutate(store.slug)}
            onEdit={() => setEditing({ mode: "edit", store })}
            onDelete={() => setDeleting(store)}
            onInspect={setInspecting}
          />
        ))}

        <Panel
          title="Template images"
          description="Sandbox snapshots stored in the local runtime image store (`sbx template ls`). Pick one as the base image in the create form."
        >
          {templates.isLoading ? (
            <div className="flex justify-center py-6">
              <Spinner />
            </div>
          ) : templateRows.length === 0 ? (
            <p className="text-sm text-muted">No template image on this host yet.</p>
          ) : (
            <TableWrap className="border-0">
              <thead>
                <tr>
                  <TH>Repository</TH>
                  <TH>Tag</TH>
                  <TH>Flavor</TH>
                  <TH>Size</TH>
                  <TH>Created</TH>
                  <TH className="w-24" />
                </tr>
              </thead>
              <tbody>
                {templateRows.map((template) => (
                  <TRow key={template.id}>
                    <TD className="font-mono text-xs">{template.repository}</TD>
                    <TD className="font-mono text-xs">{template.tag}</TD>
                    <TD className="text-xs text-muted">{template.flavor || "—"}</TD>
                    <TD className="text-xs text-muted">{formatSize(template.size)}</TD>
                    <TD className="text-xs text-muted">{formatTime(template.created_at)}</TD>
                    <TD className="text-right">
                      <Button size="sm" variant="ghost" onClick={() => setRemovingTemplate(template)}>
                        <span className="text-danger">Remove</span>
                      </Button>
                    </TD>
                  </TRow>
                ))}
              </tbody>
            </TableWrap>
          )}
        </Panel>
      </div>

      <StoreDialog
        open={editing !== null}
        store={editing?.mode === "edit" ? editing.store : null}
        busy={create.isPending || update.isPending}
        onClose={() => setEditing(null)}
        onSubmit={(body) => {
          if (editing?.mode === "edit") update.mutate({ slug: editing.store.slug, body });
          else create.mutate(body);
        }}
      />

      <ConfirmDialog
        open={deleting !== null}
        title={`Delete repository ${deleting?.name ?? ""}?`}
        body="Its checkout and discovered kits are removed. Sandboxes that already reference its kits keep the reference."
        confirmLabel="Delete"
        busy={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting.slug)}
        onClose={() => setDeleting(null)}
      />

      <ConfirmDialog
        open={removingTemplate !== null}
        title={`Remove template ${removingTemplate ? `${removingTemplate.repository}:${removingTemplate.tag}` : ""}?`}
        body="The image is deleted from the local sandbox runtime image store."
        confirmLabel="Remove"
        busy={removeTemplate.isPending}
        onConfirm={() => removingTemplate && removeTemplate.mutate(templateRef(removingTemplate))}
        onClose={() => setRemovingTemplate(null)}
      />

      <KitDialog item={inspecting} onClose={() => setInspecting(null)} />
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
  onInspect,
}: {
  store: KitStore;
  items: KitItemView[];
  refreshing: boolean;
  onRefresh: () => void;
  onEdit: () => void;
  onDelete: () => void;
  onInspect: (item: KitItemView) => void;
}) {
  const toast = useToasts();

  const validate = useApiMutation({
    mutationFn: ({ store, name }: { store: string; name: string }) => api.validateKit(store, name),
    onSuccess: (result) => {
      toast.push({
        tone: result.ok ? "success" : "danger",
        title: result.ok ? "Kit is valid" : "Kit is invalid",
        body: result.output,
      });
    },
  });

  return (
    <Panel
      title={
        <span className="flex flex-wrap items-center gap-2">
          {store.name}
          {store.error ? <Badge tone="danger">sync warning</Badge> : null}
          {store.auth === "ssh" ? <Badge tone="success">ssh</Badge> : null}
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
          {store.synced_at ? `Synced ${store.synced_at}` : "Never synced"} · {items.length} kit(s)
        </span>
        {store.error ? <span className="text-warning">{store.error}</span> : null}
      </div>

      {items.length === 0 ? (
        <p className="p-4 text-sm text-muted">No kit discovered. Check the repository layout, then Refresh.</p>
      ) : (
        <TableWrap className="border-0">
          <thead>
            <tr>
              <TH>Kind</TH>
              <TH>Name</TH>
              <TH>Image</TH>
              <TH>Requires</TH>
              <TH>Description</TH>
              <TH className="w-40" />
            </tr>
          </thead>
          <tbody>
            {items.map((item) => (
              <TRow key={item.ref}>
                <TD>
                  <KindBadge kind={item.kind} />
                </TD>
                <TD>
                  <span className="font-medium">{item.display_name || item.name}</span>
                  {item.version ? <span className="ml-2 text-xs text-faint">v{item.version}</span> : null}
                </TD>
                <TD className="font-mono text-xs text-muted">{item.image || "—"}</TD>
                <TD className="text-xs text-muted">{item.requires_agent || "—"}</TD>
                <TD className="text-muted">{item.description || "—"}</TD>
                <TD className="text-right">
                  <Button
                    size="sm"
                    variant="ghost"
                    loading={
                      validate.isPending &&
                      validate.variables?.store === item.store &&
                      validate.variables?.name === item.name
                    }
                    onClick={() => validate.mutate({ store: item.store, name: item.name })}
                  >
                    Validate
                  </Button>
                  <Button size="sm" variant="ghost" onClick={() => onInspect(item)}>
                    Details
                  </Button>
                </TD>
              </TRow>
            ))}
          </tbody>
        </TableWrap>
      )}
    </Panel>
  );
}

function KindBadge({ kind }: { kind: string }) {
  return kind === "sandbox" ? <Badge tone="accent">sandbox</Badge> : <Badge>mixin</Badge>;
}

function KitDialog({ item, onClose }: { item: KitItemView | null; onClose: () => void }) {
  const spec = item?.spec;
  const setup = spec?.setup;
  const env = useMemo(() => Object.entries(spec?.environment?.variables ?? {}), [spec]);
  const args = useMemo(() => Object.entries(spec?.arguments ?? {}), [spec]);

  return (
    <Modal
      open={item !== null}
      onClose={onClose}
      title={item ? `${item.display_name || item.name} (${item.kind})` : "Kit"}
      description={spec?.description || undefined}
      size="lg"
      footer={
        <Button variant="ghost" onClick={onClose}>
          Close
        </Button>
      }
    >
      {item ? (
        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-1.5">
            <span className="text-xs font-medium text-muted">Reference for `sbx create --kit`</span>
            <CommandLine command={item.ref} />
          </div>

          <DescriptionList>
            <Description label="Kind">{item.kind || "—"}</Description>
            <Description label="Version">{item.version || "—"}</Description>
            <Description label="Schema">{spec?.schemaVersion || "—"}</Description>
            <Description label="Image">
              <span className="font-mono text-xs">{item.image || "—"}</span>
            </Description>
            <Description label="Requires agent">{item.requires_agent || "any"}</Description>
            <Description label="Repository">{item.store_name}</Description>
            {spec?.sourceURL ? (
              <Description label="Source">
                <a href={spec.sourceURL} className="text-xs text-accent hover:underline">
                  {spec.sourceURL}
                </a>
              </Description>
            ) : null}
          </DescriptionList>

          {spec?.sandbox?.entrypoint?.length || spec?.sandbox?.command?.default?.length ? (
            <Section title="Entrypoint">
              {spec?.sandbox?.entrypoint?.length ? (
                <CommandLine command={spec.sandbox.entrypoint.join(" ")} />
              ) : null}
              {spec?.sandbox?.command?.default?.length ? (
                <CommandLine command={spec.sandbox.command.default.join(" ")} />
              ) : null}
            </Section>
          ) : null}

          {spec?.ports?.length ? (
            <Section title="Ports">
              <span className="flex flex-wrap gap-1.5">
                {spec.ports.map((port, index) => (
                  <Chip key={index}>
                    {port.container}/{port.protocol || "tcp"}
                    {port.name ? <span className="text-faint"> {port.name}</span> : null}
                  </Chip>
                ))}
              </span>
            </Section>
          ) : null}

          {env.length > 0 ? (
            <Section title="Environment">
              <TableWrap>
                <tbody>
                  {env.map(([name, value]) => (
                    <TRow key={name}>
                      <TD className="font-mono text-xs">{name}</TD>
                      <TD className="font-mono text-xs text-muted">{value}</TD>
                    </TRow>
                  ))}
                </tbody>
              </TableWrap>
            </Section>
          ) : null}

          {spec?.permissions?.network?.allow?.length || spec?.permissions?.network?.deny?.length ? (
            <Section title="Network policy">
              <div className="flex flex-col gap-2">
                {spec?.permissions?.network?.allow?.length ? (
                  <span className="flex flex-wrap items-center gap-1.5">
                    <span className="text-xs text-muted">allow</span>
                    {spec.permissions.network.allow.map((host) => (
                      <Chip key={host}>{host}</Chip>
                    ))}
                  </span>
                ) : null}
                {spec?.permissions?.network?.deny?.length ? (
                  <span className="flex flex-wrap items-center gap-1.5">
                    <span className="text-xs text-muted">deny</span>
                    {spec.permissions.network.deny.map((host) => (
                      <Chip key={host}>{host}</Chip>
                    ))}
                  </span>
                ) : null}
              </div>
            </Section>
          ) : null}

          {setup?.install?.length || setup?.startup?.length ? (
            <Section title="Setup">
              <div className="flex flex-col gap-2">
                {(setup?.install ?? []).map((hook, index) => (
                  <CommandLine key={`install-${index}`} command={hook.command ?? ""} />
                ))}
                {(setup?.startup ?? []).map((hook, index) => (
                  <CommandLine key={`startup-${index}`} command={hook.command ?? ""} />
                ))}
              </div>
            </Section>
          ) : null}

          {setup?.files?.length ? (
            <Section title="Files">
              <TableWrap>
                <thead>
                  <tr>
                    <TH>Path</TH>
                    <TH>Mode</TH>
                    <TH>Description</TH>
                  </tr>
                </thead>
                <tbody>
                  {(setup.files ?? []).map((file) => (
                    <TRow key={file.path}>
                      <TD className="font-mono text-xs">{file.path}</TD>
                      <TD className="font-mono text-xs text-muted">{file.mode || "—"}</TD>
                      <TD className="text-xs text-muted">{file.description || "—"}</TD>
                    </TRow>
                  ))}
                </tbody>
              </TableWrap>
            </Section>
          ) : null}

          {args.length > 0 ? (
            <Section title="Arguments (--kit-arg name=value)">
              <TableWrap>
                <thead>
                  <tr>
                    <TH>Name</TH>
                    <TH>Required</TH>
                    <TH>Default</TH>
                    <TH>Description</TH>
                  </tr>
                </thead>
                <tbody>
                  {args.map(([name, arg]) => (
                    <TRow key={name}>
                      <TD className="font-mono text-xs">{name}</TD>
                      <TD className="text-xs">{arg.required ? <span className="text-warning">required</span> : "no"}</TD>
                      <TD className="font-mono text-xs text-muted">{arg.default ?? "—"}</TD>
                      <TD className="text-xs text-muted">
                        {arg.description || "—"}
                        {arg.enum?.length ? <span className="text-faint"> ({arg.enum.join(" | ")})</span> : null}
                        {arg.pattern ? <span className="text-faint"> /{arg.pattern}/</span> : null}
                      </TD>
                    </TRow>
                  ))}
                </tbody>
              </TableWrap>
            </Section>
          ) : null}
        </div>
      ) : null}
    </Modal>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-1.5">
      <span className="text-xs font-medium text-muted">{title}</span>
      {children}
    </div>
  );
}

function StoreDialog({
  open,
  store,
  busy,
  onSubmit,
  onClose,
}: {
  open: boolean;
  store: KitStore | null;
  busy: boolean;
  onSubmit: (body: KitStoreInput) => void;
  onClose: () => void;
}) {
  const [input, setInput] = useState<KitStoreInput>(EMPTY_STORE);
  const [loadedSlug, setLoadedSlug] = useState<string | "new" | null>(null);

  const target = store ? store.slug : ("new" as const);
  if (!open && loadedSlug !== null) {
    setLoadedSlug(null);
  }
  if (open && loadedSlug !== target) {
    setLoadedSlug(target);
    setInput(
      store
        ? {
            name: store.name,
            description: store.description,
            url: store.url,
            ref: store.ref,
            auth: store.auth,
          }
        : EMPTY_STORE,
    );
  }

  function update<K extends keyof KitStoreInput>(key: K, value: KitStoreInput[K]) {
    setInput((current) => ({ ...current, [key]: value }));
  }

  const valid = input.name.trim() !== "" && input.url.trim() !== "";

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={store ? `Edit repository ${store.name}` : "Add a kit repository"}
      description="A git repository whose subdirectories hold kit artifacts (spec.yaml + optional files/), such as https://github.com/docker/sbx-kits-contrib. HTTPS URLs may embed credentials; SSH URLs need the ~/.ssh option below."
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
              placeholder="sbx-kits-contrib"
            />
          </Field>
          <Field label="Description">
            <Input
              value={input.description}
              onChange={(event) => update("description", event.target.value)}
              placeholder="What this repository provides"
            />
          </Field>
        </div>
        <Field label="Git URL">
          <Input
            value={input.url}
            onChange={(event) => update("url", event.target.value)}
            placeholder="https://github.com/docker/sbx-kits-contrib"
            className="font-mono text-xs"
          />
        </Field>
        <Field label="Branch or tag" hint="Leave empty to follow the default branch. Pin a tag for reproducible kit references.">
          <Input
            value={input.ref}
            onChange={(event) => update("ref", event.target.value)}
            placeholder="main"
            className="font-mono text-xs"
          />
        </Field>
        <CheckboxField
          label="authenticate with ~/.ssh keys"
          hint="Uses your ssh-agent or the default private keys of ~/.ssh, with host keys verified against known_hosts. Needed for private repositories over SSH."
          checked={input.auth === "ssh"}
          onChange={(value) => update("auth", value ? "ssh" : "")}
        />
      </div>
    </Modal>
  );
}

function templateRef(template: Template): string {
  if (!template.repository || !template.tag) return template.id;
  return `${template.repository}:${template.tag}`;
}

function formatSize(bytes: number): string {
  if (!bytes) return "—";
  const gb = bytes / 1024 ** 3;
  if (gb >= 1) return `${gb.toFixed(2)} GB`;
  return `${Math.round(bytes / 1024 ** 2)} MB`;
}
