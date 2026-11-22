import { apiGet, apiPost } from './client'
import type {
  ApiTokenItem,
  DomainDnsResponse,
  ExternalAccount,
  ExternalAccountPayload,
  ExternalPreset,
  ExternalSyncResult,
  ExternalTestResult,
  ForwardSettingsRequest,
  ForwardSettingsView,
  MailDomain,
  MailRule,
  QueueItem,
  RuleRequest,
  TokenCreatedResponse,
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

export const externalApi = {
  list: async () => (await apiPost<ExternalAccount[] | null>('/api/external/list', {})) ?? [],
  create: (payload: ExternalAccountPayload) =>
    apiPost<ExternalAccount>('/api/external/create', payload),
  update: (payload: ExternalAccountPayload & { id: number }) =>
    apiPost<ExternalAccount>('/api/external/update', payload),
  remove: (id: number) => apiPost<null>('/api/external/delete', { id }),
  test: (payload: ExternalAccountPayload) =>
    apiPost<ExternalTestResult>('/api/external/test', payload),
  syncNow: (id: number) => apiPost<ExternalSyncResult>('/api/external/sync_now', { id }),
  presets: () => apiGet<ExternalPreset[]>('/api/external/presets'),
}

export const tokenApi = {
  list: async () => (await apiPost<ApiTokenItem[] | null>('/api/token/list', {})) ?? [],
  create: (payload: { name?: string; expires_at?: string | null }) =>
    apiPost<TokenCreatedResponse>('/api/token/create', payload),
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
