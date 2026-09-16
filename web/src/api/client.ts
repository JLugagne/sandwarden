import { Desktop } from "@/bindings/github.com/JLugagne/sandwarden/internal/desktop";
import { Notify } from "@/bindings/github.com/JLugagne/sandwarden/internal/desktop/notifier";
import type {
  CacheInput,
  CacheMount,
  CreateSandboxRequest,
  CustomSecretRequest,
  Health,
  ImportSecretsRequest,
  JobView,
  PolicyActionResult,
  PolicyLog,
  PolicyRule,
  ProfileView,
  RegistrySecretRequest,
  Rule,
  SandboxDetail,
  SandboxSummary,
  SecretList,
  ServiceSecretRequest,
  SkillItem,
  SkillStore,
  SkillStoreInput,
} from "@/types";

/**
 * Typed adapter over the Wails bindings. The public surface matches the old
 * REST client so pages did not have to change; the return shapes are kept
 * (`{ job_id }`, `{ result }`, …) because that is what the callers expect.
 */

const asArray = <T>(value: T[] | null | undefined): T[] => value ?? [];

export const api = {
  health: () => Desktop.Health() as Promise<Health>,
  notify: (title: string, body: string) => Notify(title, body),

  listSandboxes: () =>
    Desktop.ListSandboxes().then((rows) => asArray(rows) as unknown as SandboxSummary[]),
  sandbox: (name: string) =>
    Desktop.SandboxDetail(name).then((detail) => detail as unknown as SandboxDetail),
  createSandbox: (body: CreateSandboxRequest) =>
    Desktop.CreateSandbox({
      agent: body.agent,
      workspaces: asArray(body.workspaces).map((workspace) => ({
        path: workspace.path,
        read_only: workspace.read_only,
      })),
      name: body.name ?? "",
      cpus: body.cpus ?? 0,
      memory: body.memory ?? "",
      profile: body.profile ?? "",
      template: body.template ?? "",
      publish: asArray(body.publish),
      env: asArray(body.env),
      deny_network: asArray(body.deny_network),
      clone: body.clone ?? false,
      attach_caches: body.attach_caches ?? false,
      job_id: body.job_id ?? "",
    }).then((jobID) => ({ job_id: jobID })),
  deleteSandbox: (name: string, force = false) => Desktop.DeleteSandbox(name, force),
  startSandbox: (name: string) => Desktop.StartSandbox(name),
  stopSandbox: (name: string) => Desktop.StopSandbox(name),
  addMount: (name: string, body: { path: string; target?: string; read_only: boolean }) =>
    Desktop.AddMount(name, {
      path: body.path,
      target: body.target ?? "",
      read_only: body.read_only,
    }),
  removeMount: (name: string, path: string, target?: string) =>
    Desktop.RemoveMount(name, path, target ?? ""),
  exec: (name: string, command: string, jobId: string) =>
    Desktop.Exec(name, { command, job_id: jobId }).then((jobID) => ({ job_id: jobID })),
  assignProfile: (name: string, profileId: number) => Desktop.AssignProfile(name, profileId),
  unassignProfile: (name: string, profileId: number) => Desktop.UnassignProfile(name, profileId),
  sandboxPolicy: (name: string) =>
    Desktop.SandboxPolicy(name).then((rules) => asArray(rules) as unknown as PolicyRule[]),
  sandboxPolicyAction: (name: string, action: string, resources: string[]) =>
    Desktop.SandboxPolicyAction(name, action, resources).then((results) => ({
      results: asArray(results) as unknown as PolicyActionResult[],
    })),
  sandboxTraffic: (name: string) =>
    Desktop.SandboxTraffic(name).then((log) => log as unknown as PolicyLog),

  profiles: () =>
    Desktop.ListProfiles().then((rows) => asArray(rows) as unknown as ProfileView[]),
  createProfile: (body: { name: string; description: string; is_default: boolean; is_global: boolean }) =>
    Desktop.CreateProfile(body).then((profile) => profile as unknown as ProfileView),
  updateProfile: (
    id: number,
    body: { name: string; description: string; is_default: boolean; is_global: boolean },
  ) => Desktop.UpdateProfile(id, body).then((profile) => profile as unknown as ProfileView),
  deleteProfile: (id: number) => Desktop.DeleteProfile(id),
  addRule: (profileId: number, body: { decision: string; pattern: string }) =>
    Desktop.AddRule(profileId, body).then((rule) => rule as unknown as Rule),
  removeRule: (profileId: number, ruleId: number) => Desktop.RemoveRule(profileId, ruleId),

  secrets: () => Desktop.ListSecrets("").then((list) => list as unknown as SecretList),
  setServiceSecret: (body: ServiceSecretRequest) =>
    Desktop.SetServiceSecret({
      service: body.service,
      scope: body.scope,
      value: body.value ?? "",
      ref: body.ref ?? "",
      command: body.command ?? "",
      refresh: body.refresh ?? "",
      overwrite: body.overwrite ?? false,
    }),
  setRegistrySecret: (body: RegistrySecretRequest) =>
    Desktop.SetRegistrySecret({
      host: body.host,
      username: body.username ?? "",
      password: body.password,
      scope: body.scope,
      overwrite: body.overwrite ?? false,
    }),
  setCustomSecret: (body: CustomSecretRequest) =>
    Desktop.SetCustomSecret({
      hosts: asArray(body.hosts),
      env: body.env,
      value: body.value ?? "",
      ref: body.ref ?? "",
      command: body.command ?? "",
      placeholder: body.placeholder ?? "",
      refresh: body.refresh ?? "",
      scope: body.scope,
      overwrite: body.overwrite ?? false,
    }),
  removeSecret: (scope: string, name: string) => Desktop.RemoveSecret(scope, name),
  removeRegistrySecret: (scope: string, host: string) => Desktop.RemoveRegistrySecret(scope, host),
  removeCustomSecret: (scope: string, placeholder: string) =>
    Desktop.RemoveCustomSecret(scope, placeholder),
  importSecrets: (body: ImportSecretsRequest) =>
    Desktop.ImportSecrets({
      service: body.service ?? "",
      all: body.all ?? false,
      dry_run: body.dry_run ?? false,
      force: body.force ?? false,
      job_id: body.job_id ?? "",
    }).then((jobID) => ({ job_id: jobID })),

  traffic: () => Desktop.Traffic().then((log) => log as unknown as PolicyLog),
  policyRules: (sandbox?: string) =>
    Desktop.PolicyRules(sandbox ?? "").then((rules) => asArray(rules) as unknown as PolicyRule[]),
  policyAction: (body: { action: string; resources?: string[]; sandbox_id?: string; id?: string }) =>
    Desktop.PolicyAction({
      action: body.action,
      resources: body.resources ?? [],
      sandbox_id: body.sandbox_id ?? "",
      id: body.id ?? "",
    }).then((results) => ({
      results: asArray(results) as unknown as PolicyActionResult[],
    })),

  job: (id: string) => Desktop.Job(id).then((job) => job as unknown as JobView),
  cancelJob: (id: string) => Desktop.CancelJob(id),
  fsPick: (start?: string) => Desktop.PickFolder(start ?? "").then((path) => ({ path })),

  caches: () => Desktop.ListCaches().then((rows) => asArray(rows) as unknown as CacheMount[]),
  createCache: (body: CacheInput) =>
    Desktop.CreateCache(body).then((cache) => cache as unknown as CacheMount),
  updateCache: (id: number, body: CacheInput) =>
    Desktop.UpdateCache(id, body).then((cache) => cache as unknown as CacheMount),
  deleteCache: (id: number) => Desktop.DeleteCache(id),
  attachCache: (name: string, cacheId: number) => Desktop.AttachCache(name, cacheId),
  detachCache: (name: string, cacheId: number) => Desktop.DetachCache(name, cacheId),
  reapplyCaches: (name: string) =>
    Desktop.ReapplyCaches(name).then((result) => ({
      applied: result.applied,
      errors: asArray(result.errors),
    })),

  skillStores: () => Desktop.ListSkillStores().then((rows) => asArray(rows) as unknown as SkillStore[]),
  skillItems: (storeId = 0) =>
    Desktop.ListSkillItems(storeId).then((rows) => asArray(rows) as unknown as SkillItem[]),
  createSkillStore: (body: SkillStoreInput) =>
    Desktop.CreateSkillStore({
      name: body.name,
      description: body.description,
      url: body.url,
      ref: body.ref,
    }).then((store) => store as unknown as SkillStore),
  updateSkillStore: (id: number, body: SkillStoreInput) =>
    Desktop.UpdateSkillStore(id, {
      name: body.name,
      description: body.description,
      url: body.url,
      ref: body.ref,
    }).then((store) => store as unknown as SkillStore),
  deleteSkillStore: (id: number) => Desktop.DeleteSkillStore(id),
  refreshSkillStore: (id: number) =>
    Desktop.RefreshSkillStore(id).then((store) => store as unknown as SkillStore),
  addProfileSkillItem: (profileId: number, itemId: number) =>
    Desktop.AddProfileSkillItem(profileId, itemId),
  removeProfileSkillItem: (profileId: number, itemId: number) =>
    Desktop.RemoveProfileSkillItem(profileId, itemId),
  attachSkillItem: (name: string, itemId: number) => Desktop.AttachSkillItem(name, itemId),
  detachSkillItem: (name: string, itemId: number) => Desktop.DetachSkillItem(name, itemId),
  reconcileSkills: (name: string) =>
    Desktop.ReconcileSkills(name).then((result) => ({
      applied: result.applied,
      removed: result.removed,
      errors: asArray(result.errors),
    })),
};
