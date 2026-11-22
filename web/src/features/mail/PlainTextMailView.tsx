import { Fragment, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import DOMPurify from 'dompurify'
import { Code2, FileText } from 'lucide-react'
import type { MailRiskFinding } from '@/api'
import { cn } from '@/lib/cn'
import { CodeBlockView } from './CodeBlockView'
import { DiffPatchView } from './DiffPatchView'
import { formatRiskCopyWarning, matchCodeBlockRiskFindings } from './inboundAnalysis'

interface PlainTextMailViewProps {
  text: string
  textFormat?: 'plain' | 'flowed' | 'markdown' | 'patch'
  hasCode?: boolean
  riskFindings?: MailRiskFinding[]
}

type TextSegment =
  | { kind: 'prose'; content: string }
  | { kind: 'code'; language: string; content: string }

const FENCE_RE = /```([a-zA-Z0-9_+#.-]*)\r?\n([\s\S]*?)```/g
const URL_RE = /(https?:\/\/[^\s<>"'`）)】\]]+)/g

function splitMarkdownFences(raw: string): TextSegment[] {
  const segments: TextSegment[] = []
  let lastIndex = 0
  FENCE_RE.lastIndex = 0
  let match: RegExpExecArray | null

  while ((match = FENCE_RE.exec(raw)) !== null) {
    if (match.index > lastIndex) {
      segments.push({ kind: 'prose', content: raw.slice(lastIndex, match.index) })
    }
    segments.push({
      kind: 'code',
      language: (match[1] ?? '').trim(),
      content: match[2] ?? '',
    })
    lastIndex = match.index + match[0].length
  }

  if (lastIndex < raw.length) {
    segments.push({ kind: 'prose', content: raw.slice(lastIndex) })
  }
  return segments
}

function linkifyLine(line: string) {
  const parts = line.split(URL_RE)
  return parts.map((part, idx) => {
    if (/^https?:\/\//.test(part)) {
      return (
        <a
          key={idx}
          href={part}
          target="_blank"
          rel="noopener noreferrer nofollow"
          className="text-[var(--accent)] underline underline-offset-2 hover:text-[var(--accent-hover)] break-all"
        >
          {part}
        </a>
      )
    }
    return <Fragment key={idx}>{part}</Fragment>
  })
}

function looksLikePatch(text: string): boolean {
  return (
    /^diff --git\s/m.test(text) ||
    (/^---\s/m.test(text) && /^\+\+\+\s/m.test(text) && /^@@\s-\d+/m.test(text))
  )
}

export function PlainTextMailView({
  text,
  textFormat = 'plain',
  hasCode = false,
  riskFindings = [],
}: PlainTextMailViewProps) {
  const { t } = useTranslation()
  const bodyRiskFindings = useMemo(
    () => riskFindings.filter((f) => f.location === 'body'),
    [riskFindings],
  )
  const isPatch = textFormat === 'patch' || looksLikePatch(text)
  const isMarkdown = textFormat === 'markdown'
  const [monospace, setMonospace] = useState(false)
  const [showRaw, setShowRaw] = useState(false)
  const [markdownHtmlParts, setMarkdownHtmlParts] = useState<Map<number, string>>(new Map())

  const segments = useMemo(() => splitMarkdownFences(text), [text])
  const hasFencedCode = useMemo(() => segments.some((s) => s.kind === 'code'), [segments])

  // Lazy-load `marked` ONLY when viewing a Markdown email!
  useEffect(() => {
    if (!isMarkdown || showRaw) return
    let cancelled = false
    void import('marked').then(({ marked }) => {
      if (cancelled) return
      const map = new Map<number, string>()
      segments.forEach((seg, idx) => {
        if (seg.kind === 'prose' && seg.content.trim()) {
          const rawHtml = marked.parse(seg.content, { async: false, gfm: true, breaks: true })
          const clean = DOMPurify.sanitize(String(rawHtml), {
            USE_PROFILES: { html: true },
            ADD_ATTR: ['target', 'rel'],
          })
          map.set(idx, clean)
        }
      })
      setMarkdownHtmlParts(map)
    })
    return () => {
      cancelled = true
    }
  }, [isMarkdown, showRaw, segments])

  const showToolbar = isPatch || isMarkdown || hasCode || hasFencedCode

  return (
    <div className="space-y-3">
      {showToolbar && (
        <div className="flex flex-wrap items-center justify-end gap-1.5">
          {(isPatch || isMarkdown) && (
            <div className="inline-flex rounded-[var(--radius-sm)] border border-[var(--border-subtle)] bg-[var(--surface-subtle)] p-0.5 text-[11.5px]">
              <button
                type="button"
                onClick={() => setShowRaw(false)}
                className={cn(
                  'inline-flex items-center gap-1 rounded-[4px] px-2 py-0.5 font-medium transition-colors',
                  !showRaw
                    ? 'bg-[var(--surface-canvas)] text-[var(--ink-primary)] shadow-[var(--shadow-xs)]'
                    : 'text-[var(--ink-muted)] hover:text-[var(--ink-primary)]',
                )}
              >
                <Code2 className="h-3 w-3" />
                {t('mail.code.viewRendered')}
              </button>
              <button
                type="button"
                onClick={() => setShowRaw(true)}
                className={cn(
                  'inline-flex items-center gap-1 rounded-[4px] px-2 py-0.5 font-medium transition-colors',
                  showRaw
                    ? 'bg-[var(--surface-canvas)] text-[var(--ink-primary)] shadow-[var(--shadow-xs)]'
                    : 'text-[var(--ink-muted)] hover:text-[var(--ink-primary)]',
                )}
              >
                <FileText className="h-3 w-3" />
                {t('mail.code.viewRaw')}
              </button>
            </div>
          )}

          {!isPatch && !isMarkdown && (
            <button
              type="button"
              onClick={() => setMonospace((v) => !v)}
              className={cn(
                'inline-flex items-center gap-1 rounded-[var(--radius-sm)] border border-[var(--border-subtle)] px-2 py-1 text-[11.5px] font-medium transition-colors',
                monospace
                  ? 'bg-[var(--accent-soft)] text-[var(--accent)] border-[var(--accent)]/30'
                  : 'bg-[var(--surface-subtle)] text-[var(--ink-secondary)] hover:text-[var(--ink-primary)]',
              )}
            >
              <Code2 className="h-3 w-3" />
              {monospace ? t('mail.code.monospaceOn') : t('mail.code.monospaceOff')}
            </button>
          )}
        </div>
      )}

      {/* Raw source fallback */}
      {showRaw ? (
        <pre
          className="mail-scroll overflow-x-auto whitespace-pre font-mono text-[13px] leading-relaxed text-[var(--ink-primary)] rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)]/50 p-4"
          style={{ tabSize: 4 }}
        >
          {text}
        </pre>
      ) : isPatch ? (
        <DiffPatchView patchText={text} />
      ) : (
        <div className="space-y-2">
          {segments.map((seg, idx) => {
            if (seg.kind === 'code') {
              const matchedFindings = matchCodeBlockRiskFindings(seg.content, bodyRiskFindings)
              return (
                <CodeBlockView
                  key={idx}
                  code={seg.content}
                  language={seg.language || undefined}
                  riskWarning={
                    matchedFindings.length > 0
                      ? formatRiskCopyWarning(matchedFindings, t)
                      : null
                  }
                />
              )
            }

            if (!seg.content.trim()) return null

            if (isMarkdown && markdownHtmlParts.has(idx)) {
              return (
                <div
                  key={idx}
                  className="prose-mail space-y-2 text-[14.5px] leading-[1.7] text-[var(--ink-primary)] [&_a]:text-[var(--accent)] [&_a]:underline [&_blockquote]:border-l-3 [&_blockquote]:border-[var(--border-strong)] [&_blockquote]:pl-3 [&_blockquote]:text-[var(--ink-secondary)] [&_code]:rounded [&_code]:bg-[var(--surface-subtle)] [&_code]:px-1.5 [&_code]:py-0.5 [&_code]:font-mono [&_code]:text-[0.88em] [&_h1]:text-[1.35em] [&_h1]:font-semibold [&_h2]:text-[1.18em] [&_h2]:font-semibold [&_h3]:text-[1.06em] [&_h3]:font-semibold [&_ol]:list-decimal [&_ol]:pl-5 [&_ul]:list-disc [&_ul]:pl-5"
                  dangerouslySetInnerHTML={{ __html: markdownHtmlParts.get(idx)! }}
                />
              )
            }

            return (
              <div
                key={idx}
                className={cn(
                  'whitespace-pre-wrap break-words text-[14.5px] leading-[1.7] text-[var(--ink-primary)]',
                  monospace ? 'font-mono text-[13px]' : 'font-sans',
                )}
                style={{ tabSize: 4 }}
              >
                {seg.content.split('\n').map((line, lIdx, arr) => (
                  <Fragment key={lIdx}>
                    {linkifyLine(line)}
                    {lIdx < arr.length - 1 ? '\n' : null}
                  </Fragment>
                ))}
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
