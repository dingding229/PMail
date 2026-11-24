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
  readonly serverMessage?: string
  /** A real 401 or an explicit NeedLogin envelope, not a bare HTTP 403. */
  readonly isUnauthenticated: boolean

  constructor(message: string, options: {
    status: number
    errorNo?: number
    data?: unknown
    serverMessage?: string
    isUnauthenticated?: boolean
  }) {
    super(message)
    this.name = 'ApiError'
    this.status = options.status
    this.errorNo = options.errorNo ?? options.status
    this.data = options.data
    this.serverMessage = options.serverMessage
    this.isUnauthenticated = options.isUnauthenticated ?? (options.status === 401)
  }
}

type AuthHandler = () => void
let unauthorizedHandler: AuthHandler | null = null
let needBootstrapHandler: AuthHandler | null = null
let maintenanceHandler: ((active: boolean) => void) | null = null
// A request can hit maintenance before the gate registers its handler (child
// effects run first on a cold load). Remember it and replay on registration.
let maintenanceSeen = false

export function setUnauthorizedHandler(handler: AuthHandler | null): void {
  unauthorizedHandler = handler
}

export function setNeedBootstrapHandler(handler: AuthHandler | null): void {
  needBootstrapHandler = handler
}

export function setMaintenanceHandler(handler: ((active: boolean) => void) | null): void {
  maintenanceHandler = handler
  if (handler && maintenanceSeen) {
    handler(true)
  }
}

function signalMaintenance(active: boolean): void {
  maintenanceSeen = active
  maintenanceHandler?.(active)
}

export function notifyMaintenanceState(active: boolean): void {
  signalMaintenance(active)
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
  cache?: RequestCache
  skipAuthRedirect?: boolean
  skipMaintenanceOverlay?: boolean
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
  registration_key_invalid: 'auth.errors.registrationKeyInvalid',
  domain_invalid: 'auth.errors.domainInvalid',
  invalid_request: 'auth.errors.invalidSetupRequest',
  mail_domain_already_configured: 'auth.errors.domainAlreadyConfigured',
  registration_rate_limit: 'auth.errors.registrationRateLimit',
  internal_error: 'common.unknownError',
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
  trash_retention_invalid: 'admin.trash.errors.retentionInvalid',
  telegram_proxy_invalid: 'proxy.errors.invalid',
  translation_disabled: 'translation.errors.disabled',
  translation_daily_limit: 'translation.errors.dailyLimit',
  translation_upstream_rate_limited: 'translation.errors.rateLimited',
  translation_provider_failed: 'translation.errors.providerFailed',
  translation_connection_failed: 'translation.errors.connectionFailed',
  translation_upstream_auth: 'translation.errors.upstreamAuth',
  translation_response_invalid: 'translation.errors.responseInvalid',
  translation_timeout: 'translation.errors.timeout',
  translation_invalid: 'translation.errors.invalid',
  translation_mail_not_found: 'translation.errors.mailNotFound',
  ai_config_disabled: 'translation.errors.aiConfigDisabled',
  ai_not_configured: 'translation.errors.aiNotConfigured',
  ai_network_blocked: 'translation.errors.aiNetworkBlocked',
  ai_busy: 'translation.errors.aiBusy',
  address_pending_limit: 'settings.addresses.errors.pendingLimit',
  address_reject_reason_invalid: 'admin.addressApproval.errors.rejectReasonInvalid',
  invalid_domain: 'admin.addressApproval.errors.invalidDomain',
  address_not_found: 'admin.addressApproval.errors.addressNotFound',
  domain_not_found: 'admin.addressApproval.errors.domainNotFound',
  address_already_reviewed: 'admin.addressApproval.errors.alreadyReviewed',
  address_exists: 'admin.addressApproval.errors.addressExists',
  live_updates_busy: 'mail.live.busy',
  backup_busy: 'admin.backup.errors.busy',
  backup_bad_passphrase: 'admin.backup.errors.badPassphrase',
  backup_version_newer: 'admin.backup.errors.versionNewer',
  backup_corrupt: 'admin.backup.errors.corrupt',
  maintenance: 'common.maintenance.title',
  quota_exceeded: 'settings.importExport.errors.quotaExceeded',
  export_busy: 'settings.importExport.errors.exportBusy',
  import_busy: 'settings.importExport.errors.importBusy',
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
    cache,
    skipAuthRedirect = false,
    skipMaintenanceOverlay = false,
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
      cache,
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

  // Maintenance mode (HTTP 503 + code=maintenance) applies to every endpoint,
  // including raw JSON ones such as /api/bootstrap on a cold page load.
  const maintenanceEnvelope = isApiEnvelope(parsed) ? parsed : null
  const dataCode =
    maintenanceEnvelope?.data && typeof maintenanceEnvelope.data === 'object' && 'code' in maintenanceEnvelope.data
      ? String((maintenanceEnvelope.data as { code?: unknown }).code ?? '')
      : ''
  const isMaintenance =
    response.status === 503 &&
    (dataCode === 'maintenance' || maintenanceEnvelope?.errorMsg === 'maintenance')
  if (isMaintenance) {
    if (!skipMaintenanceOverlay) {
      signalMaintenance(true)
    }
    throw new ApiError(resolveApiErrorMessage('maintenance', 503), {
      status: 503,
      errorNo: maintenanceEnvelope?.errorNo ?? 503,
      data: maintenanceEnvelope?.data,
      serverMessage: 'maintenance',
    })
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
  const serverMessage = typeof envelope?.errorMsg === 'string' ? envelope.errorMsg.trim() : undefined

  // 1. errorNo 402 (NeedSetup / NeedRegister) -> re-check /api/bootstrap and route to /register
  if (envelope?.errorNo === API_ERROR_NO.NEED_SETUP) {
    if (!skipAuthRedirect && needBootstrapHandler) {
      needBootstrapHandler()
    }
    const msg = resolveApiErrorMessage(envelope.errorMsg, response.status)
    throw new ApiError(msg, { status: response.status, errorNo: API_ERROR_NO.NEED_SETUP, data: envelope.data, serverMessage })
  }

  // 2. errorNo 405 (NoAccessPrivileges / origin_forbidden / token_scope_forbidden) -> display error only, NEVER redirect to login!
  if (envelope?.errorNo === API_ERROR_NO.NO_ACCESS) {
    const msg = resolveApiErrorMessage(envelope.errorMsg, response.status || 403)
    throw new ApiError(msg, {
      status: response.status,
      errorNo: API_ERROR_NO.NO_ACCESS,
      data: envelope.data,
      serverMessage,
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
      serverMessage,
      isUnauthenticated: true,
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
      serverMessage,
    })
  }

  // 6. Rate limited
  if (response.status === 429 || envelope?.errorNo === API_ERROR_NO.RATE_LIMITED) {
    const msg = resolveApiErrorMessage(envelope?.errorMsg, 429)
    throw new ApiError(msg, { status: 429, errorNo: 429, data: envelope?.data, serverMessage })
  }

  if (!response.ok) {
    const msg = resolveApiErrorMessage(dataCode || envelope?.errorMsg, response.status)
    throw new ApiError(msg, {
      status: response.status,
      errorNo: envelope?.errorNo ?? response.status,
      data: envelope?.data,
      serverMessage,
    })
  }

  if (envelope) {
    if (envelope.errorNo !== 0) {
      const msg = resolveApiErrorMessage(envelope.errorMsg, response.status)
      throw new ApiError(msg, {
        status: response.status,
        errorNo: envelope.errorNo,
        data: envelope.data,
        serverMessage,
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

/**
 * Multipart upload with real progress callback (0..100).
 * Falls back to fetch-based apiUpload when XMLHttpRequest is unavailable (e.g. Node tests).
 */
export function apiUploadWithProgress<T>(
  path: string,
  formData: FormData,
  onProgress?: (percent: number) => void,
): Promise<T> {
  if (typeof XMLHttpRequest === 'undefined') {
    onProgress?.(50)
    return apiUpload<T>(path, formData).then((res) => {
      onProgress?.(100)
      return res
    })
  }

  return new Promise<T>((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open('POST', path, true)
    xhr.withCredentials = true

    const lang = getAppLanguage()
    xhr.setRequestHeader('X-PMail-Lang', lang)
    xhr.setRequestHeader('Lang', lang.startsWith('zh') ? 'zh' : 'en')
    const token = getBearerToken()
    if (token) {
      xhr.setRequestHeader('Authorization', `Bearer ${token}`)
    }

    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable && e.total > 0) {
        onProgress?.(Math.min(99, Math.round((e.loaded / e.total) * 100)))
      }
    }

    xhr.onerror = () => {
      reject(new ApiError(i18n.t('common.networkError'), { status: 0, errorNo: 0 }))
    }

    xhr.onload = () => {
      onProgress?.(100)
      let parsed: unknown = null
      try {
        parsed = xhr.responseText ? JSON.parse(xhr.responseText) : null
      } catch {
        parsed = null
      }
      const envelope = isApiEnvelope(parsed) ? parsed : null
      const dataCode =
        envelope?.data && typeof envelope.data === 'object' && 'code' in envelope.data
          ? String((envelope.data as { code?: unknown }).code ?? '')
          : ''

      if (xhr.status === 503 && (dataCode === 'maintenance' || envelope?.errorMsg === 'maintenance')) {
        signalMaintenance(true)
        reject(new ApiError(resolveApiErrorMessage('maintenance', 503), { status: 503, errorNo: 503, data: envelope?.data }))
        return
      }
      if (xhr.status === 401 || envelope?.errorNo === API_ERROR_NO.NEED_LOGIN) {
        unauthorizedHandler?.()
        reject(new ApiError(resolveApiErrorMessage(envelope?.errorMsg, 401), { status: 401, errorNo: 401, isUnauthenticated: true }))
        return
      }
      if (xhr.status < 200 || xhr.status >= 300) {
        const msg = resolveApiErrorMessage(dataCode || envelope?.errorMsg, xhr.status)
        reject(new ApiError(msg, { status: xhr.status, errorNo: envelope?.errorNo ?? xhr.status, data: envelope?.data }))
        return
      }
      if (envelope) {
        if (envelope.errorNo !== 0) {
          reject(new ApiError(resolveApiErrorMessage(dataCode || envelope.errorMsg, xhr.status), { status: xhr.status, errorNo: envelope.errorNo, data: envelope.data }))
          return
        }
        resolve(envelope.data as T)
        return
      }
      resolve(parsed as T)
    }

    xhr.send(formData)
  })
}
