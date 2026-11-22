import { useCallback, useEffect, useState } from 'react'
import { getStorageItem, setStorageItem } from '@/lib/storage'
import type { LocalDraft } from '@/features/mail/types'

const DRAFTS_KEY = 'local_drafts'

function readDrafts(): LocalDraft[] {
  const list = getStorageItem<LocalDraft[]>(DRAFTS_KEY, [])
  return Array.isArray(list) ? list : []
}

const listeners = new Set<() => void>()

function emitChange() {
  listeners.forEach((fn) => fn())
}

export function saveLocalDraft(draft: LocalDraft): void {
  const existing = readDrafts()
  const idx = existing.findIndex((d) => d.id === draft.id)
  const next =
    idx >= 0
      ? existing.map((d, i) => (i === idx ? draft : d))
      : [draft, ...existing]
  next.sort((a, b) => (b.updatedAt > a.updatedAt ? 1 : -1))
  setStorageItem(DRAFTS_KEY, next)
  emitChange()
}

export function deleteLocalDraft(id: string): void {
  const existing = readDrafts()
  setStorageItem(
    DRAFTS_KEY,
    existing.filter((d) => d.id !== id),
  )
  emitChange()
}

export function useLocalDrafts() {
  const [drafts, setDrafts] = useState<LocalDraft[]>(readDrafts)

  useEffect(() => {
    const sync = () => setDrafts(readDrafts())
    listeners.add(sync)
    window.addEventListener('storage', sync)
    return () => {
      listeners.delete(sync)
      window.removeEventListener('storage', sync)
    }
  }, [])

  const upsert = useCallback((draft: LocalDraft) => saveLocalDraft(draft), [])
  const remove = useCallback((id: string) => deleteLocalDraft(id), [])

  return { drafts, upsertDraft: upsert, removeDraft: remove }
}
