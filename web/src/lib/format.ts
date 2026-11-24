const AVATAR_PALETTES = [
  { bg: 'bg-blue-500/15 dark:bg-blue-400/20', text: 'text-blue-700 dark:text-blue-300' },
  { bg: 'bg-emerald-500/15 dark:bg-emerald-400/20', text: 'text-emerald-700 dark:text-emerald-300' },
  { bg: 'bg-violet-500/15 dark:bg-violet-400/20', text: 'text-violet-700 dark:text-violet-300' },
  { bg: 'bg-amber-500/15 dark:bg-amber-400/20', text: 'text-amber-700 dark:text-amber-300' },
  { bg: 'bg-rose-500/15 dark:bg-rose-400/20', text: 'text-rose-700 dark:text-rose-300' },
  { bg: 'bg-cyan-500/15 dark:bg-cyan-400/20', text: 'text-cyan-700 dark:text-cyan-300' },
  { bg: 'bg-indigo-500/15 dark:bg-indigo-400/20', text: 'text-indigo-700 dark:text-indigo-300' },
  { bg: 'bg-teal-500/15 dark:bg-teal-400/20', text: 'text-teal-700 dark:text-teal-300' },
] as const

export function getAvatarPalette(seed: string) {
  let hash = 0
  for (let i = 0; i < seed.length; i++) {
    hash = (hash * 31 + seed.charCodeAt(i)) | 0
  }
  const index = Math.abs(hash) % AVATAR_PALETTES.length
  return AVATAR_PALETTES[index] ?? AVATAR_PALETTES[0]
}

export function getInitials(name?: string, email?: string): string {
  const cleanName = (name ?? '').trim().replace(/^["']|["']$/g, '')
  if (cleanName) {
    // If starts with CJK character, use first character
    const firstChar = Array.from(cleanName)[0] ?? ''
    if (/[\u4e00-\u9fff\u3400-\u4dbf\u3040-\u30ff\uac00-\ud7af]/.test(firstChar)) {
      return firstChar
    }
    const parts = cleanName.split(/\s+/).filter(Boolean)
    if (parts.length >= 2) {
      return `${parts[0]?.[0] ?? ''}${parts[1]?.[0] ?? ''}`.toUpperCase()
    }
    return cleanName.slice(0, 2).toUpperCase()
  }
  const cleanEmail = (email ?? '').trim()
  if (cleanEmail) {
    return cleanEmail.slice(0, 1).toUpperCase()
  }
  return '?'
}

export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  const exponent = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  const val = bytes / Math.pow(1024, exponent)
  return `${val >= 10 || exponent === 0 ? Math.round(val) : val.toFixed(1)} ${units[exponent]}`
}

/**
 * Formats a registration key into XXXX-XXXX-XXXX-XXXX, uppercased, stripping non-alphanumeric chars.
 */
export function formatRegistrationKey(raw: string): string {
  const cleaned = raw.toUpperCase().replace(/[^A-Z0-9]/g, '').slice(0, 16)
  const chunks: string[] = []
  for (let i = 0; i < cleaned.length; i += 4) {
    chunks.push(cleaned.slice(i, i + 4))
  }
  return chunks.join('-')
}

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

export function isValidEmail(email: string): boolean {
  return EMAIL_RE.test(email.trim())
}

export interface ParsedEmailAddress {
  name: string
  email: string
}

export interface NormalizedAttachmentItem {
  index: number
  filename: string
  contentType: string
  size?: number
  contentId?: string
  inline?: boolean
  previewable?: boolean
}

export function resolveAddressList(
  normalized: { name?: string; address?: string }[] | undefined | null,
  legacyRaw: unknown,
): ParsedEmailAddress[] {
  if (Array.isArray(normalized) && normalized.length > 0) {
    return normalized
      .map((item) => ({
        name: (item.name ?? '').trim(),
        email: (item.address ?? '').trim(),
      }))
      .filter((x) => Boolean(x.email))
  }
  return parseAddressList(legacyRaw)
}

export function resolveAttachmentsList(
  normalized:
    | {
        index: number
        filename: string
        content_type: string
        size?: number
        content_id?: string
        inline?: boolean
        previewable?: boolean
      }[]
    | undefined
    | null,
  legacy:
    | {
        Filename: string
        ContentType: string
        Index: number
        Size?: number
        ContentID?: string
      }[]
    | undefined
    | null,
): NormalizedAttachmentItem[] {
  if (Array.isArray(normalized) && normalized.length > 0) {
    return normalized.map((att) => ({
      index: att.index,
      filename: att.filename,
      contentType: att.content_type,
      size: att.size,
      contentId: att.content_id,
      inline: att.inline,
      previewable: att.previewable,
    }))
  }
  if (Array.isArray(legacy) && legacy.length > 0) {
    return legacy.map((att) => ({
      index: att.Index,
      filename: att.Filename,
      contentType: att.ContentType,
      size: att.Size,
      contentId: att.ContentID,
    }))
  }
  return []
}

export function parseAddressList(raw: unknown): ParsedEmailAddress[] {
  if (!raw) return []
  if (Array.isArray(raw)) {
    return raw
      .map((item) => {
        if (typeof item === 'string') return parseSingleAddress(item)
        if (item && typeof item === 'object') {
          const obj = item as Record<string, unknown>
          const email = String(obj.EmailAddress ?? obj.email ?? obj.address ?? '').trim()
          const name = String(obj.Name ?? obj.name ?? '').trim()
          if (!email) return null
          return { name, email }
        }
        return null
      })
      .filter((x): x is ParsedEmailAddress => x !== null)
  }
  if (typeof raw === 'string') {
    const trimmed = raw.trim()
    // Older rows store a missing Reply-To as the literal string "null".
    if (!trimmed || trimmed === 'null') return []
    if (trimmed.startsWith('[') || trimmed.startsWith('{')) {
      try {
        const parsed = JSON.parse(trimmed)
        return parseAddressList(Array.isArray(parsed) ? parsed : [parsed])
      } catch {
        // Fallback to plain address string parsing
      }
    }
    return trimmed
      .split(/[,;]/)
      .map((part) => parseSingleAddress(part))
      .filter((x): x is ParsedEmailAddress => x !== null)
  }
  return []
}

export function parseSingleAddress(raw: string): ParsedEmailAddress | null {
  const trimmed = raw.trim()
  if (!trimmed) return null
  const angleMatch = trimmed.match(/^(.*)<([^>]+)>$/)
  if (angleMatch) {
    const name = (angleMatch[1] ?? '').trim().replace(/^["']|["']$/g, '')
    const email = (angleMatch[2] ?? '').trim()
    if (!email) return null
    return { name, email }
  }
  return { name: '', email: trimmed }
}

export function stripHtmlToText(html: string): string {
  return html
    .replace(/<style[\s\S]*?<\/style>/gi, ' ')
    .replace(/<script[\s\S]*?<\/script>/gi, ' ')
    .replace(/<br\s*\/?>/gi, '\n')
    .replace(/<\/(p|div|li|h[1-6]|tr)>/gi, '\n')
    .replace(/<[^>]+>/g, '')
    .replace(/&nbsp;/gi, ' ')
    .replace(/&lt;/gi, '<')
    .replace(/&gt;/gi, '>')
    .replace(/&amp;/gi, '&')
    .replace(/\n{3,}/g, '\n\n')
    .trim()
}
