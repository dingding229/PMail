export interface ApiEnvelope<T = unknown> {
  errorNo: number
  errorMsg: string
  data: T
}

/** GET /api/bootstrap returns raw JSON (not wrapped in { errorNo, errorMsg, data }) */
export interface BootstrapResponse {
  version: string
  need_register: boolean
  logged_in: boolean
  default_lang: string
  web_domain: string
  primary_domain: string
  domains: string[]
  features: {
    register_key?: boolean
  }
}

export interface RegisterRequest {
  registration_key: string
  account: string
  password: string
  display_name?: string
}

export interface LoginRequest {
  account: string
  password: string
}

export interface AuthSessionUser {
  account: string
  name: string
  is_admin: number
  domains: string[]
}

export interface MailboxAddress {
  id: number
  user_id: number
  local_part: string
  domain_id: number
  is_primary: boolean
  can_send: boolean
  domain: string
  address: string
  created: string
  created_iso?: string
}

export interface UserProfile {
  id: number
  account: string
  name: string
  display_name: string
  disabled: number
  is_admin: number
  quota_mb: number
  used_bytes: number
  lang?: string
  addresses: MailboxAddress[] | null
  created: string
  created_iso?: string
  last_login_at: string | null
  last_login_at_iso?: string | null
}

export interface UserPageResponse {
  current_page: number
  total_page: number
  list: UserProfile[]
}

export interface MailParticipant {
  Name: string
  EmailAddress: string
}

/** C3 normalized address object */
export interface NormalizedAddress {
  name: string
  address: string
}

/** C3 normalized attachment object */
export interface NormalizedAttachment {
  index: number
  filename: string
  content_type: string
  size: number
  content_id?: string
  inline?: boolean
  previewable?: boolean
}

export interface SearchTag {
  type: number // -1 any, 0 received, 1 sent
  status: number // -1 any, 0 inbox, 1 sent, 3 deleted, 4 draft, 5 junk
  group_id: number // -1 any, 0 default
  starred?: boolean
}

export interface EmailListRequest {
  keyword?: string
  tag: string
  current_page: number
  page_size: number
}

export type MailAuthStatus = 'pass' | 'fail' | 'none' | 'not_applicable'

export type MailAuthCheckResult =
  | 'pass'
  | 'fail'
  | 'none'
  | 'neutral'
  | 'softfail'
  | 'temperror'
  | 'permerror'
  | 'not_applicable'

export type MailAuthDmarcPolicy = 'none' | 'quarantine' | 'reject'

export interface MailAuthCheck {
  result: MailAuthCheckResult
  reason: string
  domain?: string
  aligned?: boolean
  policy?: MailAuthDmarcPolicy
}

export interface MailAuthResults {
  spf: MailAuthCheck
  dkim: MailAuthCheck
  dmarc: MailAuthCheck
  trusted_relay: boolean
}

export type MailSpamVerdict = 'ham' | 'spam' | 'unknown'

export interface MailSpamSummary {
  verdict: MailSpamVerdict
}

export interface MailSpamReason {
  code: string
  weight: number
  detail?: string
}

export interface MailSpamAssessment {
  score: number
  verdict: MailSpamVerdict
  reasons: MailSpamReason[]
}

export type MailRiskLevel = 'none' | 'low' | 'medium' | 'high'
export type MailRiskFindingSeverity = 'low' | 'medium' | 'high'
export type MailRiskFindingLocation = 'body' | 'attachment'

export interface MailRiskSummary {
  level: MailRiskLevel
}

export interface MailRiskFinding {
  code: string
  severity: MailRiskFindingSeverity
  location: MailRiskFindingLocation
  attachment_index?: number
  line?: number
  excerpt?: string
}

export interface MailRiskAssessment {
  level: MailRiskLevel
  findings: MailRiskFinding[]
}

export interface MailVerificationCodeSummary {
  code: string
}

export interface MailVerificationCode {
  code: string
  confidence: number
  source: 'subject' | 'body'
}

export interface SpamSettings {
  enabled: boolean
  threshold: number
  bayes_enabled: boolean
}

export type SpamRuleAction = 'allow' | 'block'
export type SpamRuleKind = 'address' | 'domain' | 'keyword'

export interface SpamRule {
  id: number
  action: SpamRuleAction
  kind: SpamRuleKind
  value: string
}

export interface SpamRuleInput {
  action: SpamRuleAction
  kind: SpamRuleKind
  value: string
}

export interface InboundSettings {
  dnsbl_enabled: boolean
  dnsbl_zones: string[]
  default_spam_threshold: number
  clamd_enabled: boolean
  clamd_addr: string
  risk_block_dangerous_attachments: boolean
}

/* ==================== C9 Contract: Telegram, System Update, Signatures ==================== */

export interface AdminTelegramStatus {
  configured: boolean
  bot_username: string | null
  proxy_set: boolean
  webhook_mode: 'polling'
}

export interface AdminTelegramUpdatePayload {
  bot_token?: string
  proxy?: string | null
}

export interface AdminTelegramTestResult {
  ok: boolean
  bot_username: string
  latency_ms: number
}

export interface TelegramQuietHours {
  start: string
  end: string
  tz: string
}

export interface TelegramFilters {
  group_ids: number[] | null
  only_verification: boolean
  include_spam: boolean
  include_preview: boolean
  quiet_hours: TelegramQuietHours | null
}

export interface UserTelegramStatus {
  available: boolean
  bound: boolean
  chat_title: string | null
  bound_at: string | null
  filters: TelegramFilters
}

export interface TelegramBindCode {
  code: string
  expires_at: string
  deep_link: string
}

export interface UpdateReleaseItem {
  version: string
  published_at: string
  notes_markdown: string
  url: string
  is_current: boolean
}

export interface AdminUpdateInfo {
  current: string
  latest: string | null
  has_update: boolean
  checked_at: string | null
  check_error: string | null
  releases: UpdateReleaseItem[]
  updater: {
    enabled: boolean
    kind: string | null
  }
  manual_commands: string[]
}

export interface UpdateApplyResponse {
  job_id: string
}

export type UpdateJobState = 'pending' | 'pulling' | 'restarting' | 'done' | 'failed'

export interface UpdateJobStatus {
  state: UpdateJobState
  message: string | null
}

export interface SignatureRecord {
  id: number
  name: string
  html: string
  address_ids: number[]
  is_default: boolean
}

export interface SignatureInput {
  name: string
  html: string
  address_ids: number[]
  is_default: boolean
}

export interface EmailListItem {
  id: number
  title: string
  desc: string
  datetime: string
  datetime_iso?: string
  send_date_iso?: string
  send_time_iso?: string
  create_time_iso?: string
  update_time_iso?: string
  is_read: boolean
  is_starred?: boolean
  sender: MailParticipant
  to: MailParticipant[] | null
  to_list?: NormalizedAddress[]
  attachments_list?: NormalizedAttachment[]
  dangerous: boolean
  auth_status?: MailAuthStatus
  auth_results?: MailAuthResults | null
  spam?: MailSpamSummary | null
  risk?: MailRiskSummary
  verification_code?: MailVerificationCodeSummary | null
  type?: number
  status?: number
  error: string
  has_attachments?: boolean
  group_id?: number
}

export interface EmailListResponse {
  current_page: number
  total_page: number
  total?: number
  list: EmailListItem[] | null
}

export interface EmailAttachment {
  Filename: string
  ContentType: string
  Index: number
  Size?: number
  ContentID?: string
}

export interface EmailDetail {
  id: number
  type: number
  subject: string
  reply_to: string
  from_name: string
  from_address: string
  to: string
  bcc: string
  cc: string
  to_list?: NormalizedAddress[]
  cc_list?: NormalizedAddress[]
  bcc_list?: NormalizedAddress[]
  reply_to_list?: NormalizedAddress[]
  text: string
  html: string
  sender: string
  attachments: EmailAttachment[]
  attachments_list?: NormalizedAttachment[]
  spf_check: number
  dkim_check: number
  auth_status?: MailAuthStatus
  auth_results?: MailAuthResults | null
  spam?: MailSpamAssessment | null
  risk?: MailRiskAssessment
  verification_code?: MailVerificationCode | null
  status: number
  cron_send_time?: string
  send_time: string
  send_time_iso?: string
  send_date: string
  send_date_iso?: string
  update_time: string
  update_time_iso?: string
  create_time: string
  create_time_iso?: string
  send_user_id?: number
  size: number
  error: string
  msg_id: string
  is_read?: number
  is_starred?: boolean
  text_format?: 'plain' | 'flowed' | 'markdown' | 'patch'
  has_code?: boolean
}

export interface SendEmailParticipant {
  name: string
  email: string
}

export interface SendEmailAttachment {
  name: string
  data: string // data:<mime>;base64,<content>
}

export interface InlineImageRef {
  cid: string
}

export interface InlineImageUploadResult {
  cid: string
  url: string
  filename: string
  content_type: string
  size: number
}

export interface SendEmailRequest {
  from?: SendEmailParticipant
  from_address?: string
  to: SendEmailParticipant[]
  cc?: SendEmailParticipant[]
  bcc?: SendEmailParticipant[]
  reply_to?: SendEmailParticipant[]
  subject: string
  text?: string
  html?: string
  attrs?: SendEmailAttachment[]
  inline_images?: InlineImageRef[]
  draft_id?: number
}

export interface DraftSaveRequest {
  id?: number
  from_address?: string
  to: SendEmailParticipant[]
  cc?: SendEmailParticipant[]
  bcc?: SendEmailParticipant[]
  subject: string
  html?: string
  text?: string
  attrs?: SendEmailAttachment[]
  inline_images?: InlineImageRef[]
}

export interface DraftSaveResponse {
  id: number
}

export interface ForwardEmailRequest {
  email_id: number
  to: string[]
  cc?: string[]
  text?: string
  html?: string
  from_address?: string
  include_attachments?: boolean
  inline_images?: InlineImageRef[]
}

export interface RecipientDeliveryResult {
  address: string
  status: 'delivered' | 'queued' | 'failed'
  error: string
}

export interface SendEmailResponse {
  status: 'sent' | 'queued' | 'failed'
  recipients: RecipientDeliveryResult[]
}

export interface GroupTreeItem {
  id: number
  label: string
  tag: string
  unread?: number
  children?: GroupTreeItem[] | null
}

export interface GroupOption {
  id: number
  name: string
  parent_id: number
  full_path: string
}

export interface AttachmentPreviewResponse {
  filename: string
  language: string
  charset: string
  truncated: boolean
  size: number
  content: string
}

/* ==================== Settings & Admin Schemas (openapi.yaml) ==================== */

export interface ForwardSetting {
  user_id: number
  enabled: boolean
  targets: string
  keep_copy: boolean
  updated: string
}

export interface ForwardSettingsView {
  setting: ForwardSetting
  targets: string[] | null
}

export interface ForwardSettingsRequest {
  enabled: boolean
  targets: string[]
  keep_copy: boolean
}

export type RuleConditionField = 'From' | 'Subject' | 'To' | 'Cc' | 'Text' | 'Html' | 'Content'
export type RuleConditionType = 'contains' | 'equal' | 'regex'
export type RuleActionCode = 1 | 2 | 3 | 4 // 1 mark read, 2 forward, 3 delete, 4 move to group

export interface RuleCondition {
  field: RuleConditionField
  type: RuleConditionType
  rule: string
}

export interface MailRule {
  id: number
  user_id: number
  name: string
  rules: RuleCondition[] | null
  action: RuleActionCode
  params: string
  sort: number
}

export interface RuleRequest {
  id?: number
  name: string
  rules: RuleCondition[]
  action: RuleActionCode
  params?: string
  sort?: number
}

export type ExternalProtocol = 'imap' | 'pop3'
export type ExternalTlsMode = 'tls' | 'starttls' | 'none'

export interface ExternalAccount {
  id: number
  user_id: number
  name: string
  protocol: ExternalProtocol
  host: string
  port: number
  tls_mode: ExternalTlsMode
  username: string
  folder: string
  interval_minutes: number
  initial_days: number
  delete_remote: boolean
  target_group_id: number
  enabled: boolean
  insecure_skip_verify: boolean
  last_sync_at: string | null
  last_sync_at_iso?: string | null
  last_success_at: string | null
  last_success_at_iso?: string | null
  last_error: string
  created: string
  created_iso?: string
  updated: string
  updated_iso?: string
}

export interface ExternalAccountPayload {
  id?: number
  name?: string
  protocol: ExternalProtocol
  host: string
  port?: number
  tls_mode: ExternalTlsMode
  username: string
  password?: string
  folder?: string
  interval_minutes?: number
  initial_days?: number
  delete_remote?: boolean
  target_group_id?: number
  enabled?: boolean
  insecure_skip_verify?: boolean
}

export interface ExternalPreset {
  name: string
  protocol: ExternalProtocol
  host: string
  port: number
  tls_mode: ExternalTlsMode
}

export interface ExternalTestResult {
  folders: string[] | null
}

export interface ExternalSyncResult {
  status: string
  message: string
}

export type ApiTokenScope = 'read' | 'send' | 'admin' | 'legacy'
export type CreateTokenScope = 'read' | 'send' | 'admin'

export interface ApiTokenItem {
  id: number
  user_id: number
  name: string
  prefix: string
  scope?: ApiTokenScope
  legacy_scope?: boolean
  never_expires?: boolean
  last_used_at: string | null
  last_used_at_iso?: string | null
  expires_at: string | null
  expires_at_iso?: string | null
  created: string
  created_iso?: string
}

export interface TokenCreatedResponse {
  token: string
  item: ApiTokenItem
}

export type QueueStatus = 'pending' | 'sending' | 'sent' | 'failed'
export type QueueKind = 'send' | 'forward' | 'bounce'

export interface QueueItem {
  id: number
  kind: QueueKind
  user_id: number
  email_id?: number
  mail_from: string
  rcpt_to: string
  attempts: number
  max_attempts: number
  next_retry_at: string
  next_retry_at_iso?: string
  status: QueueStatus
  last_error: string
  created: string
  created_iso?: string
  updated: string
  updated_iso?: string
}

export interface MailDomain {
  id: number
  name: string
  is_default: boolean
  dkim_selector: string
  created: string
  created_iso?: string
  updated: string
  updated_iso?: string
}

export type DnsRecordStatus = 'ok' | 'missing' | 'mismatch' | 'error' | 'advice'

export interface DnsRecord {
  key: string
  purpose: string
  type: string
  host: string
  fqdn: string
  value: string
  value_chunks?: string[]
  suggested_value?: string
  priority?: number
  ttl: number
  required: boolean
  status: DnsRecordStatus
  actual: string[]
  description: string
  fix_hint?: string
}

export interface DomainDnsResponse {
  domain: string
  mail_host: string
  web_domain: string
  server_ipv4?: string
  server_ipv6?: string
  all_required_ok: boolean
  checked_at: string
  checked_at_iso?: string
  records: DnsRecord[]
}

export const SYSTEM_GROUP_IDS = {
  INBOX: 2000000000,
  SENT: 2000000001,
  DRAFTS: 2000000002,
  DELETED: 2000000003,
  JUNK: 2000000004,
} as const
