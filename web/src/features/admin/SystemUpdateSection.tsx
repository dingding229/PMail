import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import DOMPurify from 'dompurify'
import { toast } from 'sonner'
import {
  AlertTriangle,
  ArrowUpCircle,
  Check,
  CheckCircle2,
  ChevronDown,
  ChevronUp,
  Copy,
  ExternalLink,
  History,
  Loader2,
  RefreshCw,
  Sparkles,
  Terminal,
} from 'lucide-react'
import { adminUpdateApi, ApiError } from '@/api'
import { getUsedIconAttributions } from '@/lib/brandAvatars'
import { formatMailFullTime } from '@/lib/date'
import { queryKeys, useAdminUpdateQuery } from '@/hooks/useMailQueries'
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
import {
  EmptyState,
  SectionCard,
  SectionHeader,
  SectionLoadingState,
} from '@/components/settings'

const SAFE_MD_TAGS = [
  'p',
  'br',
  'strong',
  'em',
  'del',
  'code',
  'pre',
  'ul',
  'ol',
  'li',
  'blockquote',
  'h1',
  'h2',
  'h3',
  'h4',
  'hr',
  'a',
]

function escapeRawHtmlForMarkdown(md: string): string {
  // Neutralize raw HTML tags while preserving Markdown syntax (code fences, links, lists, headings)
  return md.replace(/<\/?[a-zA-Z][^>]*>/g, (match) =>
    match.replace(/</g, '&lt;').replace(/>/g, '&gt;'),
  )
}

function enforceSafeLinks(html: string): string {
  if (typeof DOMParser === 'undefined') return html
  const doc = new DOMParser().parseFromString(html, 'text/html')
  for (const a of Array.from(doc.querySelectorAll('a'))) {
    const href = a.getAttribute('href') || ''
    if (!/^https?:\/\//i.test(href) && !href.startsWith('mailto:')) {
      a.removeAttribute('href')
    } else {
      a.setAttribute('target', '_blank')
      a.setAttribute('rel', 'noopener noreferrer')
    }
  }
  return doc.body.innerHTML
}

function SafeReleaseNotes({ markdown }: { markdown: string }) {
  const [renderedHtml, setRenderedHtml] = useState<string>('')

  useEffect(() => {
    let cancelled = false
    void import('marked').then(({ marked }) => {
      if (cancelled) return
      const safeSource = escapeRawHtmlForMarkdown(markdown || '')
      const rawHtml = String(marked.parse(safeSource, { async: false, gfm: true, breaks: true }))
      const clean = DOMPurify.sanitize(rawHtml, {
        ALLOWED_TAGS: SAFE_MD_TAGS,
        ALLOWED_ATTR: ['href', 'title'],
      })
      setRenderedHtml(enforceSafeLinks(clean))
    })
    return () => {
      cancelled = true
    }
  }, [markdown])

  if (!renderedHtml) {
    return (
      <pre className="whitespace-pre-wrap font-sans text-[12.5px] leading-relaxed text-[var(--ink-secondary)]">
        {markdown}
      </pre>
    )
  }

  return (
    <div
      className="space-y-2 text-[13px] leading-relaxed text-[var(--ink-secondary)] [&_a]:text-[var(--accent)] [&_a]:underline [&_code]:rounded [&_code]:bg-[var(--surface-subtle)] [&_code]:px-1.5 [&_code]:py-0.5 [&_code]:font-mono [&_code]:text-[12px] [&_h2]:mt-2 [&_h2]:text-[13.5px] [&_h2]:font-semibold [&_h2]:text-[var(--ink-primary)] [&_h3]:mt-2 [&_h3]:text-[13px] [&_h3]:font-semibold [&_h3]:text-[var(--ink-primary)] [&_li]:ml-4 [&_ul]:list-disc [&_ol]:list-decimal"
      dangerouslySetInnerHTML={{ __html: renderedHtml }}
    />
  )
}

export function SystemUpdateSection() {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()

  const { data: info, isLoading } = useAdminUpdateQuery(true)

  const [expandedVersions, setExpandedVersions] = useState<Set<string>>(new Set())
  const [confirmVersion, setConfirmVersion] = useState<string | null>(null)
  const [submittedJobId, setSubmittedJobId] = useState<string | null>(null)
  const [copiedManual, setCopiedManual] = useState(false)

  // Expand latest release by default once data arrives
  useEffect(() => {
    const first = info?.releases?.[0]?.version
    if (first) {
      setExpandedVersions((prev) => (prev.size === 0 ? new Set([first]) : prev))
    }
  }, [info?.releases])

  // Recover a running job or its rollback result after a reload. Poll one
  // request at a time; a service restart is not itself a failure.
  const jobId = submittedJobId ?? info?.job?.job_id
  const jobQuery = useQuery({
    queryKey: [...queryKeys.adminUpdate, 'job', jobId],
    queryFn: ({ signal }) => adminUpdateApi.jobStatus(jobId!, AbortSignal.any([signal, AbortSignal.timeout(5000)])),
    enabled: Boolean(jobId),
    retry: false,
    refetchInterval: ({ state }) => {
      if (state.data?.state === 'done' || state.data?.state === 'failed') return false
      if (state.error instanceof ApiError && [401, 403, 404].includes(state.error.status)) return false
      return 2000
    },
  })
  const job = jobQuery.data ?? (info?.job?.job_id === jobId ? info?.job : undefined)
  const jobPollError = jobQuery.error instanceof ApiError && [401, 403, 404].includes(jobQuery.error.status)
    ? jobQuery.error.message : null
  const jobState = job?.state ?? (jobId ? 'pending' : null)
  const jobMessage = jobPollError ?? job?.message ?? (jobQuery.error ? t('admin.update.reconnecting') : null)
  const terminal = jobState === 'done' || jobState === 'failed'
  const isUpdating = Boolean(jobId && !terminal && !jobPollError)

  useEffect(() => {
    if (jobQuery.data?.state === 'done' || jobQuery.data?.state === 'failed') {
      void queryClient.invalidateQueries({ queryKey: queryKeys.adminUpdate, exact: true })
    }
  }, [jobQuery.data?.state, queryClient])

  const checkMutation = useMutation({
    mutationFn: () => adminUpdateApi.check(),
    onSuccess: (updated) => {
      queryClient.setQueryData(queryKeys.adminUpdate, updated)
      if (updated.check_error || !updated.latest) {
        toast.warning(updated.check_error || t('admin.update.checkFailedTitle'))
      } else if (updated.has_update) {
        toast.info(t('admin.update.newVersionTitle', { version: updated.latest }))
      } else {
        toast.success(t('admin.update.upToDateTitle'))
      }
    },
    onError: (err) => {
      if (err instanceof ApiError && err.status === 429) {
        toast.error(t('admin.update.errors.rateLimited'))
        return
      }
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const applyMutation = useMutation({
    mutationFn: (version: string) => adminUpdateApi.apply(version),
    onSuccess: (res) => {
      setConfirmVersion(null)
      setSubmittedJobId(res.job_id)
    },
    onError: (err) => {
      setConfirmVersion(null)
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const toggleRelease = (version: string) => {
    setExpandedVersions((prev) => {
      const next = new Set(prev)
      if (next.has(version)) next.delete(version)
      else next.add(version)
      return next
    })
  }

  const handleCopyManual = async () => {
    if (!info?.manual_commands?.length) return
    try {
      await navigator.clipboard.writeText(info.manual_commands.join('\n'))
      setCopiedManual(true)
      toast.success(t('common.copied'))
      window.setTimeout(() => setCopiedManual(false), 1800)
    } catch {
      toast.error(t('common.copyFailed'))
    }
  }

  return (
    <div className="space-y-5">
      {/* Version Overview & Action Card */}
      <SectionCard>
        <SectionHeader
          icon={ArrowUpCircle}
          title={t('admin.update.title')}
          description={t('admin.update.desc')}
          action={
            <Button
              type="button"
              variant="secondary"
              size="sm"
              disabled={checkMutation.isPending || isUpdating}
              onClick={() => checkMutation.mutate()}
            >
              <RefreshCw
                className={`h-3.5 w-3.5 ${checkMutation.isPending ? 'animate-spin' : ''}`}
              />
              {checkMutation.isPending ? t('admin.update.checking') : t('admin.update.checkNow')}
            </Button>
          }
        />

        {isLoading || !info ? (
          <SectionLoadingState rows={2} rowHeight="h-20" />
        ) : (
          <>
            {/* Version Metrics Grid */}
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
              <div className="rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] px-4 py-3">
                <p className="text-[11.5px] text-[var(--ink-muted)]">
                  {t('admin.update.currentVersion')}
                </p>
                <p className="mt-1 font-mono text-[16px] font-semibold text-[var(--ink-primary)]">
                  {info.current}
                </p>
              </div>
              <div className="rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] px-4 py-3">
                <p className="text-[11.5px] text-[var(--ink-muted)]">
                  {t('admin.update.latestVersion')}
                </p>
                <div className="mt-1 flex items-center gap-2">
                  <span className="font-mono text-[16px] font-semibold text-[var(--ink-primary)]">
                    {info.latest || t('admin.update.latestUnknown')}
                  </span>
                  {info.has_update && info.latest && !info.check_error && (
                    <Badge variant="accent">{t('admin.update.availableBadge')}</Badge>
                  )}
                </div>
              </div>
              <div className="rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] px-4 py-3">
                <p className="text-[11.5px] text-[var(--ink-muted)]">
                  {t('admin.update.lastChecked')}
                </p>
                <p className="mt-1 text-[13px] font-medium tabular-nums text-[var(--ink-primary)]">
                  {info.checked_at
                    ? formatMailFullTime(info.checked_at, i18n.language)
                    : t('common.never')}
                </p>
              </div>
            </div>

            {/* Update Progress Banner */}
            {jobState && (
              <div
                role="status"
                aria-live="polite"
                className={`flex items-start gap-2.5 rounded-[var(--radius-md)] border px-4 py-3 text-[13px] ${
                  jobState === 'done'
                    ? 'border-[var(--success)]/35 bg-[var(--success-soft)] text-[var(--success)]'
                    : (jobState === 'failed' || jobPollError)
                      ? 'border-[var(--danger)]/35 bg-[var(--danger-soft)] text-[var(--danger)]'
                      : 'border-[var(--accent)]/35 bg-[var(--accent-soft)] text-[var(--accent-ink)]'
                }`}
              >
                {jobState === 'done' ? (
                  <CheckCircle2 className="mt-0.5 h-4.5 w-4.5 shrink-0" />
                ) : (jobState === 'failed' || jobPollError) ? (
                  <AlertTriangle className="mt-0.5 h-4.5 w-4.5 shrink-0" />
                ) : (
                  <Loader2 className="mt-0.5 h-4.5 w-4.5 shrink-0 animate-spin" />
                )}
                <div className="min-w-0 flex-1 space-y-1">
                  <p className="font-semibold">{t('admin.update.progressTitle')}</p>
                  <p className="text-[12.5px]">{t(`admin.update.states.${jobPollError ? 'failed' : jobState}`)}</p>
                  {jobMessage && (
                    <p className="text-[12.5px] opacity-85">{jobMessage}</p>
                  )}
                </div>
              </div>
            )}

            {/* Update Status Banner: never claim "up to date" when check_error is set or latest is empty */}
            {info.check_error || !info.latest ? (
              <div className="flex flex-wrap items-center justify-between gap-3 rounded-[var(--radius-md)] border border-[var(--warning)]/35 bg-[var(--warning-soft)] px-4 py-3 text-[13px] text-[var(--warning)]">
                <div className="flex min-w-0 items-start gap-2.5">
                  <AlertTriangle className="mt-0.5 h-4.5 w-4.5 shrink-0" />
                  <div className="min-w-0 space-y-0.5">
                    <p className="font-semibold">{t('admin.update.checkFailedTitle')}</p>
                    <p className="text-[12.5px] opacity-90">
                      {info.check_error || t('admin.update.checkFailedDesc')}
                    </p>
                  </div>
                </div>
                <Button
                  type="button"
                  variant="secondary"
                  size="sm"
                  disabled={checkMutation.isPending || isUpdating}
                  onClick={() => checkMutation.mutate()}
                >
                  <RefreshCw
                    className={`h-3.5 w-3.5 ${checkMutation.isPending ? 'animate-spin' : ''}`}
                  />
                  {t('app.retry')}
                </Button>
              </div>
            ) : info.has_update ? (
              <div className="flex flex-wrap items-center justify-between gap-3 rounded-[var(--radius-md)] border border-[var(--accent)]/35 bg-[var(--accent-soft)]/70 px-4 py-3.5">
                <div className="min-w-0 space-y-0.5">
                  <p className="text-[14px] font-semibold text-[var(--ink-primary)]">
                    {t('admin.update.newVersionTitle', { version: info.latest })}
                  </p>
                  <p className="text-[12.5px] text-[var(--ink-secondary)]">
                    {t('admin.update.newVersionDesc')}
                  </p>
                </div>
                {info.updater.enabled && (
                  <Button
                    type="button"
                    size="sm"
                    disabled={isUpdating || applyMutation.isPending}
                    onClick={() => setConfirmVersion(info.latest)}
                  >
                    <ArrowUpCircle className="h-4 w-4" />
                    {t('admin.update.applyNow')}
                  </Button>
                )}
              </div>
            ) : (
              <div className="flex items-center gap-2.5 rounded-[var(--radius-md)] border border-[var(--success)]/30 bg-[var(--success-soft)]/70 px-4 py-3 text-[13px] text-[var(--success)]">
                <CheckCircle2 className="h-4.5 w-4.5 shrink-0" />
                <div>
                  <p className="font-medium">{t('admin.update.upToDateTitle')}</p>
                  <p className="text-[12px] opacity-85">{t('admin.update.upToDateDesc', {
                      version: info.current.startsWith('v') ? info.current : `v${info.current}`,
                    })}</p>
                </div>
              </div>
            )}

            {/* Manual Commands (when updater disabled) */}
            {!info.updater.enabled && (
              <div className="rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] p-4 space-y-3">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="flex items-center gap-2 text-[13.5px] font-semibold text-[var(--ink-primary)]">
                    <Terminal className="h-4 w-4 text-[var(--ink-muted)]" />
                    <span>{t('admin.update.manualTitle')}</span>
                  </div>
                  {info.manual_commands.length > 0 && (
                    <Button
                      type="button"
                      variant="secondary"
                      size="xs"
                      onClick={() => void handleCopyManual()}
                    >
                      {copiedManual ? (
                        <Check className="h-3.5 w-3.5 text-[var(--success)]" />
                      ) : (
                        <Copy className="h-3.5 w-3.5" />
                      )}
                      {t('common.copy')}
                    </Button>
                  )}
                </div>
                <p className="text-[12.5px] text-[var(--ink-secondary)]">
                  {info.updater.reason || t('admin.update.manualDesc')}
                </p>
                {info.manual_commands.length > 0 && (
                  <pre className="mail-scroll overflow-x-auto rounded-[var(--radius-sm)] border border-[var(--border-subtle)] bg-[var(--surface-subtle)] p-3 font-mono text-[12px] leading-relaxed text-[var(--ink-primary)] select-all">
                    {info.manual_commands.join('\n')}
                  </pre>
                )}

              </div>
            )}
          </>
        )}
      </SectionCard>

      {/* Release Notes / Changelog */}
      {!isLoading && info && (
        <SectionCard>
          <SectionHeader
            icon={History}
            iconTone="neutral"
            title={t('admin.update.changelogTitle')}
          />

          {info.releases.length === 0 ? (
            <EmptyState icon={History} title={t('admin.update.noReleases')} compact />
          ) : (
            <div className="divide-y divide-[var(--border-hairline)] rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)]">
              {info.releases.map((rel, idx) => {
                const isOpen = expandedVersions.has(rel.version)
                return (
                  <div key={rel.version} className="p-4">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <button
                        type="button"
                        onClick={() => toggleRelease(rel.version)}
                        className="flex flex-wrap items-center gap-2 text-left"
                      >
                        <span className="font-mono text-[14px] font-semibold text-[var(--ink-primary)]">
                          {rel.version}
                        </span>
                        {idx === 0 && (
                          <Badge variant="accent">{t('admin.update.latestTag')}</Badge>
                        )}
                        {rel.is_current && (
                          <Badge variant="success">{t('admin.update.currentTag')}</Badge>
                        )}
                        <span className="text-[12px] tabular-nums text-[var(--ink-muted)]">
                          {formatMailFullTime(rel.published_at, i18n.language)}
                        </span>
                      </button>

                      <div className="flex items-center gap-2">
                        {rel.url && (
                          <a
                            href={rel.url}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="inline-flex items-center gap-1 text-[12px] text-[var(--accent)] hover:underline"
                          >
                            {t('admin.update.viewOnGithub')}
                            <ExternalLink className="h-3 w-3" />
                          </a>
                        )}
                        <Button
                          type="button"
                          variant="ghost"
                          size="xs"
                          onClick={() => toggleRelease(rel.version)}
                        >
                          {isOpen ? t('admin.update.collapseNotes') : t('admin.update.expandNotes')}
                          {isOpen ? (
                            <ChevronUp className="h-3.5 w-3.5" />
                          ) : (
                            <ChevronDown className="h-3.5 w-3.5" />
                          )}
                        </Button>
                      </div>
                    </div>

                    {isOpen && (
                      <div className="mt-3 border-t border-[var(--border-hairline)] pt-3">
                        <SafeReleaseNotes markdown={rel.notes_markdown} />
                      </div>
                    )}
                  </div>
                )
              })}
            </div>
          )}
        </SectionCard>
      )}

      {/* Third-party Brand Icons Attribution */}
      <SectionCard>
        <SectionHeader
          icon={Sparkles}
          title={t('admin.update.thirdPartyIconsTitle')}
          description={t('admin.update.thirdPartyIconsDesc')}
        />
        <div className="divide-y divide-[var(--border-hairline)] rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)]">
          {getUsedIconAttributions().map((lib) => (
            <div
              key={lib.id}
              className="flex flex-wrap items-center justify-between gap-2 px-4 py-2.5 text-[12.5px]"
            >
              <div className="min-w-0">
                <a
                  href={lib.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="font-medium text-[var(--ink-primary)] hover:text-[var(--accent)] hover:underline"
                >
                  {lib.name}
                </a>
                <span className="ml-2 font-mono text-[11.5px] text-[var(--ink-muted)]">
                  {lib.pkg}
                </span>
              </div>
              <Badge variant="default">{lib.license}</Badge>
            </div>
          ))}
        </div>
      </SectionCard>

      {/* Apply Update Confirm Dialog */}
      <Dialog open={confirmVersion !== null} onOpenChange={(open) => { if (!open) setConfirmVersion(null) }}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>
              {t('admin.update.applyConfirmTitle', { version: confirmVersion || '' })}
            </DialogTitle>
            <DialogDescription>{t('admin.update.applyConfirmDesc')}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => setConfirmVersion(null)}
            >
              {t('common.cancel')}
            </Button>
            <Button
              type="button"
              size="sm"
              disabled={applyMutation.isPending}
              onClick={() => { if (confirmVersion) applyMutation.mutate(confirmVersion) }}
            >
              {applyMutation.isPending && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
              {t('admin.update.applyConfirmAction')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
