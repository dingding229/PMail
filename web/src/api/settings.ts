import { apiDelete, apiGet, apiPost, apiPut } from './client'
import type {
  AdminTelegramStatus,
  AdminTelegramTestResult,
  AdminTelegramUpdatePayload,
  AdminTranslationPolicy,
  AdminTranslationPolicyPatch,
  AdminTranslationTestResult,
  AdminUpdateInfo,
  ApiTokenItem,
  DomainDnsResponse,
  ForwardSettingsRequest,
  ForwardSettingsView,
  ClamAVStatus,
  InboundSettings,
  InboundSettingsUpdate,
  MailDomain,
  MailRule,
  QueueItem,
  RuleRequest,
  SignatureInput,
  SignatureRecord,
  SpamRule,
  SpamRuleInput,
  SpamSettings,
  TelegramBindCode,
  TelegramFilters,
  TokenCreatedResponse,
  TrashSettings,
  UpdateApplyResponse,
  UpdateJobStatus,
  UserAIConfigInput,
  UserAIConfigView,
  UserAITestResult,
  UserTelegramStatus,
  UserTranslationPreferences,
} from './types'

export const forwardApi = {
  get: () => apiPost<ForwardSettingsView>('/api/forward/get', {}),
  set: (payload: ForwardSettingsRequest) =>
    apiPost<ForwardSettingsView>('/api/forward/set', payload),
}

export const ruleApi = {
  list: async () => (await apiPost<MailRule[] | null>('/api/rule/get', {})) ?? [],
  add: (payload: RuleRequest) => apiPost<string>('/api/rule/add', payload),
  update: (payload: RuleRequest) => apiPost<string>('/api/rule/update', payload),
  remove: (id: number) => apiPost<string>('/api/rule/del', { id }),
}

export const tokenApi = {
  list: async () => (await apiPost<ApiTokenItem[] | null>('/api/token/list', {})) ?? [],
  create: (payload: {
    name?: string
    scope?: 'read' | 'send' | 'admin'
    never_expires?: boolean
    expires_at?: string | null
  }) => apiPost<TokenCreatedResponse>('/api/token/create', payload),
  remove: (id: number) => apiPost<null>('/api/token/delete', { id }),
}

export const queueApi = {
  list: async () => (await apiPost<QueueItem[] | null>('/api/queue/list', {})) ?? [],
  retry: (id: number) => apiPost<null>('/api/queue/retry', { id }),
  remove: (id: number) => apiPost<null>('/api/queue/delete', { id }),
}

export const domainApi = {
  list: async () => (await apiPost<MailDomain[] | null>('/api/domain/list', {})) ?? [],
  create: (name: string) => apiPost<MailDomain>('/api/domain/create', { name }),
  remove: (id: number) => apiPost<null>('/api/domain/delete', { id }),
  setDefault: (id: number) => apiPost<null>('/api/domain/set_default', { id }),
  dns: (id: number, refresh = false) =>
    apiPost<DomainDnsResponse>('/api/domain/dns', { id, refresh }),
  zoneDownloadUrl: (id: number) => `/api/domain/dns/zone?id=${id}`,
}

export const spamSettingsApi = {
  get: () => apiGet<SpamSettings>('/api/settings/spam'),
  update: (payload: SpamSettings) => apiPut<SpamSettings>('/api/settings/spam', payload),
}

export const spamRulesApi = {
  list: async () => (await apiGet<SpamRule[] | null>('/api/spam/rules')) ?? [],
  create: (payload: SpamRuleInput) => apiPost<SpamRule>('/api/spam/rules', payload),
  remove: (id: number) => apiDelete<string>('/api/spam/rules', { id }),
}

export const adminInboundApi = {
  clamav: () => apiGet<ClamAVStatus>('/api/admin/inbound/clamav', { cache: 'no-store' }),
  setClamAV: (enabled: boolean) => apiPut<ClamAVStatus>('/api/admin/inbound/clamav', { enabled }),
  updateClamAV: () => apiPost<{ queued: boolean }>('/api/admin/inbound/clamav/update', {}),
  get: () => apiGet<InboundSettings>('/api/admin/inbound'),
  update: (payload: InboundSettingsUpdate) => apiPut<InboundSettings>('/api/admin/inbound', payload),
}

export const adminTelegramApi = {
  get: () => apiGet<AdminTelegramStatus>('/api/admin/telegram'),
  update: (payload: AdminTelegramUpdatePayload) =>
    apiPut<AdminTelegramStatus>('/api/admin/telegram', payload),
  test: () => apiPost<AdminTelegramTestResult>('/api/admin/telegram/test', {}),
}

export const userTelegramApi = {
  get: () => apiGet<UserTelegramStatus>('/api/settings/telegram'),
  bind: () => apiPost<TelegramBindCode>('/api/settings/telegram/bind', {}),
  updateFilters: (filters: TelegramFilters) =>
    apiPut<UserTelegramStatus>('/api/settings/telegram', { filters }),
  test: () => apiPost<null>('/api/settings/telegram/test', {}),
  unbind: () => apiDelete<null>('/api/settings/telegram'),
}

export const adminUpdateApi = {
  get: () => apiGet<AdminUpdateInfo>('/api/admin/update'),
  check: () => apiPost<AdminUpdateInfo>('/api/admin/update/check', {}),
  apply: (version: string) => apiPost<UpdateApplyResponse>('/api/admin/update/apply', { version }),
  jobStatus: (jobId: string, signal?: AbortSignal) =>
    apiGet<UpdateJobStatus>(`/api/admin/update/apply/${encodeURIComponent(jobId)}`, { signal, cache: 'no-store', skipAuthRedirect: true }),
}

export const signaturesApi = {
  list: async () => (await apiGet<SignatureRecord[] | null>('/api/settings/signatures')) ?? [],
  create: (payload: SignatureInput) => apiPost<SignatureRecord>('/api/settings/signatures', payload),
  update: (id: number, payload: SignatureInput) =>
    apiPut<SignatureRecord>(`/api/settings/signatures/${id}`, payload),
  remove: (id: number) => apiDelete<null>(`/api/settings/signatures/${id}`),
}

export const adminTrashApi = {
  get: () => apiGet<TrashSettings>('/api/admin/trash'),
  update: (payload: TrashSettings) => apiPut<TrashSettings>('/api/admin/trash', payload),
}

export const adminTranslationApi = {
  get: () => apiGet<AdminTranslationPolicy>('/api/admin/translation'),
  update: (payload: AdminTranslationPolicyPatch) =>
    apiPut<AdminTranslationPolicy>('/api/admin/translation', payload),
  test: () => apiPost<AdminTranslationTestResult>('/api/admin/translation/test', {}),
}

export const userTranslationApi = {
  get: () => apiGet<UserTranslationPreferences>('/api/settings/translation'),
  update: (payload: UserTranslationPreferences) =>
    apiPut<UserTranslationPreferences>('/api/settings/translation', payload),
}

export const userAIApi = {
  get: () => apiGet<UserAIConfigView>('/api/settings/ai'),
  update: (payload: UserAIConfigInput) => apiPut<UserAIConfigView>('/api/settings/ai', payload),
  remove: () => apiDelete<null>('/api/settings/ai'),
  test: () => apiPost<UserAITestResult>('/api/settings/ai/test', {}),
}
