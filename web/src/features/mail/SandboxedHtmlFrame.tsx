import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useTheme } from '@/lib/theme'
import { processEmailHtml } from './emailHtml'

interface SandboxedHtmlFrameProps {
  html: string
  /** Mail id and token for loading external images through the server's image proxy. */
  emailId: number
  imageProxyToken?: string
  allowExternalImages: boolean
  onDetectExternalImages?: (hasExternal: boolean) => void
}

function escapeHtmlAttr(str: string): string {
  return str
    .replace(/&/g, '&amp;')
    .replace(/"/g, '&quot;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
}

export function SandboxedHtmlFrame({
  html,
  emailId,
  imageProxyToken = '',
  allowExternalImages,
  onDetectExternalImages,
}: SandboxedHtmlFrameProps) {
  const { t } = useTranslation()
  const { resolved } = useTheme()
  const iframeRef = useRef<HTMLIFrameElement | null>(null)
  const [height, setHeight] = useState(240)
  const [highlightedMap, setHighlightedMap] = useState<
    Map<string, { html: string; langLabel: string; rawCode: string }>
  >(new Map())

  const blockedLabel = t('mail.security.blockedImageShort')
  const blockedNotice = t('mail.security.externalImagesBlocked')
  const { bodyHtml, hasExternalImages, codeBlocks } = useMemo(
    () =>
      processEmailHtml(
        html,
        emailId,
        imageProxyToken,
        allowExternalImages,
        blockedLabel,
        blockedNotice,
      ),
    [html, emailId, imageProxyToken, allowExternalImages, blockedLabel, blockedNotice],
  )

  useEffect(() => {
    onDetectExternalImages?.(hasExternalImages)
  }, [hasExternalImages, onDetectExternalImages])

  // Lazily load Shiki during browser idle time and yield between blocks so long HTML emails never block frame paint
  useEffect(() => {
    if (codeBlocks.length === 0) {
      setHighlightedMap(new Map())
      return
    }
    let cancelled = false
    let idleId: number | null = null
    let timeoutId: number | null = null

    const runHighlight = () => {
      void import('@/lib/shiki').then(
        async ({ detectCodeLanguage, highlightCodeToHtml, normalizeCodeLanguage }) => {
          const next = new Map<string, { html: string; langLabel: string; rawCode: string }>()
          for (let i = 0; i < codeBlocks.length; i++) {
            const block = codeBlocks[i]!
            if (cancelled) return
            const lang =
              normalizeCodeLanguage(block.declaredLang) ?? detectCodeLanguage(block.rawCode)
            const shikiHtml = await highlightCodeToHtml(
              block.rawCode.replace(/\n$/, ''),
              lang,
              resolved,
            )
            if (cancelled) return
            next.set(block.tokenId, {
              html: shikiHtml,
              langLabel: t(`mail.code.languages.${lang}` as const),
              rawCode: block.rawCode,
            })
            // Yield back to the event loop between blocks on multi-block emails
            if (i < codeBlocks.length - 1) {
              await new Promise((r) => window.setTimeout(r, 0))
            }
          }
          if (!cancelled) setHighlightedMap(next)
        },
      )
    }

    const ric =
      typeof window !== 'undefined' ? window.requestIdleCallback?.bind(window) : undefined
    const cic =
      typeof window !== 'undefined' ? window.cancelIdleCallback?.bind(window) : undefined

    if (ric) {
      idleId = ric(runHighlight, { timeout: 120 })
    } else {
      timeoutId = globalThis.setTimeout(runHighlight, 0) as unknown as number
    }

    return () => {
      cancelled = true
      if (idleId !== null && cic) {
        cic(idleId)
      }
      if (timeoutId !== null) globalThis.clearTimeout(timeoutId)
    }
  }, [codeBlocks, resolved, t])

  const renderedBodyHtml = useMemo(() => {
    if (highlightedMap.size === 0) return bodyHtml
    const parser = new DOMParser()
    const doc = parser.parseFromString(bodyHtml, 'text/html')

    const copyLabel = escapeHtmlAttr(t('mail.code.copy'))
    const wrapLabel = escapeHtmlAttr(t('mail.code.wrapOff'))

    doc.querySelectorAll('pre[data-pmail-code-id]').forEach((pre) => {
      const id = pre.getAttribute('data-pmail-code-id')
      if (!id) return
      const item = highlightedMap.get(id)
      if (!item) return

      const wrapper = doc.createElement('div')
      wrapper.setAttribute('class', 'pmail-code-card')
      wrapper.setAttribute('data-code-id', id)
      wrapper.innerHTML = `
        <div class="pmail-code-bar">
          <span class="pmail-code-lang">${escapeHtmlAttr(item.langLabel)}</span>
          <div class="pmail-code-actions">
            <button type="button" class="pmail-code-btn pmail-wrap-btn" data-code-id="${id}">${wrapLabel}</button>
            <button type="button" class="pmail-code-btn pmail-copy-btn" data-code-id="${id}">${copyLabel}</button>
          </div>
        </div>
        <div class="pmail-code-body">${item.html}</div>
      `
      pre.replaceWith(wrapper)
    })

    return doc.body.innerHTML
  }, [bodyHtml, highlightedMap, t])

  const srcDoc = useMemo(() => {
    const isDark = resolved === 'dark'
    // Images may come only from the mail itself or PMail ('self': inline
    // attachments and the image proxy). Nothing in a mail can reach another
    // server directly, so no request can reveal the reader's IP.
    const csp =
      "default-src 'none'; style-src 'unsafe-inline'; img-src data: cid: blob: 'self'; font-src data:;"

    const baseCss = `
      :root { color-scheme: ${isDark ? 'dark' : 'light'}; }
      html, body {
        margin: 0;
        padding: 0;
        background: transparent;
        color: ${isDark ? '#e2e8f0' : '#1e293b'};
        font-family: -apple-system, BlinkMacSystemFont, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", "Segoe UI", Roboto, sans-serif;
        font-size: 14.5px;
        line-height: 1.7;
        word-wrap: break-word;
        overflow-wrap: anywhere;
        overflow: hidden;
      }
      a { color: ${isDark ? '#60a5fa' : '#2563eb'}; text-decoration: underline; text-underline-offset: 2px; }
      img { max-width: 100%; height: auto; border-radius: 6px; }
      [data-pmail-blocked="1"] {
        display: inline-flex;
        align-items: center;
        justify-content: center;
        gap: 6px;
        box-sizing: border-box;
        max-width: 100%;
        min-width: 20px;
        min-height: 20px;
        vertical-align: middle;
        overflow: hidden;
        white-space: nowrap;
        line-height: 1.2;
        padding: 4px 8px;
        border: 1px dashed ${isDark ? 'rgba(248,250,252,0.22)' : 'rgba(15,23,42,0.18)'};
        background: ${isDark ? 'rgba(255,255,255,0.04)' : 'rgba(15,23,42,0.03)'};
        border-radius: 8px;
        font-size: 12.5px;
        color: ${isDark ? '#94a3b8' : '#64748b'};
      }
      [data-pmail-blocked-compact="1"] { padding: 0; }
      [data-pmail-blocked="1"] svg { flex: none; opacity: 0.75; }
      [data-pmail-blocked-text="1"] { overflow: hidden; text-overflow: ellipsis; min-width: 0; }
      table { max-width: 100%; border-collapse: collapse; }
      td code, th code { tab-size: 4; }
      blockquote {
        margin: 10px 0;
        padding: 4px 0 4px 12px;
        border-left: 3px solid ${isDark ? 'rgba(248,250,252,0.18)' : 'rgba(15,23,42,0.16)'};
        color: ${isDark ? '#94a3b8' : '#475569'};
      }
      code {
        font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
        font-size: 0.88em;
        background: ${isDark ? 'rgba(255,255,255,0.08)' : 'rgba(15,23,42,0.055)'};
        padding: 1.5px 5px;
        border-radius: 4px;
      }
      pre {
        background: ${isDark ? '#0d1117' : '#f6f8fa'};
        border: 1px solid ${isDark ? 'rgba(248,250,252,0.1)' : 'rgba(15,23,42,0.09)'};
        border-radius: 8px;
        padding: 12px 14px;
        overflow-x: auto;
        font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
        font-size: 13px;
        line-height: 1.6;
        tab-size: 4;
      }
      pre code { background: transparent; padding: 0; font-size: inherit; }

      /* Shiki enhanced code block card */
      .pmail-code-card {
        margin: 14px 0;
        border-radius: 10px;
        border: 1px solid ${isDark ? 'rgba(248,250,252,0.12)' : 'rgba(15,23,42,0.1)'};
        overflow: hidden;
        background: ${isDark ? '#0d1117' : '#ffffff'};
      }
      .pmail-code-bar {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 8px;
        padding: 6px 12px;
        background: ${isDark ? '#161b24' : '#f6f7f9'};
        border-bottom: 1px solid ${isDark ? 'rgba(248,250,252,0.08)' : 'rgba(15,23,42,0.08)'};
        font-size: 11.5px;
      }
      .pmail-code-lang {
        font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
        font-weight: 600;
        color: ${isDark ? '#e2e8f0' : '#0f172a'};
      }
      .pmail-code-actions {
        display: flex;
        align-items: center;
        gap: 6px;
      }
      .pmail-code-btn {
        appearance: none;
        border: 1px solid ${isDark ? 'rgba(248,250,252,0.12)' : 'rgba(15,23,42,0.12)'};
        background: ${isDark ? 'rgba(255,255,255,0.04)' : '#ffffff'};
        color: ${isDark ? '#cbd5e1' : '#334155'};
        border-radius: 5px;
        padding: 2px 8px;
        font-size: 11px;
        font-family: inherit;
        cursor: pointer;
        transition: background 120ms ease;
      }
      .pmail-code-btn:hover {
        background: ${isDark ? 'rgba(255,255,255,0.1)' : '#eff2f6'};
      }
      .pmail-code-btn[data-active="1"] {
        border-color: ${isDark ? '#3b82f6' : '#2563eb'};
        color: ${isDark ? '#60a5fa' : '#2563eb'};
      }
      .pmail-code-body pre.shiki {
        margin: 0;
        border: 0;
        border-radius: 0;
        padding: 12px 14px;
        overflow-x: auto;
        font-size: 12.5px;
        line-height: 1.6;
        tab-size: 4;
      }
      .pmail-code-card[data-wrap="1"] .pmail-code-body pre.shiki,
      .pmail-code-card[data-wrap="1"] .pmail-code-body code {
        white-space: pre-wrap !important;
        word-break: break-word !important;
      }
      hr { border: none; border-top: 1px solid ${isDark ? 'rgba(248,250,252,0.1)' : 'rgba(15,23,42,0.09)'}; margin: 16px 0; }
    `

    return `<!doctype html><html><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="${csp}"><meta name="referrer" content="no-referrer"><style>${baseCss}</style></head><body>${renderedBodyHtml}</body></html>`
  }, [renderedBodyHtml, resolved])

  const measureHeight = () => {
    const iframe = iframeRef.current
    if (!iframe) return
    try {
      const doc = iframe.contentDocument
      if (!doc) return
      const bodyH = doc.body?.scrollHeight ?? 0
      const docH = doc.documentElement?.scrollHeight ?? 0
      const next = Math.max(120, bodyH, docH) + 8
      setHeight(next)
    } catch {
      // Ignore if restricted
    }
  }

  useEffect(() => {
    const iframe = iframeRef.current
    if (!iframe) return
    let ro: ResizeObserver | null = null

    const attachHandlers = () => {
      measureHeight()
      try {
        const doc = iframe.contentDocument
        if (!doc) return

        // Attach Copy & Word-Wrap handlers from parent (iframe itself has NO allow-scripts!)
        doc.querySelectorAll<HTMLButtonElement>('.pmail-copy-btn').forEach((btn) => {
          btn.onclick = async () => {
            const id = btn.getAttribute('data-code-id')
            const item = id ? highlightedMap.get(id) : undefined
            if (!item) return
            try {
              await navigator.clipboard.writeText(item.rawCode)
              btn.textContent = t('mail.code.copied')
              toast.success(t('mail.code.copied'))
              window.setTimeout(() => {
                btn.textContent = t('mail.code.copy')
              }, 1500)
            } catch {
              toast.error(t('common.copyFailed'))
            }
          }
        })

        doc.querySelectorAll<HTMLButtonElement>('.pmail-wrap-btn').forEach((btn) => {
          btn.onclick = () => {
            const id = btn.getAttribute('data-code-id')
            const card = id ? doc.querySelector(`.pmail-code-card[data-code-id="${id}"]`) : null
            if (!card) return
            const isWrapped = card.getAttribute('data-wrap') === '1'
            card.setAttribute('data-wrap', isWrapped ? '0' : '1')
            btn.setAttribute('data-active', isWrapped ? '0' : '1')
            btn.textContent = isWrapped ? t('mail.code.wrapOff') : t('mail.code.wrapOn')
            measureHeight()
          }
        })

        if (doc.body && typeof ResizeObserver !== 'undefined') {
          let rafId = 0
          ro = new ResizeObserver(() => {
            if (rafId) window.cancelAnimationFrame(rafId)
            rafId = window.requestAnimationFrame(() => {
              rafId = 0
              measureHeight()
            })
          })
          ro.observe(doc.body)
        }
      } catch {
        // Fallback already measured
      }
    }

    iframe.addEventListener('load', attachHandlers)
    return () => {
      iframe.removeEventListener('load', attachHandlers)
      ro?.disconnect()
    }
  }, [srcDoc, highlightedMap, t])

  return (
    <iframe
      ref={iframeRef}
      title={t('mail.code.iframeTitle')}
      sandbox="allow-same-origin allow-popups allow-popups-to-escape-sandbox"
      srcDoc={srcDoc}
      style={{ height: `${height}px`, contain: 'layout style' }}
      className="block w-full border-0 bg-transparent transition-[height] duration-150"
    />
  )
}
