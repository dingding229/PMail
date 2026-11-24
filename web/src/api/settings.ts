import { apiDelete, apiGet, apiPost, apiPut, apiUploadWithProgress } from './client'
import type {
  AddressStatus,
  AdminAddressRequestPage,
  AdminAddressSettings,
  AdminBackupsResponse,
  AdminOverview,
  AdminTelegramStatus,
  AdminTelegramTestResult,
  AdminTelegramUpdatePayload,
  AdminTranslationPolicy,
  AdminTranslationPolicyPatch,
  AdminTranslationTestResult,
  AdminUpdateInfo,
  ApiTokenItem,
  BackupCreatePayload,
  BackupFileItem,
  BackupRestorePayload,
  DomainDnsResponse,
  ForwardSettingsRequest,
  ForwardSettingsView,
  ClamAVStatus,
  InboundSettings,
  InboundSettingsUpdate,
  MailboxAddress,
  MailDomain,
  MailExportJobStatus,
  MailExportPayload,
  MailImportJobStatus,
  MailReportListResponse,
  MailReportRecordsResponse,
  MailReportSummaryResponse,
  MailRule,
  ReportMailApplyResponse,
  ReportMailPreference,
  QueueItem,
  RestoreStatusResponse,
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

export const adminAddressApprovalApi = {
  getOverview: () => apiGet<AdminOverview>('/api/admin/overview'),
  getSettings: () => apiGet<AdminAddressSettings>('/api/admin/address-settings'),
  updateSettings: (payload: AdminAddressSettings) =>
    apiPut<AdminAddressSettings>('/api/admin/address-settings', payload),
  listRequests: (params: { status: AddressStatus; page?: number; page_size?: number }) => {
    const search = new URLSearchParams()
    search.set('status', params.status)
    search.set('page', String(params.page ?? 1))
    search.set('page_size', String(params.page_size ?? 20))
    return apiGet<AdminAddressRequestPage>(`/api/admin/address-requests?${search.toString()}`)
  },
  approve: (id: number) => apiPost<MailboxAddress>(`/api/admin/address-requests/${id}/approve`, {}),
  reject: (id: number, reason?: string) =>
    apiPost<MailboxAddress>(
      `/api/admin/address-requests/${id}/reject`,
      reason && reason.trim() ? { reason: reason.trim() } : {},
    ),
}

export const adminBackupApi = {
  list: () => apiGet<AdminBackupsResponse>('/api/admin/backups', { cache: 'no-store' }),
  create: (payload: BackupCreatePayload) =>
    apiPost<{ job_id: string }>('/api/admin/backups', payload),
  downloadUrl: (name: string) =>
    `/api/admin/backups/${encodeURIComponent(name)}/download`,
  remove: (name: string) =>
    apiDelete<null>(`/api/admin/backups/${encodeURIComponent(name)}`),
  upload: (file: File, onProgress?: (percent: number) => void) => {
    const formData = new FormData()
    formData.append('file', file)
    return apiUploadWithProgress<BackupFileItem>('/api/admin/backups/upload', formData, onProgress)
  },
  restore: (name: string, payload: BackupRestorePayload) =>
    apiPost<{ started: boolean }>(
      `/api/admin/backups/${encodeURIComponent(name)}/restore`,
      payload,
      { skipMaintenanceOverlay: true },
    ),
  restoreStatus: (signal?: AbortSignal) =>
    apiGet<RestoreStatusResponse>('/api/admin/backups/restore-status', {
      signal,
      cache: 'no-store',
      skipAuthRedirect: true,
      skipMaintenanceOverlay: true,
    }),
  ping: (signal?: AbortSignal) =>
    apiGet<{ ok?: boolean } | string>('/api/ping', {
      signal,
      cache: 'no-store',
      skipAuthRedirect: true,
      skipMaintenanceOverlay: true,
      rawJson: true,
    }),
}

export const mailImportExportApi = {
  startExport: (payload: MailExportPayload) =>
    apiPost<MailExportJobStatus>('/api/email/export', payload),
  exportStatus: (id: string, signal?: AbortSignal) =>
    apiGet<MailExportJobStatus>(`/api/email/export/${encodeURIComponent(id)}`, {
      signal,
      cache: 'no-store',
    }),
  exportDownloadUrl: (id: string) =>
    `/api/email/export/${encodeURIComponent(id)}/download`,
  startImport: (file: File, target: string, onProgress?: (percent: number) => void) => {
    const formData = new FormData()
    formData.append('file', file)
    formData.append('target', target)
    return apiUploadWithProgress<MailImportJobStatus>('/api/email/import', formData, onProgress)
  },
  importStatus: (id: string, signal?: AbortSignal) =>
    apiGet<MailImportJobStatus>(`/api/email/import/${encodeURIComponent(id)}`, {
      signal,
      cache: 'no-store',
    }),
}

export const adminMailReportsApi = {
  summary: (params: { domain?: string; from?: string; to?: string }, signal?: AbortSignal) => {
    const sp = new URLSearchParams()
    if (params.domain) sp.set('domain', params.domain)
    if (params.from) sp.set('from', params.from)
    if (params.to) sp.set('to', params.to)
    const qs = sp.toString()
    return apiGet<MailReportSummaryResponse>(
      `/api/admin/mail-reports/summary${qs ? `?${qs}` : ''}`,
      { signal, cache: 'no-store' },
    )
  },
  records: (
    params: { domain?: string; from?: string; to?: string; result?: string; page?: number },
    signal?: AbortSignal,
  ) => {
    const sp = new URLSearchParams()
    if (params.domain) sp.set('domain', params.domain)
    if (params.from) sp.set('from', params.from)
    if (params.to) sp.set('to', params.to)
    if (params.result) sp.set('result', params.result)
    if (params.page && params.page > 0) sp.set('page', String(params.page))
    const qs = sp.toString()
    return apiGet<MailReportRecordsResponse>(
      `/api/admin/mail-reports/records${qs ? `?${qs}` : ''}`,
      { signal, cache: 'no-store' },
    )
  },
  reports: (
    params: { domain?: string; from?: string; to?: string; page?: number },
    signal?: AbortSignal,
  ) => {
    const sp = new URLSearchParams()
    if (params.domain) sp.set('domain', params.domain)
    if (params.from) sp.set('from', params.from)
    if (params.to) sp.set('to', params.to)
    if (params.page && params.page > 0) sp.set('page', String(params.page))
    const qs = sp.toString()
    return apiGet<MailReportListResponse>(
      `/api/admin/mail-reports/reports${qs ? `?${qs}` : ''}`,
      { signal, cache: 'no-store' },
    )
  },
}

export const reportMailApi = {
  getPreference: () => apiGet<ReportMailPreference>('/api/settings/report-mail'),
  savePreference: (payload: ReportMailPreference) =>
    apiPut<ReportMailPreference>('/api/settings/report-mail', payload),
  apply: () => apiPost<ReportMailApplyResponse>('/api/settings/report-mail/apply', {}),
}
