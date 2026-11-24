import { delay, http, HttpResponse } from 'msw'
import dayjs from 'dayjs'
import { version as mockBuildVersion } from '../../package.json'
import {
  SYSTEM_GROUP_IDS,
  type AddressFilter,
  type AddressStatus,
  type AdminAddressRequest,
  type AdminTelegramStatus,
  type AdminTelegramUpdatePayload,
  type AdminTranslationPolicy,
  type AdminTranslationPolicyPatch,
  type AdminUpdateInfo,
  type AppPassword,
  type ApiTokenItem,
  type BootstrapResponse,
  type DomainDnsResponse,
  type EmailAIExtractResult,
  type EmailAIRequest,
  type EmailAISummaryResult,
  type EmailTranslateRequest,
  type EmailTranslationResult,
  type GroupOption,
  type ClamAVStatus,
  type InboundSettings,
  type InboundSettingsUpdate,
  type MailboxAddress,
  type MailDomain,
  type MailRule,
  type ProxyInput,
  type ProxyView,
  type QueueItem,
  type SignatureInput,
  type SignatureRecord,
  type SpamRule,
  type SpamRuleInput,
  type SpamSettings,
  type TelegramFilters,
  type TrashSettings,
  type TranslationLanguage,
  type UpdateJobState,
  type UserAIConfigInput,
  type UserAIConfigView,
  type UserProfile,
  type UserTelegramStatus,
  type UserTranslationPreferences,
} from '@/api'
import {
  buildSyntheticEmail,
  createSeedEmails,
  INITIAL_CUSTOM_GROUPS,
  MOCK_USER,
  resolveMockAuthStatus,
  toEmailDetail,
  VIRTUAL_TOTAL_INBOX,
  type MockEmailRecord,
} from './seed'

const VALID_REGISTER_KEY = 'ABCD-EFGH-JKLM-NPQR'

/* ==================== Inbound Analysis (Spam / Risk / Admin Inbound) ==================== */
let mockSpamSettings: SpamSettings = {
  enabled: true,
  threshold: 6,
  bayes_enabled: true,
}

let nextSpamRuleId = 60
let mockSpamRules: SpamRule[] = [
  { id: 51, action: 'allow', kind: 'domain', value: 'acme-cloud.cn' },
  { id: 52, action: 'allow', kind: 'address', value: 'notifications@github.com' },
  { id: 53, action: 'block', kind: 'domain', value: 'promo-blast.example' },
  { id: 54, action: 'block', kind: 'keyword', value: '限时百倍返利' },
]

let mockClamAV: ClamAVStatus = {
  enabled: false, env_controlled: false, state: 'disabled', updating: false,
  database: null, last_update: null, scanned: 0, blocked: 0, memory_warning: false,
}

function getMockClamAV(): ClamAVStatus {
  const mode =
    typeof window !== 'undefined' ? localStorage.getItem('pmail_mock_clamav') : null
  if (mode === 'running') {
    return {
      enabled: true,
      env_controlled: false,
      state: 'running',
      updating: false,
      database: { version: 27462, updated_at: dayjs().subtract(2, 'hour').toISOString() },
      last_update: { status: 'success', at: dayjs().subtract(2, 'hour').toISOString() },
      scanned: 1284,
      blocked: 3,
      memory_warning: true,
    }
  }
  if (mode === 'env_disabled') {
    return {
      enabled: false,
      env_controlled: true,
      state: 'disabled',
      updating: false,
      database: null,
      last_update: null,
      scanned: 0,
      blocked: 0,
      memory_warning: false,
    }
  }
  return mockClamAV
}

let mockInboundSettings: InboundSettings = {
  dnsbl_enabled: true,
  dnsbl_zones: ['zen.spamhaus.org', 'bl.spamcop.net'],
  default_spam_threshold: 6,
  risk_block_dangerous_attachments: true,
  catch_all: { enabled: true, env_controlled: false, recipient_id: 0 },
  catch_all_recipients: [{ id: 1, account: 'admin', name: 'Administrator' }],
}

/* ==================== C9 & C26 & C27 Contract Stores ==================== */
let mockTrashSettings: TrashSettings = {
  trash_retention_days: 30,
}

let mockAdminTelegram: AdminTelegramStatus = {
  configured: true,
  bot_username: 'pmail_notify_bot',
  proxy_set: true,
  proxy: {
    type: 'socks5',
    host: '127.0.0.1',
    port: 1080,
    username: 'tg-relay',
    password_set: true,
  },
  proxy_source: 'db',
  webhook_mode: 'polling',
}

function getMockAdminTelegram(): AdminTelegramStatus {
  const envOverride =
    typeof window !== 'undefined' && localStorage.getItem('pmail_mock_tg_proxy_env') === '1'
  if (envOverride) {
    return {
      ...mockAdminTelegram,
      proxy_set: true,
      proxy: {
        type: 'socks5',
        host: 'proxy.corp.internal',
        port: 1080,
        username: 'env-user',
        password_set: true,
      },
      proxy_source: 'env',
    }
  }
  return mockAdminTelegram
}

let mockAdminTranslation: AdminTranslationPolicy = {
  web_enabled: true,
  ai_enabled: true,
  default_engine: 'web',
  provider: 'edge_web',
  url: '',
  deepl_tier: 'auto',
  proxy_mode: 'telegram',
  max_chars: 8000,
  daily_limit_per_user: 200,
  timeout: 30,
  ai_user_config_enabled: true,
  ai_allow_private_network: false,
  api_key_set: false,
  proxy: null,
}

let mockUserTranslation: UserTranslationPreferences = {
  target_lang: '',
  auto_translate: false,
  translate_engine: '',
  ai_auto_summary: false,
}

let mockUserAI: UserAIConfigView = {
  configured: true,
  ai_user_config_enabled: true,
  ai_allow_private_network: false,
  api_key_set: true,
  base_url: 'https://api.openai.com/v1',
  model: 'gpt-4o-mini',
  timeout: 30,
  proxy_mode: 'system',
  proxy: null,
}

function getMockUserAI(): UserAIConfigView {
  const disabled =
    typeof window !== 'undefined' && localStorage.getItem('pmail_mock_ai_disabled') === '1'
  return {
    ...mockUserAI,
    ai_user_config_enabled: disabled ? false : mockAdminTranslation.ai_user_config_enabled,
    ai_allow_private_network: mockAdminTranslation.ai_allow_private_network,
  }
}

function applyProxyInput(current: ProxyView | null, input: ProxyInput): ProxyView {
  const nextPasswordSet =
    input.password === undefined
      ? Boolean(current?.password_set)
      : input.password.length > 0
  return {
    type: input.type,
    host: input.host.trim(),
    port: Number(input.port),
    username: input.username?.trim() || '',
    password_set: nextPasswordSet,
  }
}

function computePurgeAt(deletedAt: number | null | undefined): number | null {
  if (!deletedAt || mockTrashSettings.trash_retention_days <= 0) return null
  return deletedAt + mockTrashSettings.trash_retention_days * 24 * 60 * 60 * 1000
}

function looksForeignForChinese(text: string): boolean {
  const latin = (text.match(/[a-zA-Z]/g) ?? []).length
  const cjk = (text.match(/[\u4e00-\u9fff]/g) ?? []).length
  return latin > 40 && latin > cjk * 2
}

let mockAddressApprovalRequired = true

let mockAddressRequests: AdminAddressRequest[] = [
  {
    id: 4,
    user_id: 1,
    applicant: 'admin',
    local_part: 'billing-ops',
    domain_id: 1,
    is_primary: false,
    can_send: false,
    status: 'pending',
    requested_at: dayjs().subtract(2, 'hour').toISOString(),
    reviewed_at: null,
    reviewed_by: null,
    reject_reason: '',
    domain: 'pmail.dev',
    address: 'billing-ops@pmail.dev',
    created: dayjs().subtract(2, 'hour').toISOString(),
  },
  {
    id: 901,
    user_id: 2,
    applicant: 'chenzhiyi',
    local_part: 'design',
    domain_id: 1,
    is_primary: false,
    can_send: false,
    status: 'pending',
    requested_at: dayjs().subtract(5, 'hour').toISOString(),
    reviewed_at: null,
    reviewed_by: null,
    reject_reason: '',
    domain: 'pmail.dev',
    address: 'design@pmail.dev',
    created: dayjs().subtract(5, 'hour').toISOString(),
  },
  {
    id: 902,
    user_id: 3,
    applicant: 'sumingyuan',
    local_part: 'releases',
    domain_id: 1,
    is_primary: false,
    can_send: false,
    status: 'pending',
    requested_at: dayjs().subtract(1, 'day').toISOString(),
    reviewed_at: null,
    reviewed_by: null,
    reject_reason: '',
    domain: 'pmail.dev',
    address: 'releases@pmail.dev',
    created: dayjs().subtract(1, 'day').toISOString(),
  },
  {
    id: 2,
    user_id: 1,
    applicant: 'admin',
    local_part: 'lin',
    domain_id: 1,
    is_primary: false,
    can_send: true,
    status: 'active',
    requested_at: '2026-09-10T10:30:00Z',
    reviewed_at: '2026-09-10T10:35:00Z',
    reviewed_by: 1,
    reject_reason: '',
    domain: 'pmail.dev',
    address: 'lin@pmail.dev',
    created: '2026-09-10T10:30:00Z',
  },
  {
    id: 903,
    user_id: 2,
    applicant: 'chenzhiyi',
    local_part: 'ux',
    domain_id: 1,
    is_primary: false,
    can_send: true,
    status: 'active',
    requested_at: dayjs().subtract(4, 'day').toISOString(),
    reviewed_at: dayjs().subtract(3, 'day').toISOString(),
    reviewed_by: 1,
    reject_reason: '',
    domain: 'pmail.dev',
    address: 'ux@pmail.dev',
    created: dayjs().subtract(4, 'day').toISOString(),
  },
  {
    id: 5,
    user_id: 1,
    applicant: 'admin',
    local_part: 'postmaster',
    domain_id: 1,
    is_primary: false,
    can_send: false,
    status: 'rejected',
    requested_at: dayjs().subtract(2, 'day').toISOString(),
    reviewed_at: dayjs().subtract(1, 'day').toISOString(),
    reviewed_by: 1,
    reject_reason: '系统保留前缀，不支持作为个人收发别名',
    domain: 'pmail.dev',
    address: 'postmaster@pmail.dev',
    created: dayjs().subtract(2, 'day').toISOString(),
  },
  {
    id: 904,
    user_id: 3,
    applicant: 'sumingyuan',
    local_part: 'security',
    domain_id: 1,
    is_primary: false,
    can_send: false,
    status: 'rejected',
    requested_at: dayjs().subtract(5, 'day').toISOString(),
    reviewed_at: dayjs().subtract(4, 'day').toISOString(),
    reviewed_by: 1,
    reject_reason: '安全通知地址由运维组统一管理',
    domain: 'pmail.dev',
    address: 'security@pmail.dev',
    created: dayjs().subtract(5, 'day').toISOString(),
  },
]

function countPendingAddressRequests(): number {
  return mockAddressRequests.filter((r) => r.status === 'pending').length
}

let mockLiveChangeSeq = 1
let mockLiveSampleIdx = 0

const MOCK_LIVE_TEMPLATES = [
  {
    from_name: 'GitHub Actions',
    from_address: 'notifications@github.com',
    subject: '[pmail/pmail] Release v0.0.8 workflow succeeded',
    text: 'All CI checks and cross-platform builds for v0.0.8 completed without errors.',
  },
  {
    from_name: 'Stripe Billing',
    from_address: 'billing@stripe.com',
    subject: '您的 11 月云服务账单已生成（INV-2026-1124）',
    text: '本月账单金额为 ¥328.00，已自动从默认支付方式扣款，详情请登录控制台查看。',
  },
  {
    from_name: '苏明远',
    from_address: 'mingyuan@pmail.dev',
    subject: 'Re: V7 实时刷新联调确认',
    text: '刚刚测完长轮询与跨标签页同步，切回页面后的恢复延迟很低，可以合入主干。',
  },
  {
    from_name: 'Cloudflare Status',
    from_address: 'noreply@notify.cloudflare.com',
    subject: 'DNSSEC key rollover completed for pmail.dev',
    text: 'The scheduled DNSSEC key rotation for pmail.dev has finished cleanly with zero downtime.',
  },
]

function abortableDelay(ms: number, signal?: AbortSignal): Promise<boolean> {
  if (ms <= 0) return Promise.resolve(true)
  return new Promise((resolve) => {
    if (signal?.aborted) {
      resolve(false)
      return
    }
    const timer = setTimeout(() => {
      signal?.removeEventListener('abort', onAbort)
      resolve(true)
    }, ms)
    const onAbort = () => {
      clearTimeout(timer)
      resolve(false)
    }
    signal?.addEventListener('abort', onAbort, { once: true })
  })
}

let mockUserTelegramFilters: TelegramFilters = {
  group_ids: null,
  only_verification: false,
  include_spam: false,
  include_preview: true,
  auto_translate: false,
  quiet_hours: {
    start: '23:00',
    end: '08:00',
    tz: 'Asia/Shanghai',
  },
}

let mockUserTelegramBound = true
const mockUserTelegramChatTitle: string | null = '林深 (@linshen_dev)'
const mockUserTelegramBoundAt: string | null = '2026-11-20T09:30:00Z'

function getMockUserTelegramStatus(): UserTelegramStatus {
  const override =
    typeof window !== 'undefined' ? localStorage.getItem('pmail_mock_tg') : null
  if (override === 'unavailable') {
    return {
      available: false,
      bound: false,
      chat_title: null,
      bound_at: null,
      filters: mockUserTelegramFilters,
    }
  }
  if (override === 'unbound') {
    return {
      available: mockAdminTelegram.configured,
      bound: false,
      chat_title: null,
      bound_at: null,
      filters: mockUserTelegramFilters,
    }
  }
  return {
    available: mockAdminTelegram.configured,
    bound: mockUserTelegramBound,
    chat_title: mockUserTelegramBound ? mockUserTelegramChatTitle : null,
    bound_at: mockUserTelegramBound ? mockUserTelegramBoundAt : null,
    filters: mockUserTelegramFilters,
  }
}

const mockUpdateJobStep = new Map<string, number>()

function buildMockUpdateInfo(): AdminUpdateInfo {
  const mode =
    typeof window !== 'undefined' ? localStorage.getItem('pmail_mock_update') : null
  if (mode === 'error') {
    return {
      current: 'v0.1.0',
      latest: null,
      has_update: false,
      checked_at: dayjs().subtract(1, 'minute').toISOString(),
      check_error: '无法连接 GitHub Releases（连接超时），请稍后重试',
      updater: {
        enabled: false,
        kind: null,
      },
      manual_commands: ['docker compose pull pmail', 'docker compose up -d pmail'],
      releases: [],
    }
  }
  const isLatest = mode === 'latest'
  const isManual = mode === 'manual'
  return {
    current: isLatest ? 'v0.2.0' : 'v0.1.0',
    latest: 'v0.2.0',
    has_update: !isLatest,
    checked_at: dayjs().subtract(6, 'minute').toISOString(),
    check_error: null,
    updater: {
      enabled: !isManual,
      kind: isManual ? null : 'self-update',
    },
    manual_commands: [
      'docker compose pull pmail',
      'docker compose up -d pmail',
    ],
    releases: [
      {
        version: 'v0.2.0',
        published_at: '2026-11-22T10:00:00Z',
        url: 'https://github.com/dingding229/PMail/releases/tag/v0.2.0',
        is_current: isLatest,
        notes_markdown:
          '### 新特性\n- 新增 **Telegram 实时推送** 与免打扰时段过滤\n- 新增 **系统在线更新** 与安全更新日志渲染\n- 写信支持 **CID 内嵌图片上传** 与多套富文本签名\n\n### 安全加固\n- 拦截高危脚本与可执行附件下载，新增二次确认\n- 支持 API Token 最小权限范围（`read` / `send` / `admin`）',
      },
      {
        version: 'v0.1.0',
        published_at: '2026-11-10T08:00:00Z',
        url: 'https://github.com/dingding229/PMail/releases/tag/v0.1.0',
        is_current: !isLatest,
        notes_markdown:
          '### 初始发行\n- 全新 React 19 + TanStack Virtual 三栏邮件工作区\n- 一次性注册密钥与分步 DNS 配置向导\n- 支持 SPF / DKIM / DMARC 认证详情展示',
      },
    ],
  }
}

const mockInlineImageStore = new Map<string, { bytes: ArrayBuffer; contentType: string; filename: string }>()

let nextSignatureId = 10
let mockSignatures: SignatureRecord[] = [
  {
    id: 1,
    name: '工作正式签名',
    html: '<p><strong>林深 · PMail Pro</strong></p><p>系统管理员 · admin@pmail.dev · <a href="https://pmail.dev" target="_blank" rel="noopener noreferrer">pmail.dev</a></p>',
    address_ids: [1],
    is_default: true,
  },
  {
    id: 2,
    name: '简洁个人签名',
    html: '<p>此致，林深</p>',
    address_ids: [2],
    is_default: false,
  },
]

function resolveInitialMockMode(): { needRegister: boolean; loggedIn: boolean } {
  if (typeof window !== 'undefined') {
    const params = new URLSearchParams(window.location.search)
    const state = params.get('mock_state') || localStorage.getItem('pmail_mock_state')
    if (state === 'register') return { needRegister: true, loggedIn: false }
    if (state === 'login') return { needRegister: false, loggedIn: false }
  }
  return { needRegister: false, loggedIn: true }
}

const initMode = resolveInitialMockMode()
let needRegister = initMode.needRegister
let loggedIn = initMode.loggedIn

/* ==================== Users & Addresses Store ==================== */
let nextUserId = 8
let nextAddressId = 16
let mockUsers: UserProfile[] = [
  { ...MOCK_USER },
  {
    id: 2,
    account: 'chen',
    totp_enabled: true,
    name: '陈知意',
    display_name: '陈知意',
    disabled: 0,
    is_admin: 0,
    quota_mb: 1024,
    used_bytes: 790 * 1024 * 1024,
    lang: 'zh-CN',
    created: '2026-09-12T09:15:00Z',
    last_login_at: dayjs().subtract(2, 'hour').toISOString(),
    addresses: [
      {
        id: 4,
        user_id: 2,
        local_part: 'chen',
        domain_id: 1,
        is_primary: true,
        can_send: true,
        domain: 'pmail.dev',
        address: 'chen@pmail.dev',
        created: '2026-09-12T09:15:00Z',
      },
      {
        id: 5,
        user_id: 2,
        local_part: 'design',
        domain_id: 2,
        is_primary: false,
        can_send: true,
        domain: 'studio.cn',
        address: 'design@studio.cn',
        created: '2026-10-02T11:00:00Z',
      },
    ],
  },
  {
    id: 3,
    account: 'lu',
    name: '陆景行',
    display_name: '陆景行',
    disabled: 1,
    is_admin: 0,
    quota_mb: 512,
    used_bytes: 94 * 1024 * 1024,
    lang: 'zh-CN',
    created: '2026-10-01T10:00:00Z',
    last_login_at: dayjs().subtract(5, 'day').toISOString(),
    addresses: [
      {
        id: 6,
        user_id: 3,
        local_part: 'lu',
        domain_id: 1,
        is_primary: true,
        can_send: true,
        domain: 'pmail.dev',
        address: 'lu@pmail.dev',
        created: '2026-10-01T10:00:00Z',
      },
    ],
  },
  {
    id: 4,
    account: 'tb21-454223-other',
    totp_enabled: true,
    name: '测试回归账号 TB21',
    display_name: '测试回归账号 TB21',
    disabled: 0,
    is_admin: 0,
    quota_mb: 0,
    used_bytes: 18 * 1024 * 1024,
    lang: 'zh-CN',
    created: '2026-11-18T08:00:00Z',
    last_login_at: dayjs().subtract(35, 'minute').toISOString(),
    addresses: [
      {
        id: 10,
        user_id: 4,
        local_part: 'tb21-454223-other',
        domain_id: 1,
        is_primary: true,
        can_send: true,
        domain: 'pmail.dev',
        address: 'tb21-454223-other@pmail.dev',
        created: '2026-11-18T08:00:00Z',
      },
    ],
  },
  {
    id: 5,
    account: 'ops-automation-bot',
    totp_enabled: false,
    name: '运维告警机器人',
    display_name: '运维告警机器人',
    disabled: 0,
    is_admin: 0,
    quota_mb: 0,
    used_bytes: 245 * 1024 * 1024,
    lang: 'zh-CN',
    created: '2026-10-15T12:00:00Z',
    last_login_at: dayjs().subtract(1, 'day').toISOString(),
    addresses: [
      {
        id: 11,
        user_id: 5,
        local_part: 'ops-automation-bot',
        domain_id: 1,
        is_primary: true,
        can_send: true,
        domain: 'pmail.dev',
        address: 'ops-automation-bot@pmail.dev',
        created: '2026-10-15T12:00:00Z',
      },
    ],
  },
  {
    id: 6,
    account: 'su-mingyuan-prod',
    totp_enabled: true,
    name: '苏明远',
    display_name: '苏明远',
    disabled: 0,
    is_admin: 1,
    quota_mb: 4096,
    used_bytes: 1620 * 1024 * 1024,
    lang: 'zh-CN',
    created: '2026-08-20T09:00:00Z',
    last_login_at: dayjs().subtract(12, 'minute').toISOString(),
    addresses: [
      {
        id: 12,
        user_id: 6,
        local_part: 'su-mingyuan-prod',
        domain_id: 1,
        is_primary: true,
        can_send: true,
        domain: 'pmail.dev',
        address: 'su-mingyuan-prod@pmail.dev',
        created: '2026-08-20T09:00:00Z',
      },
    ],
  },
  {
    id: 7,
    account: 'qa-regression-user',
    totp_enabled: false,
    name: '质量验收专用账号',
    display_name: '质量验收专用账号',
    disabled: 0,
    is_admin: 0,
    quota_mb: 0,
    used_bytes: 4 * 1024 * 1024,
    lang: 'en',
    created: '2026-11-20T14:30:00Z',
    last_login_at: '',
    addresses: [
      {
        id: 13,
        user_id: 7,
        local_part: 'qa-regression-user',
        domain_id: 1,
        is_primary: true,
        can_send: true,
        domain: 'pmail.dev',
        address: 'qa-regression-user@pmail.dev',
        created: '2026-11-20T14:30:00Z',
      },
    ],
  },
]

const getMe = () => mockUsers[0]!

// Deliberately simple credentials for VITE_MOCK=1 only. Authenticator code:
// 123456. Real verification and storage are implemented exclusively on the server.
let mockFactorSetup = false
let mockChallenge: { id: string; expires: number } | undefined
let mockRecovery = new Set<string>()
let mockAppPasswords: AppPassword[] = []
let mockAppID = 0
function mockSecret() {
  const alphabet = 'ABCDEFGHJKLMNPQRSTUVWXYZ23456789'
  return Array.from(crypto.getRandomValues(new Uint8Array(16)), (byte) => alphabet[byte & 31]).join('')
}
function mockRecoverySet() {
  mockRecovery = new Set(Array.from({ length: 10 }, () => mockSecret().match(/.{4}/g)!.join('-')))
  return [...mockRecovery]
}
function checkMockFactor(code: string | undefined, recovery?: boolean) {
  return !!code && (recovery ? mockRecovery.delete(code) : code === '123456')
}
function mockLoginResult() {
  const me = getMe()
  loggedIn = true
  return ok({ account: me.account, name: me.name, is_admin: me.is_admin, domains: mockDomains.map((d) => d.name) })
}


/* ==================== Groups & Emails Store ==================== */
let customGroups: GroupOption[] = [...INITIAL_CUSTOM_GROUPS]
let nextGroupId = 20
let nextEmailId = 500

const emailStore = new Map<number, MockEmailRecord>()
for (const item of createSeedEmails()) {
  if (item.id === 101 || item.id === 103 || item.id === 106) {
    item.is_starred = true
  }
  const addresses = getMe().addresses ?? []
  const names = item.type === 1 || item.status === 4 ? [item.from_address] : [...item.to, ...item.cc].map(({ EmailAddress }) => EmailAddress)
  item.mailbox_address_ids = addresses.filter((a) => names.some((name) => name.toLowerCase() === a.address.toLowerCase())).map(({ id }) => id)
  if (item.id === 201) item.mailbox_address_ids = [1, 2]
  if (item.id === 202) {
    item.mailbox_address_ids = []
    item.catch_all_addresses = ['legacy-notices@pmail.dev']
  }
  emailStore.set(item.id, item)
}
const deletedSyntheticIds = new Set<number>()
const readSyntheticOverrides = new Map<number, boolean>()
const starredSyntheticOverrides = new Map<number, boolean>()

/* ==================== Settings Stores ==================== */
let forwardState = {
  enabled: true,
  targets: ['backup@acme-cloud.cn'],
  keep_copy: true,
  updated: dayjs().subtract(2, 'day').toISOString(),
}

let nextRuleId = 10
let mockRules: MailRule[] = [
  {
    id: 1,
    user_id: 1,
    name: 'GitHub 通知自动归类',
    rules: [{ field: 'From', type: 'contains', rule: 'notifications@github.com' }],
    action: 4,
    params: '12',
    sort: 30,
  },
  {
    id: 2,
    user_id: 1,
    name: '云服务器账单自动归入账单分组',
    rules: [
      { field: 'Subject', type: 'contains', rule: '账单' },
      { field: 'From', type: 'contains', rule: 'billing@' },
    ],
    action: 4,
    params: '11',
    sort: 20,
  },
  {
    id: 3,
    user_id: 1,
    name: '自动标记监控日报为已读',
    rules: [{ field: 'Subject', type: 'regex', rule: '每日巡检报告' }],
    action: 1,
    params: '',
    sort: 10,
  },
]

let nextTokenId = 10
let mockTokens: ApiTokenItem[] = [
  {
    id: 1,
    user_id: 1,
    name: 'CI 部署告警通知',
    prefix: 'pmail_9a4f',
    scope: 'send',
    legacy_scope: false,
    never_expires: false,
    last_used_at: dayjs().subtract(35, 'minute').toISOString(),
    expires_at: dayjs().add(68, 'day').toISOString(),
    created: '2026-10-01T08:00:00Z',
  },
  {
    id: 2,
    user_id: 1,
    name: '只读监控巡检',
    prefix: 'pmail_7e2d',
    scope: 'read',
    legacy_scope: false,
    never_expires: false,
    last_used_at: dayjs().subtract(2, 'hour').toISOString(),
    expires_at: dayjs().add(25, 'day').toISOString(),
    created: '2026-11-05T09:15:00Z',
  },
  {
    id: 3,
    user_id: 1,
    name: '本地 CLI 脚本（旧版）',
    prefix: 'pmail_3c1b',
    scope: 'legacy',
    legacy_scope: true,
    never_expires: true,
    last_used_at: null,
    expires_at: null,
    created: '2026-08-10T14:20:00Z',
  },
]

let mockQueue: QueueItem[] = [
  {
    id: 101,
    kind: 'send',
    user_id: 1,
    email_id: 110,
    mail_from: 'admin@pmail.dev',
    rcpt_to: 'ghost@nonexistent-domain.example',
    attempts: 5,
    max_attempts: 5,
    next_retry_at: dayjs().subtract(1, 'hour').toISOString(),
    status: 'failed',
    last_error: '远端 SMTP 拒收：550 5.1.1 User unknown in virtual mailbox table',
    created: dayjs().subtract(3, 'hour').toISOString(),
    updated: dayjs().subtract(1, 'hour').toISOString(),
  },
  {
    id: 102,
    kind: 'send',
    user_id: 1,
    email_id: 109,
    mail_from: 'admin@pmail.dev',
    rcpt_to: 'mingyuan.su@acme-cloud.cn',
    attempts: 1,
    max_attempts: 5,
    next_retry_at: dayjs().add(4, 'minute').toISOString(),
    status: 'pending',
    last_error: '临时退避：421 4.7.0 Greylisted, please try again in 5 minutes',
    created: dayjs().subtract(8, 'minute').toISOString(),
    updated: dayjs().subtract(2, 'minute').toISOString(),
  },
  {
    id: 103,
    kind: 'forward',
    user_id: 2,
    email_id: 104,
    mail_from: 'chen@pmail.dev',
    rcpt_to: 'partner@external-studio.io',
    attempts: 1,
    max_attempts: 5,
    next_retry_at: dayjs().toISOString(),
    status: 'sent',
    last_error: '',
    created: dayjs().subtract(1, 'day').toISOString(),
    updated: dayjs().subtract(1, 'day').toISOString(),
  },
]

/* ==================== Domains & DNS Store ==================== */
const mockPrimaryDomain = 'pmail.dev'
let nextDomainId = 10
let mockDomains: MailDomain[] = [
  {
    id: 1,
    name: mockPrimaryDomain,
    is_default: true,
    dkim_selector: 'default',
    created: '2026-09-01T08:00:00Z',
    updated: '2026-09-01T08:00:00Z',
  },
  {
    id: 2,
    name: 'studio.cn',
    is_default: false,
    dkim_selector: 'default',
    created: '2026-11-18T12:00:00Z',
    updated: '2026-11-18T12:00:00Z',
  },
]

const DKIM_CHUNK_1 =
  'v=DKIM1; k=rsa; p=MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAu7x9kP2LmQ8vT4nR1wY6sF3jH5dG0cB8aN2mK9pL4xV7zW1qE3rT5yU8iO0pA2sD4fG6hJ8kL0zX2cV4bN6mQ8wE0rT2yU4iO6pA8sD0fG2hJ4kL6zX8cV0bN2mQ4wE6rT8yU0iO2pA4sD6fG8h'
const DKIM_CHUNK_2 =
  'J0kL2zX4cV6bN8mQ0wE2rT4yU6iO8pA0sD2fG4hJ6kL8zX0cV2bN4mQ6wE8rT0yU2iO4pA6sD8fG0hJ2kL4zX6cV8bN0mQ2wE4rT6yU8iO0pA2sD4fG6hJ8kL0zX2cV4bN6mIDAQAB'
const FULL_DKIM_VALUE = `${DKIM_CHUNK_1}${DKIM_CHUNK_2}`

function buildMockDns(dom: MailDomain, allGreen: boolean): DomainDnsResponse {
  const mailHost = `mail.${dom.name}`
  const webDomain = dom.id === 1 ? 'mail.pmail.dev' : `web.${dom.name}`
  const dkimHost = `${dom.dkim_selector || 'default'}._domainkey`
  const nowIso = new Date().toISOString()

  if (allGreen) {
    return {
      domain: dom.name,
      mail_host: mailHost,
      web_domain: webDomain,
      server_ipv4: '203.0.113.42',
      server_ipv6: '2001:db8:42::1',
      all_required_ok: true,
      checked_at: nowIso,
      checked_at_iso: nowIso,
      records: [
        {
          key: 'mx',
          purpose: 'receiving',
          type: 'MX',
          host: '@',
          fqdn: dom.name,
          value: mailHost,
          priority: 10,
          ttl: 3600,
          required: true,
          status: 'ok',
          actual: [`10 ${mailHost}.`],
          description: `在 DNS 服务商面板中：类型选 MX，主机记录填 @，记录值填 ${mailHost}，优先级填 10。`,
        },
        {
          key: 'mail_a',
          purpose: 'mail_host',
          type: 'A',
          host: 'mail',
          fqdn: mailHost,
          value: '203.0.113.42',
          ttl: 3600,
          required: true,
          status: 'ok',
          actual: ['203.0.113.42'],
          description: `在 DNS 服务商面板中：类型选 A，主机记录填 mail，记录值填 203.0.113.42。`,
        },
        {
          key: 'spf',
          purpose: 'authentication',
          type: 'TXT',
          host: '@',
          fqdn: dom.name,
          value: 'v=spf1 mx a ~all',
          ttl: 3600,
          required: true,
          status: 'ok',
          actual: ['v=spf1 mx a ~all'],
          description: '在 DNS 服务商面板中：类型选 TXT，主机记录填 @，声明允许本邮件服务器代发邮件。',
        },
        {
          key: 'dkim',
          purpose: 'authentication',
          type: 'TXT',
          host: dkimHost,
          fqdn: `${dkimHost}.${dom.name}`,
          value: FULL_DKIM_VALUE,
          value_chunks: [DKIM_CHUNK_1, DKIM_CHUNK_2],
          ttl: 3600,
          required: true,
          status: 'ok',
          actual: [FULL_DKIM_VALUE],
          description: `在 DNS 服务商面板中：类型选 TXT，主机记录填 ${dkimHost}，粘贴下方公钥值。`,
        },
        {
          key: 'dmarc',
          purpose: 'authentication',
          type: 'TXT',
          host: '_dmarc',
          fqdn: `_dmarc.${dom.name}`,
          value: `v=DMARC1; p=quarantine; rua=mailto:postmaster@${dom.name}`,
          ttl: 3600,
          required: false,
          status: 'ok',
          actual: [`v=DMARC1; p=quarantine; rua=mailto:postmaster@${dom.name}`],
          description: '声明收件方遇到 SPF/DKIM 校验失败时的处理策略，显著提高进箱率。',
        },
        {
          key: 'ptr',
          purpose: 'reverse_dns',
          type: 'PTR',
          host: '42.113.0.203.in-addr.arpa',
          fqdn: '42.113.0.203.in-addr.arpa',
          value: mailHost,
          ttl: 3600,
          required: false,
          status: 'advice',
          actual: [mailHost],
          description: 'PTR 需在云服务器 / VPS 服务商控制台配置，将公网 IP 反解指向邮件主机名。',
          fix_hint: '已检测到当前 PTR 与邮件主机名一致。',
        },
      ],
    }
  }

  return {
    domain: dom.name,
    mail_host: mailHost,
    web_domain: webDomain,
    server_ipv4: '203.0.113.42',
    all_required_ok: false,
    checked_at: nowIso,
    checked_at_iso: nowIso,
    records: [
      {
        key: 'mx',
        purpose: 'receiving',
        type: 'MX',
        host: '@',
        fqdn: dom.name,
        value: mailHost,
        priority: 10,
        ttl: 3600,
        required: true,
        status: 'ok',
        actual: [`10 ${mailHost}.`],
        description: `在 DNS 服务商面板中：类型选 MX，主机记录填 @，记录值填 ${mailHost}，优先级填 10。`,
      },
      {
        key: 'mail_a',
        purpose: 'mail_host',
        type: 'A',
        host: 'mail',
        fqdn: mailHost,
        value: '203.0.113.42',
        ttl: 3600,
        required: true,
        status: 'ok',
        actual: ['203.0.113.42'],
        description: '在 DNS 服务商面板中：类型选 A，主机记录填 mail，记录值填 203.0.113.42。',
      },
      {
        key: 'web_a',
        purpose: 'web',
        type: 'A',
        host: 'web',
        fqdn: webDomain,
        value: '203.0.113.42',
        ttl: 3600,
        required: true,
        status: 'missing',
        actual: [],
        description: `在 DNS 服务商面板中：类型选 A，主机记录填 web，记录值填 203.0.113.42。`,
        fix_hint: `未查询到 ${webDomain} 的 A 记录，请核对 DNS 面板配置。`,
      },
      {
        key: 'spf',
        purpose: 'authentication',
        type: 'TXT',
        host: '@',
        fqdn: dom.name,
        value: 'v=spf1 mx a ~all',
        suggested_value: 'v=spf1 include:_spf.old-provider.net mx a ~all',
        ttl: 3600,
        required: true,
        status: 'mismatch',
        actual: ['v=spf1 include:_spf.old-provider.net ~all'],
        description: '在 DNS 服务商面板中：类型选 TXT，主机记录填 @，记录值填 v=spf1 mx a ~all。',
        fix_hint: '检测到已有旧的 SPF 记录，请勿新建第二条 TXT，而是在原有 SPF 中合并加入 mx 与 a 机制。',
      },
      {
        key: 'dkim',
        purpose: 'authentication',
        type: 'TXT',
        host: dkimHost,
        fqdn: `${dkimHost}.${dom.name}`,
        value: FULL_DKIM_VALUE,
        value_chunks: [DKIM_CHUNK_1, DKIM_CHUNK_2],
        ttl: 3600,
        required: true,
        status: 'missing',
        actual: [],
        description: `在 DNS 服务商面板中：类型选 TXT，主机记录填 ${dkimHost}，记录值粘贴下方 DKIM 公钥（如服务商限制 255 字符，请切换分段形式）。`,
        fix_hint: `未查询到 ${dkimHost}.${dom.name} 的 TXT 记录，请确认主机记录未重复拼接域名后缀。`,
      },
      {
        key: 'dmarc',
        purpose: 'authentication',
        type: 'TXT',
        host: '_dmarc',
        fqdn: `_dmarc.${dom.name}`,
        value: `v=DMARC1; p=quarantine; rua=mailto:postmaster@${dom.name}`,
        ttl: 3600,
        required: false,
        status: 'missing',
        actual: [],
        description: '在 DNS 服务商面板中：类型选 TXT，主机记录填 _dmarc，提升 Gmail/QQ 等大厂邮箱进箱率。',
        fix_hint: '建议添加 DMARC 记录以启用退信聚合报告与防伪造保护。',
      },
      {
        key: 'ptr',
        purpose: 'reverse_dns',
        type: 'PTR',
        host: '42.113.0.203.in-addr.arpa',
        fqdn: '42.113.0.203.in-addr.arpa',
        value: mailHost,
        ttl: 3600,
        required: false,
        status: 'advice',
        actual: ['203-0-113-42.cloud-vps.example'],
        description: 'PTR 需在云服务器 / VPS 服务商控制台设置，而非域名 DNS 面板。',
        fix_hint: `建议前往 VPS 控制台将 203.0.113.42 的反向解析修改为 ${mailHost}。`,
      },
      {
        key: 'imap_alias',
        purpose: 'mail_host',
        type: 'CNAME',
        host: 'imap',
        fqdn: `imap.${dom.name}`,
        value: `${mailHost}.`,
        ttl: 3600,
        required: false,
        status: 'missing',
        actual: [],
        description: '可选：允许邮件客户端使用 imap 域名连接收信服务器。',
      },
      {
        key: 'autoconfig',
        purpose: 'client_configuration',
        type: 'CNAME',
        host: 'autoconfig',
        fqdn: `autoconfig.${dom.name}`,
        value: mailHost,
        ttl: 3600,
        required: false,
        status: 'missing',
        actual: [],
        description: '可选：方便 Thunderbird / Apple Mail 等客户端自动识别 IMAP/SMTP 端口配置。',
      },
      {
        key: 'imaps',
        purpose: 'client_configuration',
        type: 'SRV',
        host: '_imaps._tcp',
        fqdn: `_imaps._tcp.${dom.name}`,
        value: `0 993 ${mailHost}.`,
        priority: 10,
        ttl: 3600,
        required: false,
        status: 'missing',
        actual: [],
        description: '可选：通过 SRV 记录向客户端宣告 IMAPS 加密收信端口。',
      },
    ],
  }
}

/* ==================== Helpers ==================== */
function ok<T>(data: T, status = 200) {
  return HttpResponse.json({ errorNo: 0, errorMsg: '', data }, { status })
}

function fail(errorMsg: string, errorNo = 100, status = 200, data: unknown = null) {
  return HttpResponse.json({ errorNo, errorMsg, data }, { status })
}

function getLang(req: Request): 'zh-CN' | 'en' {
  const header = req.headers.get('X-PMail-Lang') || req.headers.get('Accept-Language') || 'zh-CN'
  return header.toLowerCase().startsWith('en') ? 'en' : 'zh-CN'
}

function getOrCreateRecord(id: number): MockEmailRecord | null {
  if (emailStore.has(id)) return emailStore.get(id) ?? null
  if (id >= 1000 && id < 1000 + VIRTUAL_TOTAL_INBOX && !deletedSyntheticIds.has(id)) {
    const rec = buildSyntheticEmail(id)
    if (readSyntheticOverrides.has(id)) rec.is_read = readSyntheticOverrides.get(id)!
    if (starredSyntheticOverrides.has(id)) rec.is_starred = starredSyntheticOverrides.get(id)!
    return rec
  }
  return null
}

function matchesTag(
  rec: MockEmailRecord,
  tag: { type?: number; status?: number; group_id?: number; starred?: boolean },
) {
  if (tag.starred) {
    return Boolean(rec.is_starred) && rec.status !== 3
  }
  if (typeof tag.type === 'number' && tag.type !== -1 && rec.type !== tag.type) return false
  if (typeof tag.status === 'number' && tag.status !== -1) {
    if (rec.status !== tag.status) return false
  } else {
    if (rec.status === 3 || rec.status === 4 || rec.status === 5) return false
  }
  if (typeof tag.group_id === 'number' && tag.group_id !== -1 && rec.group_id !== tag.group_id) {
    return false
  }
  return true
}

function mockAddressIDs(from: string) {
  return (getMe().addresses ?? []).filter((a) => a.address.toLowerCase() === from.toLowerCase()).map(({ id }) => id)
}

function mockMailboxAddresses(rec: MockEmailRecord) {
  return (getMe().addresses ?? []).filter(({ id }) => rec.mailbox_address_ids?.includes(id))
}

function allowedMockFilter(address?: AddressFilter) {
  return address == null || address === 'all' || address === 'other' || (getMe().addresses ?? []).some(({ id }) => id === address)
}

function matchesMockAddress(rec: MockEmailRecord, address?: AddressFilter) {
  if (address == null || address === 'all') return true
  if (address === 'other') return Boolean(rec.catch_all_addresses?.length)
  return rec.mailbox_address_ids?.includes(address) ?? false
}

function toListItem(rec: MockEmailRecord, includeTrashMeta = false) {
  const iso = dayjs(rec.send_date).isValid()
    ? dayjs(rec.send_date).toISOString()
    : new Date().toISOString()
  return {
    ...(includeTrashMeta
      ? {
          deleted_at: rec.deleted_at ?? null,
          purge_at: computePurgeAt(rec.deleted_at),
        }
      : {}),
    mailbox_addresses: mockMailboxAddresses(rec),
    catch_all_addresses: rec.catch_all_addresses,
    id: rec.id,
    title: rec.subject,
    desc: rec.text,
    datetime: rec.send_date,
    datetime_iso: iso,
    send_date_iso: iso,
    create_time_iso: iso,
    update_time_iso: iso,
    send_time_iso: iso,
    is_read: rec.is_read,
    is_starred: Boolean(rec.is_starred),
    sender: { Name: rec.from_name, EmailAddress: rec.from_address },
    to: rec.to,
    to_list: rec.to.map((x) => ({ name: x.Name, address: x.EmailAddress })),
    attachments_list: rec.attachments.map((att) => ({
      index: att.Index,
      filename: att.Filename,
      content_type: att.ContentType,
      size: att.Size,
      inline: false,
      previewable: Boolean(att.previewContent),
    })),
    dangerous: rec.dangerous,
    auth_status: resolveMockAuthStatus(rec),
    spam: rec.spam
      ? { verdict: rec.spam.verdict }
      : rec.status === 5
        ? { verdict: 'spam' as const }
        : null,
    risk: rec.risk ? { level: rec.risk.level } : { level: 'none' as const },
    verification_code: rec.verification_code ? { code: rec.verification_code.code } : null,
    type: rec.type,
    status: rec.status,
    error: rec.error,
    has_attachments: rec.attachments.length > 0,
    group_id: rec.group_id,
  }
}

export const handlers = [
  /** GET /api/bootstrap returns raw unwrapped JSON per docs/openapi.yaml */
  http.get('/api/bootstrap', async () => {
    await delay(35)
    return HttpResponse.json<BootstrapResponse>({
      version: mockBuildVersion,
      need_register: needRegister,
      logged_in: loggedIn,
      default_lang: 'zh-CN',
      web_domain: `mail.${mockPrimaryDomain}`,
      primary_domain: mockPrimaryDomain,
      domains: loggedIn ? mockDomains.map((d) => d.name) : [],
      features: { register_key: true },
    })
  }),

  http.post('/api/register', async ({ request }) => {
    await delay(200)
    const lang = getLang(request)
    if (!needRegister) {
      return fail(lang === 'en' ? 'System is already initialized' : '系统已初始化，无法重复注册', 405, 403)
    }
    const body = (await request.json()) as {
      registration_key?: string
      account?: string
      password?: string
      display_name?: string
    }
    if ((body.registration_key || '').trim().toUpperCase() !== VALID_REGISTER_KEY) {
      return fail(
        lang === 'en'
          ? `Invalid registration key (mock key: ${VALID_REGISTER_KEY})`
          : `注册密钥无效（Mock 调试密钥：${VALID_REGISTER_KEY}）`,
        100,
        400,
      )
    }
    needRegister = false
    loggedIn = true
    const me = getMe()
    me.account = (body.account || 'admin').toLowerCase()
    me.name = body.display_name || me.account
    me.display_name = me.name
    return ok({
      account: me.account,
      name: me.name,
      is_admin: 1,
      domains: mockDomains.map((d) => d.name),
    })
  }),

  http.post('/api/login', async ({ request }) => {
    await delay(160)
    const lang = getLang(request)
    const body = (await request.json()) as { account?: string; password?: string }
    if (!body.account || !body.password) {
      return fail(lang === 'en' ? 'Account and password are required' : '请输入账号和密码', 100)
    }
    if (body.password === 'wrong') {
      return fail(lang === 'en' ? 'Invalid account or password' : '账号或密码错误', 100)
    }
    getMe().account = body.account
    if (getMe().totp_enabled) {
      mockChallenge = { id: crypto.randomUUID(), expires: Date.now() + 300_000 }
      return ok({ two_factor_required: true, challenge: mockChallenge.id, expires_in: 300 })
    }
    return mockLoginResult()
  }),

  http.post('/api/login/2fa', async ({ request }) => {
    const { challenge, code, recovery } = await request.json() as { challenge?: string; code?: string; recovery?: boolean }
    if (!mockChallenge || challenge !== mockChallenge.id || Date.now() >= mockChallenge.expires) {
      return fail(getLang(request) === 'en' ? 'Sign-in challenge expired.' : '登录验证已过期。', 100, 409)
    }
    if (!checkMockFactor(code, recovery)) return fail(getLang(request) === 'en' ? 'Invalid code.' : '验证码无效。', 100, 400)
    mockChallenge = undefined
    return mockLoginResult()
  }),

  http.post('/api/logout', async () => {
    await delay(60)
    loggedIn = false
    return ok('success')
  }),

  http.get('/api/2fa', () => loggedIn ? ok({ enabled: !!getMe().totp_enabled }) : fail('未登录', 403, 401)),
  http.post('/api/2fa/setup', () => {
    if (!loggedIn) return fail('未登录', 403, 401)
    if (getMe().totp_enabled) return fail('两步验证已开启。', 100, 409)
    mockFactorSetup = true
    const secret = 'PMF4VVWHN3R2H3LA'
    const account = getMe().addresses?.find((address) => address.is_primary)?.address ?? getMe().account
    return ok({ secret, otpauth_uri: `otpauth://totp/${encodeURIComponent(`PMail Pro:${account}`)}?secret=${secret}&issuer=PMail%20Pro` })
  }),
  http.post('/api/2fa/enable', async ({ request }) => {
    if (!loggedIn) return fail('未登录', 403, 401)
    const { code } = await request.json() as { code?: string }
    if (!mockFactorSetup || !checkMockFactor(code)) return fail(getLang(request) === 'en' ? 'Mock mode code: 123456.' : '模拟模式验证码：123456。', 100, 400)
    mockFactorSetup = false
    getMe().totp_enabled = true
    return ok({ enabled: true, recovery_codes: mockRecoverySet() })
  }),
  ...(['disable', 'recovery_codes'] as const).map((action) => http.post(`/api/2fa/${action}`, async ({ request }) => {
    if (!loggedIn) return fail('未登录', 403, 401)
    const body = await request.json() as { password?: string; code?: string; recovery?: boolean }
    if (!body.password || body.password === 'wrong' || !getMe().totp_enabled || !checkMockFactor(body.code, body.recovery)) {
      return fail(getLang(request) === 'en' ? 'Invalid password or code.' : '密码或验证码无效。', 100, 400)
    }
    if (action === 'recovery_codes') return ok({ enabled: true, recovery_codes: mockRecoverySet() })
    getMe().totp_enabled = false
    mockRecovery.clear()
    return ok({ enabled: false })
  })),
  http.get('/api/app_passwords', () => loggedIn ? ok(mockAppPasswords) : fail('未登录', 403, 401)),
  http.post('/api/app_passwords', async ({ request }) => {
    if (!loggedIn) return fail('未登录', 403, 401)
    const { name } = await request.json() as { name: string }
    const app = { id: ++mockAppID, name: name.trim(), created_at: new Date().toISOString(), last_used_at: null }
    mockAppPasswords = [app, ...mockAppPasswords]
    return ok({ ...app, password: mockSecret() })
  }),
  http.delete('/api/app_passwords/:id', ({ params }) => {
    if (!loggedIn) return fail('未登录', 403, 401)
    mockAppPasswords = mockAppPasswords.filter((value) => value.id !== Number(params.id))
    return ok(null)
  }),
  http.post('/api/user/reset_2fa', async ({ request }) => {
    if (!loggedIn || getMe().is_admin !== 1) return fail('无权访问', 405, 403)
    const { id } = await request.json() as { id: number }
    const target = mockUsers.find((value) => value.id === id && id !== getMe().id)
    if (!target) return fail('无效账号', 100, 400)
    target.totp_enabled = false
    return ok(null)
  }),

  /* ==================== User & Admin Users ==================== */
  http.post('/api/user/me', async ({ request }) => {
    await delay(35)
    if (!loggedIn) return fail('未登录', 403, 401)
    const body = (await request.json().catch(() => ({}))) as {
      display_name?: string
      lang?: string
    }
    const me = getMe()
    if (body.display_name) {
      me.display_name = body.display_name
      me.name = body.display_name
    }
    if (body.lang) me.lang = body.lang
    return ok(me)
  }),

  http.post('/api/user/change_password', async ({ request }) => {
    await delay(150)
    const body = (await request.json()) as { old_password?: string; new_password?: string }
    if (!body.old_password || !body.new_password || body.new_password.length < 8) {
      return fail('密码格式不正确', 100)
    }
    loggedIn = false
    return ok(null)
  }),

  http.post('/api/user/list', async ({ request }) => {
    await delay(60)
    const body = (await request.json().catch(() => ({}))) as {
      current_page?: number
      page_size?: number
    }
    const page = Math.max(1, body.current_page ?? 1)
    const size = Math.max(1, body.page_size ?? 15)
    const totalPage = Math.max(1, Math.ceil(mockUsers.length / size))
    const slice = mockUsers.slice((page - 1) * size, page * size)
    return ok({ current_page: page, total_page: totalPage, list: slice })
  }),

  http.post('/api/user/create', async ({ request }) => {
    await delay(120)
    const body = (await request.json()) as {
      account: string
      display_name?: string
      quota_mb?: number
    }
    const id = nextUserId++
    const defaultDom = mockDomains.find((d) => d.is_default) ?? mockDomains[0]!
    const addr: MailboxAddress = {
      id: nextAddressId++,
      user_id: id,
      local_part: body.account,
      domain_id: defaultDom.id,
      is_primary: true,
      can_send: true,
      domain: defaultDom.name,
      address: `${body.account}@${defaultDom.name}`,
      created: new Date().toISOString(),
    }
    const created: UserProfile = {
      id,
      account: body.account,
      name: body.display_name || body.account,
      display_name: body.display_name || body.account,
      disabled: 0,
      is_admin: 0,
      quota_mb: body.quota_mb ?? 1024,
      used_bytes: 0,
      created: new Date().toISOString(),
      last_login_at: null,
      addresses: [addr],
    }
    mockUsers = [...mockUsers, created]
    return ok(created)
  }),

  http.post('/api/user/edit', async ({ request }) => {
    await delay(100)
    const body = (await request.json()) as {
      id: number
      display_name?: string
      disabled?: number
      quota_mb?: number
    }
    const target = mockUsers.find((u) => u.id === body.id)
    if (!target) return fail('用户不存在', 100, 404)
    if (body.display_name !== undefined) {
      target.display_name = body.display_name
      target.name = body.display_name
    }
    if (body.disabled !== undefined) target.disabled = body.disabled
    if (body.quota_mb !== undefined) target.quota_mb = body.quota_mb
    return ok(target)
  }),

  http.post('/api/user/reset_password', async () => {
    await delay(100)
    return ok(null)
  }),

  http.post('/api/user/delete', async ({ request }) => {
    await delay(100)
    const body = (await request.json()) as { id: number }
    const target = mockUsers.find((u) => u.id === body.id)
    if (target) target.disabled = 1
    return ok(null)
  }),

  /* ==================== Addresses ==================== */
  http.post('/api/address/list', async ({ request }) => {
    await delay(40)
    const body = (await request.json().catch(() => ({}))) as { user_id?: number }
    const uid = body.user_id || getMe().id
    const u = mockUsers.find((x) => x.id === uid)
    return ok(u?.addresses ?? [])
  }),

  http.post('/api/address/create', async ({ request }) => {
    await delay(100)
    const body = (await request.json()) as {
      user_id?: number
      local_part: string
      domain_id: number
      can_send?: boolean
    }
    const uid = body.user_id || getMe().id
    const u = mockUsers.find((x) => x.id === uid)
    const dom = mockDomains.find((d) => d.id === body.domain_id) ?? mockDomains[0]!
    if (!u) return fail('用户不存在', 100)
    const forcePending =
      typeof window !== 'undefined' && localStorage.getItem('pmail_mock_addr_pending') === '1'
    const shouldBePending = forcePending || (mockAddressApprovalRequired && u.is_admin !== 1)
    if (shouldBePending) {
      const pendingCount = (u.addresses ?? []).filter((a) => a.status === 'pending').length
      if (pendingCount >= 10) {
        return fail('待审批地址数量已达上限', 100, 409, { code: 'address_pending_limit' })
      }
    }
    const nowIso = new Date().toISOString()
    const addr: MailboxAddress = {
      id: nextAddressId++,
      user_id: uid,
      local_part: body.local_part.toLowerCase(),
      domain_id: dom.id,
      is_primary: !shouldBePending && (u.addresses?.length ?? 0) === 0,
      can_send: shouldBePending ? false : (body.can_send ?? true),
      status: shouldBePending ? 'pending' : 'active',
      requested_at: nowIso,
      reviewed_at: shouldBePending ? null : nowIso,
      reviewed_by: shouldBePending ? null : getMe().id,
      reject_reason: '',
      domain: dom.name,
      address: `${body.local_part.toLowerCase()}@${dom.name}`,
      created: nowIso,
    }
    u.addresses = [...(u.addresses ?? []), addr]
    if (shouldBePending) {
      mockAddressRequests = [
        { ...addr, status: 'pending', applicant: u.account },
        ...mockAddressRequests,
      ]
    }
    return ok(addr)
  }),

  http.post('/api/address/update', async ({ request }) => {
    await delay(70)
    const body = (await request.json()) as { id: number; can_send: boolean }
    for (const u of mockUsers) {
      const found = u.addresses?.find((a) => a.id === body.id)
      if (found) found.can_send = body.can_send
    }
    return ok(null)
  }),

  http.post('/api/address/set_primary', async ({ request }) => {
    await delay(70)
    const body = (await request.json()) as { id: number }
    for (const u of mockUsers) {
      if (u.addresses?.some((a) => a.id === body.id)) {
        u.addresses = u.addresses.map((a) => ({
          ...a,
          is_primary: a.id === body.id,
          can_send: a.id === body.id ? true : a.can_send,
        }))
      }
    }
    return ok(null)
  }),

  http.post('/api/address/delete', async ({ request }) => {
    await delay(70)
    const body = (await request.json()) as { id: number }
    for (const u of mockUsers) {
      if (u.addresses) {
        u.addresses = u.addresses.filter((a) => a.id !== body.id)
      }
    }
    mockAddressRequests = mockAddressRequests.filter((r) => r.id !== body.id)
    return ok(null)
  }),

  /* ==================== Admin Overview & Address Approval (V7-C29) ==================== */
  http.get('/api/admin/overview', async () => {
    await delay(30)
    return ok({
      pending_address_requests: countPendingAddressRequests(),
    })
  }),

  http.get('/api/admin/address-settings', async () => {
    await delay(30)
    return ok({
      address_approval_required: mockAddressApprovalRequired,
    })
  }),

  http.put('/api/admin/address-settings', async ({ request }) => {
    await delay(80)
    const body = (await request.json()) as { address_approval_required?: boolean }
    if (typeof body.address_approval_required === 'boolean') {
      mockAddressApprovalRequired = body.address_approval_required
    }
    return ok({
      address_approval_required: mockAddressApprovalRequired,
    })
  }),

  http.get('/api/admin/address-requests', async ({ request }) => {
    await delay(50)
    const url = new URL(request.url)
    const status = (url.searchParams.get('status') || 'pending') as AddressStatus
    const page = Math.max(1, Number(url.searchParams.get('page') || 1))
    const pageSize = Math.min(100, Math.max(1, Number(url.searchParams.get('page_size') || 20)))
    const filtered = mockAddressRequests.filter((r) => r.status === status)
    const start = (page - 1) * pageSize
    return ok({
      items: filtered.slice(start, start + pageSize),
      page,
      page_size: pageSize,
      total: filtered.length,
      pending_address_requests: countPendingAddressRequests(),
    })
  }),

  http.post('/api/admin/address-requests/:id/approve', async ({ params }) => {
    await delay(100)
    const id = Number(params.id)
    const req = mockAddressRequests.find((r) => r.id === id)
    if (!req) {
      return fail('申请不存在', 100, 404, { code: 'address_not_found' })
    }
    if (req.status !== 'pending') {
      return fail('该申请已审核', 100, 409, { code: 'address_already_reviewed' })
    }
    const nowIso = new Date().toISOString()
    req.status = 'active'
    req.can_send = true
    req.reviewed_at = nowIso
    req.reviewed_by = getMe().id
    req.reject_reason = ''
    for (const u of mockUsers) {
      const addr = u.addresses?.find((a) => a.id === id)
      if (addr) {
        addr.status = 'active'
        addr.can_send = true
        addr.reviewed_at = nowIso
        addr.reviewed_by = getMe().id
        addr.reject_reason = ''
      }
    }
    return ok(req)
  }),

  http.post('/api/admin/address-requests/:id/reject', async ({ params, request }) => {
    await delay(100)
    const id = Number(params.id)
    const body = (await request.json().catch(() => ({}))) as { reason?: string }
    const reason = (body.reason ?? '').trim()
    if (Array.from(reason).length > 200) {
      return fail('拒绝原因不能超过 200 字', 100, 400, { code: 'address_reject_reason_invalid' })
    }
    const req = mockAddressRequests.find((r) => r.id === id)
    if (!req) {
      return fail('申请不存在', 100, 404, { code: 'address_not_found' })
    }
    if (req.status !== 'pending') {
      return fail('该申请已审核', 100, 409, { code: 'address_already_reviewed' })
    }
    const nowIso = new Date().toISOString()
    req.status = 'rejected'
    req.can_send = false
    req.reviewed_at = nowIso
    req.reviewed_by = getMe().id
    req.reject_reason = reason
    for (const u of mockUsers) {
      const addr = u.addresses?.find((a) => a.id === id)
      if (addr) {
        addr.status = 'rejected'
        addr.can_send = false
        addr.reviewed_at = nowIso
        addr.reviewed_by = getMe().id
        addr.reject_reason = reason
      }
    }
    return ok(req)
  }),

  /* ==================== Forward / Rules / Tokens / Queue ==================== */
  http.post('/api/forward/get', async () => {
    await delay(40)
    return ok({
      setting: {
        user_id: 1,
        enabled: forwardState.enabled,
        targets: JSON.stringify(forwardState.targets),
        keep_copy: forwardState.keep_copy,
        updated: forwardState.updated,
      },
      targets: forwardState.targets,
    })
  }),

  http.post('/api/forward/set', async ({ request }) => {
    await delay(100)
    const body = (await request.json()) as {
      enabled: boolean
      targets: string[]
      keep_copy: boolean
    }
    forwardState = {
      enabled: body.enabled,
      targets: body.targets ?? [],
      keep_copy: body.keep_copy ?? true,
      updated: new Date().toISOString(),
    }
    return ok({
      setting: {
        user_id: 1,
        enabled: forwardState.enabled,
        targets: JSON.stringify(forwardState.targets),
        keep_copy: forwardState.keep_copy,
        updated: forwardState.updated,
      },
      targets: forwardState.targets,
    })
  }),

  http.post('/api/rule/get', async () => {
    await delay(40)
    return ok(mockRules)
  }),

  http.post('/api/rule/add', async ({ request }) => {
    await delay(90)
    const body = (await request.json()) as Omit<MailRule, 'user_id'>
    mockRules = [
      ...mockRules,
      {
        ...body,
        id: nextRuleId++,
        user_id: 1,
        sort: body.sort ?? 10,
        params: body.params ?? '',
      },
    ]
    return ok('success')
  }),

  http.post('/api/rule/update', async ({ request }) => {
    await delay(80)
    const body = (await request.json()) as MailRule
    mockRules = mockRules.map((r) => (r.id === body.id ? { ...r, ...body } : r))
    return ok('success')
  }),

  http.post('/api/rule/del', async ({ request }) => {
    await delay(80)
    const body = (await request.json()) as { id: number }
    mockRules = mockRules.filter((r) => r.id !== body.id)
    return ok('success')
  }),

  http.post('/api/token/list', async () => {
    await delay(40)
    return ok(mockTokens)
  }),

  http.post('/api/token/create', async ({ request }) => {
    await delay(120)
    const body = (await request.json()) as {
      name?: string
      scope?: 'read' | 'send' | 'admin'
      never_expires?: boolean
      expires_at?: string | null
    }
    if (body.never_expires && body.expires_at) {
      return fail('never_expires 与 expires_at 不能同时设置', 100, 400)
    }
    const rand = Math.random().toString(36).slice(2, 10) + Math.random().toString(36).slice(2, 14)
    const plaintext = `pmail_${rand}`
    const neverExpires = Boolean(body.never_expires)
    const item: ApiTokenItem = {
      id: nextTokenId++,
      user_id: 1,
      name: body.name || 'API Token',
      prefix: plaintext.slice(0, 10),
      scope: body.scope ?? 'read',
      legacy_scope: false,
      never_expires: neverExpires,
      last_used_at: null,
      expires_at: neverExpires ? null : (body.expires_at ?? dayjs().add(90, 'day').toISOString()),
      created: new Date().toISOString(),
    }
    mockTokens = [item, ...mockTokens]
    return ok({ token: plaintext, item })
  }),

  http.post('/api/token/delete', async ({ request }) => {
    await delay(80)
    const body = (await request.json()) as { id: number }
    mockTokens = mockTokens.filter((t) => t.id !== body.id)
    return ok(null)
  }),

  http.post('/api/queue/list', async () => {
    await delay(45)
    return ok(mockQueue)
  }),

  http.post('/api/queue/retry', async ({ request }) => {
    await delay(90)
    const body = (await request.json()) as { id: number }
    mockQueue = mockQueue.map((q) =>
      q.id === body.id
        ? { ...q, status: 'pending', next_retry_at: new Date().toISOString(), updated: new Date().toISOString() }
        : q,
    )
    return ok(null)
  }),

  http.post('/api/queue/delete', async ({ request }) => {
    await delay(80)
    const body = (await request.json()) as { id: number }
    mockQueue = mockQueue.filter((q) => q.id !== body.id)
    return ok(null)
  }),

  /* ==================== Domains & DNS Wizard ==================== */
  http.post('/api/domain/list', async () => {
    await delay(45)
    return ok(mockDomains)
  }),

  http.post('/api/domain/create', async ({ request }) => {
    await delay(120)
    const body = (await request.json()) as { name: string }
    const created: MailDomain = {
      id: nextDomainId++,
      name: body.name.toLowerCase(),
      is_default: false,
      dkim_selector: 'default',
      created: new Date().toISOString(),
      updated: new Date().toISOString(),
    }
    mockDomains = [...mockDomains, created]
    return ok(created)
  }),

  http.post('/api/domain/set_default', async ({ request }) => {
    await delay(80)
    const body = (await request.json()) as { id: number }
    mockDomains = mockDomains.map((d) => ({ ...d, is_default: d.id === body.id }))
    return ok(null)
  }),

  http.post('/api/domain/delete', async ({ request }) => {
    await delay(90)
    const body = (await request.json()) as { id: number }
    const target = mockDomains.find((d) => d.id === body.id)
    if (target?.is_default) return fail('不能删除默认域名', 100, 400)
    mockDomains = mockDomains.filter((d) => d.id !== body.id)
    return ok(null)
  }),

  http.post('/api/domain/dns', async ({ request }) => {
    const body = (await request.json()) as { id: number; refresh?: boolean }
    await delay(body.refresh ? 420 : 60)
    const dom = mockDomains.find((d) => d.id === body.id) ?? mockDomains[0]!
    return ok(buildMockDns(dom, dom.id === 1))
  }),

  http.get('/api/domain/dns/zone', ({ request }) => {
    const url = new URL(request.url)
    const id = Number(url.searchParams.get('id') || '1')
    const dom = mockDomains.find((d) => d.id === id) ?? mockDomains[0]!
    const zone = `; PMail Pro BIND Zone File for ${dom.name}\n$ORIGIN ${dom.name}.\n$TTL 3600\n@\tIN\tMX\t10 mail.${dom.name}.\nmail\tIN\tA\t203.0.113.42\n@\tIN\tTXT\t"v=spf1 mx a ~all"\n${dom.dkim_selector}._domainkey\tIN\tTXT\t("${DKIM_CHUNK_1}" "${DKIM_CHUNK_2}")\n_dmarc\tIN\tTXT\t"v=DMARC1; p=quarantine; rua=mailto:postmaster@${dom.name}"\n`
    return new HttpResponse(zone, {
      status: 200,
      headers: {
        'Content-Type': 'text/plain; charset=utf-8',
        'Content-Disposition': `attachment; filename="${dom.name}.zone"`,
      },
    })
  }),

  /* ==================== Groups & Mail ==================== */
  http.post('/api/group', async ({ request }) => {
    await delay(35)
    if (!loggedIn) return fail('未登录', 403, 401)
    const { address } = await request.json() as { address?: AddressFilter }
    if (!allowedMockFilter(address)) return fail('邮箱地址不属于当前用户', 405, 403)
    const allSeed = Array.from(emailStore.values()).filter((record) => matchesMockAddress(record, address))
    const inboxUnread = allSeed.filter(
      (e) => e.type === 0 && e.status === 0 && e.group_id === 0 && !e.is_read,
    ).length
    const starredCount = allSeed.filter((e) => e.is_starred && e.status !== 3).length
    const draftsCount = allSeed.filter((e) => e.status === 4).length
    const junkUnread = allSeed.filter((e) => e.status === 5 && !e.is_read).length

    return ok([
      {
        id: 0,
        label: '收件箱',
        tag: JSON.stringify({ type: 0, status: -1, group_id: 0 }),
        unread: inboxUnread,
      },
      {
        id: 0,
        label: '星标邮件',
        tag: JSON.stringify({ type: -1, status: -1, group_id: -1, starred: true }),
        unread: starredCount,
      },
      {
        id: 0,
        label: '已发送',
        tag: JSON.stringify({ type: 1, status: -1 }),
        unread: 0,
      },
      {
        id: 0,
        label: '草稿箱',
        tag: JSON.stringify({ type: 0, status: 4 }),
        total: draftsCount,
        unread: allSeed.filter((e) => e.status === 4 && !e.is_read).length,
      },
      {
        id: 0,
        label: '广告箱',
        tag: JSON.stringify({ type: -1, status: 5 }),
        unread: junkUnread,
      },
      {
        id: 0,
        label: '已删除',
        tag: JSON.stringify({ type: -1, status: 3 }),
        unread: 0,
      },
      ...customGroups.map((g) => ({
        id: g.id,
        label: g.name,
        tag: JSON.stringify({ type: -1, status: -1, group_id: g.id }),
        unread: allSeed.filter((e) => e.group_id === g.id && e.status !== 3 && !e.is_read).length,
      })),
    ])
  }),

  http.post('/api/group/list', async () => {
    await delay(30)
    if (!loggedIn) return fail('未登录', 403, 401)
    return ok([
      { id: SYSTEM_GROUP_IDS.INBOX, name: '收件箱', parent_id: 0, full_path: '/' },
      { id: SYSTEM_GROUP_IDS.JUNK, name: '广告箱', parent_id: 0, full_path: '/' },
      { id: SYSTEM_GROUP_IDS.DELETED, name: '已删除', parent_id: 0, full_path: '/' },
      ...customGroups,
    ])
  }),

  http.post('/api/group/add', async ({ request }) => {
    await delay(90)
    const body = (await request.json()) as { name?: string; parent_id?: number }
    const name = (body.name || '').trim()
    if (!name) return fail('分组名称不能为空', 100)
    const id = nextGroupId++
    const parent = customGroups.find((g) => g.id === body.parent_id)
    const fullPath = parent ? `${parent.full_path}/${name}` : `/${name}`
    customGroups = [...customGroups, { id, name, parent_id: body.parent_id ?? 0, full_path: fullPath }]
    return ok(id)
  }),

  http.post('/api/group/del', async ({ request }) => {
    await delay(90)
    const body = (await request.json()) as { id?: number }
    customGroups = customGroups.filter((g) => g.id !== body.id)
    for (const rec of emailStore.values()) {
      if (rec.group_id === body.id) rec.group_id = 0
    }
    return ok(true)
  }),

  http.get('/api/email/address_counts', () => {
    if (!loggedIn) return fail('未登录', 403, 401)
    const records = Array.from(emailStore.values())
    const unread = records.filter((rec) => rec.type === 0 && rec.status === 0 && rec.group_id === 0 && !rec.is_read)
    return ok({
      addresses: (getMe().addresses ?? [])
        .filter((a) => !a.status || a.status === 'active')
        .map((a) => ({ ...a, unread: unread.filter((rec) => matchesMockAddress(rec, a.id)).length })),
      all_unread: unread.length,
      other: { unread: unread.filter((rec) => matchesMockAddress(rec, 'other')).length, has_mail: records.some((rec) => matchesMockAddress(rec, 'other')) },
    })
  }),

  http.get('/api/email/changes', async ({ request }) => {
    if (!loggedIn) return fail('未登录', 403, 401)
    const url = new URL(request.url)
    const cursor = url.searchParams.get('cursor') ?? ''
    const waitParam = Number(url.searchParams.get('wait') ?? '25')
    const waitSec = Number.isFinite(waitParam) ? Math.max(0, Math.min(25, waitParam)) : 25

    // First call with empty cursor returns immediately
    if (!cursor) {
      return ok({
        cursor: String(mockLiveChangeSeq),
        changed: false,
        new_count: 0,
        new_items: [],
      })
    }

    const liveDisabled =
      typeof window !== 'undefined' && localStorage.getItem('pmail_mock_live') === '0'
    const customIntervalMs =
      typeof window !== 'undefined'
        ? Number(localStorage.getItem('pmail_mock_live_interval_ms') || '0')
        : 0
    const intervalMs = customIntervalMs > 0 ? customIntervalMs : 15_000

    if (liveDisabled) {
      const waitMs = waitSec > 0 ? Math.min(waitSec * 1000, 15_000) : 50
      await abortableDelay(waitMs, request.signal)
      return ok({
        cursor: String(mockLiveChangeSeq),
        changed: false,
        new_count: 0,
        new_items: [],
      })
    }

    const delayMs = waitSec > 0 ? Math.min(waitSec * 1000, intervalMs) : 60
    const completed = await abortableDelay(delayMs, request.signal)
    if (!completed) {
      return ok({
        cursor: String(mockLiveChangeSeq),
        changed: false,
        new_count: 0,
        new_items: [],
      })
    }

    const tpl = MOCK_LIVE_TEMPLATES[mockLiveSampleIdx % MOCK_LIVE_TEMPLATES.length]!
    mockLiveSampleIdx += 1
    const newId = nextEmailId++
    const now = dayjs()
    const newRecord: MockEmailRecord = {
      id: newId,
      type: 0,
      status: 0,
      group_id: 0,
      mailbox_address_ids: [1],
      is_read: false,
      is_starred: false,
      dangerous: false,
      auth_status: 'pass',
      error: '',
      subject: tpl.subject,
      from_name: tpl.from_name,
      from_address: tpl.from_address,
      to: [{ Name: '林深', EmailAddress: 'admin@pmail.dev' }],
      cc: [],
      reply_to: [],
      send_date: now.format('YYYY-MM-DD HH:mm:ss'),
      text: tpl.text,
      html: `<p>${tpl.text}</p>`,
      size: 1840,
      msg_id: `<live-${newId}@pmail.dev>`,
      attachments: [],
    }
    emailStore.set(newId, newRecord)
    mockLiveChangeSeq += 1

    return ok({
      cursor: String(mockLiveChangeSeq),
      changed: true,
      new_count: 1,
      new_items: [
        {
          id: newId,
          subject: newRecord.subject,
          from_name: newRecord.from_name,
          from_address: newRecord.from_address,
          group_id: 0,
          created: now.toISOString(),
          datetime_iso: now.toISOString(),
        },
      ],
    })
  }),

  http.post('/api/email/list', async ({ request }) => {
    await delay(50)
    if (!loggedIn) return fail('未登录', 403, 401)
    const body = (await request.json()) as {
      address?: AddressFilter
      keyword?: string
      tag?: string
      current_page?: number
      page_size?: number
    }
    if (!allowedMockFilter(body.address)) return fail('邮箱地址不属于当前用户', 405, 403)
    const page = Math.max(1, body.current_page ?? 1)
    const pageSize = Math.max(1, body.page_size ?? 50)
    const kw = (body.keyword || '').trim().toLowerCase()

    let tagObj: { type?: number; status?: number; group_id?: number; starred?: boolean } = {
      type: -1,
      status: -1,
      group_id: -1,
    }
    try {
      if (body.tag) tagObj = JSON.parse(body.tag)
    } catch {
      // default
    }

    const isDefaultInbox = tagObj.type === 0 && tagObj.group_id === 0 && !tagObj.starred

    const seedMatched = Array.from(emailStore.values())
      .filter((rec) => matchesTag(rec, tagObj) && matchesMockAddress(rec, body.address))
      .filter((rec) => {
        if (!kw) return true
        return (
          rec.subject.toLowerCase().includes(kw) ||
          rec.from_name.toLowerCase().includes(kw) ||
          rec.from_address.toLowerCase().includes(kw) ||
          rec.text.toLowerCase().includes(kw)
        )
      })
      .sort((a, b) => (b.send_date > a.send_date ? 1 : -1))

    if (isDefaultInbox && !kw && (body.address == null || body.address === 'all')) {
      const seedCount = seedMatched.length
      const syntheticCount = VIRTUAL_TOTAL_INBOX - deletedSyntheticIds.size
      const total = seedCount + syntheticCount
      const totalPage = Math.max(1, Math.ceil(total / pageSize))
      const start = (page - 1) * pageSize
      const result: ReturnType<typeof toListItem>[] = []

      for (let i = start; i < start + pageSize && i < total; i++) {
        if (i < seedCount) {
          const item = seedMatched[i]
          if (item) result.push(toListItem(item))
        } else {
          const synId = 1000 + (i - seedCount)
          if (!deletedSyntheticIds.has(synId)) {
            const syn = getOrCreateRecord(synId)
            if (syn) result.push(toListItem(syn))
          }
        }
      }

      return ok({ current_page: page, total_page: totalPage, total, list: result })
    }

    const total = seedMatched.length
    const totalPage = Math.max(1, Math.ceil(total / pageSize))
    const isTrashList = tagObj.status === 3
    const slice = seedMatched
      .slice((page - 1) * pageSize, page * pageSize)
      .map((item) => toListItem(item, isTrashList))

    return ok({ current_page: page, total_page: totalPage, total, list: slice })
  }),

  http.post('/api/email/detail', async ({ request }) => {
    await delay(40)
    if (!loggedIn) return fail('未登录', 403, 401)
    const body = (await request.json()) as { id?: number }
    const rec = body.id ? getOrCreateRecord(body.id) : null
    if (!rec) return fail('邮件不存在', 100, 404)
    rec.is_read = true
    if (rec.id >= 1000) readSyntheticOverrides.set(rec.id, true)
    const userAI = getMockUserAI()
    const engines: ('web' | 'ai')[] = []
    if (mockAdminTranslation.web_enabled) engines.push('web')
    if (mockAdminTranslation.ai_enabled && userAI.ai_user_config_enabled && userAI.configured) {
      engines.push('ai')
    }
    const aiAvailable = Boolean(userAI.ai_user_config_enabled && userAI.configured)
    const isTrash = rec.status === 3
    return ok({
      ...toEmailDetail(rec),
      mailbox_addresses: mockMailboxAddresses(rec),
      ...(isTrash
        ? {
            deleted_at: rec.deleted_at ?? null,
            purge_at: computePurgeAt(rec.deleted_at),
          }
        : {}),
      translation_available: engines.length > 0,
      needs_translation: looksForeignForChinese(`${rec.subject} ${rec.text}`),
      translation_engines: engines,
      ai_available: aiAvailable,
    })
  }),

  http.post('/api/email/read', async ({ request }) => {
    await delay(40)
    const body = (await request.json()) as { ids?: number[]; is_read?: boolean }
    const targetRead = body.is_read ?? true
    for (const id of body.ids ?? []) {
      const rec = emailStore.get(id)
      if (rec) rec.is_read = targetRead
      else if (id >= 1000) readSyntheticOverrides.set(id, targetRead)
    }
    return ok('success')
  }),

  http.post('/api/email/star', async ({ request }) => {
    await delay(40)
    const body = (await request.json()) as { ids?: number[]; starred?: boolean }
    const targetStar = Boolean(body.starred)
    for (const id of body.ids ?? []) {
      const rec = emailStore.get(id)
      if (rec) rec.is_starred = targetStar
      else if (id >= 1000) starredSyntheticOverrides.set(id, targetStar)
    }
    return ok('success')
  }),

  http.post('/api/email/draft/save', async ({ request }) => {
    await delay(80)
    const body = (await request.json()) as {
      id?: number
      from_address?: string
      to?: { name?: string; email: string }[]
      cc?: { name?: string; email: string }[]
      subject?: string
      html?: string
      text?: string
    }
    const id = body.id && emailStore.has(body.id) ? body.id : nextEmailId++
    const me = getMe()
    emailStore.set(id, {
      id,
      type: 0,
      status: 4,
      group_id: 0,
      is_read: true,
      is_starred: false,
      dangerous: false,
      error: '',
      subject: body.subject || '(无主题草稿)',
      from_name: me.display_name || me.name,
      from_address: body.from_address || 'admin@pmail.dev',
      mailbox_address_ids: mockAddressIDs(body.from_address || 'admin@pmail.dev'),
      to: (body.to ?? []).map((x) => ({ Name: x.name || '', EmailAddress: x.email })),
      cc: (body.cc ?? []).map((x) => ({ Name: x.name || '', EmailAddress: x.email })),
      reply_to: [],
      send_date: dayjs().format('YYYY-MM-DD HH:mm:ss'),
      text: body.text || '',
      html: body.html || '',
      size: (body.html || body.text || '').length + 256,
      msg_id: `<draft-${id}@pmail.dev>`,
      attachments: [],
    })
    return ok({ id })
  }),

  http.post('/api/email/draft/delete', async ({ request }) => {
    await delay(60)
    const body = (await request.json()) as { id?: number; ids?: number[] }
    const toDelete = body.ids ?? (body.id ? [body.id] : [])
    for (const id of toDelete) {
      emailStore.delete(id)
    }
    return ok('success')
  }),

  http.post('/api/email/del', async ({ request }) => {
    await delay(50)
    const body = (await request.json()) as { ids?: number[]; forcedDel?: boolean }
    for (const id of body.ids ?? []) {
      const rec = emailStore.get(id)
      if (rec) {
        if (body.forcedDel || rec.status === 3) {
          emailStore.delete(id)
        } else {
          rec.prev_status = rec.status
          rec.prev_group_id = rec.group_id
          rec.status = 3
          rec.group_id = 0
          rec.deleted_at = Date.now()
        }
      } else if (id >= 1000) {
        deletedSyntheticIds.add(id)
      }
    }
    return ok('success')
  }),

  http.post('/api/email/restore', async ({ request }) => {
    await delay(60)
    if (!loggedIn) return fail('未登录', 403, 401)
    const body = (await request.json()) as { ids?: number[] }
    const seen = new Set<number>()
    let restored = 0
    for (const id of body.ids ?? []) {
      if (seen.has(id)) continue
      seen.add(id)
      const rec = emailStore.get(id)
      if (!rec || rec.status !== 3) continue
      rec.status = rec.prev_status ?? (rec.type === 1 ? 1 : 0)
      rec.group_id = rec.prev_group_id ?? 0
      rec.deleted_at = null
      rec.prev_status = undefined
      rec.prev_group_id = undefined
      restored++
    }
    return ok({ restored })
  }),

  http.post('/api/email/trash/empty', async () => {
    await delay(90)
    if (!loggedIn) return fail('未登录', 403, 401)
    let deleted = 0
    for (const [id, rec] of Array.from(emailStore.entries())) {
      if (rec.status === 3) {
        emailStore.delete(id)
        deleted++
      }
    }
    return ok({ deleted })
  }),

  http.post('/api/email/move', async ({ request }) => {
    await delay(50)
    const body = (await request.json()) as { ids?: number[]; group_id?: number }
    const gid = body.group_id ?? SYSTEM_GROUP_IDS.INBOX
    for (const id of body.ids ?? []) {
      if (id >= 1000 && gid === SYSTEM_GROUP_IDS.INBOX) {
        deletedSyntheticIds.delete(id)
        continue
      }
      const rec = emailStore.get(id)
      if (!rec) continue
      if (gid === SYSTEM_GROUP_IDS.INBOX) {
        rec.status = 0
        rec.group_id = 0
      } else if (gid === SYSTEM_GROUP_IDS.JUNK) {
        rec.status = 5
        rec.group_id = 0
      } else if (gid === SYSTEM_GROUP_IDS.DELETED) {
        rec.status = 3
      } else if (gid === SYSTEM_GROUP_IDS.SENT) {
        rec.status = 1
        rec.group_id = 0
      } else {
        rec.status = 0
        rec.group_id = gid
      }
    }
    return ok('success')
  }),

  http.post('/api/email/send', async ({ request }) => {
    await delay(220)
    const body = (await request.json()) as {
      from?: { name?: string; email?: string }
      from_address?: string
      to?: { name?: string; email: string }[]
      cc?: { name?: string; email: string }[]
      bcc?: { name?: string; email: string }[]
      subject?: string
      text?: string
      html?: string
    }
    const allRecs = [...(body.to ?? []), ...(body.cc ?? []), ...(body.bcc ?? [])]
    const recipients = allRecs.map((r) => {
      if (r.email.includes('fail@')) {
        return { address: r.email, status: 'failed' as const, error: '550 5.1.1 Mailbox unavailable' }
      }
      if (r.email.endsWith('@pmail.dev')) {
        return { address: r.email, status: 'delivered' as const, error: '' }
      }
      return { address: r.email, status: 'queued' as const, error: '' }
    })

    const id = nextEmailId++
    const me = getMe()
    emailStore.set(id, {
      id,
      type: 1,
      status: 1,
      group_id: 0,
      is_read: true,
      is_starred: false,
      dangerous: false,
      error: '',
      subject: body.subject || '(无主题)',
      from_name: body.from?.name || me.name,
      from_address: body.from_address || body.from?.email || 'admin@pmail.dev',
      mailbox_address_ids: mockAddressIDs(body.from_address || body.from?.email || 'admin@pmail.dev'),
      to: (body.to ?? []).map((x) => ({ Name: x.name || '', EmailAddress: x.email })),
      cc: (body.cc ?? []).map((x) => ({ Name: x.name || '', EmailAddress: x.email })),
      reply_to: [],
      send_date: dayjs().format('YYYY-MM-DD HH:mm:ss'),
      text: body.text || '',
      html: body.html || '',
      size: (body.html || body.text || '').length + 512,
      msg_id: `<sent-${id}@pmail.dev>`,
      attachments: [],
    })

    const hasQueued = recipients.some((r) => r.status === 'queued')
    const allFailed = recipients.every((r) => r.status === 'failed')
    return ok({ status: allFailed ? 'failed' : hasQueued ? 'queued' : 'sent', recipients })
  }),

  http.post('/api/email/forward', async ({ request }) => {
    await delay(220)
    const body = (await request.json()) as { to?: string[]; cc?: string[] }
    const all = [...(body.to ?? []), ...(body.cc ?? [])]
    return ok({
      status: 'queued',
      recipients: all.map((address) => ({
        address,
        status: address.endsWith('@pmail.dev') ? 'delivered' : 'queued',
        error: '',
      })),
    })
  }),

  /* ==================== OpenAPI Dot-Separated Attachment Routes ==================== */
  http.get('/api/attachments/preview/:id', async ({ params }) => {
    await delay(70)
    const [emailIdStr, indexStr] = String(params.id || '').split('.')
    const rec = getOrCreateRecord(Number(emailIdStr))
    const att = rec?.attachments.find((a) => a.Index === Number(indexStr))
    if (!att) return fail('附件不存在', 100, 404)
    const extLang = att.Filename.endsWith('.json')
      ? 'json'
      : att.Filename.endsWith('.patch') || att.Filename.endsWith('.diff')
        ? 'diff'
        : att.Filename.endsWith('.py')
          ? 'python'
          : 'markdown'
    return ok({
      filename: att.Filename,
      language: extLang,
      charset: 'utf-8',
      truncated: false,
      size: att.Size,
      content: att.previewContent || `// ${att.Filename}\nMock preview content`,
    })
  }),

  /* ==================== Inbound Analysis (C7c / C8 Contract) ==================== */
  http.post('/api/email/spam', async ({ request }) => {
    await delay(90)
    const body = (await request.json()) as { ids?: number[]; is_spam?: boolean }
    const ids = body.ids ?? []
    const isSpam = Boolean(body.is_spam)
    for (const id of ids) {
      const rec = getOrCreateRecord(id)
      if (rec) {
        rec.status = isSpam ? 5 : 0
        rec.group_id = 0
        rec.spam = isSpam
          ? {
              score: 9.5,
              verdict: 'spam',
              reasons: [{ code: 'bayes_spam', weight: 9.5 }],
            }
          : {
              score: 0,
              verdict: 'ham',
              reasons: [],
            }
      }
    }
    return ok('')
  }),

  http.get('/api/settings/spam', async () => {
    await delay(40)
    return ok(mockSpamSettings)
  }),

  http.put('/api/settings/spam', async ({ request }) => {
    await delay(80)
    const body = (await request.json()) as SpamSettings
    mockSpamSettings = {
      enabled: Boolean(body.enabled),
      threshold: Math.max(1, Math.min(20, Number(body.threshold) || 6)),
      bayes_enabled: Boolean(body.bayes_enabled),
    }
    return ok(mockSpamSettings)
  }),

  http.get('/api/spam/rules', async () => {
    await delay(40)
    return ok(mockSpamRules)
  }),

  http.post('/api/spam/rules', async ({ request }) => {
    await delay(80)
    const body = (await request.json()) as SpamRuleInput
    const value = (body.value || '').trim()
    if (!value) {
      return fail('匹配值不能为空', 100, 400)
    }
    if (mockSpamRules.length >= 500) {
      return fail('黑白名单最多可添加 500 条规则', 100, 409, { code: 'spam_rule_limit' })
    }
    const exists = mockSpamRules.some(
      (r) =>
        r.action === body.action &&
        r.kind === body.kind &&
        r.value.toLowerCase() === value.toLowerCase(),
    )
    if (exists) {
      return fail('此规则已存在', 100, 409, { code: 'spam_rule_conflict' })
    }
    const created: SpamRule = {
      id: nextSpamRuleId++,
      action: body.action,
      kind: body.kind,
      value,
    }
    mockSpamRules = [created, ...mockSpamRules]
    return ok(created)
  }),

  http.delete('/api/spam/rules', async ({ request }) => {
    await delay(60)
    const body = (await request.json()) as { id?: number }
    mockSpamRules = mockSpamRules.filter((r) => r.id !== body.id)
    return ok('')
  }),

  http.get('/api/admin/inbound/clamav', () => ok(getMockClamAV())),
  http.put('/api/admin/inbound/clamav', async ({ request }) => {
    const { enabled } = await request.json() as { enabled: boolean }
    mockClamAV = { ...mockClamAV, enabled, state: enabled ? 'running' : 'disabled' }
    return ok(mockClamAV)
  }),
  http.post('/api/admin/inbound/clamav/update', () => {
    if (!mockClamAV.enabled) return fail('ClamAV is disabled', 100, 409)
    const at = new Date().toISOString()
    mockClamAV = { ...mockClamAV, database: { version: 42, updated_at: at }, last_update: { status: 'success', at } }
    return ok({ queued: true })
  }),
  http.get('/api/admin/inbound', async () => {
    await delay(40)
    return ok(mockInboundSettings)
  }),

  http.put('/api/admin/inbound', async ({ request }) => {
    await delay(80)
    const body = (await request.json()) as InboundSettingsUpdate
    mockInboundSettings = {
      dnsbl_enabled: Boolean(body.dnsbl_enabled),
      dnsbl_zones: Array.isArray(body.dnsbl_zones) ? body.dnsbl_zones : [],
      default_spam_threshold: Math.max(
        1,
        Math.min(20, Number(body.default_spam_threshold) || 6),
      ),
      risk_block_dangerous_attachments: Boolean(body.risk_block_dangerous_attachments),
      catch_all: { ...mockInboundSettings.catch_all, ...body.catch_all },
      catch_all_recipients: mockInboundSettings.catch_all_recipients,
    }
    return ok(mockInboundSettings)
  }),

  /* ==================== C9 & C26 & C27 Contract: Trash, Telegram, Translation, AI ==================== */
  http.get('/api/admin/trash', async () => {
    await delay(35)
    return ok(mockTrashSettings)
  }),

  http.put('/api/admin/trash', async ({ request }) => {
    await delay(80)
    const body = (await request.json()) as Partial<TrashSettings>
    const days = Number(body.trash_retention_days)
    if (!Number.isInteger(days) || days < 0 || days > 3650) {
      return fail('trash_retention_invalid', 100, 400)
    }
    mockTrashSettings = { trash_retention_days: days }
    return ok(mockTrashSettings)
  }),

  http.get('/api/admin/telegram', async () => {
    await delay(40)
    return ok(getMockAdminTelegram())
  }),

  http.put('/api/admin/telegram', async ({ request }) => {
    await delay(100)
    const body = (await request.json()) as AdminTelegramUpdatePayload
    if (body.bot_token !== undefined) {
      if (body.bot_token === '') {
        mockAdminTelegram = {
          configured: false,
          bot_username: null,
          proxy_set: false,
          proxy: null,
          proxy_source: 'none',
          webhook_mode: 'polling',
        }
      } else if (body.bot_token.trim() === 'invalid' || !body.bot_token.includes(':')) {
        return fail('telegram_token_invalid', 100, 400)
      } else {
        mockAdminTelegram = {
          ...mockAdminTelegram,
          configured: true,
          bot_username: 'pmail_notify_bot',
        }
      }
    }
    if (body.clear_proxy || body.proxy === null || body.proxy === '') {
      mockAdminTelegram = {
        ...mockAdminTelegram,
        proxy_set: false,
        proxy: null,
        proxy_source: 'none',
      }
    } else if (body.proxy && typeof body.proxy === 'object') {
      if (!body.proxy.host?.trim() || !body.proxy.port || body.proxy.port < 1 || body.proxy.port > 65535) {
        return fail('telegram_proxy_invalid', 100, 400)
      }
      const nextProxy = applyProxyInput(mockAdminTelegram.proxy, body.proxy)
      mockAdminTelegram = {
        ...mockAdminTelegram,
        proxy_set: true,
        proxy: nextProxy,
        proxy_source: 'db',
      }
    }
    return ok(getMockAdminTelegram())
  }),

  http.get('/api/admin/translation', async () => {
    await delay(40)
    return ok(mockAdminTranslation)
  }),

  http.put('/api/admin/translation', async ({ request }) => {
    await delay(90)
    const body = (await request.json()) as AdminTranslationPolicyPatch
    const next: AdminTranslationPolicy = {
      ...mockAdminTranslation,
      ...(body.web_enabled !== undefined ? { web_enabled: body.web_enabled } : {}),
      ...(body.ai_enabled !== undefined ? { ai_enabled: body.ai_enabled } : {}),
      ...(body.default_engine !== undefined ? { default_engine: body.default_engine } : {}),
      ...(body.provider !== undefined ? { provider: body.provider } : {}),
      ...(body.url !== undefined ? { url: body.url } : {}),
      ...(body.deepl_tier !== undefined ? { deepl_tier: body.deepl_tier } : {}),
      ...(body.proxy_mode !== undefined ? { proxy_mode: body.proxy_mode } : {}),
      ...(body.max_chars !== undefined ? { max_chars: body.max_chars } : {}),
      ...(body.daily_limit_per_user !== undefined
        ? { daily_limit_per_user: body.daily_limit_per_user }
        : {}),
      ...(body.timeout !== undefined ? { timeout: body.timeout } : {}),
      ...(body.ai_user_config_enabled !== undefined
        ? { ai_user_config_enabled: body.ai_user_config_enabled }
        : {}),
      ...(body.ai_allow_private_network !== undefined
        ? { ai_allow_private_network: body.ai_allow_private_network }
        : {}),
    }
    if (body.api_key !== undefined) {
      next.api_key_set = body.api_key.trim().length > 0
    }
    if (body.clear_proxy) {
      next.proxy = null
    } else if (body.proxy) {
      if (!body.proxy.host?.trim() || !body.proxy.port || body.proxy.port < 1 || body.proxy.port > 65535) {
        return fail('translation_invalid', 100, 400)
      }
      next.proxy = applyProxyInput(mockAdminTranslation.proxy, body.proxy)
    }
    mockAdminTranslation = next
    return ok(mockAdminTranslation)
  }),

  http.post('/api/admin/translation/test', async () => {
    await delay(180)
    return ok({
      subject: 'PMail 翻译服务连接测试',
      text: '你好，这是一条用于验证当前网页翻译服务商连通性的测试短句。',
      provider: mockAdminTranslation.provider,
      latency_ms: 214,
    })
  }),

  http.get('/api/settings/translation', async () => {
    await delay(35)
    return ok(mockUserTranslation)
  }),

  http.put('/api/settings/translation', async ({ request }) => {
    await delay(80)
    const body = (await request.json()) as UserTranslationPreferences
    mockUserTranslation = {
      target_lang: body.target_lang ?? '',
      auto_translate: Boolean(body.auto_translate),
      translate_engine: body.translate_engine ?? '',
      ai_auto_summary: Boolean(body.ai_auto_summary),
    }
    return ok(mockUserTranslation)
  }),

  http.get('/api/settings/ai', async () => {
    await delay(40)
    return ok(getMockUserAI())
  }),

  http.put('/api/settings/ai', async ({ request }) => {
    await delay(100)
    const current = getMockUserAI()
    if (!current.ai_user_config_enabled) {
      return fail('ai_config_disabled', 100, 409)
    }
    const body = (await request.json()) as UserAIConfigInput
    if (!body.base_url?.trim() || !body.model?.trim()) {
      return fail('translation_invalid', 100, 400)
    }
    const urlChanged = body.base_url.trim() !== (mockUserAI.base_url ?? '')
    const nextKeySet =
      body.api_key !== undefined
        ? body.api_key.trim().length > 0
        : urlChanged
          ? false
          : mockUserAI.api_key_set
    let nextProxy = mockUserAI.proxy
    if (body.clear_proxy || body.proxy_mode !== 'custom') {
      nextProxy = null
    } else if (body.proxy) {
      if (!body.proxy.host?.trim() || !body.proxy.port || body.proxy.port < 1 || body.proxy.port > 65535) {
        return fail('translation_invalid', 100, 400)
      }
      nextProxy = applyProxyInput(mockUserAI.proxy, body.proxy)
    }
    mockUserAI = {
      ...mockUserAI,
      configured: true,
      base_url: body.base_url.trim(),
      model: body.model.trim(),
      timeout: body.timeout ?? 30,
      proxy_mode: body.proxy_mode ?? 'none',
      api_key_set: nextKeySet,
      proxy: nextProxy,
    }
    return ok(getMockUserAI())
  }),

  http.delete('/api/settings/ai', async () => {
    await delay(80)
    mockUserAI = {
      ...mockUserAI,
      configured: false,
      api_key_set: false,
      base_url: '',
      model: '',
      timeout: 30,
      proxy_mode: 'none',
      proxy: null,
    }
    return ok(null)
  }),

  http.post('/api/settings/ai/test', async () => {
    await delay(200)
    const ai = getMockUserAI()
    if (!ai.ai_user_config_enabled) return fail('ai_config_disabled', 100, 409)
    if (!ai.configured) return fail('ai_not_configured', 100, 409)
    return ok({
      model: ai.model || 'gpt-4o-mini',
      latency_ms: 342,
    })
  }),

  http.post('/api/email/translate', async ({ request }) => {
    await delay(260)
    const errMode =
      typeof window !== 'undefined' ? localStorage.getItem('pmail_mock_translate_err') : null
    if (errMode) {
      const status =
        errMode === 'translation_daily_limit' || errMode === 'translation_upstream_rate_limited'
          ? 429
          : errMode === 'translation_disabled' || errMode === 'ai_not_configured'
            ? 409
            : errMode === 'translation_timeout'
              ? 504
              : 502
      return fail(errMode, 100, status)
    }
    const body = (await request.json()) as EmailTranslateRequest
    const rec = getOrCreateRecord(body.id)
    if (!rec) return fail('translation_mail_not_found', 100, 404)
    const engine = body.engine || mockUserTranslation.translate_engine || mockAdminTranslation.default_engine
    const targetLang: TranslationLanguage = body.target_lang || mockUserTranslation.target_lang || 'zh-CN'
    const isZh = targetLang.startsWith('zh')
    const result: EmailTranslationResult = {
      subject: isZh
        ? `[译] ${rec.subject}`
        : `[Translated] ${rec.subject}`,
      text: isZh
        ? `（已翻译为 ${targetLang}）\n\n您好，林深：\n\n这是对邮件「${rec.subject}」的纯文本译文示例。原始正文中的关键日期、链接与说明已完整保留，方便快速审阅与跟进。\n\n—— 译文仅按纯文本安全展示`
        : `(Translated to ${targetLang})\n\nHello,\n\nThis is a plain-text translation preview for "${rec.subject}". Key dates, links, and details are preserved safely without HTML rendering.`,
      source_lang: isZh ? 'en' : 'zh-CN',
      target_lang: targetLang,
      provider: engine === 'ai' ? 'openai_compatible' : mockAdminTranslation.provider,
      engine,
      cached: !body.force,
      truncated: rec.text.length > mockAdminTranslation.max_chars,
      ...(engine === 'ai' ? { model: mockUserAI.model || 'gpt-4o-mini' } : {}),
    }
    return ok(result)
  }),

  http.post('/api/email/ai', async ({ request }) => {
    await delay(320)
    const errMode =
      typeof window !== 'undefined' ? localStorage.getItem('pmail_mock_ai_err') : null
    if (errMode) {
      const status = errMode === 'ai_busy' ? 429 : errMode === 'ai_not_configured' ? 409 : 502
      return fail(errMode, 100, status)
    }
    const ai = getMockUserAI()
    if (!ai.ai_user_config_enabled) return fail('ai_config_disabled', 100, 409)
    if (!ai.configured) return fail('ai_not_configured', 100, 409)
    const body = (await request.json()) as EmailAIRequest
    const rec = getOrCreateRecord(body.id)
    if (!rec) return fail('translation_mail_not_found', 100, 404)
    const lang: TranslationLanguage = body.lang || mockUserTranslation.target_lang || 'zh-CN'
    if (body.action === 'summary') {
      const summaryRes: EmailAISummaryResult = {
        action: 'summary',
        lang,
        model: ai.model || 'gpt-4o-mini',
        cached: !body.force,
        truncated: false,
        summary: `该邮件来自 ${rec.from_name || rec.from_address}，围绕「${rec.subject}」同步了当前进展与需要确认的关键事项。建议在截止时间前完成审核并回复确认。`,
        key_points: [
          '核心议题已整理并附带验证步骤，需要收件人复核确认。',
          '请在今日 18:00 前完成线上变更窗口与回归检查。',
          '如遇异常或需延期，请直接回复原线程同步处理人。',
        ],
      }
      return ok(summaryRes)
    }
    const extractRes: EmailAIExtractResult = {
      action: 'extract',
      lang,
      model: ai.model || 'gpt-4o-mini',
      cached: !body.force,
      truncated: false,
      action_items: [
        { text: '复核并合并 PR #412 的前端交互改动', due: '2026-11-25 18:00' },
        { text: '确认生产环境代理与限流配额配置', due: '本周五前' },
      ],
      dates: [
        { text: '2026年11月25日 18:00', iso: '2026-11-25T18:00:00+08:00' },
        { text: '2026年11月30日 账单截止日', iso: '2026-11-30' },
      ],
      amounts: [
        { text: '¥ 1,280.00', value: 1280, currency: 'CNY' },
      ],
      links: [
        { text: '查看 PR #412 详情', url: 'https://github.com/dingding229/PMail/pull/412' },
        { text: 'PMail 官方文档', url: 'https://pmail.dev/docs/v7' },
      ],
      codes: [
        { type: 'verification', value: rec.verification_code?.code || '849201' },
        { type: 'order', value: 'INV-2026-8841' },
      ],
      contacts: [
        { name: rec.from_name || '发件人', email: rec.from_address, phone: '+86 138-0013-8000' },
      ],
    }
    return ok(extractRes)
  }),

  http.post('/api/admin/telegram/test', async () => {
    await delay(140)
    if (!mockAdminTelegram.configured) {
      return fail('管理员尚未配置 Telegram 机器人', 100, 400, {
        code: 'telegram_not_configured',
      })
    }
    return ok({
      ok: true,
      bot_username: mockAdminTelegram.bot_username || 'pmail_notify_bot',
      latency_ms: 118,
    })
  }),

  http.get('/api/settings/telegram', async () => {
    await delay(40)
    return ok(getMockUserTelegramStatus())
  }),

  http.post('/api/settings/telegram/bind', async () => {
    await delay(80)
    if (!mockAdminTelegram.configured) {
      return fail('管理员尚未配置 Telegram 机器人', 100, 400, {
        code: 'telegram_not_configured',
      })
    }
    const code = `PM-${Math.random().toString(36).slice(2, 8).toUpperCase()}`
    const bot = mockAdminTelegram.bot_username || 'pmail_notify_bot'
    return ok({
      code,
      expires_at: dayjs().add(10, 'minute').toISOString(),
      deep_link: `https://t.me/${bot}?start=${code}`,
    })
  }),

  http.put('/api/settings/telegram', async ({ request }) => {
    await delay(80)
    const body = (await request.json()) as { filters?: TelegramFilters }
    if (body.filters) {
      mockUserTelegramFilters = body.filters
    }
    return ok(getMockUserTelegramStatus())
  }),

  http.post('/api/settings/telegram/test', async () => {
    await delay(120)
    return ok(null)
  }),

  http.delete('/api/settings/telegram', async () => {
    await delay(80)
    mockUserTelegramBound = false
    if (typeof window !== 'undefined') {
      localStorage.setItem('pmail_mock_tg', 'unbound')
    }
    return ok(null)
  }),

  http.get('/api/admin/update', async () => {
    await delay(40)
    return ok(buildMockUpdateInfo())
  }),

  http.post('/api/admin/update/check', async () => {
    await delay(120)
    return ok(buildMockUpdateInfo())
  }),

  http.post('/api/admin/update/apply', async ({ request }) => {
    await delay(100)
    const info = buildMockUpdateInfo()
    if (!info.updater.enabled) {
      return fail('当前未启用在线更新器，请使用下方手动命令更新', 100, 409, {
        code: 'updater_disabled',
      })
    }
    const body = await request.json() as { version?: string }
    if (body.version !== info.latest) return fail('版本已变更，请重新确认', 100, 409)
    const jobId = `upd_${Date.now()}`
    mockUpdateJobStep.set(jobId, 0)
    return HttpResponse.json({ errorNo: 0, errorMsg: '', data: { job_id: jobId } }, { status: 202 })
  }),

  http.get('/api/admin/update/apply/:jobId', async ({ params }) => {
    await delay(80)
    const jobId = String(params.jobId || '')
    const step = mockUpdateJobStep.get(jobId) ?? 0
    mockUpdateJobStep.set(jobId, step + 1)
    const states: UpdateJobState[] = ['pending', 'downloading', 'verifying', 'staging', 'restarting', 'done']
    const state = states[Math.min(step, states.length - 1)] ?? 'done'
    if (state === 'done' && typeof window !== 'undefined') {
      localStorage.setItem('pmail_mock_update', 'latest')
    }
    return ok({ job_id: jobId, version: 'v0.2.0', state, message: null })
  }),

  http.post('/api/compose/inline-image', async ({ request }) => {
    await delay(180)
    const formData = await request.formData()
    const file = formData.get('file')
    if (!(file instanceof File)) {
      return fail('未检测到上传图片文件', 100, 400)
    }
    if (file.size > 10 * 1024 * 1024) {
      return fail('内嵌图片大小不能超过 10 MB', 100, 413, { code: 'inline_image_too_large' })
    }
    const allowed = new Set(['image/png', 'image/jpeg', 'image/gif', 'image/webp'])
    if (file.type && !allowed.has(file.type.toLowerCase())) {
      return fail('仅支持 PNG、JPEG、GIF、WebP 格式图片', 100, 400, { code: 'inline_image_type' })
    }
    const cid = `img-${Date.now()}-${Math.random().toString(36).slice(2, 7)}@pmail.local`
    const bytes = await file.arrayBuffer()
    mockInlineImageStore.set(cid, {
      bytes,
      contentType: file.type || 'image/png',
      filename: file.name || 'image.png',
    })
    return ok({
      cid,
      url: `/api/compose/inline-image/${encodeURIComponent(cid)}`,
      filename: file.name || 'image.png',
      content_type: file.type || 'image/png',
      size: file.size,
    })
  }),

  http.get('/api/compose/inline-image/:cid', async ({ params }) => {
    const cid = decodeURIComponent(String(params.cid || ''))
    const stored = mockInlineImageStore.get(cid)
    if (stored) {
      return new HttpResponse(stored.bytes, {
        status: 200,
        headers: { 'Content-Type': stored.contentType },
      })
    }
    const fallbackSvg =
      '<svg xmlns="http://www.w3.org/2000/svg" width="320" height="120" viewBox="0 0 320 120"><rect width="320" height="120" rx="10" fill="#e2e8f0"/><text x="50%" y="50%" dominant-baseline="middle" text-anchor="middle" fill="#475569" font-family="sans-serif" font-size="14">Inline CID Image</text></svg>'
    return new HttpResponse(fallbackSvg, {
      status: 200,
      headers: { 'Content-Type': 'image/svg+xml' },
    })
  }),

  http.get('/api/settings/signatures', async () => {
    await delay(40)
    return ok(mockSignatures)
  }),

  http.post('/api/settings/signatures', async ({ request }) => {
    await delay(80)
    if (mockSignatures.length >= 20) {
      return fail('最多可创建 20 套签名', 100, 409)
    }
    const body = (await request.json()) as SignatureInput
    const created: SignatureRecord = {
      id: nextSignatureId++,
      name: body.name.trim(),
      html: body.html,
      address_ids: body.address_ids ?? [],
      is_default: Boolean(body.is_default),
    }
    if (created.is_default) {
      mockSignatures = mockSignatures.map((s) => ({ ...s, is_default: false }))
    }
    mockSignatures = [created, ...mockSignatures]
    return ok(created)
  }),

  http.put('/api/settings/signatures/:id', async ({ params, request }) => {
    await delay(80)
    const id = Number(params.id)
    const body = (await request.json()) as SignatureInput
    if (body.is_default) {
      mockSignatures = mockSignatures.map((s) => ({ ...s, is_default: false }))
    }
    let updated: SignatureRecord | undefined
    mockSignatures = mockSignatures.map((s) => {
      if (s.id !== id) return s
      updated = {
        id,
        name: body.name.trim(),
        html: body.html,
        address_ids: body.address_ids ?? [],
        is_default: Boolean(body.is_default),
      }
      return updated
    })
    if (!updated) return fail('签名不存在', 100, 404)
    return ok(updated)
  }),

  http.delete('/api/settings/signatures/:id', async ({ params }) => {
    await delay(70)
    const id = Number(params.id)
    mockSignatures = mockSignatures.filter((s) => s.id !== id)
    return ok(null)
  }),

  http.get('/api/attachments/download/:id', async ({ params, request }) => {
    const [emailIdStr, indexStr] = String(params.id || '').split('.')
    const index = Number(indexStr)
    const rec = getOrCreateRecord(Number(emailIdStr))
    const att = rec?.attachments.find((a) => a.Index === index)
    const url = new URL(request.url)
    const confirmRisk = url.searchParams.get('confirm_risk') === '1'
    const isHighRisk = Boolean(
      rec?.risk?.findings.some(
        (f) => f.location === 'attachment' && f.attachment_index === index,
      ),
    )
    if (isHighRisk && !confirmRisk) {
      return fail('请确认高危附件的下载风险', 100, 409, {
        code: 'risk_confirmation_required',
      })
    }
    const content = att?.previewContent || `Mock file: ${att?.Filename || 'attachment.bin'}`
    return new HttpResponse(content, {
      status: 200,
      headers: {
        'Content-Type': att?.ContentType || 'application/octet-stream',
        'Content-Disposition': `attachment; filename="${att?.Filename || 'attachment'}"`,
      },
    })
  }),

  http.get('/api/attachments/:id', async ({ params }) => {
    const [emailIdStr, indexStr] = String(params.id || '').split('.')
    const rec = getOrCreateRecord(Number(emailIdStr))
    const att = rec?.attachments.find((a) => a.Index === Number(indexStr))
    const content = att?.previewContent || `Mock inline file: ${att?.Filename || 'attachment'}`
    return new HttpResponse(content, {
      status: 200,
      headers: {
        'Content-Type': att?.ContentType || 'application/octet-stream',
      },
    })
  }),
]
