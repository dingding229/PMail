import { apiGet, apiPost } from './client'
import type {
  AuthSessionUser,
  BootstrapResponse,
  LoginRequest,
  RegisterRequest,
} from './types'

export const authApi = {
  /** GET /api/bootstrap returns raw JSON (unwrapped) per docs/openapi.yaml */
  bootstrap: (signal?: AbortSignal) =>
    apiGet<BootstrapResponse>('/api/bootstrap', { skipAuthRedirect: true, rawJson: true, cache: 'no-store', signal }),

  register: (payload: RegisterRequest) =>
    apiPost<AuthSessionUser>('/api/register', payload, { skipAuthRedirect: true }),

  /** First-run only: set the primary mail domain with the registration key. */
  setupDomain: (payload: { registration_key: string; domain: string }) =>
    apiPost<string>('/api/setup/domain', payload, { skipAuthRedirect: true }),

  login: (payload: LoginRequest) =>
    apiPost<AuthSessionUser>('/api/login', payload, { skipAuthRedirect: true }),

  logout: () => apiPost<string>('/api/logout', {}, { skipAuthRedirect: true }),
}
