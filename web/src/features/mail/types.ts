import type { EmailDetail, SearchTag } from '@/api'

export type SystemFolderKey = 'inbox' | 'starred' | 'sent' | 'drafts' | 'junk' | 'trash'

export type FolderSelection =
  | { kind: 'system'; key: SystemFolderKey }
  | { kind: 'group'; id: number; name: string; tag: string }

export type MailFilterMode = 'all' | 'unread' | 'attachments'

export interface LocalDraft {
  id: string
  fromAddress: string
  to: string[]
  cc: string[]
  bcc: string[]
  subject: string
  html: string
  text: string
  updatedAt: string
  mode: 'new' | 'reply' | 'replyAll' | 'forward'
  sourceEmailId?: number
}

export interface ComposeContext {
  mode: 'new' | 'reply' | 'replyAll' | 'forward' | 'draft' | 'serverDraft'
  sourceEmail?: EmailDetail
  draft?: LocalDraft
  /** True only if the user explicitly clicked "Load external images" in ReadingPane for this email */
  allowExternalImages?: boolean
}

export const SYSTEM_FOLDER_TAGS: Record<Exclude<SystemFolderKey, 'starred'>, SearchTag> = {
  inbox: { type: 0, status: -1, group_id: 0 },
  sent: { type: 1, status: -1, group_id: -1 },
  drafts: { type: 0, status: 4, group_id: -1 },
  junk: { type: -1, status: 5, group_id: -1 },
  trash: { type: -1, status: 3, group_id: -1 },
}

export function getSearchTagForFolder(folder: FolderSelection): string {
  if (folder.kind === 'group') {
    return folder.tag || JSON.stringify({ type: -1, status: -1, group_id: folder.id })
  }
  if (folder.key === 'starred') {
    // Request across all folders; starred IDs are filtered on client/mock and compatible with future backend starred tag
    return JSON.stringify({ type: -1, status: -1, group_id: -1, starred: true })
  }
  return JSON.stringify(SYSTEM_FOLDER_TAGS[folder.key])
}

export function folderToRouteSegment(folder: FolderSelection): string {
  if (folder.kind === 'group') return `group-${folder.id}`
  return folder.key
}

export function parseFolderSegment(segment?: string): FolderSelection {
  if (!segment) return { kind: 'system', key: 'inbox' }
  if (
    segment === 'inbox' ||
    segment === 'starred' ||
    segment === 'sent' ||
    segment === 'drafts' ||
    segment === 'junk' ||
    segment === 'trash'
  ) {
    return { kind: 'system', key: segment }
  }
  if (segment.startsWith('group-')) {
    const id = Number(segment.slice('group-'.length))
    if (Number.isInteger(id) && id > 0) {
      return {
        kind: 'group',
        id,
        name: '',
        tag: JSON.stringify({ type: -1, status: -1, group_id: id }),
      }
    }
  }
  return { kind: 'system', key: 'inbox' }
}
