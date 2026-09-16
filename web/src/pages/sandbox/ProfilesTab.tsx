import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { api } from "@/api/client";
import { useApiMutation } from "@/hooks/useApiMutation";
import { queryKeys } from "@/store/realtime";
import { Badge, Button, Panel, Select, Spinner, TableWrap, TD, TH, TRow } from "@/components/ui";
import type { SandboxDetail } from "@/types";

export function ProfilesTab({ name, detail }: { name: string; detail: SandboxDetail }) {
  const profiles = useQuery({ queryKey: queryKeys.profiles, queryFn: api.profiles });
  const [selected, setSelected] = useState("");

  const assign = useApiMutation({
    mutationFn: (profileId: number) => api.assignProfile(name, profileId),
    success: "Profile assigned",
    onSuccess: () => setSelected(""),
  });
  const unassign = useApiMutation({
    mutationFn: (profileId: number) => api.unassignProfile(name, profileId),
    success: "Profile unassigned",
  });

  const assigned = detail.profiles ?? [];
  const assignedIds = new Set(assigned.map((profile) => profile.id));
  const available = (profiles.data ?? []).filter((profile) => !assignedIds.has(profile.id));

  return (
    <div className="flex flex-col gap-4">
      <Panel
        title="Assigned profiles"
        description="Applying a profile compiles its rules into sandbox-scoped policy entries."
      >
        {assigned.length === 0 ? (
          <p className="text-sm text-muted">No profile assigned.</p>
        ) : (
          <TableWrap className="border-0">
            <thead>
              <tr>
                <TH>Profile</TH>
                <TH>Flags</TH>
                <TH>Description</TH>
                <TH className="w-24" />
              </tr>
            </thead>
            <tbody>
              {assigned.map((profile) => (
                <TRow key={profile.id}>
                  <TD className="font-medium">{profile.name}</TD>
                  <TD>
                    <span className="flex gap-1.5">
                      {profile.is_default ? <Badge tone="warning">default</Badge> : null}
                      {profile.is_global ? <Badge tone="accent">global</Badge> : null}
                    </span>
                  </TD>
                  <TD className="text-muted">{profile.description || "—"}</TD>
                  <TD className="text-right">
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => unassign.mutate(profile.id)}
                      loading={unassign.isPending && unassign.variables === profile.id}
                    >
                      Unassign
                    </Button>
                  </TD>
                </TRow>
              ))}
            </tbody>
          </TableWrap>
        )}
      </Panel>

      <Panel title="Assign a profile">
        {profiles.isLoading ? (
          <Spinner />
        ) : available.length === 0 ? (
          <p className="text-sm text-muted">
            Every profile is already assigned.{" "}
            <Link to="/profiles" className="text-accent hover:underline">
              Manage profiles
            </Link>
          </p>
        ) : (
          <div className="flex items-center gap-2">
            <Select value={selected} onChange={(event) => setSelected(event.target.value)} className="max-w-xs">
              <option value="">Choose a profile…</option>
              {available.map((profile) => (
                <option key={profile.id} value={profile.id}>
                  {profile.name}
                </option>
              ))}
            </Select>
            <Button
              variant="primary"
              disabled={!selected}
              loading={assign.isPending}
              onClick={() => assign.mutate(Number(selected))}
            >
              Assign
            </Button>
          </div>
        )}
      </Panel>
    </div>
  );
}
