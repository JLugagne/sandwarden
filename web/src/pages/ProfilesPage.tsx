import { useState } from "react";
import { Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import { errorMessage, useApiMutation } from "@/hooks/useApiMutation";
import { queryKeys } from "@/store/realtime";
import { useToasts } from "@/components/Toaster";
import {
  Badge,
  Button,
  CheckboxField,
  ConfirmDialog,
  DecisionBadge,
  EmptyState,
  Field,
  IconFolder,
  IconPlus,
  Input,
  MenuSelect,
  Modal,
  PageHeader,
  Panel,
  Select,
  Spinner,
  TableWrap,
  Tabs,
  TD,
  TH,
  TRow,
} from "@/components/ui";
import { skillMenuGroups } from "@/lib/catalog";
import { KindBadge } from "@/pages/SkillsPage";
import type { CacheMount, ProfileMount, ProfileView, SkillItem } from "@/types";

type ProfileBody = { name: string; description: string; is_default: boolean; is_global: boolean };
type MountInput = { host_path: string; target_path: string; read_only: boolean };

export function ProfilesPage() {
  const [editing, setEditing] = useState<ProfileView | null>(null);
  const [deleting, setDeleting] = useState<ProfileView | null>(null);

  const profiles = useQuery({ queryKey: queryKeys.profiles, queryFn: api.profiles });
  const catalog = useQuery({ queryKey: queryKeys.skillItems, queryFn: () => api.skillItems() });
  const caches = useQuery({ queryKey: queryKeys.caches, queryFn: api.caches });

  const create = useApiMutation({
    mutationFn: api.createProfile,
    success: (profile) => `Profile ${profile.name} created`,
  });
  const update = useApiMutation({
    mutationFn: ({ id, body }: { id: number; body: ProfileBody }) => api.updateProfile(id, body),
    success: (profile) => `Profile ${profile.name} updated`,
    onSuccess: () => setEditing(null),
  });
  const remove = useApiMutation({
    mutationFn: (id: number) => api.deleteProfile(id),
    success: "Profile deleted",
    onSuccess: () => setDeleting(null),
  });
  const addRule = useApiMutation({
    mutationFn: ({ profileId, decision, pattern }: { profileId: number; decision: string; pattern: string }) =>
      api.addRule(profileId, { decision, pattern }),
    success: "Rule added",
  });
  const removeRule = useApiMutation({
    mutationFn: ({ profileId, ruleId }: { profileId: number; ruleId: number }) =>
      api.removeRule(profileId, ruleId),
    success: "Rule removed",
  });
  const addItem = useApiMutation({
    mutationFn: ({ profileId, itemId }: { profileId: number; itemId: number }) =>
      api.addProfileSkillItem(profileId, itemId),
    success: "Skill added to profile",
  });
  const removeItem = useApiMutation({
    mutationFn: ({ profileId, itemId }: { profileId: number; itemId: number }) =>
      api.removeProfileSkillItem(profileId, itemId),
    success: "Skill removed from profile",
  });
  const addMount = useApiMutation({
    mutationFn: ({ profileId, body }: { profileId: number; body: MountInput }) =>
      api.addProfileMount(profileId, body),
    success: "Default mount added",
    invalidate: [queryKeys.profiles],
  });
  const removeMount = useApiMutation({
    mutationFn: ({ profileId, mountId }: { profileId: number; mountId: number }) =>
      api.removeProfileMount(profileId, mountId),
    success: "Default mount removed",
    invalidate: [queryKeys.profiles],
  });
  const addCache = useApiMutation({
    mutationFn: ({ profileId, cacheId }: { profileId: number; cacheId: number }) =>
      api.addProfileCache(profileId, cacheId),
    success: "Default cache added",
    invalidate: [queryKeys.profiles],
  });
  const removeCache = useApiMutation({
    mutationFn: ({ profileId, cacheId }: { profileId: number; cacheId: number }) =>
      api.removeProfileCache(profileId, cacheId),
    success: "Default cache removed",
    invalidate: [queryKeys.profiles],
  });

  const rows = profiles.data ?? [];
  // Follow the refreshed query data so the dialog sees mounts/caches/rules
  // added while it is open.
  const openProfile = editing ? rows.find((profile) => profile.id === editing.id) ?? editing : null;

  return (
    <>
      <PageHeader
        title="Profiles"
        subtitle="Reusable bundles: allow/deny rules, skills, default mounts and caches applied to every assigned sandbox."
      />

      <div className="flex flex-col gap-4">
        <NewProfileForm onSubmit={(body) => create.mutate(body)} busy={create.isPending} />

        <ErrorNoteIfAny error={profiles.error} />
        {profiles.isLoading ? (
          <div className="flex justify-center py-10">
            <Spinner />
          </div>
        ) : rows.length === 0 ? (
          <EmptyState title="No profiles yet" description="Create one above, then add rules, skills, mounts and caches." />
        ) : (
          rows.map((profile) => (
            <ProfileCard
              key={profile.id}
              profile={profile}
              onEdit={() => setEditing(profile)}
              onDelete={() => setDeleting(profile)}
            />
          ))
        )}
      </div>

      <ProfileDialog
        profile={openProfile}
        caches={caches.data ?? []}
        catalog={catalog.data ?? []}
        busy={update.isPending}
        onClose={() => setEditing(null)}
        onSave={(body) => openProfile && update.mutate({ id: openProfile.id, body })}
        addRule={addRule}
        removeRule={removeRule}
        addItem={addItem}
        removeItem={removeItem}
        addMount={addMount}
        removeMount={removeMount}
        addCache={addCache}
        removeCache={removeCache}
      />
      <ConfirmDialog
        open={deleting !== null}
        title={`Delete profile ${deleting?.name ?? ""}?`}
        body="Its rules are removed from every sandbox it is assigned to, and its default mounts and caches are released."
        confirmLabel="Delete"
        busy={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting.id)}
        onClose={() => setDeleting(null)}
      />
    </>
  );
}

function ErrorNoteIfAny({ error }: { error: unknown }) {
  if (!error) return null;
  return (
    <div className="rounded-md border border-danger/40 bg-danger-soft px-3 py-2 text-sm text-danger">
      {error instanceof Error ? error.message : String(error)}
    </div>
  );
}

function Counter({ label, value }: { label: string; value: number }) {
  return (
    <span className="inline-flex items-center gap-1.5 rounded-sm border border-border bg-canvas px-2 py-1 text-2xs text-faint">
      <span className="font-semibold text-fg">{value}</span>
      {label}
    </span>
  );
}

function ProfileCard({
  profile,
  onEdit,
  onDelete,
}: {
  profile: ProfileView;
  onEdit: () => void;
  onDelete: () => void;
}) {
  const sandboxes = profile.sandboxes ?? [];
  return (
    <Panel
      title={
        <span className="flex flex-wrap items-center gap-2">
          {profile.name}
          {profile.is_default ? <Badge tone="warning">default</Badge> : null}
          {profile.is_global ? <Badge tone="accent">global</Badge> : null}
        </span>
      }
      description={profile.description || undefined}
      actions={
        <>
          <Button size="sm" variant="ghost" onClick={onEdit}>
            Edit
          </Button>
          <Button size="sm" variant="ghost" onClick={onDelete}>
            <span className="text-danger">Delete</span>
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-3">
        <div className="flex flex-wrap gap-1.5">
          <Counter label="rules" value={(profile.rules ?? []).length} />
          <Counter label="skills" value={(profile.items ?? []).length} />
          <Counter label="mounts" value={(profile.mounts ?? []).length} />
          <Counter label="caches" value={(profile.caches ?? []).length} />
          <Counter label="sandboxes" value={sandboxes.length} />
        </div>
        {sandboxes.length === 0 ? (
          <p className="text-sm text-muted">Not assigned. Assign it from a sandbox’s Profiles tab.</p>
        ) : (
          <div className="flex flex-wrap gap-1.5">
            {sandboxes.map((sandbox) => (
              <Link
                key={sandbox}
                to={`/sandboxes/${encodeURIComponent(sandbox)}?tab=profiles`}
                className="rounded-sm border border-border bg-canvas px-2 py-1 font-mono text-xs hover:border-accent hover:text-accent"
              >
                {sandbox}
              </Link>
            ))}
          </div>
        )}
      </div>
    </Panel>
  );
}

function NewProfileForm({ onSubmit, busy }: { onSubmit: (body: ProfileBody) => void; busy: boolean }) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [isDefault, setIsDefault] = useState(false);
  const [isGlobal, setIsGlobal] = useState(false);

  return (
    <Panel title="New profile">
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Name">
          <Input value={name} onChange={(event) => setName(event.target.value)} placeholder="production-api" />
        </Field>
        <Field label="Description">
          <Input
            value={description}
            onChange={(event) => setDescription(event.target.value)}
            placeholder="What this profile is for"
          />
        </Field>
      </div>
      <div className="mt-3 flex flex-wrap items-center gap-5">
        <CheckboxField
          label="default for new sandboxes"
          hint="Applied automatically unless a sandbox opts out."
          checked={isDefault}
          onChange={setIsDefault}
        />
        <CheckboxField
          label="global (all sandboxes)"
          hint="Rules apply everywhere, including future sandboxes."
          checked={isGlobal}
          onChange={setIsGlobal}
        />
        <Button
          variant="primary"
          className="ml-auto"
          disabled={!name.trim()}
          loading={busy}
          onClick={() => {
            onSubmit({ name: name.trim(), description: description.trim(), is_default: isDefault, is_global: isGlobal });
            setName("");
            setDescription("");
            setIsDefault(false);
            setIsGlobal(false);
          }}
        >
          <IconPlus /> Create
        </Button>
      </div>
    </Panel>
  );
}

function GeneralSection({
  profile,
  name,
  description,
  isDefault,
  isGlobal,
  setName,
  setDescription,
  setIsDefault,
  setIsGlobal,
}: {
  profile: ProfileView;
  name: string;
  description: string;
  isDefault: boolean;
  isGlobal: boolean;
  setName: (value: string) => void;
  setDescription: (value: string) => void;
  setIsDefault: (value: boolean) => void;
  setIsGlobal: (value: boolean) => void;
}) {
  const sandboxes = profile.sandboxes ?? [];
  return (
    <div className="flex flex-col gap-4">
      <Field label="Name">
        <Input value={name} onChange={(event) => setName(event.target.value)} />
      </Field>
      <Field label="Description">
        <Input value={description} onChange={(event) => setDescription(event.target.value)} />
      </Field>
      <CheckboxField label="default for new sandboxes" hint="Applied automatically unless a sandbox opts out." checked={isDefault} onChange={setIsDefault} />
      <CheckboxField label="global (all sandboxes)" hint="Rules apply everywhere, including future sandboxes." checked={isGlobal} onChange={setIsGlobal} />
      <div className="border-t border-border pt-3">
        <h3 className="mb-2 text-xs font-semibold tracking-wide text-faint uppercase">Assigned sandboxes</h3>
        {sandboxes.length === 0 ? (
          <p className="text-sm text-muted">Not assigned. Assign it from a sandbox’s Profiles tab.</p>
        ) : (
          <div className="flex flex-wrap gap-1.5">
            {sandboxes.map((sandbox) => (
              <Link
                key={sandbox}
                to={`/sandboxes/${encodeURIComponent(sandbox)}?tab=profiles`}
                className="rounded-sm border border-border bg-canvas px-2 py-1 font-mono text-xs hover:border-accent hover:text-accent"
              >
                {sandbox}
              </Link>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

function RulesSection({
  profile,
  addRule,
  removeRule,
}: {
  profile: ProfileView;
  addRule: {
    isPending: boolean;
    mutate: (variables: { profileId: number; decision: string; pattern: string }) => void;
  };
  removeRule: {
    isPending: boolean;
    variables?: { profileId: number; ruleId: number };
    mutate: (variables: { profileId: number; ruleId: number }) => void;
  };
}) {
  const rules = profile.rules ?? [];
  return (
    <div className="flex flex-col">
      <p className="mb-3 text-sm text-muted">
        Allow/deny patterns compiled into sandbox policy rules. Global profiles apply everywhere, including future sandboxes.
      </p>
      <TableWrap className="rounded-sm border border-border">
        <thead>
          <tr>
            <TH>Decision</TH>
            <TH>Pattern</TH>
            <TH className="w-24" />
          </tr>
        </thead>
        <tbody>
          {rules.length === 0 ? (
            <TRow>
              <TD colSpan={3} className="text-muted">
                No rules yet.
              </TD>
            </TRow>
          ) : (
            rules.map((rule) => (
              <TRow key={rule.id}>
                <TD>
                  <DecisionBadge decision={rule.decision} />
                </TD>
                <TD className="font-mono text-xs">{rule.pattern}</TD>
                <TD className="text-right">
                  <Button
                    size="sm"
                    variant="ghost"
                    loading={removeRule.isPending && removeRule.variables?.ruleId === rule.id}
                    onClick={() => removeRule.mutate({ profileId: profile.id, ruleId: rule.id })}
                  >
                    Remove
                  </Button>
                </TD>
              </TRow>
            ))
          )}
        </tbody>
      </TableWrap>
      <AddRuleRow busy={addRule.isPending} onAdd={(decision, pattern) => addRule.mutate({ profileId: profile.id, decision, pattern })} />
    </div>
  );
}

function AddRuleRow({ busy, onAdd }: { busy: boolean; onAdd: (decision: string, pattern: string) => void }) {
  const [decision, setDecision] = useState("allow");
  const [pattern, setPattern] = useState("");
  return (
    <div className="mt-4 flex flex-wrap items-center gap-2">
      <Select value={decision} onChange={(event) => setDecision(event.target.value)} className="w-28">
        <option value="allow">allow</option>
        <option value="deny">deny</option>
      </Select>
      <Input
        value={pattern}
        onChange={(event) => setPattern(event.target.value)}
        placeholder="*.example.com or example.com:443"
        className="max-w-md flex-1 font-mono text-xs"
        onKeyDown={(event) => {
          if (event.key === "Enter" && pattern.trim()) {
            onAdd(decision, pattern.trim());
            setPattern("");
          }
        }}
      />
      <Button
        disabled={!pattern.trim()}
        loading={busy}
        onClick={() => {
          onAdd(decision, pattern.trim());
          setPattern("");
        }}
      >
        Add rule
      </Button>
    </div>
  );
}

function SkillsSection({
  profile,
  catalog,
  add,
  remove,
  addBusy,
  removeBusyId,
}: {
  profile: ProfileView;
  catalog: SkillItem[];
  add: (itemId: number) => void;
  remove: (itemId: number) => void;
  addBusy: boolean;
  removeBusyId: number | null;
}) {
  const current = profile.items ?? [];
  const currentIds = new Set(current.map((item) => item.id));
  const available = catalog.filter((item) => !currentIds.has(item.id));
  const groups = skillMenuGroups(available);

  return (
    <div className="flex flex-col">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm text-muted">
          Skills and commands mounted read-only at ~/.agents in every sandbox this profile is assigned to.
        </p>
        <Link to="/skills" className="text-xs text-accent hover:underline">
          Manage stores
        </Link>
      </div>
      {current.length === 0 ? (
        <p className="mb-3 text-sm text-muted">Nothing selected.</p>
      ) : (
        <TableWrap className="rounded-sm border border-border">
          <thead>
            <tr>
              <TH>Kind</TH>
              <TH>Name</TH>
              <TH>Store</TH>
              <TH>Plugin</TH>
              <TH className="w-24" />
            </tr>
          </thead>
          <tbody>
            {current.map((item) => (
              <TRow key={item.id}>
                <TD>
                  <KindBadge kind={item.kind} />
                </TD>
                <TD className="font-medium">{item.name}</TD>
                <TD className="text-muted">{item.store_name}</TD>
                <TD className="text-muted">{item.plugin || "—"}</TD>
                <TD className="text-right">
                  <Button size="sm" variant="ghost" loading={removeBusyId === item.id} onClick={() => remove(item.id)}>
                    Remove
                  </Button>
                </TD>
              </TRow>
            ))}
          </tbody>
        </TableWrap>
      )}
      <div className="mt-4">
        {available.length === 0 ? (
          <p className="text-sm text-muted">Every discovered item is already selected.</p>
        ) : (
          <MenuSelect
            groups={groups}
            onChange={(itemId) => add(Number(itemId))}
            placeholder="Choose a skill or command…"
            searchPlaceholder="Search skills and commands…"
            disabled={addBusy}
            className="max-w-md"
          />
        )}
      </div>
    </div>
  );
}

function MountsSection({
  profile,
  add,
  remove,
  addBusy,
  removeBusyId,
}: {
  profile: ProfileView;
  add: (body: MountInput) => void;
  remove: (mountId: number) => void;
  addBusy: boolean;
  removeBusyId: number | null;
}) {
  const toast = useToasts();
  const [host, setHost] = useState("");
  const [target, setTarget] = useState("");
  const [readOnly, setReadOnly] = useState(false);
  const mounts = profile.mounts ?? [];

  async function pickFolder() {
    try {
      const picked = await api.fsPick(host || undefined);
      if (picked.path) setHost(picked.path);
    } catch (error) {
      toast.push({ tone: "warning", title: "Folder picker unavailable", body: errorMessage(error) });
    }
  }

  return (
    <div className="flex flex-col">
      <p className="mb-3 text-sm text-muted">
        Bind mounts applied by default to every sandbox this profile is assigned to (re-applied whenever the sandbox runs).
      </p>
      {mounts.length === 0 ? (
        <p className="mb-3 text-sm text-muted">No default mount.</p>
      ) : (
        <TableWrap className="rounded-sm border border-border">
          <thead>
            <tr>
              <TH>Host path</TH>
              <TH>Sandbox target</TH>
              <TH>Mode</TH>
              <TH className="w-24" />
            </tr>
          </thead>
          <tbody>
            {mounts.map((mount: ProfileMount) => (
              <TRow key={mount.id}>
                <TD className="font-mono text-xs">{mount.host_path}</TD>
                <TD className="font-mono text-xs">{mount.target_path || mount.host_path}</TD>
                <TD>{mount.read_only ? <Badge tone="warning">ro</Badge> : <Badge>rw</Badge>}</TD>
                <TD className="text-right">
                  <Button
                    size="sm"
                    variant="ghost"
                    loading={removeBusyId === mount.id}
                    onClick={() => remove(mount.id)}
                  >
                    Remove
                  </Button>
                </TD>
              </TRow>
            ))}
          </tbody>
        </TableWrap>
      )}
      <div className="mt-4 grid gap-3 sm:grid-cols-2">
        <Field label="Host path">
          <div className="flex items-center gap-2">
            <Input
              value={host}
              onChange={(event) => setHost(event.target.value)}
              placeholder="/absolute/host/path"
              className="flex-1 font-mono text-xs"
            />
            <Button size="icon" variant="ghost" title="Choose folder…" onClick={() => void pickFolder()}>
              <IconFolder className="size-3.5" />
            </Button>
          </div>
        </Field>
        <Field label="Sandbox target (optional)" hint="Defaults to the same path as the host.">
          <Input
            value={target}
            onChange={(event) => setTarget(event.target.value)}
            placeholder="/absolute/sandbox/path"
            className="font-mono text-xs"
          />
        </Field>
      </div>
      <div className="mt-3 flex items-center gap-4">
        <CheckboxField label="read only" checked={readOnly} onChange={setReadOnly} />
        <Button
          variant="primary"
          disabled={!host.trim()}
          loading={addBusy}
          onClick={() => {
            add({ host_path: host.trim(), target_path: target.trim(), read_only: readOnly });
            setHost("");
            setTarget("");
            setReadOnly(false);
          }}
        >
          <IconPlus /> Add mount
        </Button>
      </div>
    </div>
  );
}

function CachesSection({
  profile,
  caches,
  add,
  remove,
  addBusy,
  removeBusyId,
}: {
  profile: ProfileView;
  caches: CacheMount[];
  add: (cacheId: number) => void;
  remove: (cacheId: number) => void;
  addBusy: boolean;
  removeBusyId: number | null;
}) {
  const [selected, setSelected] = useState("");
  const current = profile.caches ?? [];
  const currentIds = new Set(current.map((cache) => cache.id));
  const available = caches.filter((cache) => !currentIds.has(cache.id) && cache.enabled);

  return (
    <div className="flex flex-col">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm text-muted">Caches from Settings attached by default to every sandbox this profile is assigned to.</p>
        <Link to="/settings" className="text-xs text-accent hover:underline">
          Configure caches
        </Link>
      </div>
      {current.length === 0 ? (
        <p className="mb-3 text-sm text-muted">No default cache.</p>
      ) : (
        <TableWrap className="rounded-sm border border-border">
          <thead>
            <tr>
              <TH>Cache</TH>
              <TH>Host → sandbox</TH>
              <TH>Mode</TH>
              <TH className="w-24" />
            </tr>
          </thead>
          <tbody>
            {current.map((cache) => (
              <TRow key={cache.id}>
                <TD className="font-medium">{cache.name}</TD>
                <TD className="font-mono text-xs">
                  {cache.host_path}
                  <span className="text-faint"> → </span>
                  {cache.target_path}
                </TD>
                <TD>{cache.read_only ? <Badge tone="warning">ro</Badge> : <Badge>rw</Badge>}</TD>
                <TD className="text-right">
                  <Button
                    size="sm"
                    variant="ghost"
                    loading={removeBusyId === cache.id}
                    onClick={() => remove(cache.id)}
                  >
                    Remove
                  </Button>
                </TD>
              </TRow>
            ))}
          </tbody>
        </TableWrap>
      )}
      <div className="mt-4">
        {available.length === 0 ? (
          <p className="text-sm text-muted">Every configured cache is already selected.</p>
        ) : (
          <div className="flex items-center gap-2">
            <Select value={selected} onChange={(event) => setSelected(event.target.value)} className="max-w-xs">
              <option value="">Choose a cache…</option>
              {available.map((cache) => (
                <option key={cache.id} value={cache.id}>
                  {cache.name} ({cache.host_path})
                </option>
              ))}
            </Select>
            <Button
              variant="primary"
              disabled={!selected}
              loading={addBusy}
              onClick={() => {
                add(Number(selected));
                setSelected("");
              }}
            >
              Add cache
            </Button>
          </div>
        )}
      </div>
    </div>
  );
}

function ProfileDialog({
  profile,
  caches,
  catalog,
  busy,
  onClose,
  onSave,
  addRule,
  removeRule,
  addItem,
  removeItem,
  addMount,
  removeMount,
  addCache,
  removeCache,
}: {
  profile: ProfileView | null;
  caches: CacheMount[];
  catalog: SkillItem[];
  busy: boolean;
  onClose: () => void;
  onSave: (body: ProfileBody) => void;
  addRule: { isPending: boolean; mutate: (variables: { profileId: number; decision: string; pattern: string }) => void };
  removeRule: {
    isPending: boolean;
    variables?: { profileId: number; ruleId: number };
    mutate: (variables: { profileId: number; ruleId: number }) => void;
  };
  addItem: { isPending: boolean; mutate: (variables: { profileId: number; itemId: number }) => void };
  removeItem: {
    isPending: boolean;
    variables?: { profileId: number; itemId: number };
    mutate: (variables: { profileId: number; itemId: number }) => void;
  };
  addMount: { isPending: boolean; mutate: (variables: { profileId: number; body: MountInput }) => void };
  removeMount: {
    isPending: boolean;
    variables?: { profileId: number; mountId: number };
    mutate: (variables: { profileId: number; mountId: number }) => void;
  };
  addCache: { isPending: boolean; mutate: (variables: { profileId: number; cacheId: number }) => void };
  removeCache: {
    isPending: boolean;
    variables?: { profileId: number; cacheId: number };
    mutate: (variables: { profileId: number; cacheId: number }) => void;
  };
}) {
  const [tab, setTab] = useState("general");
  const [name, setName] = useState(profile?.name ?? "");
  const [description, setDescription] = useState(profile?.description ?? "");
  const [isDefault, setIsDefault] = useState(profile?.is_default ?? false);
  const [isGlobal, setIsGlobal] = useState(profile?.is_global ?? false);
  const [loadedId, setLoadedId] = useState(profile?.id ?? -1);

  if (profile && profile.id !== loadedId) {
    setLoadedId(profile.id);
    setTab("general");
    setName(profile.name);
    setDescription(profile.description);
    setIsDefault(profile.is_default);
    setIsGlobal(profile.is_global);
  }
  if (!profile && loadedId !== -1) {
    setLoadedId(-1);
  }

  return (
    <Modal
      open={profile !== null}
      onClose={onClose}
      title={`Edit ${profile?.name ?? "profile"}`}
      description={profile?.description || undefined}
      size="lg"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Close
          </Button>
          <Button
            variant="primary"
            loading={busy}
            disabled={!name.trim()}
            onClick={() => onSave({ name: name.trim(), description: description.trim(), is_default: isDefault, is_global: isGlobal })}
          >
            Save
          </Button>
        </>
      }
    >
      {profile ? (
        <div className="flex flex-col gap-4">
          <Tabs
            tabs={[
              { id: "general", label: "General" },
              { id: "rules", label: "Rules", count: (profile.rules ?? []).length },
              { id: "skills", label: "Skills", count: (profile.items ?? []).length },
              { id: "mounts", label: "Mounts", count: (profile.mounts ?? []).length },
              { id: "caches", label: "Caches", count: (profile.caches ?? []).length },
            ]}
            active={tab}
            onChange={setTab}
          />
          {tab === "general" ? (
            <GeneralSection
              profile={profile}
              name={name}
              description={description}
              isDefault={isDefault}
              isGlobal={isGlobal}
              setName={setName}
              setDescription={setDescription}
              setIsDefault={setIsDefault}
              setIsGlobal={setIsGlobal}
            />
          ) : null}
          {tab === "rules" ? <RulesSection profile={profile} addRule={addRule} removeRule={removeRule} /> : null}
          {tab === "skills" ? (
            <SkillsSection
              profile={profile}
              catalog={catalog}
              add={(itemId) => addItem.mutate({ profileId: profile.id, itemId })}
              remove={(itemId) => removeItem.mutate({ profileId: profile.id, itemId })}
              addBusy={addItem.isPending}
              removeBusyId={removeItem.isPending ? removeItem.variables?.itemId ?? null : null}
            />
          ) : null}
          {tab === "mounts" ? (
            <MountsSection
              profile={profile}
              add={(body) => addMount.mutate({ profileId: profile.id, body })}
              remove={(mountId) => removeMount.mutate({ profileId: profile.id, mountId })}
              addBusy={addMount.isPending}
              removeBusyId={removeMount.isPending ? removeMount.variables?.mountId ?? null : null}
            />
          ) : null}
          {tab === "caches" ? (
            <CachesSection
              profile={profile}
              caches={caches}
              add={(cacheId) => addCache.mutate({ profileId: profile.id, cacheId })}
              remove={(cacheId) => removeCache.mutate({ profileId: profile.id, cacheId })}
              addBusy={addCache.isPending}
              removeBusyId={removeCache.isPending ? removeCache.variables?.cacheId ?? null : null}
            />
          ) : null}
        </div>
      ) : null}
    </Modal>
  );
}
