import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  AlertCircle,
  Calendar,
  CheckSquare,
  Coins,
  Copy,
  ExternalLink,
  Hash,
  Link2,
  RefreshCw,
  Sparkles,
  User,
  X,
} from 'lucide-react'
import type { EmailAIExtractResult, EmailAISummaryResult } from '@/api'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'

export interface MailAIPanelProps {
  summary: EmailAISummaryResult | null
  summaryLoading: boolean
  summaryError: string | null
  extract: EmailAIExtractResult | null
  extractLoading: boolean
  extractError: string | null
  onRefreshSummary: () => void
  onRefreshExtract: () => void
  onCloseSummary: () => void
  onCloseExtract: () => void
}

function isSafeHttpUrl(url: string): boolean {
  try {
    const parsed = new URL(url)
    return parsed.protocol === 'http:' || parsed.protocol === 'https:'
  } catch {
    return false
  }
}

export function MailAIPanel({
  summary,
  summaryLoading,
  summaryError,
  extract,
  extractLoading,
  extractError,
  onRefreshSummary,
  onRefreshExtract,
  onCloseSummary,
  onCloseExtract,
}: MailAIPanelProps) {
  const { t } = useTranslation()

  const showSummary = summaryLoading || Boolean(summaryError) || Boolean(summary)
  const showExtract = extractLoading || Boolean(extractError) || Boolean(extract)

  if (!showSummary && !showExtract) return null

  const copyValue = (text: string) => {
    void navigator.clipboard
      .writeText(text)
      .then(() => toast.success(t('common.copied')))
      .catch(() => toast.error(t('common.copyFailed')))
  }

  const hasExtractContent = extract
    ? extract.action_items.length > 0 ||
      extract.dates.length > 0 ||
      extract.amounts.length > 0 ||
      extract.links.length > 0 ||
      extract.codes.length > 0 ||
      extract.contacts.length > 0
    : false

  return (
    <div className="mb-4 space-y-3">
      {/* AI Summary Card */}
      {showSummary && (
        <section
          aria-label={t('ai.summaryTitle')}
          className="rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-subtle)]/70 p-4 space-y-3"
        >
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div className="flex flex-wrap items-center gap-2">
              <span className="inline-flex items-center gap-1.5 text-[13px] font-semibold text-[var(--ink-primary)]">
                <Sparkles className="h-4 w-4 text-[var(--accent)]" />
                <span>{t('ai.summaryTitle')}</span>
              </span>
              {summary && (
                <>
                  <Badge variant="default">{summary.model}</Badge>
                  {summary.cached && (
                    <Badge variant="default">{t('translation.cachedBadge')}</Badge>
                  )}
                  {summary.truncated && (
                    <Badge variant="warning">{t('translation.truncatedBadge')}</Badge>
                  )}
                </>
              )}
              <span className="text-[11px] text-[var(--ink-muted)]">{t('ai.disclaimer')}</span>
            </div>

            <div className="flex items-center gap-1">
              <Button
                type="button"
                variant="ghost"
                size="xs"
                disabled={summaryLoading}
                onClick={onRefreshSummary}
              >
                <RefreshCw className={`h-3.5 w-3.5 ${summaryLoading ? 'animate-spin' : ''}`} />
                {t('ai.regenerate')}
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="iconSm"
                aria-label={t('common.close')}
                onClick={onCloseSummary}
              >
                <X className="h-3.5 w-3.5" />
              </Button>
            </div>
          </div>

          {summaryLoading && (
            <div className="space-y-2" aria-busy="true">
              <Skeleton className="h-4 w-full rounded" />
              <Skeleton className="h-4 w-11/12 rounded" />
              <Skeleton className="h-4 w-3/4 rounded" />
            </div>
          )}

          {!summaryLoading && summaryError && (
            <div
              role="alert"
              className="flex items-start gap-2 rounded-[var(--radius-sm)] border border-[var(--danger)]/30 bg-[var(--danger-soft)] px-3 py-2 text-[12.5px] text-[var(--danger)]"
            >
              <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
              <span>{summaryError}</span>
            </div>
          )}

          {/* Strictly plain-text rendering */}
          {!summaryLoading && summary && (
            <div className="space-y-2.5 rounded-[var(--radius-sm)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] p-3.5">
              <p className="text-[13.5px] leading-relaxed text-[var(--ink-primary)] whitespace-pre-wrap break-words">
                {summary.summary}
              </p>
              {summary.key_points.length > 0 && (
                <ul className="list-disc space-y-1 pl-5 text-[13px] leading-relaxed text-[var(--ink-secondary)]">
                  {summary.key_points.map((pt, idx) => (
                    <li key={idx} className="whitespace-pre-wrap break-words">
                      {pt}
                    </li>
                  ))}
                </ul>
              )}
            </div>
          )}
        </section>
      )}

      {/* AI Extract Card */}
      {showExtract && (
        <section
          aria-label={t('ai.extractTitle')}
          className="rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-subtle)]/70 p-4 space-y-3"
        >
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div className="flex flex-wrap items-center gap-2">
              <span className="inline-flex items-center gap-1.5 text-[13px] font-semibold text-[var(--ink-primary)]">
                <Sparkles className="h-4 w-4 text-[var(--accent)]" />
                <span>{t('ai.extractTitle')}</span>
              </span>
              {extract && (
                <>
                  <Badge variant="default">{extract.model}</Badge>
                  {extract.cached && (
                    <Badge variant="default">{t('translation.cachedBadge')}</Badge>
                  )}
                  {extract.truncated && (
                    <Badge variant="warning">{t('translation.truncatedBadge')}</Badge>
                  )}
                </>
              )}
              <span className="text-[11px] text-[var(--ink-muted)]">{t('ai.disclaimer')}</span>
            </div>

            <div className="flex items-center gap-1">
              <Button
                type="button"
                variant="ghost"
                size="xs"
                disabled={extractLoading}
                onClick={onRefreshExtract}
              >
                <RefreshCw className={`h-3.5 w-3.5 ${extractLoading ? 'animate-spin' : ''}`} />
                {t('ai.regenerate')}
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="iconSm"
                aria-label={t('common.close')}
                onClick={onCloseExtract}
              >
                <X className="h-3.5 w-3.5" />
              </Button>
            </div>
          </div>

          {extractLoading && (
            <div className="grid grid-cols-1 gap-2.5 sm:grid-cols-2" aria-busy="true">
              <Skeleton className="h-20 w-full rounded-[var(--radius-sm)]" />
              <Skeleton className="h-20 w-full rounded-[var(--radius-sm)]" />
            </div>
          )}

          {!extractLoading && extractError && (
            <div
              role="alert"
              className="flex items-start gap-2 rounded-[var(--radius-sm)] border border-[var(--danger)]/30 bg-[var(--danger-soft)] px-3 py-2 text-[12.5px] text-[var(--danger)]"
            >
              <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
              <span>{extractError}</span>
            </div>
          )}

          {!extractLoading && extract && !hasExtractContent && (
            <p className="rounded-[var(--radius-sm)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] px-3.5 py-3 text-[12.5px] text-[var(--ink-muted)]">
              {t('ai.extractEmpty')}
            </p>
          )}

          {!extractLoading && extract && hasExtractContent && (
            <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
              {/* Action items */}
              {extract.action_items.length > 0 && (
                <ExtractGroupCard
                  icon={<CheckSquare className="h-3.5 w-3.5 text-[var(--accent)]" />}
                  title={t('ai.groups.actionItems')}
                >
                  {extract.action_items.map((item, idx) => (
                    <ExtractRow
                      key={idx}
                      copyLabel={t('common.copy')}
                      onCopy={() =>
                        copyValue(item.due ? `${item.text} (${item.due})` : item.text)
                      }
                    >
                      <p className="text-[12.5px] text-[var(--ink-primary)] whitespace-pre-wrap break-words">
                        {item.text}
                      </p>
                      {item.due && (
                        <p className="mt-0.5 text-[11px] text-[var(--warning)]">
                          {t('ai.dueLabel')}：{item.due}
                        </p>
                      )}
                    </ExtractRow>
                  ))}
                </ExtractGroupCard>
              )}

              {/* Dates */}
              {extract.dates.length > 0 && (
                <ExtractGroupCard
                  icon={<Calendar className="h-3.5 w-3.5 text-[var(--accent)]" />}
                  title={t('ai.groups.dates')}
                >
                  {extract.dates.map((item, idx) => (
                    <ExtractRow
                      key={idx}
                      copyLabel={t('common.copy')}
                      onCopy={() => copyValue(item.iso || item.text)}
                    >
                      <p className="text-[12.5px] text-[var(--ink-primary)] break-words">
                        {item.text}
                      </p>
                      {item.iso && (
                        <p className="font-mono text-[11px] text-[var(--ink-muted)]">
                          {item.iso}
                        </p>
                      )}
                    </ExtractRow>
                  ))}
                </ExtractGroupCard>
              )}

              {/* Amounts */}
              {extract.amounts.length > 0 && (
                <ExtractGroupCard
                  icon={<Coins className="h-3.5 w-3.5 text-[var(--accent)]" />}
                  title={t('ai.groups.amounts')}
                >
                  {extract.amounts.map((item, idx) => (
                    <ExtractRow
                      key={idx}
                      copyLabel={t('common.copy')}
                      onCopy={() => copyValue(item.text)}
                    >
                      <p className="font-mono text-[13px] font-semibold tabular-nums text-[var(--ink-primary)]">
                        {item.text}
                      </p>
                    </ExtractRow>
                  ))}
                </ExtractGroupCard>
              )}

              {/* Codes */}
              {extract.codes.length > 0 && (
                <ExtractGroupCard
                  icon={<Hash className="h-3.5 w-3.5 text-[var(--accent)]" />}
                  title={t('ai.groups.codes')}
                >
                  {extract.codes.map((item, idx) => (
                    <ExtractRow
                      key={idx}
                      copyLabel={t('common.copy')}
                      onCopy={() => copyValue(item.value)}
                    >
                      <div className="flex flex-wrap items-center gap-1.5">
                        <Badge variant="default">
                          {t(`ai.codeTypes.${item.type}`, { defaultValue: item.type })}
                        </Badge>
                        <code className="font-mono text-[12.5px] font-semibold text-[var(--ink-primary)] break-all">
                          {item.value}
                        </code>
                      </div>
                    </ExtractRow>
                  ))}
                </ExtractGroupCard>
              )}

              {/* Links (always show full URL + safe external link) */}
              {extract.links.length > 0 && (
                <ExtractGroupCard
                  icon={<Link2 className="h-3.5 w-3.5 text-[var(--accent)]" />}
                  title={t('ai.groups.links')}
                >
                  {extract.links.map((item, idx) => {
                    const safe = isSafeHttpUrl(item.url)
                    return (
                      <ExtractRow
                        key={idx}
                        copyLabel={t('common.copy')}
                        onCopy={() => copyValue(item.url)}
                      >
                        <p className="text-[12.5px] font-medium text-[var(--ink-primary)] break-words">
                          {item.text}
                        </p>
                        {safe ? (
                          <a
                            href={item.url}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="mt-0.5 inline-flex items-center gap-1 break-all font-mono text-[11px] text-[var(--accent-ink)] hover:underline"
                          >
                            <span>{item.url}</span>
                            <ExternalLink className="h-3 w-3 shrink-0" />
                          </a>
                        ) : (
                          <p className="mt-0.5 break-all font-mono text-[11px] text-[var(--ink-muted)]">
                            {item.url}
                          </p>
                        )}
                      </ExtractRow>
                    )
                  })}
                </ExtractGroupCard>
              )}

              {/* Contacts */}
              {extract.contacts.length > 0 && (
                <ExtractGroupCard
                  icon={<User className="h-3.5 w-3.5 text-[var(--accent)]" />}
                  title={t('ai.groups.contacts')}
                >
                  {extract.contacts.map((item, idx) => {
                    const copyTarget = [item.name, item.email, item.phone]
                      .filter(Boolean)
                      .join(' · ')
                    return (
                      <ExtractRow
                        key={idx}
                        copyLabel={t('common.copy')}
                        onCopy={() => copyValue(copyTarget)}
                      >
                        {item.name && (
                          <p className="text-[12.5px] font-medium text-[var(--ink-primary)]">
                            {item.name}
                          </p>
                        )}
                        {item.email && (
                          <p className="font-mono text-[11.5px] text-[var(--ink-secondary)] break-all">
                            {item.email}
                          </p>
                        )}
                        {item.phone && (
                          <p className="font-mono text-[11.5px] text-[var(--ink-muted)]">
                            {item.phone}
                          </p>
                        )}
                      </ExtractRow>
                    )
                  })}
                </ExtractGroupCard>
              )}
            </div>
          )}
        </section>
      )}
    </div>
  )
}

function ExtractGroupCard({
  icon,
  title,
  children,
}: {
  icon: React.ReactNode
  title: string
  children: React.ReactNode
}) {
  return (
    <div className="rounded-[var(--radius-sm)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] p-3 space-y-2">
      <div className="flex items-center gap-1.5 text-[12px] font-semibold text-[var(--ink-secondary)]">
        {icon}
        <span>{title}</span>
      </div>
      <div className="divide-y divide-[var(--border-hairline)]">{children}</div>
    </div>
  )
}

function ExtractRow({
  children,
  copyLabel,
  onCopy,
}: {
  children: React.ReactNode
  copyLabel: string
  onCopy: () => void
}) {
  return (
    <div className="flex items-start justify-between gap-2 py-1.5 first:pt-0 last:pb-0">
      <div className="min-w-0 flex-1">{children}</div>
      <Button
        type="button"
        variant="ghost"
        size="iconSm"
        aria-label={copyLabel}
        title={copyLabel}
        onClick={onCopy}
        className="h-6 w-6 shrink-0"
      >
        <Copy className="h-3 w-3" />
      </Button>
    </div>
  )
}
