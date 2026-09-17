import { useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/api/client";
import { useJob } from "@/hooks/useJob";
import { errorMessage } from "@/hooks/useApiMutation";
import { jobHub, newJobId } from "@/store/jobs";
import { queryKeys } from "@/store/realtime";
import { useStickToBottom, useToasts } from "@/components/Toaster";
import {
  Button,
  Checkbox,
  CheckboxField,
  Field,
  IconFolder,
  IconPlus,
  IconTrash,
  Input,
  LogView,
  MenuSelect,
  Modal,
  Tabs,
  TextArea,
} from "@/components/ui";
import { splitList } from "@/lib/format";
import { agentMenuGroups } from "@/lib/catalog";
import type { KitItemView, Template, WorkspaceInput } from "@/types";

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

function templateReference(template: Template): string {
  if (!template.repository || !template.tag) return template.id;
  return `${template.repository}:${template.tag}`;
}

// Mirrors internal/app/secretguard.go: known credential prefixes first, then
// a conservative long high-entropy token rule. Keep the two in sync.
const SECRET_PREFIXES: { prefix: string; reason: string }[] = [
  { prefix: "sk-", reason: 'API key prefix "sk-"' },
  { prefix: "github_pat_", reason: 'GitHub token prefix "github_pat_"' },
  { prefix: "ghp_", reason: 'GitHub token prefix "ghp_"' },
  { prefix: "gho_", reason: 'GitHub token prefix "gho_"' },
  { prefix: "ghu_", reason: 'GitHub token prefix "ghu_"' },
  { prefix: "ghs_", reason: 'GitHub token prefix "ghs_"' },
  { prefix: "ghr_", reason: 'GitHub token prefix "ghr_"' },
  { prefix: "xoxa-", reason: 'Slack token prefix "xoxa-"' },
  { prefix: "xoxb-", reason: 'Slack token prefix "xoxb-"' },
  { prefix: "xoxp-", reason: 'Slack token prefix "xoxp-"' },
  { prefix: "xoxr-", reason: 'Slack token prefix "xoxr-"' },
  { prefix: "xoxs-", reason: 'Slack token prefix "xoxs-"' },
  { prefix: "xapp-", reason: 'Slack token prefix "xapp-"' },
  { prefix: "AKIA", reason: 'AWS access key id prefix "AKIA"' },
  { prefix: "ASIA", reason: 'AWS access key id prefix "ASIA"' },
  { prefix: "AIza", reason: 'Google API key prefix "AIza"' },
  { prefix: "glpat-", reason: 'GitLab token prefix "glpat-"' },
  { prefix: "gldt-", reason: 'GitLab token prefix "gldt-"' },
  { prefix: "glrt-", reason: 'GitLab token prefix "glrt-"' },
  { prefix: "npm_", reason: 'npm token prefix "npm_"' },
  { prefix: "pypi-", reason: 'PyPI token prefix "pypi-"' },
  { prefix: "dckr_pat_", reason: 'Docker token prefix "dckr_pat_"' },
];

const SECRET_MIN_TOKEN_LENGTH = 32;
const SECRET_MIN_ENTROPY = 3.5;

function shannonEntropy(value: string): number {
  const counts = new Map<string, number>();
  for (const char of value) counts.set(char, (counts.get(char) ?? 0) + 1);
  let entropy = 0;
  for (const count of counts.values()) {
    const p = count / value.length;
    entropy -= p * Math.log2(p);
  }
  return entropy;
}

export function secretValueReason(value: string): string {
  let candidate = value.trim();
  if (candidate.length >= 2 && (candidate[0] === '"' || candidate[0] === "'") && candidate.endsWith(candidate[0])) {
    candidate = candidate.slice(1, -1);
  }
  if (!candidate) return "";
  const lower = candidate.toLowerCase();
  if (lower.startsWith("-----begin ") && lower.includes("private key-----")) return "PEM private key block";
  for (const rule of SECRET_PREFIXES) {
    if (candidate.startsWith(rule.prefix)) return rule.reason;
  }
  if (candidate.length < SECRET_MIN_TOKEN_LENGTH || !/^[A-Za-z0-9_-]+$/.test(candidate)) return "";
  if (!/[A-Za-z]/.test(candidate) || !/[0-9]/.test(candidate)) return "";
  return shannonEntropy(candidate) >= SECRET_MIN_ENTROPY ? "long high-entropy token" : "";
}

export function firstSecretEnvEntry(env: string): { key: string; reason: string } | null {
  for (const raw of env.split(/[,\n]/)) {
    const line = raw.trim();
    if (!line) continue;
    const separator = line.indexOf("=");
    const key = (separator === -1 ? line : line.slice(0, separator)).trim();
    if (!key) continue;
    const value = separator === -1 ? "" : line.slice(separator + 1);
    const reason = secretValueReason(value);
    if (reason) return { key, reason };
  }
  return null;
}

export function CreateSandboxDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const toast = useToasts();
  const queryClient = useQueryClient();

  const [tab, setTab] = useState("general");
  const [agent, setAgent] = useState("claude");
  const [workspaces, setWorkspaces] = useState<WorkspaceInput[]>(EMPTY_WORKSPACES);
  const [name, setName] = useState("");
  const [cpus, setCPUs] = useState("");
  const [memory, setMemory] = useState("");
  const [profile, setProfile] = useState("");
  const [template, setTemplate] = useState("");
  const [kits, setKits] = useState("");
  const [publish, setPublish] = useState("");
  const [env, setEnv] = useState("");
  const [denyNetwork, setDenyNetwork] = useState("");
  const [clone, setClone] = useState(false);
  const [attachCaches, setAttachCaches] = useState(true);

  const templates = useQuery({ queryKey: queryKeys.templates, queryFn: api.templates });
  const kitItems = useQuery({ queryKey: queryKeys.kitItems, queryFn: () => api.kitItems() });
  const agentGroups = useMemo(() => agentMenuGroups(AGENTS, kitItems.data ?? []), [kitItems.data]);
  const mixinGroups = useMemo(() => {
    const groups = new Map<string, KitItemView[]>();
    for (const item of kitItems.data ?? []) {
      if (item.kind === "sandbox") continue;
      const store = item.store_name || "—";
      groups.set(store, [...(groups.get(store) ?? []), item]);
    }
    return [...groups.entries()].sort(([left], [right]) => left.localeCompare(right));
  }, [kitItems.data]);
  const selectedKits = splitList(kits);

  const [jobId, setJobId] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const reported = useRef<string | null>(null);
  const job = useJob(jobId);
  const created = jobId !== null && job.status === "done";
  const logRef = useStickToBottom<HTMLPreElement>(job.output);
  const envIssue = useMemo(() => firstSecretEnvEntry(env), [env]);

  useEffect(() => {
    if (jobId) setTab("progress");
  }, [jobId]);

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
    setTab("general");
    setWorkspaces(EMPTY_WORKSPACES);
    setName("");
    setCPUs("");
    setMemory("");
    setProfile("");
    setTemplate("");
    setKits("");
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

  function toggleKit(ref: string) {
    setKits((current) => {
      const refs = splitList(current);
      const next = refs.includes(ref) ? refs.filter((item) => item !== ref) : [...refs, ref];
      return next.join("\n");
    });
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
    if (busy || created) return;
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
        kits: splitList(kits),
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
      description="Every `sbx create` option is one tab away; the output streams in the Progress tab."
      size="lg"
      footer={
        created ? (
          <Button variant="primary" onClick={close}>
            Close
          </Button>
        ) : (
          <>
            <Button variant="ghost" onClick={close} disabled={busy}>
              Cancel
            </Button>
            <Button variant="primary" onClick={submit} loading={busy} disabled={!agent || envIssue !== null}>
              Create
            </Button>
          </>
        )
      }
    >
      <div className="flex flex-col gap-5">
        <Tabs
          tabs={[
            { id: "general", label: "General" },
            { id: "resources", label: "Resources" },
            { id: "network", label: "Network" },
            { id: "options", label: "Options" },
            ...(jobId ? [{ id: "progress", label: "Progress" }] : []),
          ]}
          active={tab}
          onChange={setTab}
        />

        {tab === "general" ? (
          <div className="flex flex-col gap-5">
            <div className="grid gap-4 sm:grid-cols-2">
              <Field label="Agent" htmlFor="cs-agent" hint="A built-in agent or an agent kit from your repositories.">
                <MenuSelect
                  id="cs-agent"
                  groups={agentGroups}
                  value={agent}
                  onChange={setAgent}
                  placeholder="Choose an agent…"
                  searchPlaceholder="Search agents and kits…"
                  emptyLabel="No agent available."
                />
              </Field>
              <Field label="Name (optional)" htmlFor="cs-name" hint="Auto-generated when left empty.">
                <Input id="cs-name" value={name} onChange={(event) => setName(event.target.value)} placeholder="my-sandbox" />
              </Field>
              <Field label="Template (optional)" htmlFor="cs-template" hint="Base image for the sandbox; local templates are suggested.">
                <Input
                  id="cs-template"
                  list="cs-templates"
                  value={template}
                  onChange={(event) => setTemplate(event.target.value)}
                  placeholder="myimage:v1.0"
                />
                <datalist id="cs-templates">
                  {(templates.data ?? []).map((item) => (
                    <option key={item.id} value={templateReference(item)} />
                  ))}
                </datalist>
              </Field>
              <Field label="Policy profile (optional)" htmlFor="cs-profile" hint="sbx --profile name, not the UI profiles.">
                <Input id="cs-profile" value={profile} onChange={(event) => setProfile(event.target.value)} />
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

            <div className="flex flex-col gap-2">
              <Field
                label="Kit mixins (optional)"
                htmlFor="cs-kits"
                hint="One reference per line: git+https://…, a ZIP path, a directory or an OCI reference (`sbx create --kit`)."
              >
                <TextArea
                  id="cs-kits"
                  rows={4}
                  value={kits}
                  onChange={(event) => setKits(event.target.value)}
                  placeholder={"git+https://github.com/docker/sbx-kits-contrib#dir=code-server&ref=main"}
                />
              </Field>
              {mixinGroups.length > 0 ? (
                <div className="rounded-sm border border-border">
                  <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border px-3 py-1.5">
                    <span className="text-2xs font-medium tracking-wide text-faint uppercase">
                      From your repositories · {selectedKits.length} selected
                    </span>
                    <Link to="/kits" className="text-xs text-accent hover:underline">
                      Manage kits
                    </Link>
                  </div>
                  <div className="max-h-56 overflow-y-auto p-3">
                    {mixinGroups.map(([store, items]) => (
                      <div key={store} className="mb-3 last:mb-0">
                        <div className="mb-1.5 text-2xs font-semibold tracking-wide text-faint uppercase">{store}</div>
                        <div className="flex flex-col gap-1.5">
                          {items.map((item) => (
                            <Checkbox
                              key={item.ref}
                              checked={selectedKits.includes(item.ref)}
                              onChange={() => toggleKit(item.ref)}
                              label={
                                <span className="inline-flex items-center gap-2">
                                  <span>{item.display_name || item.name}</span>
                                  {item.version ? <span className="text-faint">v{item.version}</span> : null}
                                </span>
                              }
                            />
                          ))}
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              ) : null}
            </div>
          </div>
        ) : null}

        {tab === "resources" ? (
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
          </div>
        ) : null}


        {tab === "network" ? (
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Published ports" htmlFor="cs-publish" hint="One per line, e.g. 8080:80 or 127.0.0.1:3000:3000/tcp.">
              <TextArea
                id="cs-publish"
                rows={4}
                value={publish}
                onChange={(event) => setPublish(event.target.value)}
                placeholder={"8080:80"}
              />
            </Field>
            <Field label="Denied network" htmlFor="cs-deny" hint="Egress host patterns denied at creation.">
              <TextArea
                id="cs-deny"
                rows={4}
                value={denyNetwork}
                onChange={(event) => setDenyNetwork(event.target.value)}
                placeholder={"telemetry.example.com"}
              />
            </Field>
          </div>
        ) : null}

        {tab === "options" ? (
          <div className="flex flex-col gap-4">
            <Field
              label="Environment"
              htmlFor="cs-env"
              hint="One KEY=VALUE per line. Values are written in clear to spec.yaml: keep secrets in the Secrets page and enter the bare KEY to read them from the host environment."
            >
              <TextArea
                id="cs-env"
                rows={4}
                value={env}
                onChange={(event) => setEnv(event.target.value)}
                placeholder={"NODE_ENV=development"}
                aria-invalid={envIssue ? true : undefined}
                className={envIssue ? "border-danger" : undefined}
              />
            </Field>
            {envIssue ? (
              <p className="text-xs text-danger">
                {envIssue.key} looks like a secret ({envIssue.reason}). Store it from the Secrets page and use the bare{" "}
                {envIssue.key} form instead.
              </p>
            ) : null}
            <div className="flex flex-col gap-3">
              <CheckboxField
                label="Clone the Git repository in-container"
                hint="Commits come back through the sandbox-<name> git remote."
                checked={clone}
                onChange={setClone}
              />
              <CheckboxField
                label="Attach shared caches"
                hint="Mounts caches flagged auto-attach in Settings, plus the caches defaulted by the assigned profiles."
                checked={attachCaches}
                onChange={setAttachCaches}
              />
            </div>
          </div>
        ) : null}

        {tab === "progress" && jobId ? (
          <div className="flex flex-col gap-1.5">
            <span className="text-xs font-medium text-muted">
              {job.status === "running" ? "Creating the sandbox…" : job.status === "error" ? "Creation failed." : "Done."}
            </span>
            <LogView text={job.output} empty="Waiting for output…" autoScrollRef={logRef} className="max-h-80" />
          </div>
        ) : null}
      </div>
    </Modal>
  );
}
