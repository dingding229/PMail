import i18n, { getAppLanguage } from '@/i18n'
import { getRawStorage, removeRawStorage, setRawStorage } from '@/lib/storage'
import type { ApiEnvelope } from './types'

export const API_ERROR_NO = {
  OK: 0,
  PARAMS_ERROR: 100,
  NEED_SETUP: 402,
  NEED_LOGIN: 403,
  NO_ACCESS: 405,
  RATE_LIMITED: 429,
  SERVER_ERROR: 500,
} as const

export class ApiError extends Error {
  readonly status: number
  readonly errorNo: number
  readonly data: unknown

  constructor(message: string, options: { status: number; errorNo?: number; data?: unknown }) {
    super(message)
    this.name = 'ApiError'
    this.status = options.status
    this.errorNo = options.errorNo ?? options.status
    this.data = options.data
  }
}

type AuthHandler = () => void
let unauthorizedHandler: AuthHandler | null = null
let needBootstrapHandler: AuthHandler | null = null

export function setUnauthorizedHandler(handler: AuthHandler | null): void {
  unauthorizedHandler = handler
}

export function setNeedBootstrapHandler(handler: AuthHandler | null): void {
  needBootstrapHandler = handler
}

export function getBearerToken(): string | null {
  return getRawStorage('api_token')
}

export function setBearerToken(token: string | null): void {
  if (!token) {
    removeRawStorage('api_token')
    return
  }
  setRawStorage('api_token', token)
}

interface RequestOptions {
  method?: 'GET' | 'POST' | 'PUT' | 'DELETE'
  body?: unknown
  signal?: AbortSignal
  skipAuthRedirect?: boolean
  /** Set true for endpoints like GET /api/bootstrap that return raw JSON without { errorNo, errorMsg, data } */
  rawJson?: boolean
}

function buildHeaders(hasJsonBody: boolean): HeadersInit {
  const lang = getAppLanguage()
  const headers: Record<string, string> = {
    'X-PMail-Lang': lang,
    Lang: lang.startsWith('zh') ? 'zh' : 'en',
  }
  if (hasJsonBody) {
    headers['Content-Type'] = 'application/json;charset=UTF-8'
  }
  const token = getBearerToken()
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }
  return headers
}

function isApiEnvelope(value: unknown): value is ApiEnvelope<unknown> {
  return (
    typeof value === 'object' &&
    value !== null &&
    'errorNo' in value &&
    typeof (value as { errorNo: unknown }).errorNo === 'number'
  )
}

const MACHINE_ERROR_KEY_MAP: Record<string, string> = {
  origin_forbidden: 'common.errors.originForbidden',
  json_media_type_required: 'common.errors.jsonMediaTypeRequired',
  token_scope_forbidden: 'common.errors.tokenScopeForbidden',
  method_not_allowed: 'common.errors.methodNotAllowed',
  payload_too_large: 'common.errors.payloadTooLarge',
  spam_rule_conflict: 'settings.spam.errors.conflict',
  spam_rule_limit: 'settings.spam.errors.limitReached',
  risk_confirmation_required: 'mail.risk.confirmDownloadRequired',
  risk_preview_forbidden: 'mail.risk.previewForbidden',
  telegram_token_invalid: 'admin.telegram.errors.tokenInvalid',
  telegram_not_configured: 'settings.telegram.errors.notConfigured',
  telegram_not_bound: 'settings.telegram.errors.notBound',
  telegram_bind_expired: 'settings.telegram.errors.bindExpired',
  telegram_rate_limited: 'settings.telegram.errors.rateLimited',
  update_check_rate_limited: 'admin.update.errors.rateLimited',
  updater_disabled: 'admin.update.errors.updaterDisabled',
  inline_image_too_large: 'compose.inlineImage.errorTooLarge',
  inline_image_type: 'compose.inlineImage.errorType',
}

function resolveApiErrorMessage(rawMessage: string | undefined, status: number): string {
  const trimmed = rawMessage?.trim()
  if (trimmed) {
    const mappedKey = MACHINE_ERROR_KEY_MAP[trimmed]
    if (mappedKey) {
      return i18n.t(mappedKey)
    }
    return trimmed
  }
  switch (status) {
    case 401:
      return i18n.t('common.unauthorized')
    case 403:
      return i18n.t('common.forbidden')
    case 405:
      return i18n.t('common.errors.methodNotAllowed')
    case 409:
      return i18n.t('common.errors.conflict')
    case 413:
      return i18n.t('common.errors.payloadTooLarge')
    case 415:
      return i18n.t('common.errors.jsonMediaTypeRequired')
    case 429:
      return i18n.t('common.rateLimited')
    default:
      return i18n.t('common.unknownError')
  }
}

export async function apiRequest<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const {
    method = 'POST',
    body,
    signal,
    skipAuthRedirect = false,
    rawJson = false,
  } = options
  const isFormData = typeof FormData !== 'undefined' && body instanceof FormData
  const hasBody = body !== undefined && method !== 'GET'

  let response: Response
  try {
    response = await fetch(path, {
      method,
      credentials: 'include',
      headers: buildHeaders(hasBody && !isFormData),
      body: hasBody ? (isFormData ? (body as FormData) : JSON.stringify(body)) : undefined,
      signal,
    })
  } catch (err) {
    if (err instanceof DOMException && err.name === 'AbortError') {
      throw err
    }
    throw new ApiError(i18n.t('common.networkError'), { status: 0, errorNo: 0 })
  }

  let parsed: unknown = null
  const contentType = response.headers.get('content-type') || ''
  if (contentType.includes('application/json')) {
    try {
      parsed = await response.json()
    } catch {
      parsed = null
    }
  } else {
    const rawText = await response.text().catch(() => '')
    if (rawText) {
      try {
        parsed = JSON.parse(rawText)
      } catch {
        if (!response.ok) {
          const fallbackMsg =
            response.status === 413 ||
            response.status === 405 ||
            response.status === 415
              ? resolveApiErrorMessage(undefined, response.status)
              : rawText || resolveApiErrorMessage(undefined, response.status)
          throw new ApiError(fallbackMsg, {
            status: response.status,
          })
        }
        return rawText as unknown as T
      }
    }
  }

  // Endpoints returning raw JSON (e.g. GET /api/bootstrap); still tolerate legacy envelope if ever returned
  if (rawJson) {
    if (!response.ok) {
      const errMsg = resolveApiErrorMessage(
        isApiEnvelope(parsed) ? parsed.errorMsg : undefined,
        response.status,
      )
      throw new ApiError(errMsg, {
        status: response.status,
        errorNo: isApiEnvelope(parsed) ? parsed.errorNo : response.status,
      })
    }
    if (isApiEnvelope(parsed)) {
      if (parsed.errorNo !== 0) {
        throw new ApiError(resolveApiErrorMessage(parsed.errorMsg, response.status), {
          status: response.status,
          errorNo: parsed.errorNo,
          data: parsed.data,
        })
      }
      return parsed.data as T
    }
    return parsed as T
  }

  const envelope = isApiEnvelope(parsed) ? parsed : null

  // 1. errorNo 402 (NeedSetup / NeedRegister) -> re-check /api/bootstrap and route to /register
  if (envelope?.errorNo === API_ERROR_NO.NEED_SETUP) {
    if (!skipAuthRedirect && needBootstrapHandler) {
      needBootstrapHandler()
    }
    const msg = resolveApiErrorMessage(envelope.errorMsg, response.status)
    throw new ApiError(msg, { status: response.status, errorNo: API_ERROR_NO.NEED_SETUP, data: envelope.data })
  }

  // 2. errorNo 405 (NoAccessPrivileges / origin_forbidden / token_scope_forbidden) -> display error only, NEVER redirect to login!
  if (envelope?.errorNo === API_ERROR_NO.NO_ACCESS) {
    const msg = resolveApiErrorMessage(envelope.errorMsg, response.status || 403)
    throw new ApiError(msg, {
      status: response.status,
      errorNo: API_ERROR_NO.NO_ACCESS,
      data: envelope.data,
    })
  }

  // 3. HTTP 401 or errorNo 403 (NeedLogin) -> redirect to /login
  // Note: HTTP 403 alone (without errorNo 403) is NOT unauthenticated; e.g. pending registration returns HTTP 403!
  const isNeedLogin = response.status === 401 || envelope?.errorNo === API_ERROR_NO.NEED_LOGIN
  if (isNeedLogin) {
    if (!skipAuthRedirect && unauthorizedHandler) {
      unauthorizedHandler()
    }
    const msg = resolveApiErrorMessage(envelope?.errorMsg, 401)
    throw new ApiError(msg, {
      status: response.status || 401,
      errorNo: envelope?.errorNo ?? 401,
      data: envelope?.data,
    })
  }

  // 4. HTTP 403 with non-403 / non-405 errorNo (e.g. system in pending-register state) -> re-query /api/bootstrap
  if (response.status === 403) {
    if (!skipAuthRedirect && needBootstrapHandler) {
      needBootstrapHandler()
    }
    const msg = resolveApiErrorMessage(envelope?.errorMsg, 403)
    throw new ApiError(msg, {
      status: 403,
      errorNo: envelope?.errorNo ?? 403,
      data: envelope?.data,
    })
  }

  // 5. Rate limited
  if (response.status === 429 || envelope?.errorNo === API_ERROR_NO.RATE_LIMITED) {
    const msg = resolveApiErrorMessage(envelope?.errorMsg, 429)
    throw new ApiError(msg, { status: 429, errorNo: 429, data: envelope?.data })
  }

  if (!response.ok) {
    const msg = resolveApiErrorMessage(envelope?.errorMsg, response.status)
    throw new ApiError(msg, {
      status: response.status,
      errorNo: envelope?.errorNo ?? response.status,
      data: envelope?.data,
    })
  }

  if (envelope) {
    if (envelope.errorNo !== 0) {
      const msg = resolveApiErrorMessage(envelope.errorMsg, response.status)
      throw new ApiError(msg, {
        status: response.status,
        errorNo: envelope.errorNo,
        data: envelope.data,
      })
    }
    return envelope.data as T
  }

  return parsed as T
}

export const apiGet = <T>(path: string, options?: Omit<RequestOptions, 'method' | 'body'>) =>
  apiRequest<T>(path, { ...options, method: 'GET' })

export const apiPost = <T>(
  path: string,
  body: unknown = {},
  options?: Omit<RequestOptions, 'method' | 'body'>,
) => apiRequest<T>(path, { ...options, method: 'POST', body })

export const apiPut = <T>(
  path: string,
  body: unknown = {},
  options?: Omit<RequestOptions, 'method' | 'body'>,
) => apiRequest<T>(path, { ...options, method: 'PUT', body })

export const apiDelete = <T>(
  path: string,
  body?: unknown,
  options?: Omit<RequestOptions, 'method' | 'body'>,
) => apiRequest<T>(path, { ...options, method: 'DELETE', body })

export const apiUpload = <T>(
  path: string,
  formData: FormData,
  options?: Omit<RequestOptions, 'method' | 'body'>,
) => apiRequest<T>(path, { ...options, method: 'POST', body: formData })
