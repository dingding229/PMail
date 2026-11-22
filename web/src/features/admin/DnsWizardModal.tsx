import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  AlertCircle,
  AlertTriangle,
  ArrowLeft,
  ArrowRight,
  Check,
  CheckCircle2,
  Clock,
  Copy,
  Download,
  Globe,
  Info,
  Loader2,
  RefreshCw,
  Server,
  Sparkles,
  X,
  XCircle,
} from 'lucide-react'
import { domainApi, type DnsRecord, type MailDomain } from '@/api'
import { formatMailFullTime } from '@/lib/date'
import { cn } from '@/lib/cn'
import { useViewportBreakpoint } from '@/hooks/useMediaQuery'
import { queryKeys } from '@/hooks/useMailQueries'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Switch } from '@/components/ui/switch'
import { Skeleton } from '@/components/ui/skeleton'
import { DnsRecordCard } from './DnsRecordCard'

interface DnsWizardModalProps {
  domain: MailDomain
  initialStep?: 1 | 2 | 3 | 4
  onClose: () => void
}

type WizardStep = 1 | 2 | 3 | 4

export function DnsWizardModal({
  domain,
  initialStep = 1,
  onClose,
}: DnsWizardModalProps) {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  const { isMobile } = useViewportBreakpoint()

  const [step, setStep] = useState<WizardStep>(initialStep)
  const bodyScrollRef = useRef<HTMLDivElement | null>(null)
  const [refreshing, setRefreshing] = useState(false)

  useEffect(() => {
    bodyScrollRef.current?.scrollTo({ top: 0 })
  }, [step])
  const [checkingKey, setCheckingKey] = useState<string | null>(null)
  const [autoCheck, setAutoCheck] = useState(false)
  const [countdown, setCountdown] = useState(30)

  const { data: dns, isLoading, isError, refetch } = useQuery({
    queryKey: queryKeys.domainDns(domain.id),
    queryFn: () => domainApi.dns(domain.id, false),
  })

  // Partition records into Required / Recommended / Optional
  const { requiredRecords, recommendedRecords, optionalRecords, ptrRecord } = useMemo(() => {
    const all = dns?.records ?? []
    const required: DnsRecord[] = []
    const recommended: DnsRecord[] = []
    const optional: DnsRecord[] = []
    let ptr: DnsRecord | null = null

    for (const r of all) {
      if (r.key === 'ptr' || r.type.toUpperCase() === 'PTR') {
        ptr = r
        recommended.push(r)
      } else if (r.required) {
        required.push(r)
      } else if (r.key === 'dmarc' || r.status === 'advice') {
        recommended.push(r)
      } else {
        optional.push(r)
      }
    }
    return {
      requiredRecords: required,
      recommendedRecords: recommended,
      optionalRecords: optional,
      ptrRecord: ptr,
    }
  }, [dns?.records])

  const requiredOkCount = requiredRecords.filter((r) => r.status === 'ok').length
  const requiredMissingCount = requiredRecords.length - requiredOkCount

  const runLiveCheck = useCallback(async () => {
    if (refreshing) return
    setRefreshing(true)
    try {
      const records = dns?.records ?? []
      // Subtle sequential checking highlight animation
      const animPromise = (async () => {
        for (const rec of records.slice(0, 6)) {
          setCheckingKey(rec.key)
          await new Promise((r) => setTimeout(r, 110))
        }
        setCheckingKey(null)
      })()

      const fresh = await domainApi.dns(domain.id, true)
      await animPromise
      queryClient.setQueryData(queryKeys.domainDns(domain.id), fresh)
      setCountdown(30)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    } finally {
      setCheckingKey(null)
      setRefreshing(false)
    }
  }, [domain.id, dns?.records, queryClient, refreshing, t])

  // Auto-recheck every 30s when enabled on Step 3
  useEffect(() => {
    if (!autoCheck || step !== 3) return
    const interval = window.setInterval(() => {
      setCountdown((prev) => {
        if (prev <= 1) {
          void runLiveCheck()
          return 30
        }
        return prev - 1
      })
    }, 1000)
    return () => window.clearInterval(interval)
  }, [autoCheck, step, runLiveCheck])

  const handleCopyAllRecords = async () => {
    if (!dns) return
    const header = [
      t('admin.dnsWizard.fields.type'),
      t('admin.dnsWizard.fields.host'),
      t('admin.dnsWizard.fields.priority'),
      t('admin.dnsWizard.fields.ttl'),
      t('admin.dnsWizard.fields.value'),
    ].join('\t')

    const rows = dns.records
      .filter((r) => r.type.toUpperCase() !== 'PTR')
      .map((r) =>
        [
          r.type,
          r.host || '@',
          r.priority ? String(r.priority) : '-',
          String(r.ttl || 3600),
          r.value,
        ].join('\t'),
      )

    try {
      await navigator.clipboard.writeText([header, ...rows].join('\n'))
      toast.success(t('admin.dnsWizard.copiedAll'))
    } catch {
      toast.error(t('common.copyFailed'))
    }
  }

  const copySingle = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text)
      toast.success(t('common.copied'))
    } catch {
      toast.error(t('common.copyFailed'))
    }
  }

  const steps: { id: WizardStep; label: string }[] = [
    { id: 1, label: t('admin.dnsWizard.steps.intro') },
    { id: 2, label: t('admin.dnsWizard.steps.records') },
    { id: 3, label: t('admin.dnsWizard.steps.verify') },
    { id: 4, label: t('admin.dnsWizard.steps.done') },
  ]

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      {!isMobile && (
        <div
          aria-hidden="true"
          onClick={onClose}
          className="fixed inset-0 bg-slate-950/50 backdrop-blur-[2px]"
        />
      )}

      <div
        role="dialog"
        aria-label={t('admin.dnsWizard.title', { domain: domain.name })}
        className={cn(
          'relative z-10 flex flex-col bg-[var(--surface-elevated)] text-[var(--ink-primary)] overflow-hidden',
          isMobile
            ? 'h-full w-full pt-safe pb-safe'
            : 'h-[min(86vh,780px)] w-[min(92vw,920px)] rounded-[var(--radius-xl)] border border-[var(--border-subtle)] shadow-[var(--shadow-lg)]',
        )}
      >
        {/* Top Header */}
        <header className="flex shrink-0 items-center justify-between gap-3 border-b border-[var(--border-hairline)] bg-[var(--surface-sidebar)] px-4 py-3 sm:px-6">
          <div className="flex min-w-0 items-center gap-2.5">
            <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-[var(--radius-sm)] bg-[var(--accent-soft)] text-[var(--accent)]">
              <Globe className="h-4 w-4" />
            </div>
            <h2 className="truncate text-[15px] font-semibold">
              {t('admin.dnsWizard.title', { domain: domain.name })}
            </h2>
          </div>
          <Button variant="ghost" size="iconSm" onClick={onClose} aria-label={t('common.close')}>
            <X className="h-4 w-4" />
          </Button>
        </header>

        {/* Stepper Bar: compact on <640px (no horizontal scrollbar), full 4-step bar on >=640px */}
        <nav
          aria-label={t('admin.dnsWizard.title', { domain: domain.name })}
          className="shrink-0 border-b border-[var(--border-hairline)] bg-[var(--surface-canvas)] px-4 py-2.5 sm:px-6"
        >
          {/* Compact mobile stepper (<640px) */}
          <div className="flex items-center justify-between gap-3 sm:hidden">
            <div className="flex items-center gap-2 min-w-0">
              <span className="inline-flex h-6 items-center justify-center rounded-full bg-[var(--accent)] px-2 text-[11.5px] font-semibold tabular-nums text-white">
                {step}/{steps.length}
              </span>
              <span className="truncate text-[13.5px] font-semibold text-[var(--ink-primary)]">
                {steps.find((s) => s.id === step)?.label}
              </span>
            </div>
            <div className="flex items-center gap-1.5 shrink-0">
              {steps.map((s) => {
                const active = step === s.id
                const completed = step > s.id
                return (
                  <button
                    key={s.id}
                    type="button"
                    onClick={() => setStep(s.id)}
                    aria-label={`${s.id}/${steps.length} ${s.label}`}
                    className={cn(
                      'h-2 rounded-full transition-all duration-150',
                      active
                        ? 'w-5 bg-[var(--accent)]'
                        : completed
                          ? 'w-2 bg-[var(--success)]'
                          : 'w-2 bg-[var(--border-strong)]',
                    )}
                  />
                )
              })}
            </div>
          </div>

          {/* Desktop / Tablet stepper (>=640px) */}
          <div className="hidden items-center justify-between gap-2 sm:flex">
            {steps.map((s, idx) => {
              const active = step === s.id
              const completed = step > s.id
              return (
                <div key={s.id} className="flex flex-1 items-center gap-2 min-w-fit">
                  <button
                    type="button"
                    onClick={() => setStep(s.id)}
                    className={cn(
                      'flex items-center gap-2 rounded-full px-2.5 py-1 text-[12.5px] font-medium transition-colors',
                      active
                        ? 'bg-[var(--accent-soft)] text-[var(--accent)]'
                        : completed
                          ? 'text-[var(--ink-primary)] hover:bg-[var(--surface-hover)]'
                          : 'text-[var(--ink-muted)] hover:bg-[var(--surface-hover)]',
                    )}
                  >
                    <span
                      className={cn(
                        'flex h-5 w-5 shrink-0 items-center justify-center rounded-full text-[11px] font-semibold',
                        active
                          ? 'bg-[var(--accent)] text-white'
                          : completed
                            ? 'bg-[var(--success)] text-white'
                            : 'bg-[var(--surface-subtle)] text-[var(--ink-muted)]',
                      )}
                    >
                      {completed ? <Check className="h-3 w-3" /> : s.id}
                    </span>
                    <span className="whitespace-nowrap">{s.label}</span>
                  </button>
                  {idx < steps.length - 1 && (
                    <div className="h-px flex-1 bg-[var(--border-hairline)]" />
                  )}
                </div>
              )
            })}
          </div>
        </nav>

        {/* Scrollable Step Body */}
        <div ref={bodyScrollRef} className="mail-scroll flex-1 overflow-y-auto px-4 py-5 sm:px-6">
          {isLoading ? (
            <div className="space-y-4">
              <Skeleton className="h-24 w-full rounded-[var(--radius-md)]" />
              <Skeleton className="h-44 w-full rounded-[var(--radius-md)]" />
              <Skeleton className="h-44 w-full rounded-[var(--radius-md)]" />
            </div>
          ) : isError || !dns ? (
            <div className="flex flex-col items-center justify-center py-16 text-center">
              <AlertCircle className="h-8 w-8 text-[var(--danger)]" />
              <p className="mt-3 text-[14px] text-[var(--ink-secondary)]">
                {t('common.unknownError')}
              </p>
              <Button variant="secondary" size="sm" className="mt-3" onClick={() => void refetch()}>
                {t('app.retry')}
              </Button>
            </div>
          ) : step === 1 ? (
            /* ==================== STEP 1: INTRO ==================== */
            <div className="mx-auto max-w-2xl space-y-5">
              <div>
                <h3 className="text-[18px] font-semibold tracking-tight text-[var(--ink-primary)]">
                  {t('admin.dnsWizard.intro.headline', { domain: dns.domain })}
                </h3>
                <p className="mt-1.5 text-[13.5px] leading-relaxed text-[var(--ink-secondary)]">
                  {t('admin.dnsWizard.intro.desc')}
                </p>
              </div>

              {/* Server Info Summary Cards */}
              <div className="rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-canvas)] p-4 space-y-3">
                <div className="flex items-center gap-2 text-[13px] font-semibold text-[var(--ink-primary)]">
                  <Server className="h-4 w-4 text-[var(--accent)]" />
                  <span>{t('admin.dnsWizard.intro.serverInfoTitle')}</span>
                </div>

                <div className="grid grid-cols-1 gap-2.5 sm:grid-cols-2">
                  {[
                    { label: t('admin.dnsWizard.intro.mailHost'), value: dns.mail_host },
                    { label: t('admin.dnsWizard.intro.webDomain'), value: dns.web_domain },
                    {
                      label: t('admin.dnsWizard.intro.ipv4'),
                      value: dns.server_ipv4 || t('admin.dnsWizard.intro.notDetected'),
                      copyable: Boolean(dns.server_ipv4),
                    },
                    {
                      label: t('admin.dnsWizard.intro.ipv6'),
                      value: dns.server_ipv6 || t('admin.dnsWizard.intro.notDetected'),
                      copyable: Boolean(dns.server_ipv6),
                    },
                  ].map((item) => (
                    <div
                      key={item.label}
                      className="flex items-center justify-between gap-2 rounded-[var(--radius-sm)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)]/60 px-3 py-2"
                    >
                      <div className="min-w-0">
                        <p className="text-[11px] text-[var(--ink-muted)]">{item.label}</p>
                        <p className="truncate font-mono text-[13px] font-medium text-[var(--ink-primary)]">
                          {item.value}
                        </p>
                      </div>
                      {item.copyable !== false && item.value && (
                        <Button
                          type="button"
                          variant="ghost"
                          size="iconSm"
                          onClick={() => void copySingle(item.value)}
                          aria-label={t('common.copy')}
                        >
                          <Copy className="h-3.5 w-3.5" />
                        </Button>
                      )}
                    </div>
                  ))}
                </div>
              </div>

              {/* Record Category Overview */}
              <div className="rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-canvas)] p-4 space-y-2.5">
                <h4 className="text-[13px] font-semibold text-[var(--ink-primary)]">
                  {t('admin.dnsWizard.intro.whatToAddTitle')}
                </h4>
                <ul className="space-y-2 text-[13px] text-[var(--ink-secondary)]">
                  <li className="flex items-start gap-2">
                    <span className="mt-1.5 h-2 w-2 shrink-0 rounded-full bg-[var(--accent)]" />
                    <span>
                      {t('admin.dnsWizard.intro.whatToAddRequired', {
                        count: requiredRecords.length,
                      })}
                    </span>
                  </li>
                  <li className="flex items-start gap-2">
                    <span className="mt-1.5 h-2 w-2 shrink-0 rounded-full bg-[var(--warning)]" />
                    <span>
                      {t('admin.dnsWizard.intro.whatToAddRecommended', {
                        count: recommendedRecords.length,
                      })}
                    </span>
                  </li>
                  {optionalRecords.length > 0 && (
                    <li className="flex items-start gap-2">
                      <span className="mt-1.5 h-2 w-2 shrink-0 rounded-full bg-[var(--ink-subtle)]" />
                      <span>
                        {t('admin.dnsWizard.intro.whatToAddOptional', {
                          count: optionalRecords.length,
                        })}
                      </span>
                    </li>
                  )}
                </ul>
              </div>
            </div>
          ) : step === 2 ? (
            /* ==================== STEP 2: RECORDS CHECKLIST ==================== */
            <div className="space-y-6">
              {/* Utility Bar: BIND Zone + Copy All */}
              <div className="flex flex-wrap items-center justify-between gap-2 rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)]/60 px-3.5 py-2.5">
                <div className="text-[12.5px] text-[var(--ink-secondary)]">
                  {t('admin.dnsWizard.verify.progressSummary', {
                    ok: requiredOkCount,
                    total: requiredRecords.length,
                  })}
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  <Button variant="outline" size="xs" onClick={() => void handleCopyAllRecords()}>
                    <Copy className="h-3.5 w-3.5" />
                    {t('admin.dnsWizard.copyAll')}
                  </Button>
                  <Button asChild variant="secondary" size="xs">
                    <a
                      href={domainApi.zoneDownloadUrl(domain.id)}
                      download={`${domain.name}.zone`}
                    >
                      <Download className="h-3.5 w-3.5" />
                      {t('admin.dnsWizard.downloadZone')}
                    </a>
                  </Button>
                </div>
              </div>

              {/* Group 1: Required */}
              <div className="space-y-3">
                <div>
                  <div className="flex items-center gap-2">
                    <h3 className="text-[14px] font-semibold text-[var(--ink-primary)]">
                      {t('admin.dnsWizard.groups.required')}
                    </h3>
                    <Badge variant="accent">{requiredRecords.length}</Badge>
                  </div>
                  <p className="text-[12px] text-[var(--ink-muted)]">
                    {t('admin.dnsWizard.groups.requiredDesc')}
                  </p>
                </div>
                <div className="space-y-3">
                  {requiredRecords.map((rec) => (
                    <DnsRecordCard key={rec.key} record={rec} />
                  ))}
                </div>
              </div>

              {/* Group 2: Recommended */}
              {recommendedRecords.length > 0 && (
                <div className="space-y-3">
                  <div>
                    <div className="flex items-center gap-2">
                      <h3 className="text-[14px] font-semibold text-[var(--ink-primary)]">
                        {t('admin.dnsWizard.groups.recommended')}
                      </h3>
                      <Badge variant="warning">{recommendedRecords.length}</Badge>
                    </div>
                    <p className="text-[12px] text-[var(--ink-muted)]">
                      {t('admin.dnsWizard.groups.recommendedDesc')}
                    </p>
                  </div>
                  <div className="space-y-3">
                    {recommendedRecords.map((rec) => (
                      <DnsRecordCard key={rec.key} record={rec} />
                    ))}
                  </div>
                </div>
              )}

              {/* Group 3: Optional */}
              {optionalRecords.length > 0 && (
                <div className="space-y-3">
                  <div>
                    <div className="flex items-center gap-2">
                      <h3 className="text-[14px] font-semibold text-[var(--ink-primary)]">
                        {t('admin.dnsWizard.groups.optional')}
                      </h3>
                      <Badge variant="default">{optionalRecords.length}</Badge>
                    </div>
                    <p className="text-[12px] text-[var(--ink-muted)]">
                      {t('admin.dnsWizard.groups.optionalDesc')}
                    </p>
                  </div>
                  <div className="space-y-3">
                    {optionalRecords.map((rec) => (
                      <DnsRecordCard key={rec.key} record={rec} />
                    ))}
                  </div>
                </div>
              )}
            </div>
          ) : step === 3 ? (
            /* ==================== STEP 3: LIVE VERIFICATION ==================== */
            <div className="mx-auto max-w-3xl space-y-5">
              <div className="rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-canvas)] p-4 sm:p-5 space-y-4">
                <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                  <div>
                    <h3 className="text-[16px] font-semibold text-[var(--ink-primary)]">
                      {t('admin.dnsWizard.verify.headline')}
                    </h3>
                    <p className="mt-0.5 text-[12.5px] text-[var(--ink-secondary)]">
                      {t('admin.dnsWizard.verify.desc')}
                    </p>
                  </div>
                  <Button
                    variant="primary"
                    size="md"
                    disabled={refreshing}
                    onClick={() => void runLiveCheck()}
                  >
                    {refreshing ? (
                      <Loader2 className="h-4 w-4 animate-spin" />
                    ) : (
                      <RefreshCw className="h-4 w-4" />
                    )}
                    {refreshing
                      ? t('admin.dnsWizard.verify.checking')
                      : t('admin.dnsWizard.verify.checkNow')}
                  </Button>
                </div>

                {/* Progress bar */}
                <div className="space-y-1.5">
                  <div className="flex items-center justify-between text-[12.5px]">
                    <span className="font-medium text-[var(--ink-primary)]">
                      {t('admin.dnsWizard.verify.progressSummary', {
                        ok: requiredOkCount,
                        total: requiredRecords.length,
                      })}
                    </span>
                    <span className="text-[11.5px] text-[var(--ink-muted)]">
                      {t('admin.dnsWizard.verify.checkedAt', {
                        time: formatMailFullTime(dns.checked_at, i18n.language),
                      })}
                    </span>
                  </div>
                  <div className="h-2 w-full overflow-hidden rounded-full bg-[var(--surface-subtle)]">
                    <div
                      style={{
                        width: `${
                          requiredRecords.length > 0
                            ? Math.round((requiredOkCount / requiredRecords.length) * 100)
                            : 100
                        }%`,
                      }}
                      className={cn(
                        'h-full rounded-full transition-all duration-300',
                        dns.all_required_ok ? 'bg-[var(--success)]' : 'bg-[var(--accent)]',
                      )}
                    />
                  </div>
                </div>

                {/* Auto-check toggle & Propagation tip */}
                <div className="flex flex-wrap items-center justify-between gap-3 border-t border-[var(--border-hairline)] pt-3">
                  <label className="inline-flex items-center gap-2.5 text-[13px] text-[var(--ink-primary)] cursor-pointer">
                    <Switch checked={autoCheck} onCheckedChange={setAutoCheck} />
                    <span>{t('admin.dnsWizard.verify.autoCheckLabel')}</span>
                  </label>
                  {autoCheck && (
                    <span className="inline-flex items-center gap-1.5 text-[12px] tabular-nums text-[var(--accent)]">
                      <Clock className="h-3.5 w-3.5" />
                      {t('admin.dnsWizard.verify.autoCheckCountdown', { sec: countdown })}
                    </span>
                  )}
                </div>

                <p className="rounded-[var(--radius-sm)] bg-[var(--surface-subtle)] px-3 py-2 text-[12px] text-[var(--ink-secondary)]">
                  {t('admin.dnsWizard.verify.propagationTip')}
                </p>
              </div>

              {/* Live Status Rows */}
              <div className="space-y-2.5">
                {dns.records.map((rec) => {
                  const isCheckingThis = checkingKey === rec.key
                  return (
                    <div
                      key={rec.key}
                      className={cn(
                        'flex flex-col gap-2 rounded-[var(--radius-md)] border bg-[var(--surface-canvas)] p-3.5 transition-all duration-200 sm:flex-row sm:items-center sm:justify-between',
                        isCheckingThis
                          ? 'border-[var(--accent)] bg-[var(--accent-soft)]/30'
                          : 'border-[var(--border-hairline)]',
                      )}
                    >
                      <div className="min-w-0 space-y-0.5">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="rounded bg-[var(--surface-subtle)] px-1.5 py-0.5 font-mono text-[11px] font-semibold">
                            {rec.type}
                          </span>
                          <span className="text-[13.5px] font-medium text-[var(--ink-primary)]">
                            {rec.purpose}
                          </span>
                          <span className="break-all font-mono text-[11.5px] text-[var(--ink-muted)]">
                            {rec.host || '@'}
                          </span>
                        </div>
                        {rec.status !== 'ok' && rec.fix_hint && (
                          <p className="text-[12px] text-[var(--ink-secondary)]">{rec.fix_hint}</p>
                        )}
                      </div>

                      <div className="shrink-0 self-end sm:self-center">
                        {isCheckingThis ? (
                          <Badge variant="accent">
                            <Loader2 className="h-3 w-3 animate-spin" />
                            {t('admin.domains.statusChecking')}
                          </Badge>
                        ) : rec.status === 'ok' ? (
                          <Badge variant="success">
                            <CheckCircle2 className="h-3 w-3" />
                            {t('admin.dnsWizard.status.ok')}
                          </Badge>
                        ) : rec.status === 'advice' ? (
                          <Badge variant="accent">
                            <Info className="h-3 w-3" />
                            {t('admin.dnsWizard.status.advice')}
                          </Badge>
                        ) : rec.status === 'missing' ? (
                          <Badge variant="warning">
                            <AlertTriangle className="h-3 w-3" />
                            {t('admin.dnsWizard.status.missing')}
                          </Badge>
                        ) : (
                          <Badge variant="danger">
                            <XCircle className="h-3 w-3" />
                            {t(`admin.dnsWizard.status.${rec.status}` as const)}
                          </Badge>
                        )}
                      </div>
                    </div>
                  )
                })}
              </div>

              {requiredMissingCount > 0 && (
                <div className="flex justify-center">
                  <Button variant="outline" size="sm" onClick={() => setStep(2)}>
                    <ArrowLeft className="h-3.5 w-3.5" />
                    {t('admin.dnsWizard.verify.backToRecords')}
                  </Button>
                </div>
              )}
            </div>
          ) : (
            /* ==================== STEP 4: COMPLETE & PTR ADVICE ==================== */
            <div className="mx-auto max-w-2xl space-y-5 py-2">
              <div
                className={cn(
                  'rounded-[var(--radius-lg)] border p-6 text-center',
                  dns.all_required_ok
                    ? 'border-[var(--success)]/30 bg-[var(--success-soft)]/50'
                    : 'border-[var(--warning)]/35 bg-[var(--warning-soft)]/50',
                )}
              >
                <div
                  className={cn(
                    'mx-auto flex h-12 w-12 items-center justify-center rounded-full',
                    dns.all_required_ok
                      ? 'bg-[var(--success)] text-white'
                      : 'bg-[var(--warning)] text-white',
                  )}
                >
                  {dns.all_required_ok ? (
                    <Sparkles className="h-6 w-6" />
                  ) : (
                    <AlertTriangle className="h-6 w-6" />
                  )}
                </div>
                <h3 className="mt-3 text-[17px] font-semibold text-[var(--ink-primary)]">
                  {dns.all_required_ok
                    ? t('admin.dnsWizard.done.allOkTitle', { domain: dns.domain })
                    : t('admin.dnsWizard.done.partialTitle', { count: requiredMissingCount })}
                </h3>
                <p className="mt-1.5 text-[13px] leading-relaxed text-[var(--ink-secondary)]">
                  {dns.all_required_ok
                    ? t('admin.dnsWizard.done.allOkDesc')
                    : t('admin.dnsWizard.done.partialDesc')}
                </p>
              </div>

              {/* Dedicated PTR VPS Advisory Card */}
              <div className="rounded-[var(--radius-lg)] border border-[var(--accent)]/25 bg-[var(--surface-canvas)] p-5 space-y-3">
                <div className="flex items-center gap-2">
                  <Info className="h-4 w-4 text-[var(--accent)] shrink-0" />
                  <h4 className="text-[14px] font-semibold text-[var(--ink-primary)]">
                    {t('admin.dnsWizard.done.ptrCardTitle')}
                  </h4>
                </div>
                <p className="text-[12.5px] leading-relaxed text-[var(--ink-secondary)]">
                  {t('admin.dnsWizard.done.ptrCardDesc')}
                </p>

                <div className="grid grid-cols-1 gap-2.5 sm:grid-cols-2">
                  <div className="rounded-[var(--radius-sm)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)]/70 p-3">
                    <div className="flex items-center justify-between gap-2">
                      <span className="text-[11px] font-medium text-[var(--ink-muted)]">
                        {t('admin.dnsWizard.done.ptrExpected')}
                      </span>
                      <button
                        type="button"
                        onClick={() => void copySingle(ptrRecord?.value || dns.mail_host)}
                        aria-label={t('common.copy')}
                        className="rounded p-1 text-[var(--ink-muted)] hover:bg-[var(--surface-hover)] hover:text-[var(--ink-primary)]"
                      >
                        <Copy className="h-3.5 w-3.5" />
                      </button>
                    </div>
                    <p className="mt-0.5 break-all font-mono text-[13px] font-medium text-[var(--ink-primary)]">
                      {ptrRecord?.value || dns.mail_host}
                    </p>
                  </div>

                  <div className="rounded-[var(--radius-sm)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)]/70 p-3">
                    <span className="text-[11px] font-medium text-[var(--ink-muted)]">
                      {t('admin.dnsWizard.done.ptrCurrent')}
                    </span>
                    <p className="mt-0.5 break-all font-mono text-[13px] text-[var(--ink-secondary)]">
                      {ptrRecord?.actual && ptrRecord.actual.length > 0
                        ? ptrRecord.actual.join(', ')
                        : t('admin.dnsWizard.noActualRecord')}
                    </p>
                  </div>
                </div>
              </div>
            </div>
          )}
        </div>

        {/* Footer Navigation */}
        <footer className="flex shrink-0 items-center justify-between gap-2 border-t border-[var(--border-hairline)] bg-[var(--surface-sidebar)] px-4 py-3 sm:px-6">
          <div>
            {step > 1 && (
              <Button
                variant="outline"
                size="sm"
                onClick={() => setStep((s) => Math.max(1, s - 1) as WizardStep)}
              >
                <ArrowLeft className="h-3.5 w-3.5" />
                {t('admin.dnsWizard.prevStep')}
              </Button>
            )}
          </div>

          <div className="flex items-center gap-2">
            {step === 1 && (
              <Button variant="primary" size="sm" onClick={() => setStep(2)}>
                {t('admin.dnsWizard.intro.startBtn')}
                <ArrowRight className="h-3.5 w-3.5" />
              </Button>
            )}
            {step === 2 && (
              <Button
                variant="primary"
                size="sm"
                onClick={() => {
                  setStep(3)
                  void runLiveCheck()
                }}
              >
                {t('admin.dnsWizard.nextStep')}
                <ArrowRight className="h-3.5 w-3.5" />
              </Button>
            )}
            {step === 3 && (
              <Button variant="primary" size="sm" onClick={() => setStep(4)}>
                {t('admin.dnsWizard.nextStep')}
                <ArrowRight className="h-3.5 w-3.5" />
              </Button>
            )}
            {step === 4 && (
              <Button variant="primary" size="sm" onClick={onClose}>
                <Check className="h-3.5 w-3.5" />
                {t('admin.dnsWizard.finish')}
              </Button>
            )}
          </div>
        </footer>
      </div>
    </div>
  )
}
