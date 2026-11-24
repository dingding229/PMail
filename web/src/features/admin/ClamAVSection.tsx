import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  AlertTriangle,
  CheckCircle2,
  Database,
  Info,
  Loader2,
  Lock,
  RefreshCw,
  Shield,
  ShieldAlert,
} from 'lucide-react'
import { adminInboundApi, type ClamAVStatus } from '@/api'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import {
  SectionCard,
  SectionErrorState,
  SectionFooter,
  SectionHeader,
  SectionLoadingState,
} from '@/components/settings'
import { formatMailFullTime } from '@/lib/date'

const statusKey = ['admin', 'inbound', 'clamav']

export function ClamAVSection() {
  const { t, i18n } = useTranslation()
  const client = useQueryClient()
  const { data, isLoading, error, refetch } = useQuery({
    queryKey: statusKey,
    queryFn: adminInboundApi.clamav,
    refetchInterval: (query) => (query.state.data?.state === 'disabled' ? 15_000 : 2_500),
  })

  const report = (err: Error) => toast.error(err.message)

  const toggle = useMutation({
    mutationFn: adminInboundApi.setClamAV,
    onMutate: () => client.cancelQueries({ queryKey: statusKey }),
    onSuccess: (status) => {
      client.setQueryData(statusKey, status)
    },
    onError: report,
  })

  const update = useMutation({
    mutationFn: adminInboundApi.updateClamAV,
    onSuccess: async () => {
      toast.success(t('admin.inbound.clamav.queued'))
      await client.invalidateQueries({ queryKey: statusKey })
    },
    onError: report,
  })

  const stateMeta: Record<
    ClamAVStatus['state'],
    {
      label: string
      variant: 'default' | 'accent' | 'success' | 'warning' | 'danger'
      spinning?: boolean
    }
  > = {
    disabled: { label: t('admin.inbound.clamav.disabled'), variant: 'default' },
    starting: { label: t('admin.inbound.clamav.starting'), variant: 'accent', spinning: true },
    downloading: { label: t('admin.inbound.clamav.downloading'), variant: 'accent', spinning: true },
    running: { label: t('admin.inbound.clamav.running'), variant: 'success' },
    error: { label: t('admin.inbound.clamav.error'), variant: 'danger' },
    stopping: { label: t('admin.inbound.clamav.stopping'), variant: 'warning', spinning: true },
  }

  const when = (date: string) => formatMailFullTime(date, i18n.language)
  const currentMeta = data ? stateMeta[data.state] : null

  return (
    <SectionCard aria-labelledby="clamav-label">
      <SectionHeader
        icon={ShieldAlert}
        iconTone={
          data?.state === 'running'
            ? 'success'
            : data?.state === 'error'
              ? 'danger'
              : 'accent'
        }
        title={t('admin.inbound.clamav.title')}
        titleId="clamav-label"
        badge={
          data && currentMeta ? (
            <div className="flex flex-wrap items-center gap-1.5">
              <Badge variant={currentMeta.variant}>
                {currentMeta.spinning ? (
                  <Loader2 className="h-3 w-3 animate-spin" />
                ) : data.state === 'running' ? (
                  <CheckCircle2 className="h-3 w-3" />
                ) : data.state === 'error' ? (
                  <AlertTriangle className="h-3 w-3" />
                ) : null}
                {currentMeta.label}
              </Badge>
              {data.updating && (
                <Badge variant="accent">
                  <Loader2 className="h-3 w-3 animate-spin" />
                  {t('admin.inbound.clamav.updating')}
                </Badge>
              )}
              {data.env_controlled && (
                <Badge variant="default">
                  <Lock className="h-3 w-3" />
                  {t('admin.inbound.envControlledBadge')}
                </Badge>
              )}
            </div>
          ) : undefined
        }
        action={
          <Switch
            id="clamav-enabled"
            aria-labelledby="clamav-label"
            aria-describedby={
              [
                !data?.enabled ? 'clamav-description' : null,
                data?.env_controlled ? 'clamav-env' : null,
              ]
                .filter(Boolean)
                .join(' ') || undefined
            }
            checked={data?.enabled ?? false}
            disabled={!data || data.env_controlled || toggle.isPending}
            onCheckedChange={(value) => toggle.mutate(value)}
          />
        }
      />

      {error && (
        <SectionErrorState
          message={error.message}
          retryLabel={t('app.retry')}
          onRetry={() => void refetch()}
        />
      )}

      {isLoading && !data ? (
        <SectionLoadingState rows={2} rowHeight="h-16" />
      ) : data ? (
        <div className="space-y-4">
          {/* Env-controlled disabled notice */}
          {data.env_controlled && (
            <div
              id="clamav-env"
              className="flex items-start gap-2.5 rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-subtle)] px-3.5 py-2.5 text-[12.5px] text-[var(--ink-secondary)]"
            >
              <Lock className="mt-0.5 h-4 w-4 shrink-0 text-[var(--ink-muted)]" />
              <div className="min-w-0 space-y-0.5">
                <p className="font-medium text-[var(--ink-primary)]">
                  {t('admin.inbound.clamav.env')}
                </p>
                <p className="text-[12px] text-[var(--ink-muted)]">
                  {t('admin.inbound.clamav.envLockedDesc')}
                </p>
              </div>
            </div>
          )}

          {/* Memory warning */}
          {data.memory_warning && (
            <div
              role="status"
              className="flex items-start gap-2.5 rounded-[var(--radius-md)] border border-[var(--warning)]/35 bg-[var(--warning-soft)] px-3.5 py-2.5 text-[12.5px] text-[var(--warning)]"
            >
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
              <span>{t('admin.inbound.clamav.memoryWarning')}</span>
            </div>
          )}

          {/* First-time activation note when not yet running */}
          {!data.enabled && (
            <div
              id="clamav-description"
              className="flex items-start gap-2.5 rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] px-3.5 py-3 text-[12.5px] text-[var(--ink-secondary)]"
            >
              <Info className="mt-0.5 h-4 w-4 shrink-0 text-[var(--accent)]" />
              <div className="min-w-0 space-y-0.5">
                <p className="font-medium text-[var(--ink-primary)]">
                  {t('admin.inbound.clamav.firstEnableTitle')}
                </p>
                <p className="text-[12px] leading-relaxed text-[var(--ink-muted)]">
                  {t('admin.inbound.clamav.firstEnableDesc')}
                </p>
              </div>
            </div>
          )}

          {/* Metrics Grid: Database, Last Update, Scanned, Blocked */}
          <dl className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div className="rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] px-4 py-3">
              <dt className="flex items-center gap-1.5 text-[11.5px] text-[var(--ink-muted)]">
                <Database className="h-3.5 w-3.5 shrink-0" />
                <span>{t('admin.inbound.clamav.database')}</span>
              </dt>
              <dd className="mt-1 font-mono text-[13px] font-medium text-[var(--ink-primary)]">
                {data.database
                  ? `${data.database.version} · ${when(data.database.updated_at)}`
                  : '—'}
              </dd>
            </div>

            <div className="rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] px-4 py-3">
              <dt className="flex items-center gap-1.5 text-[11.5px] text-[var(--ink-muted)]">
                <RefreshCw className="h-3.5 w-3.5 shrink-0" />
                <span>{t('admin.inbound.clamav.lastUpdate')}</span>
              </dt>
              <dd className="mt-1 flex flex-wrap items-center gap-1.5 text-[13px] font-medium text-[var(--ink-primary)]">
                {data.last_update ? (
                  <>
                    <Badge
                      variant={data.last_update.status === 'success' ? 'success' : 'danger'}
                    >
                      {data.last_update.status === 'success'
                        ? t('admin.inbound.clamav.success')
                        : t('admin.inbound.clamav.failed')}
                    </Badge>
                    <span className="text-[12px] font-normal tabular-nums text-[var(--ink-secondary)]">
                      {when(data.last_update.at)}
                    </span>
                  </>
                ) : (
                  '—'
                )}
              </dd>
            </div>

            <div className="rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] px-4 py-3">
              <dt className="flex items-center gap-1.5 text-[11.5px] text-[var(--ink-muted)]">
                <Shield className="h-3.5 w-3.5 shrink-0" />
                <span>{t('admin.inbound.clamav.scanned')}</span>
              </dt>
              <dd className="mt-1 font-mono text-[18px] font-semibold tabular-nums text-[var(--ink-primary)]">
                {data.scanned}
              </dd>
            </div>

            <div className="rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] px-4 py-3">
              <dt className="flex items-center gap-1.5 text-[11.5px] text-[var(--ink-muted)]">
                <ShieldAlert className="h-3.5 w-3.5 shrink-0" />
                <span>{t('admin.inbound.clamav.blocked')}</span>
              </dt>
              <dd
                className={`mt-1 font-mono text-[18px] font-semibold tabular-nums ${
                  data.blocked > 0 ? 'text-[var(--danger)]' : 'text-[var(--ink-primary)]'
                }`}
              >
                {data.blocked}
              </dd>
            </div>
          </dl>

          <SectionFooter>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={!data.enabled || data.updating || update.isPending || toggle.isPending}
              onClick={() => update.mutate()}
            >
              <RefreshCw
                className={`h-3.5 w-3.5 ${
                  data.updating || update.isPending ? 'animate-spin' : ''
                }`}
              />
              {t('admin.inbound.clamav.update')}
            </Button>
          </SectionFooter>
        </div>
      ) : null}
    </SectionCard>
  )
}
