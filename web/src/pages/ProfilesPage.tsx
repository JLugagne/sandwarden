import { useState } from "react";
import { Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import { useApiMutation } from "@/hooks/useApiMutation";
import { queryKeys } from "@/store/realtime";
import {
  Badge,
  Button,
  CheckboxField,
  ConfirmDialog,
  DecisionBadge,
  EmptyState,
  Field,
  IconPlus,
  Input,
  Modal,
  PageHeader,
  Panel,
  Select,
  Spinner,
  TableWrap,
  TD,
  TH,
  TRow,
} from "@/components/ui";
import { KindBadge } from "@/pages/SkillsPage";
import type { ProfileView, SkillItem } from "@/types";

export function ProfilesPage() {
  const [editing, setEditing] = useState<ProfileView | null>(null);
  const [deleting, setDeleting] = useState<ProfileView | null>(null);

  const profiles = useQuery({ queryKey: queryKeys.profiles, queryFn: api.profiles });
  const catalog = useQuery({ queryKey: queryKeys.skillItems, queryFn: () => api.skillItems() });

  const create = useApiMutation({
    mutationFn: api.createProfile,
    success: (profile) => `Profile ${profile.name} created`,
  });
  const update = useApiMutation({
    mutationFn: ({ id, body }: { id: number; body: Parameters<typeof api.updateProfile>[1] }) =>
      api.updateProfile(id, body),
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

  const rows = profiles.data ?? [];

  return (
    <>
      <PageHeader
        title="Profiles"
        subtitle="Reusable allow/deny bundles compiled into sandbox policy rules."
      />

      <div className="flex flex-col gap-4">
        <NewProfileForm
          onSubmit={(body) => create.mutate(body)}
          busy={create.isPending}
        />

        <ErrorNoteIfAny error={profiles.error} />
        {profiles.isLoading ? (
          <div className="flex justify-center py-10">
            <Spinner />
          </div>
        ) : rows.length === 0 ? (
          <EmptyState title="No profiles yet" description="Create one above, add rules, then assign it from a sandbox." />
        ) : (
          rows.map((profile) => (
            <Panel
              key={profile.id}
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
                  <Button size="sm" variant="ghost" onClick={() => setEditing(profile)}>
                    Edit
                  </Button>
                  <Button size="sm" variant="ghost" onClick={() => setDeleting(profile)}>
                    <span className="text-danger">Delete</span>
                  </Button>
                </>
              }
              bodyClassName="p-0"
            >
              <TableWrap className="border-0 border-b border-border">
                <thead>
                  <tr>
                    <TH>Decision</TH>
                    <TH>Pattern</TH>
                    <TH className="w-24" />
                  </tr>
                </thead>
                <tbody>
                  {(profile.rules ?? []).length === 0 ? (
                    <TRow>
                      <TD colSpan={3} className="text-muted">
                        No rules yet.
                      </TD>
                    </TRow>
                  ) : (
                    (profile.rules ?? []).map((rule) => (
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
              <AddRuleRow
                busy={addRule.isPending}
                onAdd={(decision, pattern) => addRule.mutate({ profileId: profile.id, decision, pattern })}
              />
              <ProfileItemsSection
                profile={profile}
                catalog={catalog.data ?? []}
                add={(itemId) => addItem.mutate({ profileId: profile.id, itemId })}
                remove={(itemId) => removeItem.mutate({ profileId: profile.id, itemId })}
                addBusy={addItem.isPending}
                removeBusyId={removeItem.isPending ? (removeItem.variables?.itemId ?? null) : null}
              />
              <div className="border-t border-border p-4">
                <h3 className="mb-2 text-xs font-semibold tracking-wide text-faint uppercase">Assigned sandboxes</h3>
                {(profile.sandboxes ?? []).length === 0 ? (
                  <p className="text-sm text-muted">Not assigned. Assign it from a sandbox’s Profiles tab.</p>
                ) : (
                  <div className="flex flex-wrap gap-1.5">
                    {(profile.sandboxes ?? []).map((sandbox) => (
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
          ))
        )}
      </div>

      <ProfileDialog
        profile={editing}
        busy={update.isPending}
        onClose={() => setEditing(null)}
        onSave={(body) => editing && update.mutate({ id: editing.id, body })}
      />
      <ConfirmDialog
        open={deleting !== null}
        title={`Delete profile ${deleting?.name ?? ""}?`}
        body="Its rules are removed from every sandbox it is assigned to."
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

function NewProfileForm({
  onSubmit,
  busy,
}: {
  onSubmit: (body: { name: string; description: string; is_default: boolean; is_global: boolean }) => void;
  busy: boolean;
}) {
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

function AddRuleRow({ busy, onAdd }: { busy: boolean; onAdd: (decision: string, pattern: string) => void }) {
  const [decision, setDecision] = useState("allow");
  const [pattern, setPattern] = useState("");
  return (
    <div className="flex flex-wrap items-center gap-2 border-t border-border p-4">
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

function ProfileItemsSection({
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
  const [selected, setSelected] = useState("");
  const current = profile.items ?? [];
  const currentIds = new Set(current.map((item) => item.id));
  const available = catalog.filter((item) => !currentIds.has(item.id));

  return (
    <div className="border-t border-border">
      <div className="flex flex-wrap items-center justify-between gap-2 p-4 pb-2">
        <h3 className="text-xs font-semibold tracking-wide text-faint uppercase">Skills & commands</h3>
        <Link to="/skills" className="text-xs text-accent hover:underline">
          Manage stores
        </Link>
      </div>
      {current.length === 0 ? (
        <p className="px-4 pb-2 text-sm text-muted">
          Nothing selected. Items are mounted read-only at ~/.agents in every sandbox this profile is
          assigned to.
        </p>
      ) : (
        <TableWrap className="border-0 border-b border-border">
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
                  <Button
                    size="sm"
                    variant="ghost"
                    loading={removeBusyId === item.id}
                    onClick={() => remove(item.id)}
                  >
                    Remove
                  </Button>
                </TD>
              </TRow>
            ))}
          </tbody>
        </TableWrap>
      )}
      <div className="flex flex-wrap items-center gap-2 p-4">
        <Select value={selected} onChange={(event) => setSelected(event.target.value)} className="max-w-md">
          <option value="">Choose a skill or command…</option>
          {available.map((item) => (
            <option key={item.id} value={item.id}>
              {item.store_name} · {item.kind === "command" ? "/" : ""}
              {item.name}
            </option>
          ))}
        </Select>
        <Button
          disabled={!selected}
          loading={addBusy}
          onClick={() => {
            add(Number(selected));
            setSelected("");
          }}
        >
          Add
        </Button>
      </div>
    </div>
  );
}

function ProfileDialog({
  profile,
  busy,
  onSave,
  onClose,
}: {
  profile: ProfileView | null;
  busy: boolean;
  onSave: (body: { name: string; description: string; is_default: boolean; is_global: boolean }) => void;
  onClose: () => void;
}) {
  const [name, setName] = useState(profile?.name ?? "");
  const [description, setDescription] = useState(profile?.description ?? "");
  const [isDefault, setIsDefault] = useState(profile?.is_default ?? false);
  const [isGlobal, setIsGlobal] = useState(profile?.is_global ?? false);
  const [loadedId, setLoadedId] = useState(profile?.id ?? -1);

  if (profile && profile.id !== loadedId) {
    setLoadedId(profile.id);
    setName(profile.name);
    setDescription(profile.description);
    setIsDefault(profile.is_default);
    setIsGlobal(profile.is_global);
  }

  return (
    <Modal
      open={profile !== null}
      onClose={onClose}
      title={`Edit ${profile?.name ?? "profile"}`}
      size="sm"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
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
      <div className="flex flex-col gap-4">
        <Field label="Name">
          <Input value={name} onChange={(event) => setName(event.target.value)} />
        </Field>
        <Field label="Description">
          <Input value={description} onChange={(event) => setDescription(event.target.value)} />
        </Field>
        <CheckboxField label="default for new sandboxes" checked={isDefault} onChange={setIsDefault} />
        <CheckboxField label="global (all sandboxes)" checked={isGlobal} onChange={setIsGlobal} />
      </div>
    </Modal>
  );
}
