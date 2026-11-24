import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import dayjs from 'dayjs'
import {
  AlertTriangle,
  BarChart3,
  ChevronLeft,
  ChevronRight,
  ExternalLink,
  Info,
  Lock,
  RefreshCw,
  ShieldAlert,
  ShieldCheck,
} from 'lucide-react'
import { adminMailReportsApi, domainApi, type MailReportSummaryResponse } from '@/api'
import { queryKeys } from '@/hooks/useMailQueries'
import { cn } from '@/lib/cn'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  DataTable,
  DataTableBody,
  DataTableCell,
  DataTableHead,
  DataTableHeaderCell,
  DataTableRow,
  SectionCard,
  SectionHeader,
  SectionLoadingState,
} from '@/components/settings'
import { StackedBarChart } from '@/components/charts/StackedBarChart'
import { HorizontalBarChart } from '@/components/charts/HorizontalBarChart'
import { computePassRate } from '@/components/charts/reportUtils'

type RangeDays = 7 | 30 | 90
const RANGE_OPTIONS: RangeDays[] = [7, 30, 90]

function formatRate(rate: number): string {
  return `${rate.toFixed(1)}%`
}

function resultBadgeVariant(result: string): 'success' | 'danger' | 'warning' | 'default' {
  const lower = (result || '').toLowerCase()
  if (lower === 'pass') return 'success'
  if (lower === 'fail') return 'danger'
  if (lower === 'softfail' || lower === 'temperror' || lower === 'permerror') return 'warning'
  return 'default'
}

export function MailReportsSection() {
  const { t } = useTranslation()
  const [selectedDomain, setSelectedDomain] = useState<string>('')
  const [rangeDays, setRangeDays] = useState<RangeDays>(30)
  const [recordsPage, setRecordsPage] = useState(1)

  const { data: domains = [] } = useQuery({
    queryKey: queryKeys.domains,
    queryFn: () => domainApi.list(),
  })

  const activeDomain = useMemo(() => {
    if (selectedDomain && domains.some((d) => d.name === selectedDomain)) {
      return selectedDomain
    }
    const def = domains.find((d) => d.is_default) ?? domains[0]
    return def?.name ?? ''
  }, [domains, selectedDomain])

  const dateRange = useMemo(() => {
    const to = dayjs().format('YYYY-MM-DD')
    const from = dayjs().subtract(rangeDays - 1, 'day').format('YYYY-MM-DD')
    return { from, to }
  }, [rangeDays])

  const summaryQuery = useQuery({
    queryKey: queryKeys.adminMailReportsSummary(activeDomain, dateRange.from, dateRange.to),
    queryFn: ({ signal }) =>
      adminMailReportsApi.summary(
        { domain: activeDomain, from: dateRange.from, to: dateRange.to },
        signal,
      ),
    enabled: Boolean(activeDomain || domains.length === 0),
  })

  const recordsQuery = useQuery({
    queryKey: queryKeys.adminMailReportsRecords(
      activeDomain,
      dateRange.from,
      dateRange.to,
      recordsPage,
    ),
    queryFn: ({ signal }) =>
      adminMailReportsApi.records(
        {
          domain: activeDomain,
          from: dateRange.from,
          to: dateRange.to,
          result: 'fail',
          page: recordsPage,
        },
        signal,
      ),
    enabled:
      Boolean(activeDomain || domains.length === 0) &&
      (summaryQuery.data?.report_count ?? 0) > 0,
  })

  const summary: MailReportSummaryResponse | undefined = summaryQuery.data
  const isEmpty =
    !summaryQuery.isLoading &&
    !summaryQuery.isError &&
    Boolean(summary) &&
    summary!.report_count === 0 &&
    summary!.totals.total === 0

  const dmarcRate = summary
    ? computePassRate(summary.totals.dmarc_pass, summary.totals.total, summary.totals.dmarc_pass_rate)
    : 0
  const spfRate = summary
    ? computePassRate(summary.totals.spf_pass, summary.totals.total, summary.totals.spf_pass_rate)
    : 0
  const dkimRate = summary
    ? computePassRate(summary.totals.dkim_pass, summary.totals.total, summary.totals.dkim_pass_rate)
    : 0

  const hasSuspicious = (summary?.sources ?? []).some((s) => s.suspicious)
  const latestPolicy = summary?.latest_policy
  const policyP = (latestPolicy?.p ?? '').toLowerCase()
  const latestDate = summary?.latest_report_at || latestPolicy?.report_date || ''

  const tls = summary?.tls
  const hasTlsData = Boolean(tls && (tls.success_count > 0 || tls.failure_count > 0))

  const records = recordsQuery.data?.items ?? []
  const recordsTotal = recordsQuery.data?.total ?? 0
  const recordsPageSize = recordsQuery.data?.page_size ?? 10
  const totalPages = Math.max(1, Math.ceil(recordsTotal / recordsPageSize))

  const dailyChartAria = summary
    ? t('admin.mailReports.dailyChartAria', {
        days: summary.daily.length,
        pass: summary.totals.dmarc_pass,
        fail: Math.max(0, summary.totals.total - summary.totals.dmarc_pass),
      })
    : ''

  const reportersChartAria = summary
    ? t('admin.mailReports.reportersChartAria', {
        summary: summary.reporters
          .map((r) => `${r.org_name} ${r.count} (${formatRate(r.pass_rate)})`)
          .join(', '),
      })
    : ''

  return (
    <div className="space-y-5">
      {/* Top toolbar card */}
      <SectionCard>
        <SectionHeader
          icon={BarChart3}
          title={t('admin.mailReports.title')}
          description={t('admin.mailReports.desc')}
        />

        <div className="flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-center sm:justify-between">
          <div className="flex flex-wrap items-center gap-2.5">
            {domains.length > 0 && (
              <Select
                value={activeDomain}
                onValueChange={(val) => {
                  setSelectedDomain(val)
                  setRecordsPage(1)
                }}
              >
                <SelectTrigger
                  aria-label={t('admin.mailReports.domainSelectAria')}
                  className="h-8 w-[190px] text-[12.5px]"
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {domains.map((d) => (
                    <SelectItem key={d.id} value={d.name}>
                      {d.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}

            <div
              role="group"
              aria-label={t('admin.mailReports.rangeGroupAria')}
              className="inline-flex rounded-[var(--radius-sm)] border border-[var(--border-subtle)] bg-[var(--surface-subtle)] p-0.5"
            >
              {RANGE_OPTIONS.map((days) => {
                const active = rangeDays === days
                return (
                  <button
                    key={days}
                    type="button"
                    aria-pressed={active}
                    onClick={() => {
                      setRangeDays(days)
                      setRecordsPage(1)
                    }}
                    className={cn(
                      'rounded-[calc(var(--radius-sm)-2px)] px-2.5 py-1 text-[12px] font-medium transition-colors',
                      active
                        ? 'bg-[var(--surface-elevated)] text-[var(--ink-primary)] shadow-xs'
                        : 'text-[var(--ink-secondary)] hover:text-[var(--ink-primary)]',
                    )}
                  >
                    {t('admin.mailReports.rangeDays', { days })}
                  </button>
                )
              })}
            </div>
          </div>

          {!isEmpty && (latestDate || latestPolicy) && (
            <div className="flex flex-wrap items-center gap-2 text-[12px] text-[var(--ink-secondary)]">
              {latestDate && (
                <span>
                  {t('admin.mailReports.latestReportDate', { date: latestDate })}
                </span>
              )}
              {latestPolicy?.p && (
                <Badge
                  variant={
                    policyP === 'reject'
                      ? 'success'
                      : policyP === 'quarantine'
                        ? 'accent'
                        : 'warning'
                  }
                  className="font-mono"
                >
                  {`p=${latestPolicy.p}`}
                </Badge>
              )}
            </div>
          )}
        </div>

        {!isEmpty && policyP === 'none' && (
          <div className="flex items-start gap-2.5 rounded-[var(--radius-md)] border border-[var(--warning)]/40 bg-[var(--warning-soft)] px-3.5 py-2.5 text-[12.5px] text-[var(--ink-primary)]">
            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-[var(--warning)]" />
            <span>{t('admin.mailReports.policyNoneHint')}</span>
          </div>
        )}
      </SectionCard>

      {/* Loading state */}
      {summaryQuery.isLoading && (
        <SectionCard>
          <SectionLoadingState />
        </SectionCard>
      )}

      {/* Error state */}
      {summaryQuery.isError && (
        <SectionCard>
          <div className="flex flex-col items-center justify-center gap-3 py-8 text-center">
            <ShieldAlert className="h-8 w-8 text-[var(--danger)]" />
            <p className="text-[13.5px] font-medium text-[var(--ink-primary)]">
              {t('admin.mailReports.loadError')}
            </p>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => summaryQuery.refetch()}
            >
              <RefreshCw className="h-3.5 w-3.5" />
              {t('common.retry')}
            </Button>
          </div>
        </SectionCard>
      )}

      {/* Empty state */}
      {isEmpty && (
        <SectionCard>
          <div className="flex flex-col items-center justify-center gap-3 py-10 text-center">
            <div className="flex h-11 w-11 items-center justify-center rounded-full bg-[var(--surface-subtle)] text-[var(--ink-muted)]">
              <BarChart3 className="h-5 w-5" />
            </div>
            <div className="space-y-1.5">
              <h3 className="text-[14.5px] font-semibold text-[var(--ink-primary)]">
                {t('admin.mailReports.emptyTitle')}
              </h3>
              <p className="mx-auto max-w-lg text-[12.5px] leading-relaxed text-[var(--ink-secondary)]">
                {t('admin.mailReports.emptyDesc')}
              </p>
            </div>
            <div className="mt-1 rounded-[var(--radius-sm)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)]/70 px-3.5 py-2 font-mono text-[12px] text-[var(--ink-secondary)]">
              {`v=DMARC1; p=none; rua=mailto:dmarc@${activeDomain || 'example.com'}`}
            </div>
          </div>
        </SectionCard>
      )}

      {/* Main data view */}
      {!summaryQuery.isLoading && !summaryQuery.isError && summary && !isEmpty && (
        <>
          {/* 4 KPI cards: 2x2 on mobile, 4 in a row on desktop */}
          <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
            <KpiCard
              testKey="total"
              label={t('admin.mailReports.kpiTotal')}
              value={summary.totals.total.toLocaleString()}
              sub={t('admin.mailReports.kpiReportsCount', { count: summary.report_count })}
              warning={false}
            />
            <KpiCard
              testKey="dmarc"
              label={t('admin.mailReports.kpiDmarc')}
              value={formatRate(dmarcRate)}
              sub={`${summary.totals.dmarc_pass.toLocaleString()} / ${summary.totals.total.toLocaleString()}`}
              warning={summary.totals.total > 0 && dmarcRate < 98}
            />
            <KpiCard
              testKey="spf"
              label={t('admin.mailReports.kpiSpf')}
              value={formatRate(spfRate)}
              sub={`${summary.totals.spf_pass.toLocaleString()} / ${summary.totals.total.toLocaleString()}`}
              warning={summary.totals.total > 0 && spfRate < 98}
            />
            <KpiCard
              testKey="dkim"
              label={t('admin.mailReports.kpiDkim')}
              value={formatRate(dkimRate)}
              sub={`${summary.totals.dkim_pass.toLocaleString()} / ${summary.totals.total.toLocaleString()}`}
              warning={summary.totals.total > 0 && dkimRate < 98}
            />
          </div>

          {/* Daily trend stacked bar chart */}
          <SectionCard>
            <SectionHeader
              title={t('admin.mailReports.dailyTrendTitle')}
              description={t('admin.mailReports.dailyTrendDesc')}
            />
            <StackedBarChart
              data={summary.daily}
              ariaLabel={dailyChartAria}
              passLabel={t('admin.mailReports.legendPass')}
              failLabel={t('admin.mailReports.legendFail')}
              totalLabel={t('admin.mailReports.legendTotal')}
            />
          </SectionCard>

          {/* Source IPs */}
          <SectionCard>
            <SectionHeader
              title={t('admin.mailReports.sourcesTitle')}
              description={t('admin.mailReports.sourcesDesc')}
            />

            {hasSuspicious && (
              <div className="flex items-start gap-2.5 rounded-[var(--radius-md)] border border-[var(--danger)]/35 bg-[var(--danger-soft)] px-3.5 py-2.5 text-[12.5px] text-[var(--danger)]">
                <ShieldAlert className="mt-0.5 h-4 w-4 shrink-0" />
                <span>{t('admin.mailReports.suspiciousExplain')}</span>
              </div>
            )}

            {/* Mobile cards */}
            <div className="space-y-2.5 md:hidden">
              {summary.sources.map((src) => (
                <div
                  key={src.source_ip}
                  data-source-ip={src.source_ip}
                  data-suspicious={src.suspicious ? 'true' : 'false'}
                  className={cn(
                    'space-y-1.5 rounded-[var(--radius-md)] border p-3',
                    src.suspicious
                      ? 'border-[var(--danger)]/45 bg-[var(--danger-soft)]/70'
                      : 'border-[var(--border-hairline)] bg-[var(--surface-subtle)]/40',
                  )}
                >
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <span className="font-mono text-[13px] font-medium text-[var(--ink-primary)]">
                      {src.source_ip}
                    </span>
                    <div className="flex items-center gap-1.5">
                      {src.is_own && (
                        <Badge variant="success">{t('admin.mailReports.tagOwnServer')}</Badge>
                      )}
                      {src.suspicious && (
                        <Badge variant="danger">{t('admin.mailReports.tagSuspicious')}</Badge>
                      )}
                    </div>
                  </div>
                  <div className="flex items-center justify-between text-[12px] text-[var(--ink-secondary)]">
                    <span>
                      {t('admin.mailReports.colCount')}:{' '}
                      <strong className="font-mono text-[var(--ink-primary)]">{src.count}</strong>
                    </span>
                    <span
                      className={cn(
                        'font-mono font-medium',
                        src.pass_rate < 98 ? 'text-[var(--warning)]' : 'text-[var(--success)]',
                      )}
                    >
                      {formatRate(src.pass_rate)}
                    </span>
                  </div>
                </div>
              ))}
            </div>

            {/* Desktop table */}
            <DataTable className="hidden md:block">
              <DataTableHead>
                <tr>
                  <DataTableHeaderCell>{t('admin.mailReports.colIp')}</DataTableHeaderCell>
                  <DataTableHeaderCell align="right">
                    {t('admin.mailReports.colCount')}
                  </DataTableHeaderCell>
                  <DataTableHeaderCell align="right">
                    {t('admin.mailReports.colPassRate')}
                  </DataTableHeaderCell>
                  <DataTableHeaderCell>{t('admin.mailReports.colTag')}</DataTableHeaderCell>
                </tr>
              </DataTableHead>
              <DataTableBody>
                {summary.sources.map((src) => (
                  <DataTableRow
                    key={src.source_ip}
                    data-source-ip={src.source_ip}
                    data-suspicious={src.suspicious ? 'true' : 'false'}
                    className={cn(src.suspicious && 'bg-[var(--danger-soft)]/70')}
                  >
                    <DataTableCell className="font-mono text-[12.5px] font-medium text-[var(--ink-primary)]">
                      {src.source_ip}
                    </DataTableCell>
                    <DataTableCell
                      align="right"
                      className="font-mono text-[12.5px] text-[var(--ink-secondary)]"
                    >
                      {src.count.toLocaleString()}
                    </DataTableCell>
                    <DataTableCell
                      align="right"
                      className={cn(
                        'font-mono text-[12.5px] font-medium',
                        src.pass_rate < 98 ? 'text-[var(--warning)]' : 'text-[var(--success)]',
                      )}
                    >
                      {formatRate(src.pass_rate)}
                    </DataTableCell>
                    <DataTableCell>
                      <div className="flex flex-wrap items-center gap-1.5">
                        {src.is_own && (
                          <Badge variant="success">{t('admin.mailReports.tagOwnServer')}</Badge>
                        )}
                        {src.suspicious && (
                          <Badge variant="danger">{t('admin.mailReports.tagSuspicious')}</Badge>
                        )}
                        {!src.is_own && !src.suspicious && (
                          <span className="text-[12px] text-[var(--ink-muted)]">—</span>
                        )}
                      </div>
                    </DataTableCell>
                  </DataTableRow>
                ))}
              </DataTableBody>
            </DataTable>
          </SectionCard>

          {/* Reporters distribution + TLS reports */}
          <div className="grid grid-cols-1 gap-5 lg:grid-cols-2">
            <SectionCard>
              <SectionHeader
                title={t('admin.mailReports.reportersTitle')}
                description={t('admin.mailReports.reportersDesc')}
              />
              {summary.reporters.length > 0 ? (
                <HorizontalBarChart
                  items={summary.reporters.map((r) => ({
                    label: r.org_name,
                    count: r.count,
                    passRate: r.pass_rate,
                  }))}
                  ariaLabel={reportersChartAria}
                />
              ) : (
                <p className="py-4 text-[12.5px] text-[var(--ink-muted)]">
                  {t('admin.mailReports.noReporters')}
                </p>
              )}
            </SectionCard>

            <SectionCard>
              <SectionHeader
                icon={Lock}
                title={t('admin.mailReports.tlsTitle')}
                description={t('admin.mailReports.tlsDesc')}
              />
              {hasTlsData && tls ? (
                <div className="space-y-4">
                  <div className="grid grid-cols-2 gap-3">
                    <div className="rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)]/50 p-3">
                      <div className="text-[12px] text-[var(--ink-secondary)]">
                        {t('admin.mailReports.tlsSuccess')}
                      </div>
                      <div className="mt-1 font-mono text-[20px] font-semibold text-[var(--success)]">
                        {tls.success_count.toLocaleString()}
                      </div>
                    </div>
                    <div className="rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)]/50 p-3">
                      <div className="text-[12px] text-[var(--ink-secondary)]">
                        {t('admin.mailReports.tlsFailure')}
                      </div>
                      <div
                        className={cn(
                          'mt-1 font-mono text-[20px] font-semibold',
                          tls.failure_count > 0 ? 'text-[var(--danger)]' : 'text-[var(--ink-primary)]',
                        )}
                      >
                        {tls.failure_count.toLocaleString()}
                      </div>
                    </div>
                  </div>

                  {tls.failure_types && tls.failure_types.length > 0 && (
                    <div className="space-y-1.5">
                      <div className="text-[12px] font-medium text-[var(--ink-secondary)]">
                        {t('admin.mailReports.tlsFailureTypes')}
                      </div>
                      <ul className="space-y-1.5">
                        {tls.failure_types.map((ft) => (
                          <li
                            key={ft.result_type}
                            className="flex items-center justify-between rounded-[var(--radius-sm)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)]/40 px-3 py-1.5 text-[12px]"
                          >
                            <span className="font-mono text-[var(--ink-primary)]">
                              {ft.result_type}
                            </span>
                            <Badge variant="danger" className="font-mono">
                              {ft.count}
                            </Badge>
                          </li>
                        ))}
                      </ul>
                    </div>
                  )}
                </div>
              ) : (
                <div className="flex items-center gap-2 rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)]/40 px-3.5 py-5 text-[12.5px] text-[var(--ink-muted)]">
                  <Info className="h-4 w-4 shrink-0" />
                  <span>{t('admin.mailReports.tlsEmpty')}</span>
                </div>
              )}
            </SectionCard>
          </div>

          {/* Failure details paginated table */}
          <SectionCard>
            <SectionHeader
              title={t('admin.mailReports.failuresTitle')}
              description={t('admin.mailReports.failuresDesc')}
            />

            {recordsQuery.isLoading ? (
              <SectionLoadingState />
            ) : records.length === 0 ? (
              <div className="flex items-center gap-2 rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)]/40 px-3.5 py-5 text-[12.5px] text-[var(--ink-secondary)]">
                <ShieldCheck className="h-4 w-4 shrink-0 text-[var(--success)]" />
                <span>{t('admin.mailReports.failuresEmpty')}</span>
              </div>
            ) : (
              <>
                {/* Mobile cards */}
                <div className="space-y-2.5 md:hidden">
                  {records.map((rec) => (
                    <div
                      key={rec.id}
                      className="space-y-2 rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)]/40 p-3 text-[12px]"
                    >
                      <div className="flex items-center justify-between gap-2">
                        <span className="font-mono font-medium text-[var(--ink-primary)]">
                          {rec.source_ip}
                        </span>
                        <span className="font-mono text-[var(--ink-muted)]">{rec.date}</span>
                      </div>
                      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-[var(--ink-secondary)]">
                        <span>
                          {t('admin.mailReports.colCount')}:{' '}
                          <strong className="font-mono text-[var(--ink-primary)]">
                            {rec.count}
                          </strong>
                        </span>
                        <span>
                          {t('admin.mailReports.colHeaderFrom')}:{' '}
                          <span className="font-mono">{rec.header_from}</span>
                        </span>
                        <span>{rec.org_name}</span>
                      </div>
                      <div className="flex flex-wrap items-center justify-between gap-2 pt-0.5">
                        <div className="flex items-center gap-1.5">
                          <Badge variant={resultBadgeVariant(rec.dkim_result)} className="font-mono">
                            {`DKIM: ${rec.dkim_result}`}
                          </Badge>
                          <Badge variant={resultBadgeVariant(rec.spf_result)} className="font-mono">
                            {`SPF: ${rec.spf_result}`}
                          </Badge>
                        </div>
                        {rec.email_id && (
                          <Button asChild variant="outline" size="xs">
                            <Link to={`/mail/inbox?mail=${rec.email_id}`}>
                              <ExternalLink className="h-3 w-3" />
                              {t('admin.mailReports.openSourceEmail')}
                            </Link>
                          </Button>
                        )}
                      </div>
                    </div>
                  ))}
                </div>

                {/* Desktop table */}
                <DataTable className="hidden md:block">
                  <DataTableHead>
                    <tr>
                      <DataTableHeaderCell>{t('admin.mailReports.colDate')}</DataTableHeaderCell>
                      <DataTableHeaderCell>{t('admin.mailReports.colIp')}</DataTableHeaderCell>
                      <DataTableHeaderCell align="right">
                        {t('admin.mailReports.colCount')}
                      </DataTableHeaderCell>
                      <DataTableHeaderCell>
                        {t('admin.mailReports.colHeaderFrom')}
                      </DataTableHeaderCell>
                      <DataTableHeaderCell>
                        {t('admin.mailReports.colAuthResults')}
                      </DataTableHeaderCell>
                      <DataTableHeaderCell>
                        {t('admin.mailReports.colReporter')}
                      </DataTableHeaderCell>
                      <DataTableHeaderCell align="right">
                        {t('admin.mailReports.colSourceEmail')}
                      </DataTableHeaderCell>
                    </tr>
                  </DataTableHead>
                  <DataTableBody>
                    {records.map((rec) => (
                      <DataTableRow key={rec.id}>
                        <DataTableCell className="whitespace-nowrap font-mono text-[12px] text-[var(--ink-secondary)]">
                          {rec.date}
                        </DataTableCell>
                        <DataTableCell className="whitespace-nowrap font-mono text-[12.5px] font-medium text-[var(--ink-primary)]">
                          {rec.source_ip}
                        </DataTableCell>
                        <DataTableCell
                          align="right"
                          className="font-mono text-[12.5px] text-[var(--ink-primary)]"
                        >
                          {rec.count}
                        </DataTableCell>
                        <DataTableCell className="font-mono text-[12px] text-[var(--ink-secondary)]">
                          {rec.header_from}
                        </DataTableCell>
                        <DataTableCell>
                          <div className="flex flex-wrap items-center gap-1.5">
                            <Badge
                              variant={resultBadgeVariant(rec.dkim_result)}
                              className="font-mono"
                            >
                              {`DKIM: ${rec.dkim_result}`}
                            </Badge>
                            <Badge
                              variant={resultBadgeVariant(rec.spf_result)}
                              className="font-mono"
                            >
                              {`SPF: ${rec.spf_result}`}
                            </Badge>
                          </div>
                        </DataTableCell>
                        <DataTableCell className="whitespace-nowrap text-[12.5px] text-[var(--ink-secondary)]">
                          {rec.org_name}
                        </DataTableCell>
                        <DataTableCell align="right">
                          {rec.email_id ? (
                            <Button asChild variant="outline" size="xs">
                              <Link to={`/mail/inbox?mail=${rec.email_id}`}>
                                <ExternalLink className="h-3 w-3" />
                                {t('admin.mailReports.openSourceEmail')}
                              </Link>
                            </Button>
                          ) : (
                            <span className="text-[12px] text-[var(--ink-muted)]">—</span>
                          )}
                        </DataTableCell>
                      </DataTableRow>
                    ))}
                  </DataTableBody>
                </DataTable>

                {totalPages > 1 && (
                  <div className="flex items-center justify-between pt-2 text-[12px] text-[var(--ink-secondary)]">
                    <span>
                      {t('admin.mailReports.pageInfo', {
                        page: recordsPage,
                        totalPages,
                        total: recordsTotal,
                      })}
                    </span>
                    <div className="flex items-center gap-1.5">
                      <Button
                        type="button"
                        variant="outline"
                        size="xs"
                        disabled={recordsPage <= 1}
                        onClick={() => setRecordsPage((p) => Math.max(1, p - 1))}
                      >
                        <ChevronLeft className="h-3.5 w-3.5" />
                        {t('common.prev')}
                      </Button>
                      <Button
                        type="button"
                        variant="outline"
                        size="xs"
                        disabled={recordsPage >= totalPages}
                        onClick={() => setRecordsPage((p) => Math.min(totalPages, p + 1))}
                      >
                        {t('common.next')}
                        <ChevronRight className="h-3.5 w-3.5" />
                      </Button>
                    </div>
                  </div>
                )}
              </>
            )}
          </SectionCard>
        </>
      )}
    </div>
  )
}

function KpiCard({
  testKey,
  label,
  value,
  sub,
  warning,
}: {
  testKey: string
  label: string
  value: string
  sub: string
  warning: boolean
}) {
  return (
    <div
      data-kpi={testKey}
      data-warning={warning ? 'true' : 'false'}
      className={cn(
        'rounded-[var(--radius-lg)] border p-4 transition-colors',
        warning
          ? 'border-[var(--warning)]/55 bg-[var(--warning-soft)]/50'
          : 'border-[var(--border-subtle)] bg-[var(--surface-elevated)]',
      )}
    >
      <div className="flex items-center justify-between gap-1.5 text-[12px] font-medium text-[var(--ink-secondary)]">
        <span>{label}</span>
        {warning && <AlertTriangle className="h-3.5 w-3.5 shrink-0 text-[var(--warning)]" />}
      </div>
      <div
        className={cn(
          'mt-1.5 font-mono text-[22px] font-semibold tracking-tight',
          warning ? 'text-[var(--warning)]' : 'text-[var(--ink-primary)]',
        )}
      >
        {value}
      </div>
      <div className="mt-1 font-mono text-[11.5px] text-[var(--ink-muted)]">{sub}</div>
    </div>
  )
}
