import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import { api } from "@/api/client";
import { useApiMutation } from "@/hooks/useApiMutation";
import { queryKeys } from "@/store/realtime";
import { useJob } from "@/hooks/useJob";
import { jobHub, newJobId } from "@/store/jobs";
import { useStickToBottom } from "@/components/Toaster";
import {
  Badge,
  Button,
  CheckboxField,
  ConfirmDialog,
  EmptyState,
  Field,
  Input,
  LogView,
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
import type { CustomSecret, Secret } from "@/types";

const SERVICES = [
  "anthropic",
  "copilot",
  "cursor",
  "devin",
  "droid",
  "github",
  "google",
  "groq",
  "mistral",
  "nebius",
  "openai",
  "openrouter",
  "xai",
];

type ScopeValue = "all" | "global" | string;

export function SecretsPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const scope: ScopeValue = searchParams.get("sandbox") ?? "all";
  const addTab = searchParams.get("add") ?? "service";

  const secrets = useQuery({ queryKey: queryKeys.secrets, queryFn: api.secrets });
  const sandboxes = useQuery({ queryKey: queryKeys.sandboxes, queryFn: api.listSandboxes });

  const [deleting, setDeleting] = useState<{ kind: "secret"; secret: Secret } | { kind: "custom"; secret: CustomSecret } | null>(null);

  const removeSecret = useApiMutation({
    mutationFn: (secret: Secret) =>
      secret.type === "registry"
        ? api.removeRegistrySecret(secret.scope, secret.name)
        : api.removeSecret(secret.scope, secret.name),
    success: "Secret removed",
    onSuccess: () => setDeleting(null),
  });
  const removeCustom = useApiMutation({
    mutationFn: (secret: CustomSecret) => api.removeCustomSecret(secret.scope, secret.placeholder),
    success: "Custom secret removed",
    onSuccess: () => setDeleting(null),
  });

  const stored = useMemo(() => {
    const list = secrets.data?.stored ?? [];
    if (scope === "all") return list;
    return list.filter((secret) => secret.scope === (scope === "global" ? "" : scope));
  }, [secrets.data, scope]);

  const custom = useMemo(() => {
    const list = secrets.data?.custom ?? [];
    if (scope === "all") return list;
    return list.filter((secret) => secret.scope === (scope === "global" ? "" : scope));
  }, [secrets.data, scope]);

  function scopeLabel(value: string): string {
    if (value === "") return "global";
    return value;
  }

  return (
    <>
      <PageHeader
        title="Secrets"
        subtitle="Service tokens, registry credentials and proxy-injected custom secrets. Values are sent to sbx over stdin and never stored by this UI."
      />

      <Panel className="mb-4">
        <div className="flex flex-wrap items-center gap-3">
          <span className="text-xs font-medium text-muted">Scope</span>
          <Select
            value={scope}
            onChange={(event) => {
              const value = event.target.value;
              const next = new URLSearchParams(searchParams);
              if (value === "all") next.delete("sandbox");
              else next.set("sandbox", value);
              setSearchParams(next, { replace: true });
            }}
            className="w-64"
          >
            <option value="all">All scopes</option>
            <option value="global">Global</option>
            {(sandboxes.data ?? []).map((sandbox) => (
              <option key={sandbox.name} value={sandbox.name}>
                {sandbox.name}
              </option>
            ))}
          </Select>
          <span className="text-xs text-faint">Secrets scoped to a sandbox are only injected there.</span>
        </div>
      </Panel>

      <div className="mb-4">
        <Tabs
          tabs={[
            { id: "service", label: "Service" },
            { id: "dynamic", label: "Dynamic" },
            { id: "registry", label: "Registry" },
            { id: "custom", label: "Custom" },
            { id: "import", label: "Import" },
          ]}
          active={addTab}
          onChange={(next) => {
            const params = new URLSearchParams(searchParams);
            params.set("add", next);
            setSearchParams(params, { replace: true });
          }}
        />
      </div>

      <div className="mb-4">
        {addTab === "service" ? <ServiceForm scope={scope} /> : null}
        {addTab === "dynamic" ? <DynamicForm scope={scope} /> : null}
        {addTab === "registry" ? <RegistryForm scope={scope} /> : null}
        {addTab === "custom" ? <CustomForm scope={scope} /> : null}
        {addTab === "import" ? <ImportForm /> : null}
      </div>

      <Panel title="Stored secrets" bodyClassName="p-0">
        {secrets.isLoading ? (
          <div className="flex justify-center p-6">
            <Spinner />
          </div>
        ) : stored.length === 0 && custom.length === 0 ? (
          <EmptyState title="No secrets in this scope." className="rounded-none border-0" />
        ) : (
          <>
            {stored.length > 0 ? (
              <TableWrap className="border-0 border-b border-border">
                <thead>
                  <tr>
                    <TH>Scope</TH>
                    <TH>Type</TH>
                    <TH>Name</TH>
                    <TH>Secret</TH>
                    <TH className="w-24" />
                  </tr>
                </thead>
                <tbody>
                  {stored.map((secret) => (
                    <TRow key={`${secret.scope}:${secret.type}:${secret.name}`}>
                      <TD>
                        <Badge tone={secret.scope === "" ? "neutral" : "accent"}>{scopeLabel(secret.scope)}</Badge>
                      </TD>
                      <TD className="text-xs text-muted">{secret.type}</TD>
                      <TD className="font-mono text-xs">{secret.name}</TD>
                      <TD className="font-mono text-xs text-muted">{secret.masked}</TD>
                      <TD className="text-right">
                        <Button size="sm" variant="ghost" onClick={() => setDeleting({ kind: "secret", secret })}>
                          <span className="text-danger">Remove</span>
                        </Button>
                      </TD>
                    </TRow>
                  ))}
                </tbody>
              </TableWrap>
            ) : null}
            {custom.length > 0 ? (
              <TableWrap className="border-0">
                <thead>
                  <tr>
                    <TH>Scope</TH>
                    <TH>Hosts</TH>
                    <TH>Env</TH>
                    <TH>Placeholder</TH>
                    <TH>Secret</TH>
                    <TH className="w-24" />
                  </tr>
                </thead>
                <tbody>
                  {custom.map((secret) => (
                    <TRow key={`${secret.scope}:${secret.placeholder}`}>
                      <TD>
                        <Badge tone={secret.scope === "" ? "neutral" : "accent"}>{scopeLabel(secret.scope)}</Badge>
                      </TD>
                      <TD className="font-mono text-xs">{(secret.targets ?? []).join(", ")}</TD>
                      <TD className="font-mono text-xs">{secret.env}</TD>
                      <TD className="font-mono text-xs text-muted">{secret.placeholder || "—"}</TD>
                      <TD className="font-mono text-xs text-muted">{secret.masked}</TD>
                      <TD className="text-right">
                        <Button size="sm" variant="ghost" onClick={() => setDeleting({ kind: "custom", secret })}>
                          <span className="text-danger">Remove</span>
                        </Button>
                      </TD>
                    </TRow>
                  ))}
                </tbody>
              </TableWrap>
            ) : null}
          </>
        )}
      </Panel>

      <ConfirmDialog
        open={deleting !== null}
        title="Remove this secret?"
        body={
          deleting?.kind === "secret" ? (
            <span className="font-mono text-xs">
              {deleting.secret.name} ({scopeLabel(deleting.secret.scope)})
            </span>
          ) : deleting?.kind === "custom" ? (
            <span className="font-mono text-xs">
              {deleting.secret.env} ({scopeLabel(deleting.secret.scope)})
            </span>
          ) : null
        }
        confirmLabel="Remove"
        busy={removeSecret.isPending || removeCustom.isPending}
        onConfirm={() => {
          if (deleting?.kind === "secret") removeSecret.mutate(deleting.secret);
          if (deleting?.kind === "custom") removeCustom.mutate(deleting.secret);
        }}
        onClose={() => setDeleting(null)}
      />
    </>
  );
}

function formScope(scope: ScopeValue, fallback: ScopeValue): ScopeValue {
  return scope === "all" ? fallback : scope;
}

/** Form scope picker. The page-level "all" filter is not a valid write scope. */
function ScopeSelect({ value, onChange }: { value: ScopeValue; onChange: (value: ScopeValue) => void }) {
  const sandboxes = useQuery({ queryKey: queryKeys.sandboxes, queryFn: api.listSandboxes });
  return (
    <Select value={value} onChange={(event) => onChange(event.target.value as ScopeValue)} className="w-48">
      <option value="global">Global</option>
      {(sandboxes.data ?? []).map((sandbox) => (
        <option key={sandbox.name} value={sandbox.name}>
          {sandbox.name}
        </option>
      ))}
    </Select>
  );
}

function ServiceForm({ scope }: { scope: ScopeValue }) {
  const [selectedScope, setSelectedScope] = useState<ScopeValue>(() => formScope(scope, "global"));
  useEffect(() => setSelectedScope(formScope(scope, "global")), [scope]);
  const [service, setService] = useState("github");
  const [value, setValue] = useState("");
  const [overwrite, setOverwrite] = useState(false);
  const save = useApiMutation({
    mutationFn: () => api.setServiceSecret({ service: service.trim(), scope: selectedScope, value, overwrite }),
    success: "Service secret saved",
    onSuccess: () => setValue(""),
  });

  return (
    <Panel title="Service secret" description="The proxy authenticates API requests on the agent's behalf; the secret never enters the sandbox.">
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Service">
          <Input list="sbx-services" value={service} onChange={(event) => setService(event.target.value)} />
          <datalist id="sbx-services">
            {SERVICES.map((name) => (
              <option key={name} value={name} />
            ))}
          </datalist>
        </Field>
        <Field label="Scope">
          <ScopeSelect value={selectedScope} onChange={setSelectedScope} />
        </Field>
      </div>
      <div className="mt-3">
        <Field label="API key / token">
          <Input type="password" autoComplete="off" value={value} onChange={(event) => setValue(event.target.value)} />
        </Field>
      </div>
      <div className="mt-3 flex items-center gap-4">
        <CheckboxField label="overwrite if it exists" checked={overwrite} onChange={setOverwrite} />
        <Button variant="primary" disabled={!service.trim() || !value} loading={save.isPending} onClick={() => save.mutate()}>
          Save
        </Button>
      </div>
    </Panel>
  );
}

function DynamicForm({ scope }: { scope: ScopeValue }) {
  const [selectedScope, setSelectedScope] = useState<ScopeValue>(() => formScope(scope, "global"));
  useEffect(() => setSelectedScope(formScope(scope, "global")), [scope]);
  const [service, setService] = useState("github");
  const [kind, setKind] = useState("command");
  const [source, setSource] = useState("");
  const [refresh, setRefresh] = useState("");
  const [overwrite, setOverwrite] = useState(false);
  const save = useApiMutation({
    mutationFn: () =>
      api.setServiceSecret({
        service: service.trim(),
        scope: selectedScope,
        command: kind === "command" ? source : undefined,
        ref: kind === "ref" ? source : undefined,
        refresh: refresh.trim() || undefined,
        overwrite,
      }),
    success: "Dynamic secret saved",
    onSuccess: () => setSource(""),
  });

  return (
    <Panel title="Dynamic secret" description="Resolve the value on demand from a command or a 1Password/AWS reference.">
      <div className="grid gap-3 sm:grid-cols-3">
        <Field label="Service">
          <Input list="sbx-services-dyn" value={service} onChange={(event) => setService(event.target.value)} />
          <datalist id="sbx-services-dyn">
            {SERVICES.map((name) => (
              <option key={name} value={name} />
            ))}
          </datalist>
        </Field>
        <Field label="Source">
          <Select value={kind} onChange={(event) => setKind(event.target.value)}>
            <option value="command">command</option>
            <option value="ref">ref (1Password / AWS)</option>
          </Select>
        </Field>
        <Field label="Scope">
          <ScopeSelect value={selectedScope} onChange={setSelectedScope} />
        </Field>
      </div>
      <div className="mt-3 grid gap-3 sm:grid-cols-2">
        <Field label={kind === "command" ? "Command output" : "op://… or arn:…"}>
          <Input value={source} onChange={(event) => setSource(event.target.value)} className="font-mono text-xs" />
        </Field>
        <Field label="Refresh (optional)" hint="e.g. 30m or on-demand.">
          <Input value={refresh} onChange={(event) => setRefresh(event.target.value)} />
        </Field>
      </div>
      <div className="mt-3 flex items-center gap-4">
        <CheckboxField label="overwrite" checked={overwrite} onChange={setOverwrite} />
        <Button variant="primary" disabled={!service.trim() || !source.trim()} loading={save.isPending} onClick={() => save.mutate()}>
          Save
        </Button>
      </div>
    </Panel>
  );
}

function RegistryForm({ scope }: { scope: ScopeValue }) {
  const [selectedScope, setSelectedScope] = useState<ScopeValue>(() => formScope(scope, "host-only"));
  useEffect(() => setSelectedScope(formScope(scope, "host-only")), [scope]);
  const [host, setHost] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [overwrite, setOverwrite] = useState(false);
  const save = useApiMutation({
    mutationFn: () => api.setRegistrySecret({ host: host.trim(), username: username.trim(), password, scope: selectedScope, overwrite }),
    success: "Registry credential saved",
    onSuccess: () => setPassword(""),
  });

  return (
    <Panel title="Registry credential" description="Used for pulls. Host-only by default: never injected into sandboxes.">
      <div className="grid gap-3 sm:grid-cols-3">
        <Field label="Registry host">
          <Input value={host} onChange={(event) => setHost(event.target.value)} placeholder="ghcr.io" />
        </Field>
        <Field label="Username (optional)">
          <Input value={username} onChange={(event) => setUsername(event.target.value)} />
        </Field>
        <Field label="Scope">
          <Select value={selectedScope} onChange={(event) => setSelectedScope(event.target.value)}>
            <option value="host-only">host only (pulls)</option>
            <option value="global">all sandboxes</option>
            {(useSandboxNames()).map((name) => (
              <option key={name} value={name}>
                {name}
              </option>
            ))}
          </Select>
        </Field>
      </div>
      <div className="mt-3">
        <Field label="Password / token">
          <Input type="password" autoComplete="off" value={password} onChange={(event) => setPassword(event.target.value)} />
        </Field>
      </div>
      <div className="mt-3 flex items-center gap-4">
        <CheckboxField label="overwrite if it exists" checked={overwrite} onChange={setOverwrite} />
        <Button variant="primary" disabled={!host.trim() || !password} loading={save.isPending} onClick={() => save.mutate()}>
          Save
        </Button>
      </div>
    </Panel>
  );
}

function CustomForm({ scope }: { scope: ScopeValue }) {
  const [selectedScope, setSelectedScope] = useState<ScopeValue>(() => formScope(scope, "global"));
  useEffect(() => setSelectedScope(formScope(scope, "global")), [scope]);
  const [hosts, setHosts] = useState("");
  const [env, setEnv] = useState("");
  const [kind, setKind] = useState("command");
  const [source, setSource] = useState("");
  const [placeholder, setPlaceholder] = useState("");
  const [overwrite, setOverwrite] = useState(false);
  const save = useApiMutation({
    mutationFn: () =>
      api.setCustomSecret({
        hosts: hosts
          .split(/[,\n]/)
          .map((value) => value.trim())
          .filter(Boolean),
        env: env.trim(),
        command: kind === "command" ? source : undefined,
        ref: kind === "ref" ? source : undefined,
        value: kind === "value" ? source : undefined,
        placeholder: placeholder.trim() || undefined,
        scope: selectedScope,
        overwrite,
      }),
    success: "Custom secret saved",
    onSuccess: () => setSource(""),
  });

  return (
    <Panel
      title="Custom secret"
      description="The sandbox sees a placeholder; the proxy swaps it for the real value on requests to the given hosts. Prefer command/ref over a literal value."
    >
      <div className="grid gap-3 sm:grid-cols-3">
        <Field label="Hosts" hint="Comma separated, wildcards allowed.">
          <Input value={hosts} onChange={(event) => setHosts(event.target.value)} placeholder="*.example.com" className="font-mono text-xs" />
        </Field>
        <Field label="Env var">
          <Input value={env} onChange={(event) => setEnv(event.target.value)} placeholder="API_KEY" className="font-mono text-xs" />
        </Field>
        <Field label="Scope">
          <ScopeSelect value={selectedScope} onChange={setSelectedScope} />
        </Field>
      </div>
      <div className="mt-3 grid gap-3 sm:grid-cols-3">
        <Field label="Source">
          <Select value={kind} onChange={(event) => setKind(event.target.value)}>
            <option value="command">command</option>
            <option value="ref">ref</option>
            <option value="value">literal value</option>
          </Select>
        </Field>
        <Field label="Command / ref / value">
          <Input value={source} onChange={(event) => setSource(event.target.value)} className="font-mono text-xs" />
        </Field>
        <Field label="Placeholder (optional)" hint="{rand} is supported.">
          <Input value={placeholder} onChange={(event) => setPlaceholder(event.target.value)} className="font-mono text-xs" />
        </Field>
      </div>
      <div className="mt-3 flex items-center gap-4">
        <CheckboxField label="overwrite (removes by placeholder first)" checked={overwrite} onChange={setOverwrite} />
        <Button
          variant="primary"
          disabled={!hosts.trim() || !env.trim() || !source.trim()}
          loading={save.isPending}
          onClick={() => save.mutate()}
        >
          Save
        </Button>
      </div>
    </Panel>
  );
}

function useSandboxNames(): string[] {
  const sandboxes = useQuery({ queryKey: queryKeys.sandboxes, queryFn: api.listSandboxes });
  return (sandboxes.data ?? []).map((sandbox) => sandbox.name);
}

function ImportForm() {
  const [jobId, setJobId] = useState<string | null>(null);
  const job = useJob(jobId);
  const logRef = useStickToBottom<HTMLPreElement>(job.output);

  async function start(dryRun: boolean, force: boolean) {
    const id = newJobId();
    jobHub.reset(id);
    setJobId(id);
    await api.importSecrets({ dry_run: dryRun, force, job_id: id }).catch(() => undefined);
  }

  return (
    <Panel
      title="Import from host environment"
      description="Detects credential env vars on the host (OPENAI_API_KEY, GH_TOKEN, …) and imports them into the global scope."
    >
      <div className="flex flex-wrap items-center gap-2">
        <Button onClick={() => void start(true, false)}>Scan (dry-run)</Button>
        <Button variant="primary" onClick={() => void start(false, false)}>
          Import new
        </Button>
        <Button onClick={() => void start(false, true)}>Import + overwrite</Button>
      </div>
      {jobId ? (
        <div className="mt-3">
          <LogView text={job.output} empty="Waiting for output…" autoScrollRef={logRef} />
        </div>
      ) : null}
    </Panel>
  );
}
