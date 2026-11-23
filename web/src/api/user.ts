import { apiPost } from './client'
import type {
  AuthSessionUser,
  MailboxAddress,
  UserPageResponse,
  UserProfile,
} from './types'

export const userApi = {
  me: (payload?: { display_name?: string; lang?: string }) =>
    apiPost<UserProfile>('/api/user/me', payload ?? {}),

  info: () => apiPost<AuthSessionUser>('/api/user/info', {}),

  changePassword: (payload: { old_password: string; new_password: string }) =>
    apiPost<null>('/api/user/change_password', payload),

  // Admin user management
  list: (payload?: { current_page?: number; page_size?: number }) =>
    apiPost<UserPageResponse>('/api/user/list', {
      current_page: payload?.current_page ?? 1,
      page_size: payload?.page_size ?? 15,
    }),

  create: (payload: {
    account: string
    display_name: string
    password: string
    disabled?: number
    quota_mb?: number
  }) => apiPost<UserProfile>('/api/user/create', payload),

  edit: (payload: {
    id: number
    account?: string
    display_name?: string
    password?: string
    disabled?: number
    quota_mb?: number
  }) => apiPost<UserProfile>('/api/user/edit', payload),

  resetPassword: (payload: { id: number; password: string }) =>
    apiPost<null>('/api/user/reset_password', payload),

  resetSecondFactor: (id: number) => apiPost<null>('/api/user/reset_2fa', { id }),

  setQuota: (payload: { id: number; quota_mb: number }) =>
    apiPost<null>('/api/user/set_quota', payload),

  remove: (id: number) => apiPost<null>('/api/user/delete', { id }),

  // Mailbox addresses
  listAddresses: (userId?: number) =>
    apiPost<MailboxAddress[]>('/api/address/list', userId ? { user_id: userId } : {}),

  createAddress: (payload: {
    user_id?: number
    local_part: string
    domain_id: number
    can_send?: boolean
  }) => apiPost<MailboxAddress>('/api/address/create', payload),

  updateAddress: (payload: { id: number; can_send: boolean }) =>
    apiPost<null>('/api/address/update', payload),

  deleteAddress: (id: number) => apiPost<null>('/api/address/delete', { id }),

  setPrimaryAddress: (id: number) => apiPost<null>('/api/address/set_primary', { id }),
}
