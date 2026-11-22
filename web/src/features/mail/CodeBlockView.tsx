import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Check, Copy, Hash, ShieldAlert, WrapText } from 'lucide-react'
import type { ThemedToken } from 'shiki/core'
import { useTheme } from '@/lib/theme'
import type { SupportedCodeLang } from '@/lib/code-languages'
import { cn } from '@/lib/cn'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

interface CodeBlockViewProps {
  code: string
  language?: string | null
  defaultShowLineNumbers?: boolean
  defaultWrap?: boolean
  maxHeightClass?: string
  className?: string
  riskWarning?: string | null
}

export function CodeBlockView({
  code,
  language,
  defaultShowLineNumbers = true,
  defaultWrap = false,
  maxHeightClass,
  className,
  riskWarning = null,
}: CodeBlockViewProps) {
  const { t } = useTranslation()
  const { resolved } = useTheme()

  const normalizedCode = useMemo(() => code.replace(/\r\n/g, '\n').replace(/\n$/, ''), [code])

  const [resolvedLang, setResolvedLang] = useState<SupportedCodeLang>('text')
  const [tokens, setTokens] = useState<ThemedToken[][] | null>(null)
  const [wrap, setWrap] = useState(defaultWrap)
  const [showLines, setShowLines] = useState(defaultShowLineNumbers)
  const [copied, setCopied] = useState(false)
  const [copyConfirmOpen, setCopyConfirmOpen] = useState(false)

  useEffect(() => {
    let cancelled = false
    void import('@/lib/shiki').then(
      async ({ detectCodeLanguage, highlightCodeTokens, normalizeCodeLanguage }) => {
        const lang = normalizeCodeLanguage(language) ?? detectCodeLanguage(normalizedCode)
        if (!cancelled) setResolvedLang(lang)
        const res = await highlightCodeTokens(normalizedCode, lang, resolved)
        if (!cancelled) setTokens(res)
      },
    )
    return () => {
      cancelled = true
    }
  }, [normalizedCode, language, resolved])

  const rawLines = useMemo(() => normalizedCode.split('\n'), [normalizedCode])
  const lineCount = rawLines.length

  const performCopy = async () => {
    try {
      await navigator.clipboard.writeText(normalizedCode)
      setCopied(true)
      toast.success(t('mail.code.copied'))
      window.setTimeout(() => setCopied(false), 1600)
    } catch {
      toast.error(t('common.copyFailed'))
    }
  }

  const handleCopyClick = () => {
    if (riskWarning) {
      setCopyConfirmOpen(true)
      return
    }
    void performCopy()
  }

  const langLabel = t(`mail.code.languages.${resolvedLang}` as const)

  return (
    <div
      className={cn(
        'my-3 overflow-hidden rounded-[var(--radius-md)] border bg-[var(--surface-subtle)]/75',
        riskWarning ? 'border-[var(--danger)]/45' : 'border-[var(--border-subtle)]',
        className,
      )}
    >
      {/* Code Block Header */}
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-[var(--border-hairline)] bg-[var(--surface-sidebar)] px-3 py-1.5 text-[11.5px]">
        <div className="flex items-center gap-2">
          <span className="font-mono font-semibold text-[var(--ink-primary)]">{langLabel}</span>
          <span className="tabular-nums text-[var(--ink-muted)]">
            {t('mail.code.linesCount', { count: lineCount })}
          </span>
          {riskWarning && (
            <Badge variant="danger">
              <ShieldAlert className="h-3 w-3" />
              {t('mail.risk.flaggedCodeBadge')}
            </Badge>
          )}
        </div>

        <div className="flex items-center gap-1">
          <button
            type="button"
            onClick={() => setShowLines((v) => !v)}
            aria-label={t('mail.code.lineNumbers')}
            title={t('mail.code.lineNumbers')}
            className={cn(
              'inline-flex items-center gap-1 rounded px-1.5 py-0.5 transition-colors',
              showLines
                ? 'bg-[var(--accent-soft)] text-[var(--accent)]'
                : 'text-[var(--ink-muted)] hover:bg-[var(--surface-hover)] hover:text-[var(--ink-primary)]',
            )}
          >
            <Hash className="h-3 w-3" />
            <span className="hidden sm:inline">{t('mail.code.lineNumbers')}</span>
          </button>

          <button
            type="button"
            onClick={() => setWrap((v) => !v)}
            aria-label={wrap ? t('mail.code.wrapOn') : t('mail.code.wrapOff')}
            title={wrap ? t('mail.code.wrapOn') : t('mail.code.wrapOff')}
            className={cn(
              'inline-flex items-center gap-1 rounded px-1.5 py-0.5 transition-colors',
              wrap
                ? 'bg-[var(--accent-soft)] text-[var(--accent)]'
                : 'text-[var(--ink-muted)] hover:bg-[var(--surface-hover)] hover:text-[var(--ink-primary)]',
            )}
          >
            <WrapText className="h-3 w-3" />
            <span className="hidden sm:inline">
              {wrap ? t('mail.code.wrapOn') : t('mail.code.wrapOff')}
            </span>
          </button>

          <button
            type="button"
            onClick={handleCopyClick}
            aria-label={t('mail.code.copy')}
            title={t('mail.code.copy')}
            className="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[var(--ink-secondary)] transition-colors hover:bg-[var(--surface-hover)] hover:text-[var(--ink-primary)]"
          >
            {copied ? (
              <Check className="h-3 w-3 text-[var(--success)]" />
            ) : (
              <Copy className="h-3 w-3" />
            )}
            <span>{copied ? t('mail.code.copied') : t('mail.code.copy')}</span>
          </button>
        </div>
      </div>

      {/* Risky code copy confirmation dialog */}
      {riskWarning && (
        <Dialog open={copyConfirmOpen} onOpenChange={setCopyConfirmOpen}>
          <DialogContent className="max-w-md">
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2 text-[var(--danger)]">
                <ShieldAlert className="h-4.5 w-4.5 shrink-0" />
                <span>{t('mail.risk.copyConfirmTitle')}</span>
              </DialogTitle>
              <DialogDescription className="pt-1 leading-relaxed">
                {riskWarning}
              </DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => setCopyConfirmOpen(false)}
              >
                {t('common.cancel')}
              </Button>
              <Button
                type="button"
                variant="danger"
                size="sm"
                onClick={() => {
                  setCopyConfirmOpen(false)
                  void performCopy()
                }}
              >
                <Copy className="h-3.5 w-3.5" />
                {t('mail.risk.copyConfirmAction')}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      )}

      {/* Highlighted Code Body */}
      <div
        className={cn(
          'mail-scroll overflow-x-auto bg-[var(--surface-canvas)] p-3 font-mono text-[12.5px] leading-[1.6]',
          maxHeightClass,
        )}
        style={{ tabSize: 4 }}
      >
        <table className="w-full border-collapse">
          <tbody>
            {rawLines.map((rawLine, idx) => {
              const lineTokens = tokens?.[idx]
              return (
                <tr key={idx} className="hover:bg-[var(--surface-hover)]/45">
                  {showLines && (
                    <td
                      aria-hidden="true"
                      className="w-10 select-none pr-3 text-right align-top tabular-nums text-[11.5px] text-[var(--ink-subtle)]"
                    >
                      {idx + 1}
                    </td>
                  )}
                  <td
                    className={cn(
                      'align-top',
                      wrap ? 'whitespace-pre-wrap break-all' : 'whitespace-pre',
                    )}
                  >
                    {lineTokens && lineTokens.length > 0 ? (
                      lineTokens.map((tok, tIdx) => (
                        <span key={tIdx} style={{ color: tok.color }}>
                          {tok.content}
                        </span>
                      ))
                    ) : (
                      <span className="text-[var(--ink-primary)]">{rawLine || ' '}</span>
                    )}
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </div>
  )
}
