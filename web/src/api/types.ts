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
  /** True while PMail waits for the primary mail domain on the first-run page. */
  need_domain?: boolean
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

export type LoginResult = AuthSessionUser | {
  two_factor_required: true
  challenge: string
  expires_in: number
}

export interface SecondFactorCode {
  code: string
  recovery: boolean
}

export interface AppPassword {
  id: number
  name: string
  created_at: string
  last_used_at: string | null
}

export interface AuthSessionUser {
  account: string
  name: string
  is_admin: number
  domains: string[]
}

export type AddressStatus = 'active' | 'pending' | 'rejected'

export interface MailboxAddress {
  id: number
  user_id: number
  local_part: string
  domain_id: number
  is_primary: boolean
  can_send: boolean
  status?: AddressStatus
  requested_at?: string | null
  reviewed_at?: string | null
  reviewed_by?: number | null
  reject_reason?: string
  domain: string
  address: string
  created: string
  created_iso?: string
}

export interface AdminOverview {
  pending_address_requests: number
}

export interface AdminAddressSettings {
  address_approval_required: boolean
}

export interface AdminAddressRequest extends MailboxAddress {
  status: AddressStatus
  applicant: string
}

export interface AdminAddressRequestPage {
  items: AdminAddressRequest[]
  page: number
  page_size: number
  total: number
  pending_address_requests: number
}

export interface UserProfile {
  id: number
  account: string
  name: string
  display_name: string
  disabled: number
  totp_enabled?: boolean
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

export type AddressFilter = 'all' | 'other' | number

export interface MailboxAddressMatch {
  id: number
  address: string
  can_send: boolean
  is_primary: boolean
}

export interface AddressCounts {
  addresses: (MailboxAddressMatch & { unread: number })[]
  all_unread: number
  other: { unread: number; has_mail: boolean }
}

export interface EmailListRequest {
  address?: AddressFilter
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
  risk_block_dangerous_attachments: boolean
  catch_all: { enabled: boolean; env_controlled: boolean; recipient_id: number }
  catch_all_recipients: { id: number; account: string; name: string }[]
}

export interface ClamAVStatus {
  enabled: boolean
  env_controlled: boolean
  state: 'disabled' | 'starting' | 'downloading' | 'running' | 'error' | 'stopping'
  updating: boolean
  database: { version: number; updated_at: string } | null
  last_update: { status: 'success' | 'failed'; at: string } | null
  scanned: number
  blocked: number
  available_memory_bytes?: number
  memory_warning: boolean
}

export type InboundSettingsUpdate = Omit<InboundSettings, 'catch_all' | 'catch_all_recipients'> & {
  catch_all?: { enabled?: boolean; recipient_id?: number }
}

/* ==================== V7-C26 & C27: Structured Proxy, Trash, Translation, User AI ==================== */

export type ProxyProtocol = 'http' | 'https' | 'socks5'

export interface ProxyInput {
  type: ProxyProtocol
  host: string
  port: number
  username?: string
  password?: string
}

export interface ProxyView {
  type: ProxyProtocol
  host: string
  port: number
  username?: string
  password_set: boolean
}

export type ProxySource = 'env' | 'db' | 'none'

export interface TrashSettings {
  trash_retention_days: number
}

export interface TrashRestoreResult {
  restored: number
}

export interface TrashDeleteResult {
  deleted: number
}

export type TranslationLanguage =
  | 'zh-CN'
  | 'zh-TW'
  | 'en'
  | 'ja'
  | 'ko'
  | 'fr'
  | 'de'
  | 'es'
  | 'ru'

export type TranslationEngine = 'ai' | 'web'

export type WebTranslationProvider = 'google_web' | 'edge_web' | 'deepl' | 'libretranslate'

export type DeepLTier = 'auto' | 'free' | 'pro'

export type AdminTranslationProxyMode = 'none' | 'custom' | 'telegram'

export interface AdminTranslationPolicy {
  web_enabled: boolean
  ai_enabled: boolean
  default_engine: TranslationEngine
  provider: WebTranslationProvider
  url?: string
  deepl_tier?: DeepLTier
  proxy_mode: AdminTranslationProxyMode
  max_chars: number
  daily_limit_per_user: number
  timeout: number
  ai_user_config_enabled: boolean
  ai_allow_private_network: boolean
  api_key_set: boolean
  proxy: ProxyView | null
}

export interface AdminTranslationPolicyPatch {
  web_enabled?: boolean
  ai_enabled?: boolean
  default_engine?: TranslationEngine
  provider?: WebTranslationProvider
  url?: string
  deepl_tier?: DeepLTier
  proxy_mode?: AdminTranslationProxyMode
  max_chars?: number
  daily_limit_per_user?: number
  timeout?: number
  ai_user_config_enabled?: boolean
  ai_allow_private_network?: boolean
  proxy?: ProxyInput
  clear_proxy?: boolean
  api_key?: string
}

export interface AdminTranslationTestResult {
  subject: string
  text: string
  provider: WebTranslationProvider
  latency_ms: number
}

export interface UserTranslationPreferences {
  target_lang: TranslationLanguage | ''
  auto_translate: boolean
  translate_engine: TranslationEngine | ''
  ai_auto_summary: boolean
}

export type UserAIProxyMode = 'none' | 'system' | 'custom'

export interface UserAIConfigView {
  configured: boolean
  ai_user_config_enabled: boolean
  ai_allow_private_network: boolean
  api_key_set: boolean
  base_url?: string
  model?: string
  timeout?: number
  proxy_mode?: UserAIProxyMode
  proxy: ProxyView | null
}

export interface UserAIConfigInput {
  base_url: string
  model: string
  timeout?: number
  proxy_mode?: UserAIProxyMode
  api_key?: string
  proxy?: ProxyInput
  clear_proxy?: boolean
}

export interface UserAITestResult {
  model: string
  latency_ms: number
}

export interface EmailTranslateRequest {
  id: number
  target_lang?: TranslationLanguage
  engine?: TranslationEngine
  force?: boolean
}

export interface EmailTranslationResult {
  subject: string
  text: string
  source_lang?: string
  target_lang: TranslationLanguage
  provider: WebTranslationProvider | 'openai_compatible'
  engine: TranslationEngine
  cached: boolean
  truncated: boolean
  model?: string
}

export interface EmailAIRequest {
  id: number
  action: 'summary' | 'extract'
  lang?: TranslationLanguage
  force?: boolean
}

export interface EmailAIActionItem {
  text: string
  due?: string
}

export interface EmailAIDateItem {
  text: string
  iso?: string
}

export interface EmailAIAmountItem {
  text: string
  value?: number
  currency?: string
}

export interface EmailAILinkItem {
  text: string
  url: string
}

export interface EmailAICodeItem {
  type: 'verification' | 'tracking' | 'order' | 'other'
  value: string
}

export interface EmailAIContactItem {
  name?: string
  email?: string
  phone?: string
}

export interface EmailAISummaryResult {
  action: 'summary'
  lang: TranslationLanguage
  model: string
  cached: boolean
  truncated: boolean
  summary: string
  key_points: string[]
}

export interface EmailAIExtractResult {
  action: 'extract'
  lang: TranslationLanguage
  model: string
  cached: boolean
  truncated: boolean
  action_items: EmailAIActionItem[]
  dates: EmailAIDateItem[]
  amounts: EmailAIAmountItem[]
  links: EmailAILinkItem[]
  codes: EmailAICodeItem[]
  contacts: EmailAIContactItem[]
}

export type EmailAIResult = EmailAISummaryResult | EmailAIExtractResult

/* ==================== C9 Contract: Telegram, System Update, Signatures ==================== */

export interface AdminTelegramStatus {
  configured: boolean
  bot_username: string | null
  proxy_set: boolean
  proxy: ProxyView | null
  proxy_source: ProxySource
  webhook_mode: 'polling'
}

export interface AdminTelegramUpdatePayload {
  bot_token?: string
  proxy?: ProxyInput | string | null
  clear_proxy?: boolean
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
  auto_translate?: boolean
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
    reason?: string
  }
  manual_commands: string[]
  job?: UpdateJobStatus
}

export interface UpdateApplyResponse {
  job_id: string
}

export type UpdateJobState = 'pending' | 'downloading' | 'verifying' | 'staging' | 'restarting' | 'done' | 'failed'

export interface UpdateJobStatus extends UpdateApplyResponse {
  version: string
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

export interface EmailChangeItem {
  id: number
  subject?: string
  title?: string
  from_name?: string
  from_address?: string
  sender?: MailParticipant
  group_id?: number
  created?: string
  datetime_iso?: string
}

export interface EmailChangesResponse {
  cursor: string
  changed: boolean
  new_count: number
  new_items: EmailChangeItem[]
}

export interface EmailListItem {
  deleted_at?: number | null
  purge_at?: number | null
  mailbox_addresses?: MailboxAddressMatch[]
  catch_all_addresses?: string[]
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
  translation_available?: boolean
  needs_translation?: boolean
  translation_engines?: TranslationEngine[]
  ai_available?: boolean
  deleted_at?: number | null
  purge_at?: number | null
  mailbox_addresses?: MailboxAddressMatch[]
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
  antivirus?: { status: 'clean' | 'not_scanned'; reason?: string }
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
  reply_to_email_id?: number
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
  total?: number
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
