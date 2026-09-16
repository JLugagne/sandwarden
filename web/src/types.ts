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
  profiles: string[] | null;
  connect: ConnectInfo;
}

export interface Profile {
  id: number;
  name: string;
  description: string;
  is_default: boolean;
  is_global: boolean;
  created_at: string;
  updated_at: string;
}

export interface Rule {
  id: number;
  profile_id: number;
  decision: string;
  pattern: string;
  created_at: string;
}

export interface ProfileView extends Profile {
  rules: Rule[] | null;
  items: SkillItem[] | null;
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
  profiles: Profile[] | null;
  mounts: MountInfo[] | null;
  secrets: Secret[] | null;
  custom_secrets: CustomSecret[] | null;
  policy_rules: PolicyRule[] | null;
  caches: SandboxCache[] | null;
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

export interface CreateSandboxRequest {
  agent: string;
  workspaces: WorkspaceInput[];
  name?: string;
  cpus?: number;
  memory?: string;
  profile?: string;
  template?: string;
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

export interface CacheMount {
  id: number;
  name: string;
  description: string;
  host_path: string;
  target_path: string;
  read_only: boolean;
  auto_attach: boolean;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface SandboxCache extends CacheMount {
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
  id: number;
  name: string;
  description: string;
  url: string;
  ref: string;
  auth: string;
  path: string;
  synced_at: string;
  error: string;
  created_at: string;
  updated_at: string;
}

export interface SkillStoreInput {
  name: string;
  description: string;
  url: string;
  ref: string;
  auth: string;
}

export interface SkillItem {
  id: number;
  store_id: number;
  store_name: string;
  kind: string;
  name: string;
  description: string;
  plugin: string;
  rel_path: string;
}

export interface SandboxSkill extends SkillItem {
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
