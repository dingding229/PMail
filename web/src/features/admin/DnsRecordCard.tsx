import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  AlertCircle,
  AlertTriangle,
  Check,
  CheckCircle2,
  ChevronDown,
  ChevronUp,
  Copy,
  Info,
  Lightbulb,
  Scissors,
  XCircle,
} from 'lucide-react'
import type { DnsRecord, DnsRecordStatus } from '@/api'
import { cn } from '@/lib/cn'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'

interface DnsRecordCardProps {
  record: DnsRecord
  checking?: boolean
}

function CopyableField({
  label,
  value,
  mono = true,
}: {
  label: string
  value: string
  mono?: boolean
}) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(value)
      setCopied(true)
      toast.success(t('common.copied'))
      window.setTimeout(() => setCopied(false), 1500)
    } catch {
      toast.error(t('common.copyFailed'))
    }
  }

  return (
    <div className="min-w-0 rounded-[var(--radius-sm)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)]/65 px-2.5 py-2">
      <div className="flex items-center justify-between gap-2">
        <span className="text-[11px] font-medium text-[var(--ink-muted)]">{label}</span>
        <button
          type="button"
          onClick={() => void handleCopy()}
          aria-label={`${t('common.copy')} ${label}`}
          title={t('common.copy')}
          className="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded text-[var(--ink-muted)] transition-colors hover:bg-[var(--surface-hover)] hover:text-[var(--ink-primary)]"
        >
          {copied ? (
            <Check className="h-3.5 w-3.5 text-[var(--success)]" />
          ) : (
            <Copy className="h-3.5 w-3.5" />
          )}
        </button>
      </div>
      <div
        className={cn(
          'mt-0.5 break-all text-[12.5px] text-[var(--ink-primary)] select-all',
          mono && 'font-mono',
        )}
      >
        {value}
      </div>
    </div>
  )
}

/**
 * Computes a merged SPF record when an existing SPF record is present in `actual`:
 * preserves existing mechanisms (e.g. include:...), inserts `mx` and `a` if absent,
 * and preserves the existing `all` qualifier (~all / -all / ?all).
 */
function computeMergedSpf(record: DnsRecord): string | null {
  if (record.suggested_value && record.suggested_value.trim()) {
    return record.suggested_value.trim()
  }
  const isSpf =
    record.key === 'spf' || record.value.trim().toLowerCase().startsWith('v=spf1')
  if (!isSpf || record.status !== 'mismatch' || !record.actual?.length) return null

  const existingSpf = record.actual.find((v) =>
    v.trim().toLowerCase().startsWith('v=spf1'),
  )
  if (!existingSpf) return null

  const tokens = existingSpf.trim().split(/\s+/).filter(Boolean)
  const rest = tokens.slice(1)
  const allIdx = rest.findIndex((tok) => /^[~?+-]?all$/i.test(tok))
  const allToken = allIdx >= 0 ? rest[allIdx]! : '~all'
  const mechanisms = allIdx >= 0 ? rest.filter((_, i) => i !== allIdx) : rest

  const lowerSet = new Set(mechanisms.map((m) => m.toLowerCase()))
  const toInsert: string[] = []
  if (!lowerSet.has('mx') && !lowerSet.has('+mx')) toInsert.push('mx')
  if (!lowerSet.has('a') && !lowerSet.has('+a')) toInsert.push('a')

  return ['v=spf1', ...toInsert, ...mechanisms, allToken].join(' ')
}

export function DnsRecordCard({ record, checking = false }: DnsRecordCardProps) {
  const { t } = useTranslation()
  const isLongValue = record.value.length > 140
  const hasChunks = Array.isArray(record.value_chunks) && record.value_chunks.length > 1
  const mergedSpfValue = computeMergedSpf(record)

  const [expanded, setExpanded] = useState(!isLongValue)
  const [showChunks, setShowChunks] = useState(false)
  const [copiedValue, setCopiedValue] = useState(false)
  const [copiedMerged, setCopiedMerged] = useState(false)

  const statusConfig: Record<
    DnsRecordStatus,
    { label: string; variant: 'success' | 'warning' | 'danger' | 'accent'; icon: typeof CheckCircle2 }
  > = {
    ok: { label: t('admin.dnsWizard.status.ok'), variant: 'success', icon: CheckCircle2 },
    missing: { label: t('admin.dnsWizard.status.missing'), variant: 'warning', icon: AlertTriangle },
    mismatch: { label: t('admin.dnsWizard.status.mismatch'), variant: 'danger', icon: XCircle },
    error: { label: t('admin.dnsWizard.status.error'), variant: 'danger', icon: AlertCircle },
    advice: { label: t('admin.dnsWizard.status.advice'), variant: 'accent', icon: Info },
  }

  const meta = statusConfig[record.status] ?? statusConfig.missing
  const StatusIcon = meta.icon

  const copyText = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text)
      setCopiedValue(true)
      toast.success(t('common.copied'))
      window.setTimeout(() => setCopiedValue(false), 1500)
    } catch {
      toast.error(t('common.copyFailed'))
    }
  }

  const displayedValue =
    isLongValue && !expanded ? `${record.value.slice(0, 128)}…` : record.value

  return (
    <div
      className={cn(
        'rounded-[var(--radius-md)] border bg-[var(--surface-canvas)] p-4 transition-all duration-200 space-y-3',
        checking
          ? 'border-[var(--accent)]/60 ring-2 ring-[var(--accent-ring)]'
          : record.status === 'ok'
            ? 'border-[var(--success)]/30'
            : record.status === 'mismatch' || record.status === 'error'
              ? 'border-[var(--danger)]/35'
              : 'border-[var(--border-subtle)]',
      )}
    >
      {/* Header Row */}
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="flex flex-wrap items-center gap-2 min-w-0">
          <span className="inline-flex items-center rounded-[5px] bg-[var(--ink-primary)] px-2 py-0.5 font-mono text-[11px] font-semibold text-[var(--ink-inverse)]">
            {record.type}
          </span>
          <span className="text-[13.5px] font-semibold text-[var(--ink-primary)]">
            {record.purpose}
          </span>
          <span className="break-all font-mono text-[11.5px] text-[var(--ink-muted)]">
            {record.fqdn}
          </span>
        </div>

        <Badge variant={meta.variant} className="shrink-0">
          <StatusIcon className="h-3 w-3" />
          {meta.label}
        </Badge>
      </div>

      {/* Localized Description */}
      {record.description && (
        <p className="text-[12.5px] leading-relaxed text-[var(--ink-secondary)]">
          {record.description}
        </p>
      )}

      {/* Copyable DNS Fields Grid */}
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
        <CopyableField label={t('admin.dnsWizard.fields.type')} value={record.type} />
        <CopyableField label={t('admin.dnsWizard.fields.host')} value={record.host || '@'} />
        {record.priority !== undefined && record.priority > 0 && (
          <CopyableField
            label={t('admin.dnsWizard.fields.priority')}
            value={String(record.priority)}
          />
        )}
        <CopyableField label={t('admin.dnsWizard.fields.ttl')} value={String(record.ttl || 3600)} />
      </div>

      {/* Record Value (with long DKIM collapse + 255-byte chunks switcher) */}
      <div className="rounded-[var(--radius-sm)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)]/65 p-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <span className="text-[11px] font-medium text-[var(--ink-muted)]">
            {t('admin.dnsWizard.fields.value')}
          </span>
          <div className="flex flex-wrap items-center gap-1">
            {hasChunks && (
              <Button
                type="button"
                variant="ghost"
                size="xs"
                onClick={() => setShowChunks((v) => !v)}
              >
                <Scissors className="h-3 w-3" />
                {showChunks
                  ? t('admin.dnsWizard.dkimHideChunks')
                  : t('admin.dnsWizard.dkimShowChunks', {
                      count: record.value_chunks?.length ?? 0,
                    })}
              </Button>
            )}
            {isLongValue && !showChunks && (
              <Button
                type="button"
                variant="ghost"
                size="xs"
                onClick={() => setExpanded((v) => !v)}
              >
                {expanded ? (
                  <>
                    <ChevronUp className="h-3 w-3" />
                    {t('admin.dnsWizard.dkimCollapse')}
                  </>
                ) : (
                  <>
                    <ChevronDown className="h-3 w-3" />
                    {t('admin.dnsWizard.dkimExpand')}
                  </>
                )}
              </Button>
            )}
            <Button
              type="button"
              variant="secondary"
              size="xs"
              onClick={() => void copyText(record.value)}
            >
              {copiedValue ? (
                <Check className="h-3 w-3 text-[var(--success)]" />
              ) : (
                <Copy className="h-3 w-3" />
              )}
              {t('common.copy')}
            </Button>
          </div>
        </div>

        {showChunks && record.value_chunks ? (
          <div className="mt-2 space-y-2">
            <p className="text-[11.5px] text-[var(--ink-muted)]">
              {t('admin.dnsWizard.dkimChunkHint')}
            </p>
            {record.value_chunks.map((chunk, i) => (
              <div
                key={i}
                className="flex items-start justify-between gap-2 rounded border border-[var(--border-hairline)] bg-[var(--surface-canvas)] p-2"
              >
                <code className="min-w-0 flex-1 break-all font-mono text-[11.5px] text-[var(--ink-primary)] select-all">
                  &quot;{chunk}&quot;
                </code>
                <button
                  type="button"
                  onClick={() => void copyText(chunk)}
                  aria-label={t('common.copy')}
                  className="shrink-0 rounded p-1 text-[var(--ink-muted)] hover:bg-[var(--surface-hover)] hover:text-[var(--ink-primary)]"
                >
                  <Copy className="h-3.5 w-3.5" />
                </button>
              </div>
            ))}
          </div>
        ) : (
          <pre className="mt-1.5 overflow-x-auto whitespace-pre-wrap break-all font-mono text-[12px] leading-relaxed text-[var(--ink-primary)] select-all">
            {displayedValue}
          </pre>
        )}
      </div>

      {/* Side-by-Side Expected vs Actual Comparison on Mismatch / Error */}
      {(record.status === 'mismatch' || record.status === 'error') && (
        <div className="space-y-2.5">
          <div className="grid grid-cols-1 gap-2.5 sm:grid-cols-2">
            <div className="rounded-[var(--radius-sm)] border border-[var(--danger)]/30 bg-[var(--danger-soft)]/60 p-2.5">
              <p className="text-[11px] font-semibold text-[var(--danger)]">
                {t('admin.dnsWizard.actualValue')}
              </p>
              <div className="mt-1 space-y-1 font-mono text-[11.5px] break-all text-[var(--ink-primary)]">
                {record.actual && record.actual.length > 0 ? (
                  record.actual.map((a, idx) => <div key={idx}>{a}</div>)
                ) : (
                  <span className="text-[var(--ink-muted)]">
                    {t('admin.dnsWizard.noActualRecord')}
                  </span>
                )}
              </div>
            </div>

            <div className="rounded-[var(--radius-sm)] border border-[var(--success)]/30 bg-[var(--success-soft)]/50 p-2.5">
              <p className="text-[11px] font-semibold text-[var(--success)]">
                {t('admin.dnsWizard.expectedValue')}
              </p>
              <div className="mt-1 font-mono text-[11.5px] break-all text-[var(--ink-primary)]">
                {record.value}
              </div>
            </div>
          </div>

          {/* SPF Merged Suggested Value Box */}
          {mergedSpfValue && (
            <div className="rounded-[var(--radius-sm)] border border-[var(--accent)]/35 bg-[var(--accent-soft)]/55 p-3">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <span className="text-[11.5px] font-semibold text-[var(--accent)]">
                  {t('admin.dnsWizard.suggestedMergedSpf')}
                </span>
                <Button
                  type="button"
                  variant="secondary"
                  size="xs"
                  onClick={async () => {
                    try {
                      await navigator.clipboard.writeText(mergedSpfValue)
                      setCopiedMerged(true)
                      toast.success(t('common.copied'))
                      window.setTimeout(() => setCopiedMerged(false), 1500)
                    } catch {
                      toast.error(t('common.copyFailed'))
                    }
                  }}
                >
                  {copiedMerged ? (
                    <Check className="h-3 w-3 text-[var(--success)]" />
                  ) : (
                    <Copy className="h-3 w-3" />
                  )}
                  {t('common.copy')}
                </Button>
              </div>
              <pre className="mt-1.5 overflow-x-auto whitespace-pre-wrap break-all font-mono text-[12px] text-[var(--ink-primary)] select-all">
                {mergedSpfValue}
              </pre>
            </div>
          )}
        </div>
      )}

      {/* Fix Hint Callout */}
      {record.fix_hint && record.status !== 'ok' && (
        <div className="flex items-start gap-2 rounded-[var(--radius-sm)] border border-[var(--warning)]/25 bg-[var(--warning-soft)]/70 px-3 py-2 text-[12px] leading-relaxed text-[var(--ink-primary)]">
          <Lightbulb className="mt-0.5 h-3.5 w-3.5 shrink-0 text-[var(--warning)]" />
          <span>{record.fix_hint}</span>
        </div>
      )}
    </div>
  )
}
