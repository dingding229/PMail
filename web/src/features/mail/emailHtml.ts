import DOMPurify from 'dompurify'
import { proxiedImageUrl } from './remoteImages'

// External resources: absolute http(s) URLs and protocol-relative ones.
const EXTERNAL_URL_RE = /^(?:https?:)?\/\//i
const EXTERNAL_CSS_URL_RE = /url\(\s*(['"]?)((?:https?:)?\/\/[^'")\s]+)\1\s*\)/gi

// Open-tracking pixels: tiny or hidden images whose only purpose is to tell the
// sender when (and from where) the mail was read. They are dropped, never loaded.
function isTrackingPixel(img: Element, w: number | null, h: number | null): boolean {
  const style = (img.getAttribute('style') || '').toLowerCase().replace(/\s+/g, '')
  if (/display:none|visibility:hidden|opacity:0(?:[;!]|$)|max-(?:width|height):[01]px/.test(style)) return true
  const tinyW = w !== null && w <= 3
  const tinyH = h !== null && h <= 3
  return (tinyW && (h === null || tinyH)) || (tinyH && w === null)
}

function rewriteSrcset(srcset: string, rewrite: (url: string) => string): string {
  return srcset
    .split(',')
    .map((part) => {
      const [url, ...rest] = part.trim().split(/\s+/)
      if (!url) return ''
      return [EXTERNAL_URL_RE.test(url) ? rewrite(url) : url, ...rest].join(' ')
    })
    .filter(Boolean)
    .join(', ')
}

function rewriteCssUrls(css: string, rewrite: ((url: string) => string) | null): string {
  return css.replace(EXTERNAL_CSS_URL_RE, (_m, _q, url: string) =>
    rewrite ? `url("${rewrite(url)}")` : 'none',
  )
}

export interface ExtractedCodeBlock {
  tokenId: string
  rawCode: string
  declaredLang: string | null
}

function extractLangClass(el: Element): string | null {
  const cls = el.getAttribute('class') || ''
  const match = cls.match(/\b(?:language|lang)-([a-zA-Z0-9_+#.-]{1,32})\b/)
  return match?.[1] ?? null
}

// Pixel size of an <img> from its width/height attributes or inline style.
function declaredSize(img: Element, dim: 'width' | 'height'): number | null {
  const attr = (img.getAttribute(dim) || '').trim()
  const style = (img.getAttribute('style') || '').match(new RegExp(`(?:^|;)\\s*${dim}\\s*:\\s*([0-9.]+)px`, 'i'))
  const raw = /^[0-9.]+$/.test(attr) ? attr : style?.[1]
  const n = raw ? Number.parseFloat(raw) : NaN
  return Number.isFinite(n) && n > 0 ? n : null
}

const BLOCKED_ICON =
  '<svg aria-hidden="true" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="9" cy="9" r="2"/><path d="m21 15-3.1-3.1a2 2 0 0 0-2.8 0L6 21"/><path d="m2 2 20 20"/></svg>'

export function processEmailHtml(
  rawHtml: string,
  emailId: number,
  proxyToken: string,
  allowExternal: boolean,
  blockedShortLabel: string,
  blockedNotice: string,
) {
  let hasExternalImages = false
  // External images are only ever loaded through the proxy; without a token
  // they stay blocked.
  const proxy =
    allowExternal && proxyToken
      ? (url: string) => proxiedImageUrl(emailId, proxyToken, url)
      : null
  const codeBlocks: ExtractedCodeBlock[] = []

  const sanitized = DOMPurify.sanitize(rawHtml, {
    USE_PROFILES: { html: true },
    FORBID_TAGS: ['script', 'iframe', 'object', 'embed', 'form', 'input', 'button', 'meta', 'base'],
    FORBID_ATTR: ['onerror', 'onload', 'onclick', 'onmouseover', 'onfocus', 'formaction'],
    ADD_ATTR: ['target', 'rel', 'class', 'style'],
  })

  const parser = new DOMParser()
  const doc = parser.parseFromString(sanitized, 'text/html')

  // Rewrite links to always open safely in a new tab
  doc.querySelectorAll('a[href]').forEach((a) => {
    a.setAttribute('target', '_blank')
    a.setAttribute('rel', 'noopener noreferrer nofollow')
  })

  // External <img>: tracking pixels are removed; other images are either
  // replaced by a placeholder or, once allowed, loaded through the proxy.
  doc.querySelectorAll('img').forEach((img) => {
    const src = (img.getAttribute('src') || '').trim()
    const srcset = (img.getAttribute('srcset') || '').trim()
    const isExtSrc = EXTERNAL_URL_RE.test(src)
    const isExtSrcset = /(?:^|,)\s*(?:https?:)?\/\//i.test(srcset)
    if (!isExtSrc && !isExtSrcset) return

    const w = declaredSize(img, 'width')
    const h = declaredSize(img, 'height')
    if (isTrackingPixel(img, w, h)) {
      img.remove()
      return
    }
    hasExternalImages = true
    if (proxy) {
      if (isExtSrc) img.setAttribute('src', proxy(src))
      if (srcset) img.setAttribute('srcset', rewriteSrcset(srcset, proxy))
      img.setAttribute('referrerpolicy', 'no-referrer')
      return
    }
    // Keep the image's box so small images (signature avatars, icons)
    // do not turn into a tall column of wrapped text.
    const placeholder = doc.createElement('span')
    placeholder.setAttribute('data-pmail-blocked', '1')
    const alt = (img.getAttribute('alt') || '').trim()
    const label = alt || blockedShortLabel
    placeholder.setAttribute('title', `${label} · ${blockedNotice}`)
    placeholder.setAttribute('role', 'img')
    placeholder.setAttribute('aria-label', label)
    const sizes: string[] = []
    if (w) sizes.push(`width:${Math.max(20, Math.round(w))}px`)
    if (h) sizes.push(`height:${Math.max(20, Math.round(h))}px`)
    if (sizes.length) placeholder.setAttribute('style', sizes.join(';'))
    placeholder.innerHTML = BLOCKED_ICON
    if (!w || w > 120) {
      const text = doc.createElement('span')
      text.setAttribute('data-pmail-blocked-text', '1')
      text.textContent = label
      placeholder.append(text)
    } else {
      placeholder.setAttribute('data-pmail-blocked-compact', '1')
    }
    img.replaceWith(placeholder)
  })

  // CSS backgrounds in style attributes and <style> blocks
  const cssHasExternal = (css: string) => {
    EXTERNAL_CSS_URL_RE.lastIndex = 0
    return EXTERNAL_CSS_URL_RE.test(css)
  }
  doc.querySelectorAll('[style]').forEach((el) => {
    const style = el.getAttribute('style') || ''
    if (!cssHasExternal(style)) return
    hasExternalImages = true
    el.setAttribute('style', rewriteCssUrls(style, proxy))
  })
  doc.querySelectorAll('style').forEach((el) => {
    const css = el.textContent || ''
    if (!cssHasExternal(css)) return
    hasExternalImages = true
    el.textContent = rewriteCssUrls(css, proxy)
  })
  // Legacy background="..." attribute on body/table/td
  doc.querySelectorAll('[background]').forEach((el) => {
    const bg = (el.getAttribute('background') || '').trim()
    if (!EXTERNAL_URL_RE.test(bg)) return
    hasExternalImages = true
    if (proxy) el.setAttribute('background', proxy(bg))
    else el.removeAttribute('background')
  })
  // Media sources are never fetched (the frame's CSP blocks them); drop
  // external ones so nothing tries.
  doc.querySelectorAll('video[poster], video[src], audio[src], source[src], track[src]').forEach((el) => {
    for (const attr of ['src', 'poster']) {
      if (EXTERNAL_URL_RE.test((el.getAttribute(attr) || '').trim())) el.removeAttribute(attr)
    }
  })

  // Strip external <link> tags
  doc.querySelectorAll('link').forEach((link) => link.remove())

  // Mark standalone <pre><code> blocks for lazy Shiki highlighting while preserving GitHub/GitLab diff tables intact
  let blockIdx = 0
  doc.querySelectorAll('pre').forEach((pre) => {
    if (pre.closest('table')) return // Never break GitHub/GitLab diff table layouts
    const codeEl = pre.querySelector('code')
    const rawCode = (codeEl?.textContent ?? pre.textContent ?? '').replace(/\r\n/g, '\n')
    if (!rawCode.trim()) return
    const declaredLang =
      (codeEl ? extractLangClass(codeEl) : null) ?? extractLangClass(pre)
    const tokenId = `pmail-code-${blockIdx++}`
    pre.setAttribute('data-pmail-code-id', tokenId)
    codeBlocks.push({ tokenId, rawCode, declaredLang })
  })

  return {
    bodyHtml: doc.body.innerHTML,
    hasExternalImages,
    codeBlocks,
  }
}
