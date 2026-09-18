import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { api } from "@/api/client";
import { errorMessage, useApiMutation } from "@/hooks/useApiMutation";
import { useConfigStaleness } from "@/hooks/useConfigStaleness";
import { useJob } from "@/hooks/useJob";
import { jobHub, newJobId } from "@/store/jobs";
import { queryKeys } from "@/store/realtime";
import { staleNames } from "@/lib/config";
import { StaleBadge } from "@/components/StaleBadge";
import {
  Badge,
  Button,
  CheckboxField,
  ConfigViewer,
  CopyButton,
  ErrorNote,
  Field,
  IconChevronRight,
  IconPlay,
  IconPlus,
  IconRefresh,
  IconStop,
  IconTrash,
  Input,
  LogView,
  Modal,
  PageHeader,
  Panel,
  Spinner,
  StatusBadge,
  Tabs,
} from "@/components/ui";
import { ConfirmDialog } from "@/components/ui";
import { CachesTab } from "@/pages/sandbox/CachesTab";
import { CompleteConfigDialog } from "@/pages/sandbox/CompleteConfigDialog";
import { MountsTab } from "@/pages/sandbox/MountsTab";
import { OverviewTab } from "@/pages/sandbox/OverviewTab";
import { ProfilesTab } from "@/pages/sandbox/ProfilesTab";
import { SecretsTab } from "@/pages/sandbox/SecretsTab";
import { SkillsTab } from "@/pages/sandbox/SkillsTab";
import { TerminalTab } from "@/pages/sandbox/TerminalTab";
import { TrafficTab } from "@/pages/sandbox/TrafficTab";
import { useStickToBottom, useToasts } from "@/components/Toaster";
import { useEffect, useRef, useState } from "react";

const TABS = ["overview", "mounts", "caches", "skills", "profiles", "secrets", "traffic", "terminal"] as const;
type TabId = (typeof TABS)[number];

export function SandboxDetailPage() {
  const { name = "" } = useParams();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const toast = useToasts();
  const [searchParams, setSearchParams] = useSearchParams();
  const [confirming, setConfirming] = useState(false);
  const [purgeConfig, setPurgeConfig] = useState(false);
  const [viewingFiles, setViewingFiles] = useState(false);
  const [completing, setCompleting] = useState(false);
  const [recreating, setRecreating] = useState(false);
  const [attachingKit, setAttachingKit] = useState(false);
  const [savingTemplate, setSavingTemplate] = useState(false);
  const [job, setJob] = useState<{ id: string; kind: "apply" | "recreate" } | null>(null);
  const [jobBusy, setJobBusy] = useState(false);
  const reportedJob = useRef<string | null>(null);
  const jobState = useJob(job?.id ?? null);
  const jobLogRef = useStickToBottom<HTMLPreElement>(jobState.output);

  const rawTab = searchParams.get("tab") ?? "overview";
  const tab: TabId = (TABS as readonly string[]).includes(rawTab) ? (rawTab as TabId) : "overview";

  const detail = useQuery({
    queryKey: queryKeys.sandbox(name),
    queryFn: () => api.sandbox(name),
    enabled: name !== "",
    retry: 0,
  });

  const staleness = useConfigStaleness();

  // The config slug mirrors the sandbox name; a missing directory hides the
  // affordance instead of surfacing an error.
  const configDir = useQuery({
    queryKey: queryKeys.sandboxConfigDir(name),
    queryFn: () => api.sandboxConfigDir(name),
    enabled: name !== "",
    retry: 0,
  });

  const start = useApiMutation({ mutationFn: () => api.startSandbox(name), success: `Starting ${name}` });
  const stop = useApiMutation({ mutationFn: () => api.stopSandbox(name), success: `Stopping ${name}` });
  const remove = useApiMutation({
    mutationFn: (purge: boolean) => api.deleteSandbox(name, true, purge),
    success: `Deleted ${name}`,
    onSuccess: () => navigate("/"),
  });
  const validate = useApiMutation({
    mutationFn: () => api.validateSandbox(name),
    onSuccess: (result) => {
      toast.push({
        tone: result.ok ? "success" : "danger",
        title: result.ok ? "Sandbox is valid" : "Sandbox is invalid",
        body: result.output,
      });
    },
  });
  const attachKit = useApiMutation({
    mutationFn: (ref: string) => api.attachKit(name, ref),
    success: `Kit attached to ${name}`,
    onSuccess: (result) => {
      if ((result.report.errors?.length ?? 0) > 0) {
        toast.push({
          tone: "danger",
          title: "Kit attached, but the sidecar re-apply reported errors",
          body: result.report.errors?.join("\n"),
        });
      }
      void queryClient.invalidateQueries({ queryKey: queryKeys.sandbox(name) });
      void queryClient.invalidateQueries({ queryKey: queryKeys.sandboxes });
    },
  });
  const saveTemplate = useApiMutation({
    mutationFn: (tag: string) => api.saveTemplate(name, tag),
    success: (_output, tag) => `Template ${tag} saved`,
    invalidate: [queryKeys.templates],
    onSuccess: () => setSavingTemplate(false),
  });

  async function runJob(kind: "apply" | "recreate") {
    if (jobBusy) return;
    const id = newJobId();
    jobHub.reset(id);
    reportedJob.current = null;
    setJobBusy(true);
    setJob({ id, kind });
    try {
      if (kind === "apply") await api.applySandbox(name, id);
      else await api.recreateSandbox(name, id);
    } catch (error) {
      setJobBusy(false);
      toast.push({
        tone: "danger",
        title: kind === "apply" ? "Apply failed to start" : "Recreate failed to start",
        body: errorMessage(error),
      });
    }
  }

  useEffect(() => {
    if (!job || jobState.status === "running" || reportedJob.current === job.id) return;
    reportedJob.current = job.id;
    setJobBusy(false);
    if (jobState.status === "error") {
      toast.push({
        tone: "danger",
        title: job.kind === "recreate" ? "Recreate failed" : "Apply failed",
        body: jobState.error ?? "sbx failed",
      });
      return;
    }
    toast.push({
      tone: "success",
      title: job.kind === "recreate" ? `Sandbox ${name} recreated` : `Sandbox ${name} applied`,
    });
    void queryClient.invalidateQueries({ queryKey: queryKeys.sandbox(name) });
    void queryClient.invalidateQueries({ queryKey: queryKeys.sandboxes });
  }, [job, jobState.status, jobState.error, name, queryClient, toast]);

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
            {staleNames(staleness.data, "sandbox").has(sandbox.name) ? <StaleBadge /> : null}
            {sandbox.agent ? <Badge>{sandbox.agent}</Badge> : null}
            {sandbox.mount_policy_denied ? <Badge tone="danger">mount denied</Badge> : null}
            {data.incomplete ? (
              <span title="Imported without its original create parameters: CPU, memory and env are not recoverable.">
                <Badge tone="warning">incomplete config</Badge>
              </span>
            ) : null}
          </span>
        }
        subtitle={
          configDir.data ? (
            <span className="flex flex-wrap items-center gap-2">
              <span className="text-xs text-faint">Config</span>
              <code className="font-mono text-xs">{configDir.data}</code>
              <CopyButton value={configDir.data} />
            </span>
          ) : undefined
        }
        actions={
          <>
            <Button
              size="md"
              disabled={!sandbox.running}
              title={sandbox.running ? undefined : "Start the sandbox first"}
              loading={jobBusy && job?.kind === "apply"}
              onClick={() => void runJob("apply")}
            >
              <IconRefresh className="size-3.5" /> Apply
            </Button>
            <Button size="md" variant="ghost" loading={validate.isPending} onClick={() => validate.mutate()}>
              Validate
            </Button>
            <Button size="md" variant="ghost" onClick={() => setViewingFiles(true)}>
              View files
            </Button>
            {sandbox.running ? (
              <Button size="md" onClick={() => stop.mutate()} loading={stop.isPending}>
                <IconStop className="size-3.5" /> Stop
              </Button>
            ) : (
              <Button size="md" onClick={() => start.mutate()} loading={start.isPending}>
                <IconPlay className="size-3.5" /> Start
              </Button>
            )}
            <Button size="md" variant="ghost" onClick={() => setAttachingKit(true)}>
              <IconPlus className="size-3.5" /> Attach kit
            </Button>
            <Button size="md" variant="ghost" onClick={() => setSavingTemplate(true)}>
              Save as template
            </Button>
            <Button
              size="md"
              variant="ghost"
              loading={jobBusy && job?.kind === "recreate"}
              onClick={() => setRecreating(true)}
            >
              <span className="text-danger">Recreate</span>
            </Button>
            <Button size="md" variant="danger" onClick={() => setConfirming(true)}>
              <IconTrash className="size-3.5" /> Delete
            </Button>
          </>
        }
      />

      {job ? (
        <Panel
          title={job.kind === "recreate" ? "Recreate" : "Apply"}
          description={
            job.kind === "recreate"
              ? "Deleting and recreating the sandbox from its configuration files."
              : "Converging the sandbox onto its configuration files."
          }
          className="mb-4"
          actions={
            jobState.status === "running" ? (
              <Button variant="ghost" onClick={() => void api.cancelJob(job.id)}>
                Cancel
              </Button>
            ) : undefined
          }
        >
          <div className="flex flex-col gap-1.5">
            <div className="flex items-center gap-2 text-xs text-faint">
              <span>job {job.id.slice(0, 8)}</span>
              <span
                className={
                  jobState.status === "error"
                    ? "text-danger"
                    : jobState.status === "done"
                      ? "text-success"
                      : "text-warning"
                }
              >
                {jobState.status}
                {jobState.error ? `: ${jobState.error}` : ""}
              </span>
            </div>
            <LogView text={jobState.output} empty="Waiting for output…" autoScrollRef={jobLogRef} className="max-h-64" />
          </div>
        </Panel>
      ) : null}

      {data.incomplete ? (
        <Panel title="Incomplete configuration" className="mb-4">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="max-w-2xl text-sm text-muted">
              This config directory was adopted from the daemon. Its original CPU, memory and environment settings were
              not recoverable, so a recreate will use defaults and will not restore them.
            </p>
            <Button size="sm" variant="outline" onClick={() => setCompleting(true)}>
              Complete this config
            </Button>
          </div>
        </Panel>
      ) : null}

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

      {tab === "overview" ? <OverviewTab name={sandbox.name} detail={data} /> : null}
      {tab === "mounts" ? <MountsTab name={sandbox.name} detail={data} /> : null}
      {tab === "caches" ? <CachesTab name={sandbox.name} detail={data} /> : null}
      {tab === "skills" ? <SkillsTab name={sandbox.name} detail={data} /> : null}
      {tab === "profiles" ? <ProfilesTab name={sandbox.name} detail={data} /> : null}
      {tab === "secrets" ? <SecretsTab name={sandbox.name} detail={data} /> : null}
      {tab === "traffic" ? <TrafficTab name={sandbox.name} /> : null}
      {tab === "terminal" ? <TerminalTab name={sandbox.name} running={sandbox.running} /> : null}

      <ConfigViewer
        open={viewingFiles}
        target={{ kind: "sandbox", slug: sandbox.name, title: sandbox.name }}
        onClose={() => setViewingFiles(false)}
      />

      <CompleteConfigDialog name={sandbox.name} open={completing} onClose={() => setCompleting(false)} />

      <ConfirmDialog
        open={confirming}
        title={`Delete ${sandbox.name}?`}
        body={
          <div className="flex flex-col gap-3">
            <span>This removes the sandbox and its runtime; open sessions are disconnected and assigned profiles are unapplied first.</span>
            <CheckboxField
              label="also delete the config directory"
              hint="Without it the sandbox files are kept and the sandbox can be recreated later."
              checked={purgeConfig}
              onChange={setPurgeConfig}
            />
          </div>
        }
        confirmLabel="Delete"
        busy={remove.isPending}
        onConfirm={() => remove.mutate(purgeConfig)}
        onClose={() => {
          setConfirming(false);
          setPurgeConfig(false);
        }}
      />

      <ConfirmDialog
        open={recreating}
        title={`Recreate ${sandbox.name} from scratch?`}
        body={
          <div className="flex flex-col gap-2">
            <span>
              The sandbox is deleted and created again from its configuration files as a fresh container: rules,
              mounts, caches and skills are re-applied.
            </span>
            {data.incomplete ? (
              <span className="text-warning">
                This config is incomplete: its original CPU, memory and environment settings were not recoverable, so
                recreating will use defaults and will not restore them.
              </span>
            ) : null}
            <span className="text-danger">
              This is destructive: everything inside the sandbox — volumes, container state and, for --clone
              sandboxes, the in-container working tree — is lost. Use "Attach kit" instead to add a mixin without
              losing them.
            </span>
          </div>
        }
        confirmLabel="Recreate"
        busy={jobBusy && job?.kind === "recreate"}
        onConfirm={() => {
          setRecreating(false);
          void runJob("recreate");
        }}
        onClose={() => setRecreating(false)}
      />

      <AttachKitDialog
        name={sandbox.name}
        open={attachingKit}
        busy={attachKit.isPending}
        onAttach={(ref) => attachKit.mutate(ref, { onSuccess: () => setAttachingKit(false) })}
        onClose={() => setAttachingKit(false)}
      />

      <SaveTemplateDialog
        name={sandbox.name}
        open={savingTemplate}
        busy={saveTemplate.isPending}
        onSave={(tag) => saveTemplate.mutate(tag)}
        onClose={() => setSavingTemplate(false)}
      />
    </>
  );
}

/**
 * Attaches a mixin kit without recreating the sandbox: sbx swaps the container
 * for one created with the kit appended, preserving kit-owned volumes and
 * --clone workspaces, and the reference is recorded in create.kits.
 */
function AttachKitDialog({
  name,
  open,
  busy,
  onAttach,
  onClose,
}: {
  name: string;
  open: boolean;
  busy: boolean;
  onAttach: (ref: string) => void;
  onClose: () => void;
}) {
  const [ref, setRef] = useState("");
  useEffect(() => {
    if (open) setRef("");
  }, [open]);
  const trimmed = ref.trim();

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={`Attach a kit to ${name}`}
      description="sbx recreates the container with the kit added; the sandbox itself is kept."
      size="sm"
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={busy}>
            Cancel
          </Button>
          <Button
            variant="primary"
            disabled={trimmed === ""}
            loading={busy}
            onClick={() => onAttach(trimmed)}
          >
            Attach kit
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-3">
        <p className="text-sm text-muted">
          The container is swapped for one created with the kit appended, so kit-owned volumes (agent session state)
          and a --clone sandbox's working tree are preserved. The reference is recorded in create.kits and survives a
          later recreate.
        </p>
        <Field label="Kit reference" hint="Local directory, ZIP file, OCI reference or git+ URL.">
          <Input
            value={ref}
            onChange={(event) => setRef(event.target.value)}
            placeholder="ghcr.io/org/mcp-postgres:1.0"
            className="font-mono"
            autoFocus
          />
        </Field>
        <span className="text-xs text-faint">
          Unlike Recreate, this keeps the sandbox: only the container is swapped, so container state is not lost.
        </span>
      </div>
    </Modal>
  );
}

/**
 * Snapshots the sandbox's container into the runtime's local image store, so
 * it can be picked as the --template base of a future create instead of
 * re-baking the same slow kits.
 */
function SaveTemplateDialog({
  name,
  open,
  busy,
  onSave,
  onClose,
}: {
  name: string;
  open: boolean;
  busy: boolean;
  onSave: (tag: string) => void;
  onClose: () => void;
}) {
  const [tag, setTag] = useState("");
  useEffect(() => {
    if (open) setTag("");
  }, [open]);
  const trimmed = tag.trim();

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={`Save ${name} as a template`}
      description="Snapshots the sandbox's current container into the local image store. Pick the resulting template as the base image in the create form to skip re-baking its kits."
      size="sm"
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={busy}>
            Cancel
          </Button>
          <Button variant="primary" disabled={trimmed === ""} loading={busy} onClick={() => onSave(trimmed)}>
            Save template
          </Button>
        </>
      }
    >
      <Field label="Template tag" hint="Repository:tag for the saved image.">
        <Input
          value={tag}
          onChange={(event) => setTag(event.target.value)}
          placeholder="myimage:v1.0"
          className="font-mono"
          autoFocus
        />
      </Field>
    </Modal>
  );
}
