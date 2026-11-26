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
  Select,
  Tabs,
  TextArea,
} from "@/components/ui";
import { cn } from "@/lib/cn";
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

/** One kit-argument row of the Advanced tab. An auto row is derived from a
 * selected kit's declared arguments and cannot outlive the selection. */
interface KitArgRow {
  key: string;
  value: string;
  required?: boolean;
  description?: string;
  auto?: boolean;
}

const SKILLS_MODES = [
  { value: "", label: "sbx default" },
  { value: "off", label: "off" },
  { value: "readonly", label: "readonly" },
  { value: "readwrite", label: "readwrite" },
];

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

export function CreateSandboxDialog({
  open,
  onClose,
  initialTemplate = "",
}: {
  open: boolean;
  onClose: () => void;
  initialTemplate?: string;
}) {
  const toast = useToasts();
  const queryClient = useQueryClient();

  const [tab, setTab] = useState("general");
  const [agent, setAgent] = useState("claude");
  const [workspaces, setWorkspaces] = useState<WorkspaceInput[]>(EMPTY_WORKSPACES);
  const [name, setName] = useState("");
  const [cpus, setCPUs] = useState("");
  const [memory, setMemory] = useState("");
  const [profile, setProfile] = useState("");
  const [template, setTemplate] = useState(initialTemplate);
  const [kits, setKits] = useState("");
  const [publish, setPublish] = useState("");
  const [env, setEnv] = useState("");
  const [denyNetwork, setDenyNetwork] = useState("");
  const [clone, setClone] = useState(false);
  const [attachCaches, setAttachCaches] = useState(true);
  const [kitArgs, setKitArgs] = useState<KitArgRow[]>([]);
  const [envFiles, setEnvFiles] = useState("");
  const [skillsMode, setSkillsMode] = useState("");
  const [staticMcp, setStaticMcp] = useState("");

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
  const selectedKits = useMemo(() => splitList(kits), [kits]);
  // Catalogue kits whose declared arguments drive the pre-filled rows: the
  // selected mixins and the agent when it is itself a kit reference.
  const selectedKitItems = useMemo(() => {
    const refs = new Set(selectedKits);
    if (agent) refs.add(agent);
    return (kitItems.data ?? []).filter((item) => refs.has(item.ref));
  }, [kitItems.data, selectedKits, agent]);

  const [jobId, setJobId] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const reported = useRef<string | null>(null);
  const job = useJob(jobId);
  const created = jobId !== null && job.status === "done";
  const logRef = useStickToBottom<HTMLPreElement>(job.output);
  const envIssue = useMemo(() => firstSecretEnvEntry(env), [env]);

  // Prefill one row per argument the selected kits declare, scoped as
  // <kit>.<arg> so it applies to that kit only, and keep manually added rows.
  useEffect(() => {
    setKitArgs((current) => {
      const manual = current.filter((row) => !row.auto);
      const previous = new Map(current.map((row) => [row.key, row.value]));
      const auto: KitArgRow[] = [];
      for (const item of selectedKitItems) {
        for (const [name, arg] of Object.entries(item.spec?.arguments ?? {})) {
          const key = `${item.name}.${name}`;
          auto.push({
            key,
            value: previous.get(key) ?? arg.default ?? "",
            required: Boolean(arg.required),
            description: arg.description ?? "",
            auto: true,
          });
        }
      }
      return [...auto, ...manual];
    });
  }, [selectedKitItems]);

  function updateKitArg(index: number, patch: Partial<KitArgRow>) {
    setKitArgs((current) => current.map((row, i) => (i === index ? { ...row, ...patch } : row)));
  }

  useEffect(() => {
    if (jobId) setTab("progress");
  }, [jobId]);

  const seeded = useRef(false);
  useEffect(() => {
    if (!open) {
      seeded.current = false;
      return;
    }
    if (seeded.current) return;
    seeded.current = true;
    if (initialTemplate) {
      setTemplate(initialTemplate);
      setTab("general");
    }
  }, [open, initialTemplate]);

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
    setKitArgs([]);
    setEnvFiles("");
    setSkillsMode("");
    setStaticMcp("");
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
        kit_args: kitArgs
          .map((row) => ({ key: row.key.trim(), value: row.value }))
          .filter((row) => row.key !== "")
          .map((row) => `${row.key}=${row.value}`),
        env_files: splitList(envFiles),
        skills_mode: skillsMode || undefined,
        static_mcp: splitList(staticMcp),
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
            { id: "advanced", label: "Advanced" },
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
              <Field label="Template (optional)" htmlFor="cs-template" hint="Base image for the sandbox; pick a local template or type any reference.">
                <Input
                  id="cs-template"
                  value={template}
                  onChange={(event) => setTemplate(event.target.value)}
                  placeholder="myimage:v1.0"
                />
              </Field>
              <Field label="Policy profile (optional)" htmlFor="cs-profile" hint="sbx --profile name, not the UI profiles.">
                <Input id="cs-profile" value={profile} onChange={(event) => setProfile(event.target.value)} />
              </Field>
            </div>

            {(templates.data ?? []).length > 0 ? (
              <div className="flex flex-col gap-2">
                <span className="text-xs font-medium text-muted">Local templates</span>
                <div className="flex flex-wrap gap-1.5">
                  {(templates.data ?? []).map((item) => {
                    const reference = templateReference(item);
                    const selected = template === reference;
                    return (
                      <button
                        key={item.id}
                        type="button"
                        aria-pressed={selected}
                        onClick={() => setTemplate(selected ? "" : reference)}
                        className={cn(
                          "rounded-sm border px-2 py-1 font-mono text-xs transition-colors",
                          selected
                            ? "border-accent/40 bg-accent-soft text-accent"
                            : "border-border bg-canvas text-muted hover:border-accent hover:text-accent",
                        )}
                      >
                        {reference}
                      </button>
                    );
                  })}
                </div>
              </div>
            ) : null}

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
                  <Input
                    value={workspace.target ?? ""}
                    onChange={(event) =>
                      setWorkspaces((current) =>
                        current.map((item, i) => (i === index ? { ...item, target: event.target.value } : item)),
                      )
                    }
                    placeholder="container target (optional)"
                    className="w-44 font-mono text-xs"
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

        {tab === "advanced" ? (
          <div className="flex flex-col gap-5">
            <div className="flex flex-col gap-2">
              <span className="text-xs font-medium text-muted">Kit arguments</span>
              <span className="text-2xs text-faint">
                One row per argument the kit declares. A bare name applies to every kit; kit.name targets one kit.
                Selecting a kit below pre-fills its declared arguments.
              </span>
              {kitArgs.length === 0 ? (
                <p className="text-xs text-faint">No kit argument. Select a kit mixin or add a row.</p>
              ) : null}
              {kitArgs.map((row, index) => (
                <div key={`${row.key}-${index}`} className="flex flex-col gap-1">
                  <div className="flex items-center gap-2">
                    <Input
                      value={row.key}
                      onChange={(event) => updateKitArg(index, { key: event.target.value })}
                      placeholder="name or kit.name"
                      aria-label={`Kit argument ${index + 1} name`}
                      className="flex-1 font-mono text-xs"
                    />
                    <Input
                      value={row.value}
                      onChange={(event) => updateKitArg(index, { value: event.target.value })}
                      placeholder="value"
                      aria-label={`Kit argument ${index + 1} value`}
                      className="flex-1 font-mono text-xs"
                    />
                    {row.required ? <span className="text-2xs text-danger">required</span> : null}
                    <Button
                      size="icon"
                      variant="ghost"
                      title="Remove kit argument"
                      onClick={() => setKitArgs((current) => current.filter((_, i) => i !== index))}
                    >
                      <IconTrash className="size-3.5" />
                    </Button>
                  </div>
                  {row.description ? <span className="pl-1 text-2xs text-faint">{row.description}</span> : null}
                </div>
              ))}
              <div>
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => setKitArgs((current) => [...current, { key: "", value: "", auto: false }])}
                >
                  <IconPlus className="size-3.5" /> Add argument
                </Button>
              </div>
            </div>

            <Field
              label="Environment files (optional)"
              htmlFor="cs-env-files"
              hint="One path per line. Env vars are read from these files at create, keeping secrets out of spec.yaml."
            >
              <TextArea
                id="cs-env-files"
                rows={3}
                value={envFiles}
                onChange={(event) => setEnvFiles(event.target.value)}
                placeholder={"/home/you/.env"}
              />
            </Field>

            <div className="grid gap-4 sm:grid-cols-2">
              <Field label="Skills mode" htmlFor="cs-skills" hint="Shared skills store mount mode; sbx default when unset.">
                <Select id="cs-skills" value={skillsMode} onChange={(event) => setSkillsMode(event.target.value)}>
                  {SKILLS_MODES.map((mode) => (
                    <option key={mode.value} value={mode.value}>
                      {mode.label}
                    </option>
                  ))}
                </Select>
              </Field>
              <Field
                label="Static MCP servers (optional)"
                htmlFor="cs-static-mcp"
                hint="Comma-separated fixed MCP set chosen at creation; `none` is allowed."
              >
                <Input
                  id="cs-static-mcp"
                  value={staticMcp}
                  onChange={(event) => setStaticMcp(event.target.value)}
                  placeholder="notion, atlassian"
                />
              </Field>
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
