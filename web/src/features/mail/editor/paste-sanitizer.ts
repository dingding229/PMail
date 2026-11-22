import DOMPurify from 'dompurify'
import type { SupportedCodeLang } from '@/lib/code-languages'

export const MAX_INLINE_IMAGE_BYTES = 10 * 1024 * 1024

export interface SanitizeHtmlOptions {
  /** Block external http:, https:, and protocol-relative // image URLs */
  blockExternalImages?: boolean
  /** Max decoded byte size allowed for data:image/*;base64 URLs (default 10 MB) */
  maxDataImageBytes?: number
  /** Optional resolver mapping cid:... -> safe same-origin attachment URL */
  resolveCidUrl?: (cid: string) => string | null
  /** Format placeholder text for blocked/unresolvable images */
  imagePlaceholder?: (label: string) => string
  /** Fallback label when image has no alt or filename */
  fallbackImageLabel?: string
}

const SAFE_CSS_PROPERTIES = new Set([
  'color',
  'background-color',
  'font-size',
  'text-align',
  'font-weight',
  'font-style',
  'text-decoration',
])

const SAFE_COLOR_RE =
  /^(?:#[0-9a-f]{3,8}|rgba?\(\s*\d{1,3}\s*,\s*\d{1,3}\s*,\s*\d{1,3}(?:\s*,\s*(?:0|1|0?\.\d+))?\s*\)|[a-z]{3,20})$/i
const SAFE_FONT_SIZE_RE = /^(?:1[0-9]|2[0-8]|3[0-2])px$/i
const SAFE_TEXT_ALIGN_RE = /^(?:left|center|right|justify)$/i
const EXTERNAL_URL_RE = /^(?:https?:)?\/\//i
const SAFE_DATA_IMAGE_RE = /^data:image\/(?:png|jpe?g|gif|webp|avif);base64,([A-Za-z0-9+/=\s]+)$/i

function sanitizeInlineStyle(rawStyle: string): string {
  const kept: string[] = []
  for (const decl of rawStyle.split(';')) {
    const colon = decl.indexOf(':')
    if (colon <= 0) continue
    const prop = decl.slice(0, colon).trim().toLowerCase()
    const val = decl.slice(colon + 1).trim()
    if (!SAFE_CSS_PROPERTIES.has(prop) || !val) continue
    // Reject url(...), @import, expression(...), javascript:, or any external URL token
    const cleanVal = val.replace(/\s*!important\s*$/i, '').trim()
    if (/url\s*\(|@import|expression\s*\(|javascript:|https?:|\/\//i.test(cleanVal)) continue

    if (prop === 'color' || prop === 'background-color') {
      if (!SAFE_COLOR_RE.test(cleanVal)) continue
    } else if (prop === 'font-size') {
      // Convert pt from Word (e.g. 12pt -> 16px) into clamped px range
      const ptMatch = cleanVal.match(/^(\d+(?:\.\d+)?)pt$/i)
      if (ptMatch) {
        const px = Math.min(32, Math.max(11, Math.round(Number(ptMatch[1]) * 1.333)))
        kept.push(`font-size: ${px}px`)
        continue
      }
      if (!SAFE_FONT_SIZE_RE.test(cleanVal)) continue
    } else if (prop === 'text-align') {
      if (!SAFE_TEXT_ALIGN_RE.test(cleanVal)) continue
    }

    kept.push(`${prop}: ${cleanVal}`)
  }
  return kept.join('; ')
}

function extractImageLabel(img: Element, src: string, fallback: string): string {
  const alt = (img.getAttribute('alt') || '').trim()
  if (alt) return alt

  const filename = (img.getAttribute('data-filename') || '').trim()
  if (filename) return filename

  if (/^cid:/i.test(src)) {
    const cidName = src.replace(/^cid:/i, '').replace(/^[<\s]+|[>\s]+$/g, '').split('@')[0]?.trim()
    if (cidName) return cidName
  } else if (EXTERNAL_URL_RE.test(src)) {
    try {
      const normalized = src.startsWith('//') ? `https:${src}` : src
      const pathname = new URL(normalized).pathname
      const seg = decodeURIComponent(pathname.split('/').filter(Boolean).pop() || '').trim()
      if (seg && seg.length <= 64) return seg
    } catch {
      // Ignore malformed URL
    }
  }

  return fallback
}

function createPlaceholderNode(doc: Document, text: string): HTMLSpanElement {
  const span = doc.createElement('span')
  span.textContent = text
  return span
}

function estimateBase64Bytes(b64: string): number {
  const compact = b64.replace(/\s+/g, '')
  const padding = compact.endsWith('==') ? 2 : compact.endsWith('=') ? 1 : 0
  return Math.max(0, Math.floor((compact.length * 3) / 4) - padding)
}

/**
 * Cleans HTML for pasting, reply/forward quoting, and signature previews:
 * - Strips scripts, event handlers, external stylesheets, <style>/<meta>/<xml>, Word MSO markup,
 *   <picture>/<source>/<video>/<audio>/<svg>, srcset, poster, background attributes, and CSS url(...).
 * - Preserves semantic formatting: headings, bold/italic/underline/strike, colors, highlights,
 *   alignment, lists, task lists, blockquotes, links, tables, and code blocks.
 * - Handles <img> resources safely: blocks external http(s):// and // images when `blockExternalImages`
 *   is enabled, resolves or replaces cid: images, and enforces size limits on data: images.
 */
export function sanitizePastedHtml(rawHtml: string, options: SanitizeHtmlOptions = {}): string {
  if (!rawHtml || !rawHtml.trim()) return ''

  const {
    blockExternalImages = false,
    maxDataImageBytes = MAX_INLINE_IMAGE_BYTES,
    resolveCidUrl,
    imagePlaceholder = (label: string) => `[图片：${label}]`,
    fallbackImageLabel = '外部图片',
  } = options

  // 1. Pre-strip MS Office conditional comments & XML islands before DOMPurify
  const preCleaned = rawHtml
    .replace(/<!--\[if[\s\S]*?<!\[endif\]-->/gi, '')
    .replace(/<!--StartFragment-->|<!--EndFragment-->/gi, '')
    .replace(/<xml[\s\S]*?<\/xml>/gi, '')
    .replace(/<style[\s\S]*?<\/style>/gi, '')
    .replace(/<\/?o:[^>]*>/gi, '')
    .replace(/<\/?w:[^>]*>/gi, '')

  // 2. Strict DOMPurify pass (forbids picture/source/video/audio/svg/style/link/script and srcset/poster/background)
  const purified = DOMPurify.sanitize(preCleaned, {
    ALLOWED_TAGS: [
      'p', 'br', 'div', 'span',
      'h1', 'h2', 'h3', 'h4', 'h5', 'h6',
      'strong', 'b', 'em', 'i', 'u', 's', 'del', 'mark', 'sub', 'sup',
      'a', 'blockquote', 'hr',
      'ul', 'ol', 'li', 'input',
      'pre', 'code',
      'table', 'thead', 'tbody', 'tfoot', 'tr', 'th', 'td',
      'img',
    ],
    ALLOWED_ATTR: [
      'href', 'target', 'rel', 'style', 'class',
      'colspan', 'rowspan',
      'src', 'alt', 'data-cid', 'data-filename', 'data-type', 'data-checked',
      'type', 'checked',
    ],
    FORBID_TAGS: [
      'script', 'style', 'link', 'meta', 'iframe', 'object', 'embed',
      'form', 'button', 'picture', 'source', 'video', 'audio', 'svg',
    ],
    FORBID_ATTR: ['srcset', 'poster', 'background', 'onerror', 'onload', 'onclick'],
    ALLOW_DATA_ATTR: false,
  })

  if (typeof DOMParser === 'undefined') return purified

  const doc = new DOMParser().parseFromString(purified, 'text/html')

  // Defense-in-depth: unwrap <picture> so its fallback <img> is safely processed into a placeholder,
  // and strip any residual forbidden active/media/external tags even if DOMPurify was bypassed.
  doc.body.querySelectorAll('picture').forEach((pic) => {
    pic.replaceWith(...Array.from(pic.childNodes))
  })
  doc.body
    .querySelectorAll('script, style, link, meta, iframe, object, embed, form, button, source, video, audio, svg')
    .forEach((el) => el.remove())

  // 3. Normalize & strip residual external/Word attributes
  doc.body.querySelectorAll('*').forEach((el) => {
    const tag = el.tagName.toLowerCase()
    el.removeAttribute('srcset')
    el.removeAttribute('poster')
    el.removeAttribute('background')

    // Keep only language-* class on <code>
    if (tag === 'code') {
      const cls = el.getAttribute('class') || ''
      const langMatch = cls.match(/\b(?:language|lang)-([a-zA-Z0-9_+#.-]{1,32})\b/)
      if (langMatch) {
        el.setAttribute('class', `language-${langMatch[1]}`)
      } else {
        el.removeAttribute('class')
      }
    } else {
      el.removeAttribute('class')
    }

    // Sanitize inline style whitelist (drops background / background-image / url(...))
    const rawStyle = el.getAttribute('style')
    if (rawStyle) {
      const safe = sanitizeInlineStyle(rawStyle)
      if (safe) {
        el.setAttribute('style', safe)
      } else {
        el.removeAttribute('style')
      }
    }

    // Ensure links are safe and strip javascript: hrefs
    if (tag === 'a') {
      const href = (el.getAttribute('href') || '').trim()
      if (!href || /^(?:javascript|vbscript|data):/i.test(href)) {
        el.removeAttribute('href')
      } else {
        el.setAttribute('target', '_blank')
        el.setAttribute('rel', 'noopener noreferrer')
      }
    }

    // Only keep checkboxes inside task lists; drop arbitrary <input>
    if (tag === 'input' && el.getAttribute('type') !== 'checkbox') {
      el.remove()
    }
  })

  // 4. Process <img> elements (external tracking pixels, cid:, and data: size limits)
  doc.body.querySelectorAll('img').forEach((img) => {
    const src = (img.getAttribute('src') || '').trim()
    const label = extractImageLabel(img, src, fallbackImageLabel)
    const replaceAsBlocked = () => {
      img.replaceWith(createPlaceholderNode(doc, imagePlaceholder(label)))
    }

    if (!src) {
      replaceAsBlocked()
      return
    }

    // External http: / https: / protocol-relative //
    if (EXTERNAL_URL_RE.test(src)) {
      if (blockExternalImages) {
        replaceAsBlocked()
      }
      return
    }

    // cid: inline attachments — resolve to safe same-origin URL or replace with placeholder
    if (/^cid:/i.test(src)) {
      const cid = src.replace(/^cid:/i, '').replace(/^[<\s]+|[>\s]+$/g, '')
      const resolvedUrl = resolveCidUrl ? resolveCidUrl(cid) : null
      if (resolvedUrl && resolvedUrl.startsWith('/')) {
        img.setAttribute('src', resolvedUrl)
      } else {
        replaceAsBlocked()
      }
      return
    }

    // data: images — allow only safe raster formats within MAX_INLINE_IMAGE_BYTES
    if (/^data:/i.test(src)) {
      const match = src.match(SAFE_DATA_IMAGE_RE)
      if (!match?.[1]) {
        replaceAsBlocked()
        return
      }
      if (estimateBase64Bytes(match[1]) > maxDataImageBytes) {
        replaceAsBlocked()
      }
      return
    }

    // Allow same-origin relative attachment URLs (e.g. /api/attachments/123.0); block unknown schemes
    if (!src.startsWith('/api/attachments/')) {
      replaceAsBlocked()
    }
  })

  // Unwrap redundant empty-attribute <span> elements that aren't placeholders
  doc.body.querySelectorAll('span').forEach((span) => {
    if (span.attributes.length === 0 && span.children.length > 0) {
      span.replaceWith(...Array.from(span.childNodes))
    }
  })

  return doc.body.innerHTML
}

/**
 * Lightweight heuristic to detect whether pasted plain text is source code
 * (so we can automatically wrap it in a syntax-labeled code block with instant undo).
 * Designed to avoid false positives on regular Chinese/English prose.
 */
export function detectPastedCode(rawText: string): {
  isCode: boolean
  language: SupportedCodeLang
} {
  const text = rawText.replace(/\r\n/g, '\n').trim()
  if (!text || text.length < 18) return { isCode: false, language: 'text' }

  const lines = text.split('\n')
  if (lines.length < 2) {
    if (/^#!\//.test(text)) return { isCode: true, language: 'bash' }
    return { isCode: false, language: 'text' }
  }

  if (
    /^diff --git /m.test(text) ||
    (/^---\s+\S+/m.test(text) && /^\+\+\+\s+\S+/m.test(text) && /^@@ /m.test(text))
  ) {
    return { isCode: true, language: 'diff' }
  }

  if (
    (text.startsWith('{') && text.endsWith('}')) ||
    (text.startsWith('[') && text.endsWith(']'))
  ) {
    try {
      JSON.parse(text)
      return { isCode: true, language: 'json' }
    } catch {
      // fall through
    }
  }

  const nonEmptyLines = lines.filter((l) => l.trim().length > 0)
  const indentedLines = nonEmptyLines.filter((l) => /^(?: {2,}|\t)/.test(l)).length
  const symbolLines = nonEmptyLines.filter((l) =>
    /[{}[\];=<>]|\bfunc\b|\bdef\b|\bconst\b|\blet\b|\bimport\b|\bexport\b|\breturn\b|\bclass\b|\bSELECT\b|\bFROM\b|\bpackage\b/.test(
      l,
    ),
  ).length

  let score = 0
  if (indentedLines >= 1) score += 1
  if (indentedLines / Math.max(1, nonEmptyLines.length) >= 0.3) score += 1
  if (symbolLines / Math.max(1, nonEmptyLines.length) >= 0.45) score += 2
  if (/[{}();]\s*$/m.test(text)) score += 1
  if (
    /^\s*(?:import|export|from|package|func|def|class|interface|type|const|let|var|fn|pub|SELECT|CREATE|FROM\s+\S+)\b/m.test(
      text,
    )
  ) {
    score += 2
  }
  if (/^#!\//m.test(text)) score += 3

  const cjkProseChars = (text.match(/[。！？；，]/g) ?? []).length
  if (cjkProseChars >= 3 && symbolLines < 2) {
    score -= 3
  }

  if (score < 3) {
    return { isCode: false, language: 'text' }
  }

  if (
    /^\s*package\s+\w+/m.test(text) ||
    /\bfunc\s+(?:\([^)]+\)\s*)?\w+\s*\(/.test(text) ||
    /:=/.test(text)
  ) {
    return { isCode: true, language: 'go' }
  }
  if (/\b(?:interface|type)\s+\w+\s*[={]|:\s*(?:string|number|boolean|Promise<)/.test(text)) {
    return { isCode: true, language: 'typescript' }
  }
  if (/\b(?:const|let|var|function|async)\s+\w+|=>\s*\{/.test(text)) {
    return { isCode: true, language: 'javascript' }
  }
  if (/^\s*(?:def|class|from\s+\S+\s+import|import\s+\w+)/m.test(text) && /:\s*$/m.test(text)) {
    return { isCode: true, language: 'python' }
  }
  if (/\b(?:fn\s+\w+|let\s+mut\b|impl\s+\w+|println!)/.test(text)) {
    return { isCode: true, language: 'rust' }
  }
  if (/\b(?:SELECT|INSERT\s+INTO|UPDATE|DELETE\s+FROM|CREATE\s+TABLE)\b/i.test(text)) {
    return { isCode: true, language: 'sql' }
  }
  if (
    /^#!\s*\/(?:usr\/bin\/env\s+)?(?:ba|z)?sh\b/m.test(text) ||
    /^\s*(?:docker|curl|npm|go|git)\s+/m.test(text)
  ) {
    return { isCode: true, language: 'bash' }
  }
  if (/^\s*FROM\s+\S+/m.test(text) && /^\s*(?:RUN|COPY|WORKDIR|CMD|ENV)\s+/m.test(text)) {
    return { isCode: true, language: 'dockerfile' }
  }
  if (/^<!DOCTYPE html|<html\b|<div\b|<section\b/i.test(text)) {
    return { isCode: true, language: 'html' }
  }
  if (/^[a-zA-Z0-9_.-]+:\s+\S+/m.test(text) && indentedLines >= 1) {
    return { isCode: true, language: 'yaml' }
  }

  return { isCode: true, language: 'text' }
}
