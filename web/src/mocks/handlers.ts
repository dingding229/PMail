import { delay, http, HttpResponse } from 'msw'
import dayjs from 'dayjs'
import { version as mockBuildVersion } from '../../package.json'
import {
  SYSTEM_GROUP_IDS,
  type ApiTokenItem,
  type DomainDnsResponse,
  type ExternalAccount,
  type ExternalPreset,
  type GroupOption,
  type MailboxAddress,
  type MailDomain,
  type MailRule,
  type QueueItem,
  type UserProfile,
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
let nextUserId = 4
let nextAddressId = 10
let mockUsers: UserProfile[] = [
  { ...MOCK_USER },
  {
    id: 2,
    account: 'chen',
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
]

const getMe = () => mockUsers[0]!

/* ==================== Groups & Emails Store ==================== */
let customGroups: GroupOption[] = [...INITIAL_CUSTOM_GROUPS]
let nextGroupId = 20
let nextEmailId = 500

const emailStore = new Map<number, MockEmailRecord>()
for (const item of createSeedEmails()) {
  if (item.id === 101 || item.id === 103 || item.id === 106) {
    item.is_starred = true
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

const EXTERNAL_PRESETS: ExternalPreset[] = [
  { name: 'Gmail (IMAP)', protocol: 'imap', host: 'imap.gmail.com', port: 993, tls_mode: 'tls' },
  { name: 'QQ 邮箱 (IMAP)', protocol: 'imap', host: 'imap.qq.com', port: 993, tls_mode: 'tls' },
  { name: '网易 163 (IMAP)', protocol: 'imap', host: 'imap.163.com', port: 993, tls_mode: 'tls' },
  { name: 'Outlook / Office365', protocol: 'imap', host: 'outlook.office365.com', port: 993, tls_mode: 'tls' },
  { name: 'iCloud Mail', protocol: 'imap', host: 'imap.mail.me.com', port: 993, tls_mode: 'tls' },
]

let nextExtId = 10
let mockExternalAccounts: ExternalAccount[] = [
  {
    id: 1,
    user_id: 1,
    name: '工作 Gmail',
    protocol: 'imap',
    host: 'imap.gmail.com',
    port: 993,
    tls_mode: 'tls',
    username: 'linshen.dev@gmail.com',
    folder: 'INBOX',
    interval_minutes: 10,
    initial_days: 30,
    delete_remote: false,
    target_group_id: 10,
    enabled: true,
    insecure_skip_verify: false,
    last_sync_at: dayjs().subtract(5, 'minute').toISOString(),
    last_success_at: dayjs().subtract(5, 'minute').toISOString(),
    last_error: '',
    created: '2026-10-12T08:00:00Z',
    updated: dayjs().subtract(5, 'minute').toISOString(),
  },
  {
    id: 2,
    user_id: 1,
    name: '旧版 QQ 邮箱',
    protocol: 'imap',
    host: 'imap.qq.com',
    port: 993,
    tls_mode: 'tls',
    username: '8829104@qq.com',
    folder: 'INBOX',
    interval_minutes: 15,
    initial_days: 30,
    delete_remote: false,
    target_group_id: 0,
    enabled: true,
    insecure_skip_verify: false,
    last_sync_at: dayjs().subtract(12, 'minute').toISOString(),
    last_success_at: dayjs().subtract(2, 'day').toISOString(),
    last_error: 'IMAP 认证失败：应用授权码已过期，请在 QQ 邮箱设置中重新生成授权码',
    created: '2026-10-18T11:30:00Z',
    updated: dayjs().subtract(12, 'minute').toISOString(),
  },
]

let nextTokenId = 10
let mockTokens: ApiTokenItem[] = [
  {
    id: 1,
    user_id: 1,
    name: 'CI 部署告警通知',
    prefix: 'pmail_9a4f',
    last_used_at: dayjs().subtract(35, 'minute').toISOString(),
    expires_at: dayjs().add(68, 'day').toISOString(),
    created: '2026-10-01T08:00:00Z',
  },
  {
    id: 2,
    user_id: 1,
    name: '本地 CLI 脚本',
    prefix: 'pmail_3c1b',
    last_used_at: null,
    expires_at: null,
    created: '2026-11-10T14:20:00Z',
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
let nextDomainId = 10
let mockDomains: MailDomain[] = [
  {
    id: 1,
    name: 'pmail.dev',
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

function toListItem(rec: MockEmailRecord) {
  const iso = dayjs(rec.send_date).isValid()
    ? dayjs(rec.send_date).toISOString()
    : new Date().toISOString()
  return {
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
    return HttpResponse.json({
      version: mockBuildVersion,
      need_register: needRegister,
      logged_in: loggedIn,
      default_lang: 'zh-CN',
      web_domain: 'mail.pmail.dev',
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
    loggedIn = true
    const me = getMe()
    me.account = body.account
    return ok({
      account: me.account,
      name: me.name,
      is_admin: me.is_admin,
      domains: mockDomains.map((d) => d.name),
    })
  }),

  http.post('/api/logout', async () => {
    await delay(60)
    loggedIn = false
    return ok('success')
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
    const addr: MailboxAddress = {
      id: nextAddressId++,
      user_id: uid,
      local_part: body.local_part.toLowerCase(),
      domain_id: dom.id,
      is_primary: (u.addresses?.length ?? 0) === 0,
      can_send: body.can_send ?? true,
      domain: dom.name,
      address: `${body.local_part.toLowerCase()}@${dom.name}`,
      created: new Date().toISOString(),
    }
    u.addresses = [...(u.addresses ?? []), addr]
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
    return ok(null)
  }),

  /* ==================== Forward / Rules / External / Tokens / Queue ==================== */
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

  http.get('/api/external/presets', async () => {
    await delay(30)
    return ok(EXTERNAL_PRESETS)
  }),

  http.post('/api/external/list', async () => {
    await delay(45)
    return ok(mockExternalAccounts)
  }),

  http.post('/api/external/create', async ({ request }) => {
    await delay(120)
    const body = (await request.json()) as Partial<ExternalAccount>
    const created: ExternalAccount = {
      id: nextExtId++,
      user_id: 1,
      name: body.name || body.username || '外部邮箱',
      protocol: body.protocol ?? 'imap',
      host: body.host ?? '',
      port: body.port ?? 993,
      tls_mode: body.tls_mode ?? 'tls',
      username: body.username ?? '',
      folder: body.folder || 'INBOX',
      interval_minutes: body.interval_minutes ?? 10,
      initial_days: body.initial_days ?? 30,
      delete_remote: body.delete_remote ?? false,
      target_group_id: body.target_group_id ?? 0,
      enabled: body.enabled ?? true,
      insecure_skip_verify: body.insecure_skip_verify ?? false,
      last_sync_at: null,
      last_success_at: null,
      last_error: '',
      created: new Date().toISOString(),
      updated: new Date().toISOString(),
    }
    mockExternalAccounts = [created, ...mockExternalAccounts]
    return ok(created)
  }),

  http.post('/api/external/update', async ({ request }) => {
    await delay(100)
    const body = (await request.json()) as Partial<ExternalAccount> & { id: number }
    mockExternalAccounts = mockExternalAccounts.map((acc) =>
      acc.id === body.id ? { ...acc, ...body, updated: new Date().toISOString() } : acc,
    )
    const updated = mockExternalAccounts.find((a) => a.id === body.id)
    return ok(updated)
  }),

  http.post('/api/external/delete', async ({ request }) => {
    await delay(80)
    const body = (await request.json()) as { id: number }
    mockExternalAccounts = mockExternalAccounts.filter((a) => a.id !== body.id)
    return ok(null)
  }),

  http.post('/api/external/test', async () => {
    await delay(350)
    return ok({ folders: ['INBOX', 'Sent', 'Archive', 'Work'] })
  }),

  http.post('/api/external/sync_now', async ({ request }) => {
    await delay(150)
    const body = (await request.json()) as { id: number }
    const now = new Date().toISOString()
    mockExternalAccounts = mockExternalAccounts.map((acc) =>
      acc.id === body.id ? { ...acc, last_sync_at: now, last_success_at: now, last_error: '' } : acc,
    )
    return ok({ status: 'started', message: 'external sync started' })
  }),

  http.post('/api/token/list', async () => {
    await delay(40)
    return ok(mockTokens)
  }),

  http.post('/api/token/create', async ({ request }) => {
    await delay(120)
    const body = (await request.json()) as { name?: string; expires_at?: string | null }
    const rand = Math.random().toString(36).slice(2, 10) + Math.random().toString(36).slice(2, 14)
    const plaintext = `pmail_${rand}`
    const item: ApiTokenItem = {
      id: nextTokenId++,
      user_id: 1,
      name: body.name || 'API Token',
      prefix: plaintext.slice(0, 10),
      last_used_at: null,
      expires_at: body.expires_at ?? null,
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
  http.post('/api/group', async () => {
    await delay(35)
    if (!loggedIn) return fail('未登录', 403, 401)
    const allSeed = Array.from(emailStore.values())
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
        unread: draftsCount,
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

  http.post('/api/email/list', async ({ request }) => {
    await delay(50)
    if (!loggedIn) return fail('未登录', 403, 401)
    const body = (await request.json()) as {
      keyword?: string
      tag?: string
      current_page?: number
      page_size?: number
    }
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
      .filter((rec) => matchesTag(rec, tagObj))
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

    if (isDefaultInbox && !kw) {
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
    const slice = seedMatched.slice((page - 1) * pageSize, page * pageSize).map(toListItem)

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
    return ok(toEmailDetail(rec))
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
        if (body.forcedDel || rec.status === 3) emailStore.delete(id)
        else rec.status = 3
      } else if (id >= 1000) {
        deletedSyntheticIds.add(id)
      }
    }
    return ok('success')
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

  http.get('/api/attachments/download/:id', async ({ params }) => {
    const [emailIdStr, indexStr] = String(params.id || '').split('.')
    const rec = getOrCreateRecord(Number(emailIdStr))
    const att = rec?.attachments.find((a) => a.Index === Number(indexStr))
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
