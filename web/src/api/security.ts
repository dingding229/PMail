import { apiDelete, apiGet, apiPost } from './client'
import type { AppPassword, SecondFactorCode } from './types'

type ConfirmFactor = SecondFactorCode & { password: string }
interface RecoveryResult { enabled: true; recovery_codes: string[] }

export const securityApi = {
  status: () => apiGet<{ enabled: boolean }>('/api/2fa', { cache: 'no-store' }),
  setup: () => apiPost<{ secret: string; otpauth_uri: string }>('/api/2fa/setup'),
  enable: (code: string) => apiPost<RecoveryResult>('/api/2fa/enable', { code }),
  disable: (payload: ConfirmFactor) => apiPost<{ enabled: false }>('/api/2fa/disable', payload),
  regenerate: (payload: ConfirmFactor) => apiPost<RecoveryResult>('/api/2fa/recovery_codes', payload),
  listAppPasswords: () => apiGet<AppPassword[]>('/api/app_passwords', { cache: 'no-store' }),
  createAppPassword: (name: string) => apiPost<AppPassword & { password: string }>('/api/app_passwords', { name }),
  deleteAppPassword: (id: number) => apiDelete<null>(`/api/app_passwords/${id}`),
}
