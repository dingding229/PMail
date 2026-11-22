const STORAGE_PREFIX = 'pmail_'

export function getStorageItem<T>(key: string, fallback: T): T {
  try {
    const raw = localStorage.getItem(`${STORAGE_PREFIX}${key}`)
    if (raw === null) return fallback
    return JSON.parse(raw) as T
  } catch {
    return fallback
  }
}

export function setStorageItem<T>(key: string, value: T): void {
  try {
    localStorage.setItem(`${STORAGE_PREFIX}${key}`, JSON.stringify(value))
  } catch {
    // Ignore storage quota / private mode errors
  }
}

export function getRawStorage(key: string): string | null {
  try {
    return localStorage.getItem(`${STORAGE_PREFIX}${key}`)
  } catch {
    return null
  }
}

export function setRawStorage(key: string, value: string): void {
  try {
    localStorage.setItem(`${STORAGE_PREFIX}${key}`, value)
  } catch {
    // Ignore storage errors
  }
}

export function removeRawStorage(key: string): void {
  try {
    localStorage.removeItem(`${STORAGE_PREFIX}${key}`)
  } catch {
    // Ignore storage errors
  }
}
