import { apiGet, apiPost } from './client'
import type {
  AuthSessionUser,
  BootstrapResponse,
  LoginRequest,
  RegisterRequest,
} from './types'

export const authApi = {
  /** GET /api/bootstrap returns raw JSON (unwrapped) per docs/openapi.yaml */
  bootstrap: () =>
    apiGet<BootstrapResponse>('/api/bootstrap', { skipAuthRedirect: true, rawJson: true }),

  register: (payload: RegisterRequest) =>
    apiPost<AuthSessionUser>('/api/register', payload, { skipAuthRedirect: true }),

  login: (payload: LoginRequest) =>
    apiPost<AuthSessionUser>('/api/login', payload, { skipAuthRedirect: true }),

  logout: () => apiPost<string>('/api/logout', {}),
}
