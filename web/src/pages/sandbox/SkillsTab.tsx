import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { api } from "@/api/client";
import { useApiMutation } from "@/hooks/useApiMutation";
import { queryKeys } from "@/store/realtime";
import {
  Badge,
  Button,
  ConfirmDialog,
  EmptyState,
  MenuSelect,
  Panel,
  Spinner,
  TableWrap,
  TD,
  TH,
  TRow,
} from "@/components/ui";
import { skillMenuGroups } from "@/lib/catalog";
import { useToasts } from "@/components/Toaster";
import { KindBadge } from "@/pages/SkillsPage";
import type { SandboxDetail, SandboxSkill } from "@/types";

export function SkillsTab({ name, detail }: { name: string; detail: SandboxDetail }) {
  const items = useQuery({ queryKey: queryKeys.skillItems, queryFn: () => api.skillItems() });
  const [detaching, setDetaching] = useState<SandboxSkill | null>(null);
  const toast = useToasts();

  const attach = useApiMutation({
    mutationFn: (itemId: number) => api.attachSkillItem(name, itemId),
    success: "Attached",
  });
  const detach = useApiMutation({
    mutationFn: (itemId: number) => api.detachSkillItem(name, itemId),
    success: "Detached",
    onSuccess: () => setDetaching(null),
  });
  const reapply = useApiMutation({
    mutationFn: () => api.reconcileSkills(name),
    success: (result) => {
      if (result.errors.length > 0) return undefined;
      const changed = result.applied + result.removed;
      return changed > 0 ? `Applied ${result.applied}, removed ${result.removed}` : "Everything is already mounted";
    },
    onSuccess: (result) => {
      if (result.errors.length > 0) {
        toast.push({ tone: "danger", title: "Skill reconcile failed", body: result.errors.join("; ") });
      }
    },
  });

  const skills = detail.skills ?? [];
  const desiredIds = new Set(skills.filter((skill) => !skill.orphan).map((skill) => skill.id));
  const available = (items.data ?? []).filter((item) => !desiredIds.has(item.id));
  const groups = skillMenuGroups(available);
  const drift = skills.filter((skill) => !skill.orphan && !skill.missing && !skill.conflict && !skill.mounted);

  return (
    <div className="flex flex-col gap-4">
      <Panel
        title="Skills and commands"
        description="Mounted read-only from skill stores: skills at /home/agent/.agents/skills/<name>, commands at /home/agent/.agents/commands/<name>.md. Profile items come from the profiles assigned to this sandbox."
        actions={
          <>
            <Button
              size="sm"
              variant="ghost"
              disabled={!detail.sandbox.running}
              loading={reapply.isPending}
              onClick={() => reapply.mutate()}
            >
              Reapply
            </Button>
            <Link to="/skills" className="text-xs text-accent hover:underline">
              Manage stores
            </Link>
          </>
        }
        bodyClassName="p-0"
      >
        {!detail.sandbox.running ? (
          <p className="border-b border-border p-4 text-xs text-warning">
            The sandbox is stopped. Start it to mount skills; desired items are re-applied automatically
            on start.
          </p>
        ) : drift.length > 0 ? (
          <p className="border-b border-border p-4 text-xs text-warning">
            {drift.length} item(s) are selected but not mounted. Use Reapply.
          </p>
        ) : null}

        {skills.length === 0 ? (
          <p className="p-4 text-sm text-muted">
            No skill selected. Add them to a profile, or attach one directly below.
          </p>
        ) : (
          <TableWrap className="border-0">
            <thead>
              <tr>
                <TH>Kind</TH>
                <TH>Name</TH>
                <TH>Store</TH>
                <TH>Sources</TH>
                <TH>Target</TH>
                <TH>State</TH>
                <TH className="w-24" />
              </tr>
            </thead>
            <tbody>
              {skills.map((skill) => (
                <TRow key={skill.orphan ? `orphan:${skill.target}` : `item:${skill.id}`}>
                  <TD>{skill.orphan ? <Badge tone="warning">manual</Badge> : <KindBadge kind={skill.kind} />}</TD>
                  <TD className="font-medium">{skill.orphan ? "—" : skill.name}</TD>
                  <TD className="text-muted">{skill.orphan ? "—" : skill.store_name || "—"}</TD>
                  <TD className="text-muted">{(skill.sources ?? []).join(", ") || "—"}</TD>
                  <TD className="font-mono text-xs">{skill.target}</TD>
                  <TD>
                    <SkillState skill={skill} />
                  </TD>
                  <TD className="text-right">
                    {!skill.orphan && (skill.sources ?? []).includes("sandbox") ? (
                      <Button size="sm" variant="ghost" onClick={() => setDetaching(skill)}>
                        Detach
                      </Button>
                    ) : null}
                  </TD>
                </TRow>
              ))}
            </tbody>
          </TableWrap>
        )}
      </Panel>

      <Panel
        title="Attach directly"
        description="Extra items for this sandbox only, on top of what the profiles provide. Profile items cannot be detached here."
      >
        {items.isLoading ? (
          <Spinner />
        ) : available.length === 0 ? (
          <EmptyState
            title="No item available to attach."
            description="Add a skill store first, or every discovered item is already selected."
          />
        ) : (
          <MenuSelect
            groups={groups}
            onChange={(itemId) => attach.mutate(Number(itemId))}
            placeholder="Choose a skill or command…"
            searchPlaceholder="Search skills and commands…"
            disabled={!detail.sandbox.running || attach.isPending}
            className="max-w-md"
          />
        )}
      </Panel>

      <ConfirmDialog
        open={detaching !== null}
        title={`Detach ${detaching?.name ?? "item"}?`}
        body="The direct selection is removed and the mount released when the sandbox is running. If a profile still selects it, it stays mounted."
        confirmLabel="Detach"
        busy={detach.isPending}
        onConfirm={() => detaching && detach.mutate(detaching.id)}
        onClose={() => setDetaching(null)}
      />
    </div>
  );
}

function SkillState({ skill }: { skill: SandboxSkill }) {
  if (skill.conflict) {
    return (
      <span title={`Target already provided by store ${skill.conflict}`}>
        <Badge tone="danger" dot>
          conflict
        </Badge>
      </span>
    );
  }
  if (skill.missing) {
    return (
      <span title="The source folder is missing from the store checkout. Refresh the store.">
        <Badge tone="danger" dot>
          missing source
        </Badge>
      </span>
    );
  }
  if (skill.orphan) {
    return <Badge tone="neutral">mounted outside selection</Badge>;
  }
  if (skill.mounted) {
    return (
      <Badge tone="success" dot>
        mounted
      </Badge>
    );
  }
  return (
    <Badge tone="warning" dot>
      not mounted
    </Badge>
  );
}
