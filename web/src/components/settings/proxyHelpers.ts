import type { ProxyInput, ProxyProtocol, ProxyView } from '@/api'

export interface ProxyDraft {
  type: ProxyProtocol
  host: string
  port: string
  username: string
  password: string
  passwordSet: boolean
  clearPassword: boolean
}

export interface ProxyValidationErrors {
  host?: 'required' | 'invalidUrl'
  port?: 'invalid'
}

export function createEmptyProxyDraft(): ProxyDraft {
  return {
    type: 'http',
    host: '',
    port: '',
    username: '',
    password: '',
    passwordSet: false,
    clearPassword: false,
  }
}

export function proxyViewToDraft(view: ProxyView | null | undefined): ProxyDraft {
  if (!view) return createEmptyProxyDraft()
  return {
    type: view.type,
    host: view.host ?? '',
    port: view.port ? String(view.port) : '',
    username: view.username ?? '',
    password: '',
    passwordSet: Boolean(view.password_set),
    clearPassword: false,
  }
}

export function isProxyDraftEmpty(draft: ProxyDraft): boolean {
  return !draft.host.trim() && !draft.port.trim() && !draft.username.trim() && !draft.password.trim()
}

export function validateProxyDraft(
  draft: ProxyDraft,
  options?: { allowEmpty?: boolean },
): ProxyValidationErrors {
  if (options?.allowEmpty && isProxyDraftEmpty(draft)) {
    return {}
  }
  const errors: ProxyValidationErrors = {}
  const host = draft.host.trim()
  if (!host) {
    errors.host = 'required'
  } else if (host.includes('://') || /\s/.test(host)) {
    errors.host = 'invalidUrl'
  }

  const portStr = draft.port.trim()
  const portNum = Number(portStr)
  if (!portStr || !/^\d+$/.test(portStr) || !Number.isInteger(portNum) || portNum < 1 || portNum > 65535) {
    errors.port = 'invalid'
  }
  return errors
}

export function buildProxyInput(draft: ProxyDraft): ProxyInput {
  const payload: ProxyInput = {
    type: draft.type,
    host: draft.host.trim(),
    port: Number(draft.port.trim()),
    username: draft.username.trim(),
  }
  if (draft.clearPassword) {
    payload.password = ''
  } else if (draft.password.length > 0) {
    payload.password = draft.password
  }
  return payload
}
