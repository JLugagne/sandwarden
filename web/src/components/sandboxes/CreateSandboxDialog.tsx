import { useEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { api } from "@/api/client";
import { useJob } from "@/hooks/useJob";
import { errorMessage } from "@/hooks/useApiMutation";
import { jobHub, newJobId } from "@/store/jobs";
import { queryKeys } from "@/store/realtime";
import { useStickToBottom, useToasts } from "@/components/Toaster";
import {
  Button,
  CheckboxField,
  Field,
  IconFolder,
  IconPlus,
  IconTrash,
  Input,
  LogView,
  Modal,
  Select,
  TextArea,
} from "@/components/ui";
import { splitList } from "@/lib/format";
import type { WorkspaceInput } from "@/types";

const AGENTS = [
  "claude",
  "codex",
  "copilot",
  "cursor",
  "devin",
  "docker-agent",
  "droid",
  "gemini",
  "kiro",
  "opencode",
  "shell",
];

const EMPTY_WORKSPACES: WorkspaceInput[] = [{ path: "", read_only: false }];

export function CreateSandboxDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const toast = useToasts();
  const queryClient = useQueryClient();

  const [agent, setAgent] = useState("claude");
  const [workspaces, setWorkspaces] = useState<WorkspaceInput[]>(EMPTY_WORKSPACES);
  const [name, setName] = useState("");
  const [cpus, setCPUs] = useState("");
  const [memory, setMemory] = useState("");
  const [profile, setProfile] = useState("");
  const [template, setTemplate] = useState("");
  const [publish, setPublish] = useState("");
  const [env, setEnv] = useState("");
  const [denyNetwork, setDenyNetwork] = useState("");
  const [clone, setClone] = useState(false);
  const [attachCaches, setAttachCaches] = useState(true);

  const [jobId, setJobId] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const reported = useRef<string | null>(null);
  const job = useJob(jobId);
  const logRef = useStickToBottom<HTMLPreElement>(job.output);

  useEffect(() => {
    if (!jobId || job.status === "running" || reported.current === jobId) return;
    reported.current = jobId;
    setBusy(false);
    if (job.status === "error") {
      toast.push({ tone: "danger", title: "Create failed", body: job.error ?? "sbx create failed" });
      return;
    }
    toast.push({ tone: "success", title: "Sandbox created", body: name.trim() || undefined });
    void queryClient.invalidateQueries({ queryKey: queryKeys.sandboxes });
  }, [job.status, jobId, job.error, name, queryClient, toast]);

  function reset() {
    setWorkspaces(EMPTY_WORKSPACES);
    setName("");
    setCPUs("");
    setMemory("");
    setProfile("");
    setTemplate("");
    setPublish("");
    setEnv("");
    setDenyNetwork("");
    setClone(false);
    setAttachCaches(true);
    setJobId(null);
    setBusy(false);
  }

  function close() {
    if (busy) return;
    reset();
    onClose();
  }

  async function pickFolder(index: number) {
    try {
      const { path } = await api.fsPick(workspaces[index]?.path || undefined);
      setWorkspaces((current) => current.map((workspace, i) => (i === index ? { ...workspace, path } : workspace)));
    } catch (error) {
      toast.push({ tone: "warning", title: "Folder picker unavailable", body: errorMessage(error) });
    }
  }

  async function submit() {
    setBusy(true);
    const id = newJobId();
    jobHub.reset(id);
    reported.current = null;
    setJobId(id);
    try {
      await api.createSandbox({
        agent,
        workspaces: workspaces
          .map((workspace) => ({ ...workspace, path: workspace.path.trim() }))
          .filter((workspace) => workspace.path !== ""),
        name: name.trim() || undefined,
        cpus: Number(cpus) || 0,
        memory: memory.trim() || undefined,
        profile: profile.trim() || undefined,
        template: template.trim() || undefined,
        publish: splitList(publish),
        env: splitList(env),
        deny_network: splitList(denyNetwork),
        clone,
        attach_caches: attachCaches,
        job_id: id,
      });
    } catch (error) {
      setBusy(false);
      toast.push({ tone: "danger", title: "Create failed", body: errorMessage(error) });
    }
  }

  return (
    <Modal
      open={open}
      onClose={close}
      title="New sandbox"
      description="Every `sbx create` option is available here; progress streams below."
      size="lg"
      footer={
        <>
          <Button variant="ghost" onClick={close} disabled={busy}>
            Cancel
          </Button>
          <Button variant="primary" onClick={submit} loading={busy} disabled={!agent}>
            Create
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-5">
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Agent" htmlFor="cs-agent">
            <Select id="cs-agent" value={agent} onChange={(event) => setAgent(event.target.value)}>
              {AGENTS.map((value) => (
                <option key={value} value={value}>
                  {value}
                </option>
              ))}
            </Select>
          </Field>
          <Field label="Name (optional)" htmlFor="cs-name" hint="Auto-generated when left empty.">
            <Input id="cs-name" value={name} onChange={(event) => setName(event.target.value)} placeholder="my-sandbox" />
          </Field>
        </div>

        <div className="flex flex-col gap-2">
          <span className="text-xs font-medium text-muted">Workspaces</span>
          {workspaces.map((workspace, index) => (
            <div key={index} className="flex items-center gap-2">
              <Input
                value={workspace.path}
                onChange={(event) =>
                  setWorkspaces((current) =>
                    current.map((item, i) => (i === index ? { ...item, path: event.target.value } : item)),
                  )
                }
                placeholder="/absolute/host/path"
                className="flex-1 font-mono text-xs"
              />
              <Button size="icon" variant="ghost" title="Choose folder…" onClick={() => void pickFolder(index)}>
                <IconFolder className="size-3.5" />
              </Button>
              <CheckboxField
                label="ro"
                checked={workspace.read_only}
                onChange={(read_only) =>
                  setWorkspaces((current) => current.map((item, i) => (i === index ? { ...item, read_only } : item)))
                }
              />
              <Button
                size="icon"
                variant="ghost"
                title="Remove workspace"
                disabled={workspaces.length === 1}
                onClick={() => setWorkspaces((current) => current.filter((_, i) => i !== index))}
              >
                <IconTrash className="size-3.5" />
              </Button>
            </div>
          ))}
          <div>
            <Button
              size="sm"
              variant="ghost"
              onClick={() => setWorkspaces((current) => [...current, { path: "", read_only: false }])}
            >
              <IconPlus className="size-3.5" /> Add workspace
            </Button>
          </div>
        </div>

        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="CPUs (optional)" htmlFor="cs-cpus" hint="0 = auto (all host CPUs).">
            <Input
              id="cs-cpus"
              type="number"
              min={0}
              value={cpus}
              onChange={(event) => setCPUs(event.target.value)}
              placeholder="0"
            />
          </Field>
          <Field label="Memory (optional)" htmlFor="cs-memory" hint="e.g. 4g. Defaults to 2 CPUs / 4 GiB.">
            <Input id="cs-memory" value={memory} onChange={(event) => setMemory(event.target.value)} placeholder="4g" />
          </Field>
          <Field label="Policy profile (optional)" htmlFor="cs-profile" hint="sbx --profile name, not the UI profiles.">
            <Input id="cs-profile" value={profile} onChange={(event) => setProfile(event.target.value)} />
          </Field>
          <Field label="Template (optional)" htmlFor="cs-template">
            <Input id="cs-template" value={template} onChange={(event) => setTemplate(event.target.value)} />
          </Field>
        </div>

        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Published ports" htmlFor="cs-publish" hint="One per line, e.g. 8080:80 or 127.0.0.1:3000:3000/tcp.">
            <TextArea
              id="cs-publish"
              rows={2}
              value={publish}
              onChange={(event) => setPublish(event.target.value)}
              placeholder={"8080:80"}
            />
          </Field>
          <Field label="Denied network" htmlFor="cs-deny" hint="Egress host patterns denied at creation.">
            <TextArea
              id="cs-deny"
              rows={2}
              value={denyNetwork}
              onChange={(event) => setDenyNetwork(event.target.value)}
              placeholder={"telemetry.example.com"}
            />
          </Field>
        </div>

        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Environment" htmlFor="cs-env" hint="One KEY=VALUE per line.">
            <TextArea
              id="cs-env"
              rows={2}
              value={env}
              onChange={(event) => setEnv(event.target.value)}
              placeholder={"NODE_ENV=development"}
            />
          </Field>
          <div className="flex flex-col gap-3 pt-5">
            <CheckboxField
              label="Clone the Git repository in-container"
              hint="Commits come back through the sandbox-<name> git remote."
              checked={clone}
              onChange={setClone}
            />
            <CheckboxField
              label="Attach shared caches"
              hint="Mounts caches flagged auto-attach in Settings (Go, npm, …)."
              checked={attachCaches}
              onChange={setAttachCaches}
            />
          </div>
        </div>

        {jobId ? (
          <div className="flex flex-col gap-1.5">
            <span className="text-xs font-medium text-muted">Progress</span>
            <LogView text={job.output} empty="Waiting for output…" autoScrollRef={logRef} className="max-h-48" />
          </div>
        ) : null}
      </div>
    </Modal>
  );
}
