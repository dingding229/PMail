import { useTranslation } from 'react-i18next'
import { AlertCircle, Eye, EyeOff, Languages, RefreshCw, X } from 'lucide-react'
import type { EmailTranslationResult } from '@/api'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'

export interface MailTranslationCardProps {
  result: EmailTranslationResult | null
  loading: boolean
  error: string | null
  showTranslation: boolean
  onToggleShow: () => void
  onRetranslate: () => void
  onClose: () => void
}

export function MailTranslationCard({
  result,
  loading,
  error,
  showTranslation,
  onToggleShow,
  onRetranslate,
  onClose,
}: MailTranslationCardProps) {
  const { t } = useTranslation()

  if (!loading && !error && !result) return null

  const providerLabel = result
    ? result.engine === 'ai' || result.provider === 'openai_compatible'
      ? t('translation.providerLabelAI', {
          model: result.model || t('translation.aiModelFallback'),
        })
      : t('translation.providerLabelWeb', {
          provider: t(`translation.providers.${result.provider}`),
        })
    : ''

  return (
    <section
      aria-label={t('translation.cardTitle')}
      className="mb-4 rounded-[var(--radius-md)] border border-[var(--accent)]/25 bg-[var(--accent-soft)]/35 p-4 space-y-3"
    >
      {/* Header */}
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex flex-wrap items-center gap-2">
          <span className="inline-flex items-center gap-1.5 text-[13px] font-semibold text-[var(--accent-ink)]">
            <Languages className="h-4 w-4 shrink-0" />
            <span>{t('translation.cardTitle')}</span>
          </span>
          {result && (
            <>
              <Badge variant="accent">
                {t(`translation.languages.${result.target_lang}`, {
                  defaultValue: result.target_lang,
                })}
              </Badge>
              <span className="text-[11.5px] text-[var(--ink-secondary)]">
                {providerLabel}
              </span>
              {result.cached && (
                <Badge variant="default">{t('translation.cachedBadge')}</Badge>
              )}
              {result.truncated && (
                <Badge variant="warning">{t('translation.truncatedBadge')}</Badge>
              )}
            </>
          )}
        </div>

        <div className="flex items-center gap-1">
          {result && (
            <Button type="button" variant="ghost" size="xs" onClick={onToggleShow}>
              {showTranslation ? (
                <>
                  <EyeOff className="h-3.5 w-3.5" />
                  {t('translation.showOriginal')}
                </>
              ) : (
                <>
                  <Eye className="h-3.5 w-3.5" />
                  {t('translation.showTranslation')}
                </>
              )}
            </Button>
          )}
          <Button
            type="button"
            variant="ghost"
            size="xs"
            disabled={loading}
            onClick={onRetranslate}
          >
            <RefreshCw className={`h-3.5 w-3.5 ${loading ? 'animate-spin' : ''}`} />
            {t('translation.retranslate')}
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="iconSm"
            aria-label={t('common.close')}
            onClick={onClose}
          >
            <X className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>

      {/* Loading */}
      {loading && (
        <div className="space-y-2 pt-1" aria-busy="true">
          <Skeleton className="h-4 w-2/5 rounded" />
          <Skeleton className="h-4 w-full rounded" />
          <Skeleton className="h-4 w-5/6 rounded" />
        </div>
      )}

      {/* Error */}
      {!loading && error && (
        <div
          role="alert"
          className="flex items-start gap-2 rounded-[var(--radius-sm)] border border-[var(--danger)]/30 bg-[var(--danger-soft)] px-3 py-2 text-[12.5px] text-[var(--danger)]"
        >
          <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
          <span>{error}</span>
        </div>
      )}

      {/* Translation output: plain text only here (never dangerouslySetInnerHTML). HTML layouts render in the sandboxed body frame. */}
      {/* HTML layouts render in the sandboxed body frame below, so the card is
          only a header there. Plain text is shown here (never as HTML). */}
      {!loading && result && showTranslation && !result.html && (
        <div className="space-y-2.5 rounded-[var(--radius-sm)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] p-3.5">
          {result.subject && (
            <p className="border-b border-[var(--border-hairline)] pb-2 text-[14px] font-semibold leading-snug text-[var(--ink-primary)] whitespace-pre-wrap break-words">
              {result.subject}
            </p>
          )}
          <div className="text-[13.5px] leading-relaxed text-[var(--ink-primary)] whitespace-pre-wrap break-words">
            {result.text}
          </div>
        </div>
      )}
    </section>
  )
}
