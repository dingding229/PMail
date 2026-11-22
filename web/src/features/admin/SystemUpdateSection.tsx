import { useEffect, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
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
  Info,
  Loader2,
  RefreshCw,
  Terminal,
} from 'lucide-react'
import { adminUpdateApi, ApiError, type UpdateJobState } from '@/api'
import { formatMailFullTime } from '@/lib/date'
import { queryKeys, useAdminUpdateQuery } from '@/hooks/useMailQueries'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

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
  const [applyConfirmOpen, setApplyConfirmOpen] = useState(false)
  const [activeJobId, setActiveJobId] = useState<string | null>(null)
  const [jobState, setJobState] = useState<UpdateJobState | null>(null)
  const [jobMessage, setJobMessage] = useState<string | null>(null)
  const [copiedManual, setCopiedManual] = useState(false)

  // Expand latest release by default once data arrives
  useEffect(() => {
    const first = info?.releases?.[0]?.version
    if (first) {
      setExpandedVersions((prev) => (prev.size === 0 ? new Set([first]) : prev))
    }
  }, [info?.releases])

  // Poll active update job with restart fault-tolerance
  useEffect(() => {
    if (!activeJobId) return
    let cancelled = false

    const poll = async () => {
      try {
        const res = await adminUpdateApi.jobStatus(activeJobId)
        if (cancelled) return
        setJobState(res.state)
        setJobMessage(res.message)
        if (res.state === 'done') {
          setActiveJobId(null)
          toast.success(t('admin.update.states.done'))
          void queryClient.invalidateQueries({ queryKey: queryKeys.adminUpdate })
        } else if (res.state === 'failed') {
          setActiveJobId(null)
          toast.error(res.message || t('admin.update.states.failed'))
        }
      } catch {
        // Expected during container/service restart: keep polling and show restarting notice
        if (!cancelled) {
          setJobState('restarting')
        }
      }
    }

    void poll()
    const timer = window.setInterval(() => {
      void poll()
    }, 2000)

    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [activeJobId, queryClient, t])

  const checkMutation = useMutation({
    mutationFn: () => adminUpdateApi.check(),
    onSuccess: (updated) => {
      queryClient.setQueryData(queryKeys.adminUpdate, updated)
      if (updated.has_update && updated.latest) {
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
    mutationFn: () => adminUpdateApi.apply(),
    onSuccess: (res) => {
      setApplyConfirmOpen(false)
      setActiveJobId(res.job_id)
      setJobState('pending')
      setJobMessage(null)
    },
    onError: (err) => {
      setApplyConfirmOpen(false)
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

  const isUpdating = Boolean(activeJobId)

  return (
    <div className="space-y-5">
      {/* Version Overview & Action Card */}
      <section className="rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-elevated)] p-5 shadow-[var(--shadow-xs)] space-y-4">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="flex items-start gap-3">
            <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-[var(--radius-md)] bg-[var(--accent-soft)] text-[var(--accent)]">
              <ArrowUpCircle className="h-4.5 w-4.5" />
            </div>
            <div>
              <h2 className="text-[15px] font-semibold text-[var(--ink-primary)]">
                {t('admin.update.title')}
              </h2>
              <p className="mt-0.5 text-[12.5px] text-[var(--ink-secondary)]">
                {t('admin.update.desc')}
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
            <RefreshCw className={`h-3.5 w-3.5 ${checkMutation.isPending ? 'animate-spin' : ''}`} />
            {checkMutation.isPending ? t('admin.update.checking') : t('admin.update.checkNow')}
          </Button>
        </div>

        {isLoading || !info ? (
          <div className="space-y-3">
            <Skeleton className="h-20 w-full rounded-[var(--radius-md)]" />
            <Skeleton className="h-24 w-full rounded-[var(--radius-md)]" />
          </div>
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
                    {info.latest || info.current}
                  </span>
                  {info.has_update && (
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

            {info.check_error && (
              <div className="flex items-start gap-2 rounded-[var(--radius-sm)] border border-[var(--warning)]/35 bg-[var(--warning-soft)] px-3 py-2 text-[12.5px] text-[var(--warning)]">
                <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
                <span>{info.check_error}</span>
              </div>
            )}

            {/* Update Progress Banner */}
            {jobState && (
              <div
                className={`flex items-start gap-2.5 rounded-[var(--radius-md)] border px-4 py-3 text-[13px] ${
                  jobState === 'done'
                    ? 'border-[var(--success)]/35 bg-[var(--success-soft)] text-[var(--success)]'
                    : jobState === 'failed'
                      ? 'border-[var(--danger)]/35 bg-[var(--danger-soft)] text-[var(--danger)]'
                      : 'border-[var(--accent)]/35 bg-[var(--accent-soft)] text-[var(--accent-ink)]'
                }`}
              >
                {jobState === 'done' ? (
                  <CheckCircle2 className="mt-0.5 h-4.5 w-4.5 shrink-0" />
                ) : jobState === 'failed' ? (
                  <AlertTriangle className="mt-0.5 h-4.5 w-4.5 shrink-0" />
                ) : (
                  <Loader2 className="mt-0.5 h-4.5 w-4.5 shrink-0 animate-spin" />
                )}
                <div className="min-w-0 flex-1 space-y-1">
                  <p className="font-semibold">{t('admin.update.progressTitle')}</p>
                  <p className="text-[12.5px]">{t(`admin.update.states.${jobState}`)}</p>
                  {jobMessage && (
                    <p className="font-mono text-[11.5px] opacity-85">{jobMessage}</p>
                  )}
                </div>
              </div>
            )}

            {/* Update Status Banner */}
            {info.has_update && info.latest ? (
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
                    onClick={() => setApplyConfirmOpen(true)}
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
                  <p className="text-[12px] opacity-85">{t('admin.update.upToDateDesc')}</p>
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
                  {t('admin.update.manualDesc')}
                </p>
                {info.manual_commands.length > 0 && (
                  <pre className="mail-scroll overflow-x-auto rounded-[var(--radius-sm)] border border-[var(--border-subtle)] bg-[var(--surface-subtle)] p-3 font-mono text-[12px] leading-relaxed text-[var(--ink-primary)] select-all">
                    {info.manual_commands.join('\n')}
                  </pre>
                )}
                <div className="flex items-start gap-2 rounded-[var(--radius-sm)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)]/50 px-3 py-2 text-[12px] text-[var(--ink-muted)]">
                  <Info className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                  <div>
                    <p className="font-medium text-[var(--ink-secondary)]">
                      {t('admin.update.enableUpdaterHintTitle')}
                    </p>
                    <p className="mt-0.5">{t('admin.update.enableUpdaterHintDesc')}</p>
                  </div>
                </div>
              </div>
            )}
          </>
        )}
      </section>

      {/* Release Notes / Changelog */}
      {!isLoading && info && (
        <section className="rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-elevated)] p-5 shadow-[var(--shadow-xs)] space-y-3">
          <h3 className="text-[14.5px] font-semibold text-[var(--ink-primary)]">
            {t('admin.update.changelogTitle')}
          </h3>

          {info.releases.length === 0 ? (
            <p className="py-6 text-center text-[13px] text-[var(--ink-muted)]">
              {t('admin.update.noReleases')}
            </p>
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
        </section>
      )}

      {/* Apply Update Confirm Dialog */}
      <Dialog open={applyConfirmOpen} onOpenChange={setApplyConfirmOpen}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>
              {t('admin.update.applyConfirmTitle', { version: info?.latest || '' })}
            </DialogTitle>
            <DialogDescription>{t('admin.update.applyConfirmDesc')}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => setApplyConfirmOpen(false)}
            >
              {t('common.cancel')}
            </Button>
            <Button
              type="button"
              size="sm"
              disabled={applyMutation.isPending}
              onClick={() => applyMutation.mutate()}
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
