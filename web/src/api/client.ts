import { Desktop } from "@/bindings/github.com/JLugagne/sandwarden/internal/desktop";
import { Notify } from "@/bindings/github.com/JLugagne/sandwarden/internal/desktop/notifier";
import type { ConfigStaleness, StaleKind } from "@/lib/config";
import type {
  AppConfig,
  CacheInput,
  CacheView,
  CompleteSandboxRequest,
  CreateSandboxRequest,
  CustomSecretRequest,
  Health,
  ImportReport,
  ImportSecretsRequest,
  JobView,
  KitAddResult,
  KitItemView,
  KitStore,
  KitStoreInput,
  KitValidation,
  MountRef,
  PolicyActionResult,
  PolicyLog,
  PolicyRule,
  ProfileView,
  RegistrySecretRequest,
  SandboxDetail,
  SandboxSummary,
  SearchResult,
  SecretList,
  ServiceSecretRequest,
  SkillItem,
  SkillRef,
  SkillStore,
  SkillStoreInput,
  Template,
  Terminal,
  VersionInfo,
} from "@/types";

/**
 * Typed adapter over the Wails bindings. The public surface matches the old
 * REST client so pages did not have to change; the return shapes are kept
 * (`{ job_id }`, `{ result }`, …) because that is what the callers expect.
 * Identifiers are string slugs everywhere.
 */

const asArray = <T>(value: T[] | null | undefined): T[] => value ?? [];

type FleetCache = Awaited<ReturnType<typeof Desktop.ListCaches>> extends (infer T)[] | null ? T : never;

const cacheView = (cache: FleetCache): CacheView => ({
  slug: cache.Slug,
  dir: cache.Dir,
  name: cache.App.name,
  description: cache.App.description,
  host_path: cache.App.host_path,
  target_path: cache.App.target_path,
  read_only: cache.App.read_only,
  auto_attach: cache.App.auto_attach,
  enabled: cache.App.enabled,
});

const cacheInput = (input: CacheInput) => ({
  name: input.name,
  description: input.description,
  host_path: input.host_path,
  target_path: input.target_path,
  read_only: input.read_only,
  auto_attach: input.auto_attach,
  enabled: input.enabled,
});

export const api = {
  health: () => Desktop.Health() as Promise<Health>,
  versionInfo: () => Desktop.VersionInfo() as Promise<VersionInfo>,
  checkUpdates: (force = false) => Desktop.CheckUpdates(force) as Promise<VersionInfo>,
  startDaemon: () => Desktop.StartDaemon(),
  notify: (title: string, body: string) => Notify(title, body),

  getConfig: () => Desktop.GetConfig().then((config) => config as unknown as AppConfig),
  setConfig: (config: AppConfig) =>
    Desktop.SetConfig({
      terminals: {
        enabled: config.terminals.enabled,
        default: config.terminals.default,
      },
      notifications: config.notifications,
    }),
  fleetDir: () => Desktop.FleetDir(),
  sandboxConfigDir: (slug: string) => Desktop.SandboxConfigDir(slug),
  profileConfigDir: (slug: string) => Desktop.ProfileConfigDir(slug),
  readSandboxConfig: (name: string) => Desktop.ReadSandboxConfig(name).then((files) => asArray(files)),
  readProfileConfig: (slug: string) => Desktop.ReadProfileConfig(slug).then((files) => asArray(files)),
  reloadFleet: () => Desktop.ReloadFleet().then((errors) => asArray(errors)),
  configStaleness: () =>
    Desktop.ConfigStaleness().then(
      (report): ConfigStaleness => ({
        stale: Boolean(report?.stale),
        changed: asArray(report?.changed).map((file) => ({
          kind: String(file.kind) as StaleKind,
          slug: file.slug,
          name: file.name,
          file: file.file,
          path: file.path,
        })),
      }),
    ),

  terminals: () => Desktop.ListTerminals().then((rows) => asArray(rows) as unknown as Terminal[]),
  openInTerminal: (terminalId: string, dir: string, command: string) =>
    Desktop.OpenInTerminal(terminalId, dir, command),

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
      kits: asArray(body.kits),
      profiles: asArray(body.profiles),
      caches: asArray(body.caches),
      skills: asArray(body.skills),
      run_args: body.run_args ?? "",
      mounts: asArray(body.mounts),
      publish: asArray(body.publish),
      env: asArray(body.env),
      deny_network: asArray(body.deny_network),
      clone: body.clone ?? false,
      attach_caches: body.attach_caches ?? false,
      job_id: body.job_id ?? "",
    }).then((jobID) => ({ job_id: jobID })),
  deleteSandbox: (name: string, force = false, purgeConfig = false) =>
    Desktop.DeleteSandbox(name, force, purgeConfig),
  importSandboxes: (names: string[] = []) =>
    Desktop.ImportSandboxes({ names }).then((report) => report as unknown as ImportReport),
  completeSandboxConfig: (name: string, body: CompleteSandboxRequest) =>
    Desktop.CompleteSandboxConfig(name, body),
  startSandbox: (name: string) => Desktop.StartSandbox(name),
  stopSandbox: (name: string) => Desktop.StopSandbox(name),
  applySandbox: (name: string, jobID: string) =>
    Desktop.ApplySandbox(name, jobID).then((id) => ({ job_id: id })),
  recreateSandbox: (name: string, jobID: string) =>
    Desktop.RecreateSandbox(name, jobID).then((id) => ({ job_id: id })),
  attachKit: (name: string, ref: string) =>
    Desktop.AttachKit(name, ref).then((result) => result as unknown as KitAddResult),
  validateSandbox: (slug: string) =>
    Desktop.ValidateSandbox(slug).then((result) => result as unknown as KitValidation),
  validateProfile: (slug: string) =>
    Desktop.ValidateProfile(slug).then((result) => result as unknown as KitValidation),
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
  setSandboxRunArgs: (name: string, args: string) => Desktop.SetSandboxRunArgs(name, args),
  assignProfile: (name: string, profileSlug: string) => Desktop.AssignProfile(name, profileSlug),
  unassignProfile: (name: string, profileSlug: string) => Desktop.UnassignProfile(name, profileSlug),
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
    slug: string,
    body: { name: string; description: string; is_default: boolean; is_global: boolean },
  ) => Desktop.UpdateProfile(slug, body).then((profile) => profile as unknown as ProfileView),
  deleteProfile: (slug: string) => Desktop.DeleteProfile(slug),
  addProfileMount: (profileSlug: string, body: { host_path: string; target_path: string; read_only: boolean }) =>
    Desktop.AddProfileMount(profileSlug, body).then((mount) => mount as unknown as MountRef),
  removeProfileMount: (profileSlug: string, hostPath: string, targetPath: string) =>
    Desktop.RemoveProfileMount(profileSlug, hostPath, targetPath),
  addProfileCache: (profileSlug: string, cacheSlug: string) =>
    Desktop.AddProfileCache(profileSlug, cacheSlug),
  removeProfileCache: (profileSlug: string, cacheSlug: string) =>
    Desktop.RemoveProfileCache(profileSlug, cacheSlug),
  detachProfileMount: (name: string, hostPath: string, targetPath: string) =>
    Desktop.DetachProfileMount(name, hostPath, targetPath),
  applyProfileMount: (name: string, hostPath: string, targetPath: string) =>
    Desktop.ApplyProfileMount(name, hostPath, targetPath),
  detachProfileCache: (name: string, cacheSlug: string) => Desktop.DetachProfileCache(name, cacheSlug),
  applyProfileCache: (name: string, cacheSlug: string) => Desktop.ApplyProfileCache(name, cacheSlug),
  addRule: (slug: string, body: { decision: string; pattern: string }) =>
    Desktop.AddRule(slug, body).then(() => undefined),
  removeRule: (slug: string, body: { decision: string; pattern: string }) =>
    Desktop.RemoveRule(slug, body),

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

  caches: () => Desktop.ListCaches().then((rows) => asArray(rows).map(cacheView)),
  createCache: (body: CacheInput) => Desktop.CreateCache(cacheInput(body)).then(cacheView),
  updateCache: (slug: string, body: CacheInput) =>
    Desktop.UpdateCache(slug, cacheInput(body)).then(cacheView),
  deleteCache: (slug: string) => Desktop.DeleteCache(slug),
  attachCache: (name: string, cacheSlug: string) => Desktop.AttachCache(name, cacheSlug),
  detachCache: (name: string, cacheSlug: string) => Desktop.DetachCache(name, cacheSlug),
  reapplyCaches: (name: string) =>
    Desktop.ReapplyCaches(name).then((result) => ({
      applied: result.applied,
      errors: asArray(result.errors),
    })),

  skillStores: () => Desktop.ListSkillStores().then((rows) => asArray(rows) as unknown as SkillStore[]),
  skillItems: async (storeSlug = "") => {
    const [stores, rows] = await Promise.all([
      Desktop.ListSkillStores(),
      Desktop.ListSkillItems(storeSlug),
    ]);
    const names = new Map(asArray(stores).map((store) => [store.slug, store.name]));
    return asArray(rows).map(
      (item): SkillItem => ({ ...item, store_name: names.get(item.store) ?? item.store }),
    );
  },
  createSkillStore: (body: SkillStoreInput) =>
    Desktop.CreateSkillStore({
      name: body.name,
      description: body.description,
      url: body.url,
      ref: body.ref,
      auth: body.auth,
    }).then((store) => store as unknown as SkillStore),
  updateSkillStore: (slug: string, body: SkillStoreInput) =>
    Desktop.UpdateSkillStore(slug, {
      name: body.name,
      description: body.description,
      url: body.url,
      ref: body.ref,
      auth: body.auth,
    }).then((store) => store as unknown as SkillStore),
  deleteSkillStore: (slug: string) => Desktop.DeleteSkillStore(slug),
  refreshSkillStore: (slug: string) =>
    Desktop.RefreshSkillStore(slug).then((store) => store as unknown as SkillStore),
  addProfileSkillItem: (profileSlug: string, ref: SkillRef) =>
    Desktop.AddProfileSkillItem(profileSlug, ref),
  removeProfileSkillItem: (profileSlug: string, ref: SkillRef) =>
    Desktop.RemoveProfileSkillItem(profileSlug, ref),
  attachSkillItem: (name: string, ref: SkillRef) => Desktop.AttachSkillItem(name, ref),
  detachSkillItem: (name: string, ref: SkillRef) => Desktop.DetachSkillItem(name, ref),
  reconcileSkills: (name: string) =>
    Desktop.ReconcileSkills(name).then((result) => ({
      applied: result.applied,
      removed: result.removed,
      errors: asArray(result.errors),
    })),

  kitStores: () => Desktop.ListKitStores().then((rows) => asArray(rows) as unknown as KitStore[]),
  kitItems: (storeSlug = "") =>
    Desktop.ListKitItems(storeSlug).then((rows) => asArray(rows) as unknown as KitItemView[]),
  createKitStore: (body: KitStoreInput) =>
    Desktop.CreateKitStore({
      name: body.name,
      description: body.description,
      url: body.url,
      ref: body.ref,
      auth: body.auth,
    }).then((store) => store as unknown as KitStore),
  updateKitStore: (slug: string, body: KitStoreInput) =>
    Desktop.UpdateKitStore(slug, {
      name: body.name,
      description: body.description,
      url: body.url,
      ref: body.ref,
      auth: body.auth,
    }).then((store) => store as unknown as KitStore),
  deleteKitStore: (slug: string) => Desktop.DeleteKitStore(slug),
  refreshKitStore: (slug: string) =>
    Desktop.RefreshKitStore(slug).then((store) => store as unknown as KitStore),
  validateKit: (storeSlug: string, name: string) =>
    Desktop.KitValidate(storeSlug, name).then((result) => result as unknown as KitValidation),

  search: (query: string) =>
    Desktop.Search(query).then((rows) => asArray(rows) as unknown as SearchResult[]),

  templates: () => Desktop.ListTemplates().then((rows) => asArray(rows) as unknown as Template[]),
  removeTemplate: (ref: string) => Desktop.RemoveTemplate(ref),
};
