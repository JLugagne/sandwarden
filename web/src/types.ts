export interface PublishedPort {
  host_ip: string;
  host_port: number;
  protocol: string;
  sandbox_port: number;
}

export interface WorkspaceMount {
  dir: string;
  read_only?: boolean;
}

export interface MountInfo {
  host_path: string;
  container_target?: string;
  read_only?: boolean;
}

export interface ConnectInfo {
  run: string;
  shell: string;
}

export interface SandboxSummary {
  name: string;
  id: string;
  agent?: string;
  status: string;
  running: boolean;
  workspace: string;
  daemon_profile?: string;
  created_at?: string;
  stopped_at?: string;
  ports?: PublishedPort[] | null;
  mount_policy_denied: boolean;
  /** Config directory adopted from the daemon without its original create parameters. */
  incomplete?: boolean;
  profiles: string[] | null;
  run_args: string;
  connect: ConnectInfo;
  cpu_percent: number;
  memory_used_bytes: number;
  memory_total_bytes: number;
}

/** A host bind mount; an empty target_path keeps the host path. */
export interface MountRef {
  host_path: string;
  target_path: string;
  read_only: boolean;
}

/** One catalog item: store slug, kind and name identify it everywhere. */
export interface SkillRef {
  store: string;
  kind: string;
  name: string;
}

/** A profile as referenced by one sandbox. */
export interface ProfileRef {
  slug: string;
  name: string;
  default: boolean;
  global: boolean;
}

/** A profile file plus its derived links. */
export interface ProfileView {
  slug: string;
  name: string;
  description: string;
  default: boolean;
  global: boolean;
  allow: string[] | null;
  deny: string[] | null;
  mounts: MountRef[] | null;
  caches: string[] | null;
  skills: SkillRef[] | null;
  sandboxes: string[] | null;
}

export interface Secret {
  scope: string;
  type: string;
  name: string;
  masked: string;
  username?: string;
  kind?: string;
  source?: string;
  refresh?: string;
}

export interface CustomSecret {
  scope: string;
  targets: string[] | null;
  env: string;
  placeholder: string;
  masked: string;
  kind?: string;
  source?: string;
  refresh?: string;
}

export interface SecretList {
  stored: Secret[] | null;
  custom: CustomSecret[] | null;
}

export interface PolicyRule {
  id: string;
  name: string;
  policy_id: string;
  scope: string;
  applies_to: string;
  sandbox_id?: string;
  resource_type: string;
  decision: string;
  resources: string[] | null;
  origin: string;
  status: string;
  editable: boolean;
}

export interface PolicyRuleRef {
  resource: string;
  rule_id: string;
  rule_name: string;
}

export interface PolicyActionResult {
  action: string;
  resources?: string[] | null;
  sandbox_id?: string;
  error?: string;
  created?: PolicyRuleRef[];
  existing?: PolicyRuleRef[];
  removed?: PolicyRuleRef[];
}

export interface LogEntry {
  host: string;
  vm_name: string;
  proxy_type: string;
  rule: string;
  last_seen: string;
  since: string;
  count_since: number;
}

export interface PolicyLog {
  blocked_hosts: LogEntry[] | null;
  allowed_hosts: LogEntry[] | null;
}

export interface SandboxDetail {
  sandbox: SandboxSummary;
  profiles: ProfileRef[] | null;
  mounts: MountInfo[] | null;
  mounts_error?: string;
  /** Config directory adopted from the daemon without its original create parameters. */
  incomplete: boolean;
  image?: string;
  image_digest?: string;
  kits?: string[] | null;
  secrets: Secret[] | null;
  custom_secrets: CustomSecret[] | null;
  policy_rules: PolicyRule[] | null;
  caches: SandboxCache[] | null;
  profile_mounts: SandboxProfileMount[] | null;
  direct_mounts?: SandboxDirectMount[] | null;
  skills?: SandboxSkill[] | null;
  additional_workspaces?: WorkspaceMount[] | null;
}

export interface BlockedEvent {
  host: string;
  sandbox: string;
  proxy_type: string;
  rule: string;
  count_since: number;
  at: string;
}

export interface JobView {
  id: string;
  status: "running" | "done" | "error";
  output: string;
  error?: string;
}

export interface JobEvent {
  kind: "output" | "done";
  chunk?: string;
  error?: string;
}

export interface EventEnvelope<T = unknown> {
  topic: string;
  data: T;
  ts: string;
}

export interface WorkspaceInput {
  path: string;
  read_only: boolean;
}

export interface MountInput {
  path: string;
  target: string;
  read_only: boolean;
}

export interface CreateSandboxRequest {
  agent: string;
  workspaces: WorkspaceInput[];
  name?: string;
  cpus?: number;
  memory?: string;
  profile?: string;
  template?: string;
  kits?: string[];
  profiles?: string[];
  caches?: string[];
  skills?: SkillRef[];
  run_args?: string;
  mounts?: MountInput[];
  publish?: string[];
  env?: string[];
  deny_network?: string[];
  clone?: boolean;
  attach_caches?: boolean;
  job_id?: string;
}

export interface ServiceSecretRequest {
  service: string;
  scope: string;
  value?: string;
  ref?: string;
  command?: string;
  refresh?: string;
  overwrite?: boolean;
}

export interface RegistrySecretRequest {
  host: string;
  username?: string;
  password: string;
  scope: string;
  overwrite?: boolean;
}

export interface CustomSecretRequest {
  hosts: string[];
  env: string;
  value?: string;
  ref?: string;
  command?: string;
  placeholder?: string;
  refresh?: string;
  scope: string;
  overwrite?: boolean;
}

export interface ImportSecretsRequest {
  service?: string;
  all?: boolean;
  dry_run?: boolean;
  force?: boolean;
  job_id?: string;
}

/** One sandbox's outcome after a bulk import. */
export type ImportStatus = "created" | "already configured" | "failed";

export interface ImportResult {
  name: string;
  status: ImportStatus;
  incomplete: boolean;
  error?: string;
}

/** Result of importing daemon sandboxes into the config directory. */
export interface ImportReport {
  results: ImportResult[] | null;
  created: number;
  configured: number;
  failed: number;
}

/** Create parameters recorded to complete an imported config. */
export interface CompleteSandboxRequest {
  cpus: number;
  memory: string;
  env: string[];
}

export interface Terminal {
  id: string;
  name: string;
  binary: string;
}

export interface Health {
  ok: boolean;
  socket: string;
  sbx_binary: string;
  daemon_running: boolean;
  daemon_status: string;
}

export interface VersionInfo {
  version: string;
  latest_version: string;
  update_available: boolean;
  release_url: string;
}

/** A shared cache definition flattened from fleet.Cache for the UI. */
export interface CacheView {
  slug: string;
  dir: string;
  name: string;
  description: string;
  host_path: string;
  target_path: string;
  read_only: boolean;
  auto_attach: boolean | null;
  enabled: boolean | null;
}

export interface SandboxCache extends CacheView {
  attached: boolean;
  direct: boolean;
  profiles: string[] | null;
  opted_out: boolean;
}

export interface SandboxDirectMount {
  host_path: string;
  target_path: string;
  read_only: boolean;
  attached: boolean;
}

export interface SandboxProfileMount {
  host_path: string;
  target_path: string;
  read_only: boolean;
  profile_names: string[] | null;
  opted_out: boolean;
  attached: boolean;
}

export interface CacheInput {
  name: string;
  description: string;
  host_path: string;
  target_path: string;
  read_only: boolean;
  auto_attach: boolean;
  enabled: boolean;
}

export interface SkillStore {
  slug: string;
  name: string;
  description: string;
  url: string;
  ref: string;
  auth: string;
  path: string;
  synced_at: string;
  error: string;
}

export interface SkillStoreInput {
  name: string;
  description: string;
  url: string;
  ref: string;
  auth: string;
}

export interface SkillItem {
  store: string;
  store_name: string;
  kind: string;
  name: string;
  description: string;
  plugin: string;
  rel_path: string;
}

export interface SandboxSkill {
  store: string;
  store_name: string;
  kind: string;
  name: string;
  description: string;
  plugin: string;
  rel_path: string;
  target: string;
  sources: string[] | null;
  mounted: boolean;
  conflict?: string;
  missing?: boolean;
  orphan?: boolean;
}

export interface SkillReconcileResult {
  applied: number;
  removed: number;
  errors: string[] | null;
}

export interface KitStore {
  slug: string;
  name: string;
  description: string;
  url: string;
  ref: string;
  auth: string;
  path: string;
  synced_at: string;
  error: string;
}

export interface KitStoreInput {
  name: string;
  description: string;
  url: string;
  ref: string;
  auth: string;
}

export interface KitRequires {
  agent?: string;
}

export interface KitCommandSet {
  default?: string[] | null;
  interactive?: string[] | null;
}

export interface KitSandbox {
  image?: string;
  entrypoint?: string[] | null;
  command?: KitCommandSet;
}

export interface KitEnvironment {
  variables?: Record<string, string> | null;
}

export interface KitNetworkPolicy {
  allow?: string[] | null;
  deny?: string[] | null;
}

export interface KitPermissions {
  network?: KitNetworkPolicy;
}

export interface KitPort {
  container: number;
  protocol?: string;
  name?: string;
}

export interface KitCommand {
  command?: string;
  user?: string;
  description?: string;
}

export interface KitFile {
  path: string;
  mode?: string;
  description?: string;
}

export interface KitSetup {
  install?: KitCommand[] | null;
  startup?: KitCommand[] | null;
  files?: KitFile[] | null;
}

export interface KitArgument {
  default?: string | null;
  required?: boolean;
  description?: string;
  enum?: string[] | null;
  pattern?: string;
}

export interface KitSpec {
  schemaVersion: string;
  kind: string;
  name: string;
  version?: string;
  displayName?: string;
  description?: string;
  sourceURL?: string;
  requires?: KitRequires;
  sandbox?: KitSandbox;
  environment?: KitEnvironment;
  permissions?: KitPermissions;
  ports?: KitPort[] | null;
  setup?: KitSetup;
  arguments?: Record<string, KitArgument> | null;
}

export interface KitItemView {
  store: string;
  kind: string;
  name: string;
  display_name: string;
  description: string;
  version: string;
  image: string;
  requires_agent: string;
  rel_path: string;
  store_name: string;
  ref: string;
  spec: KitSpec;
}

export interface KitValidation {
  ok: boolean;
  output: string;
}

/** Outcome of one sidecar convergence pass. */
export interface ApplyReport {
  rules_applied: number;
  mounts_applied: number;
  caches_applied: number;
  skills_applied: number;
  skills_removed: number;
  warnings: string[] | null;
  errors: string[] | null;
}

/** Outcome of attaching a mixin kit to an existing sandbox. */
export interface KitAddResult {
  sandbox: string;
  ref: string;
  output?: string;
  report: ApplyReport;
}

export interface Template {
  id: string;
  repository: string;
  tag: string;
  flavor: string;
  created_at: string;
  size: number;
}

/** Kinds the global search can return: fleet config entities and catalog items. */
export type SearchResultKind = "sandbox" | "profile" | "cache" | "skill" | "command" | "kit";

export interface SearchResult {
  kind: SearchResultKind;
  store: string;
  store_name: string;
  name: string;
  display_name?: string;
  /** Config directory slug: the routing identifier of a profile or cache hit. */
  slug?: string;
  description: string;
  plugin?: string;
  kit_kind?: string;
  /** Base agent of a sandbox hit. */
  agent?: string;
  score: number;
  snippet?: string;
}

export interface TerminalPrefs {
  /** Enabled terminal ids; null means every detected terminal is enabled. */
  enabled: string[] | null;
  /** Preferred terminal id; empty falls back to the first enabled terminal. */
  default: string;
}

/** Global sandwarden configuration file (terminals + notifications). */
export interface AppConfig {
  terminals: TerminalPrefs;
  notifications: boolean;
}
