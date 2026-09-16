import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { api } from "@/api/client";
import { useApiMutation } from "@/hooks/useApiMutation";
import { queryKeys } from "@/store/realtime";
import {
  Badge,
  Button,
  ErrorNote,
  IconChevronRight,
  IconPlay,
  IconStop,
  IconTrash,
  PageHeader,
  Spinner,
  StatusBadge,
  Tabs,
} from "@/components/ui";
import { ConfirmDialog } from "@/components/ui";
import { CachesTab } from "@/pages/sandbox/CachesTab";
import { MountsTab } from "@/pages/sandbox/MountsTab";
import { OverviewTab } from "@/pages/sandbox/OverviewTab";
import { ProfilesTab } from "@/pages/sandbox/ProfilesTab";
import { SecretsTab } from "@/pages/sandbox/SecretsTab";
import { SkillsTab } from "@/pages/sandbox/SkillsTab";
import { TerminalTab } from "@/pages/sandbox/TerminalTab";
import { TrafficTab } from "@/pages/sandbox/TrafficTab";
import { useState } from "react";

const TABS = ["overview", "mounts", "caches", "skills", "profiles", "secrets", "traffic", "terminal"] as const;
type TabId = (typeof TABS)[number];

export function SandboxDetailPage() {
  const { name = "" } = useParams();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const [confirming, setConfirming] = useState(false);

  const rawTab = searchParams.get("tab") ?? "overview";
  const tab: TabId = (TABS as readonly string[]).includes(rawTab) ? (rawTab as TabId) : "overview";

  const detail = useQuery({
    queryKey: queryKeys.sandbox(name),
    queryFn: () => api.sandbox(name),
    enabled: name !== "",
    retry: 0,
  });

  const start = useApiMutation({ mutationFn: () => api.startSandbox(name), success: `Starting ${name}` });
  const stop = useApiMutation({ mutationFn: () => api.stopSandbox(name), success: `Stopping ${name}` });
  const remove = useApiMutation({
    mutationFn: () => api.deleteSandbox(name),
    success: `Deleted ${name}`,
    onSuccess: () => navigate("/"),
  });

  if (detail.isLoading) {
    return (
      <div className="flex justify-center py-24">
        <Spinner />
      </div>
    );
  }

  if (detail.isError || !detail.data) {
    return (
      <>
        <PageHeader
          title={name || "Sandbox"}
          breadcrumb={
            <Link to="/" className="hover:text-fg">
              Sandboxes
            </Link>
          }
        />
        <ErrorNote error={detail.error ?? new Error("sandbox not found")} />
      </>
    );
  }

  const data = detail.data;
  const sandbox = data.sandbox;
  const counts: Partial<Record<TabId, number>> = {
    mounts: data.mounts?.length ?? 0,
    caches: data.caches?.length ?? 0,
    skills: data.skills?.length ?? 0,
    profiles: data.profiles?.length ?? 0,
    secrets: (data.secrets?.length ?? 0) + (data.custom_secrets?.length ?? 0),
    traffic: data.policy_rules?.length ?? 0,
  };

  return (
    <>
      <PageHeader
        breadcrumb={
          <span className="inline-flex items-center gap-1">
            <Link to="/" className="hover:text-fg">
              Sandboxes
            </Link>
            <IconChevronRight className="size-3" />
            <span className="font-mono">{sandbox.name}</span>
          </span>
        }
        title={
          <span className="flex flex-wrap items-center gap-3">
            {sandbox.name}
            <StatusBadge running={sandbox.running} status={sandbox.status} />
            {sandbox.agent ? <Badge>{sandbox.agent}</Badge> : null}
            {sandbox.mount_policy_denied ? <Badge tone="danger">mount denied</Badge> : null}
          </span>
        }
        actions={
          <>
            {sandbox.running ? (
              <Button size="md" onClick={() => stop.mutate()} loading={stop.isPending}>
                <IconStop className="size-3.5" /> Stop
              </Button>
            ) : (
              <Button size="md" onClick={() => start.mutate()} loading={start.isPending}>
                <IconPlay className="size-3.5" /> Start
              </Button>
            )}
            <Button size="md" variant="danger" onClick={() => setConfirming(true)}>
              <IconTrash className="size-3.5" /> Delete
            </Button>
          </>
        }
      />

      <Tabs
        tabs={[
          { id: "overview", label: "Overview" },
          { id: "mounts", label: "Mounts", count: counts.mounts },
          { id: "caches", label: "Caches", count: counts.caches },
          { id: "skills", label: "Skills", count: counts.skills },
          { id: "profiles", label: "Profiles", count: counts.profiles },
          { id: "secrets", label: "Secrets", count: counts.secrets },
          { id: "traffic", label: "Traffic", count: counts.traffic },
          { id: "terminal", label: "Terminal" },
        ]}
        active={tab}
        onChange={(next) => setSearchParams(next === "overview" ? {} : { tab: next }, { replace: true })}
        className="mb-4"
      />

      {tab === "overview" ? <OverviewTab detail={data} /> : null}
      {tab === "mounts" ? <MountsTab name={sandbox.name} detail={data} /> : null}
      {tab === "caches" ? <CachesTab name={sandbox.name} detail={data} /> : null}
      {tab === "skills" ? <SkillsTab name={sandbox.name} detail={data} /> : null}
      {tab === "profiles" ? <ProfilesTab name={sandbox.name} detail={data} /> : null}
      {tab === "secrets" ? <SecretsTab name={sandbox.name} detail={data} /> : null}
      {tab === "traffic" ? <TrafficTab name={sandbox.name} /> : null}
      {tab === "terminal" ? <TerminalTab name={sandbox.name} running={sandbox.running} /> : null}

      <ConfirmDialog
        open={confirming}
        title={`Delete ${sandbox.name}?`}
        body="This removes the sandbox and its runtime; assigned profiles are unapplied first."
        confirmLabel="Delete"
        busy={remove.isPending}
        onConfirm={() => remove.mutate()}
        onClose={() => setConfirming(false)}
      />
    </>
  );
}
