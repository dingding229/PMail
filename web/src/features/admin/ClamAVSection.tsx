import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { adminInboundApi } from '@/api'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { formatMailFullTime } from '@/lib/date'

const statusKey = ['admin', 'inbound', 'clamav']

export function ClamAVSection() {
  const { t, i18n } = useTranslation()
  const client = useQueryClient()
  const { data, error } = useQuery({
    queryKey: statusKey,
    queryFn: adminInboundApi.clamav,
    refetchInterval: (query) => query.state.data?.state === 'disabled' ? 15_000 : 2_500,
  })
  const report = (err: Error) => toast.error(err.message)
  const toggle = useMutation({
    mutationFn: adminInboundApi.setClamAV,
    onMutate: () => client.cancelQueries({ queryKey: statusKey }),
    onSuccess: (status) => { client.setQueryData(statusKey, status) },
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
  const states = {
    disabled: t('admin.inbound.clamav.disabled'),
    starting: t('admin.inbound.clamav.starting'),
    downloading: t('admin.inbound.clamav.downloading'),
    running: t('admin.inbound.clamav.running'),
    error: t('admin.inbound.clamav.error'),
    stopping: t('admin.inbound.clamav.stopping'),
  }
  const when = (date: string) => formatMailFullTime(date, i18n.language)

  return (
    <section aria-labelledby="clamav-label" className="space-y-3 rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] p-4">
      <div className="flex items-start justify-between gap-4">
        <div>
          <Label id="clamav-label" htmlFor="clamav-enabled">{t('admin.inbound.clamav.title')}</Label>
          <p id="clamav-description" className="mt-1 text-[12px] text-[var(--ink-muted)]">{t('admin.inbound.clamav.description')}</p>
          {data?.env_controlled && <p id="clamav-env" className="mt-1 text-[12px] text-[var(--ink-secondary)]">{t('admin.inbound.clamav.env')}</p>}
        </div>
        <Switch id="clamav-enabled" aria-describedby={data?.env_controlled ? 'clamav-description clamav-env' : 'clamav-description'}
          checked={data?.enabled ?? false} disabled={!data || data.env_controlled || toggle.isPending} onCheckedChange={(value) => toggle.mutate(value)} />
      </div>
      {error && <p className="text-[12px] text-[var(--danger)]" role="alert">{error.message}</p>}
      {data && (
        <>
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant={data.state === 'running' ? 'success' : data.state === 'error' ? 'danger' : 'default'}>{states[data.state]}</Badge>
            {data.updating && <Badge variant="default">{t('admin.inbound.clamav.updating')}</Badge>}
          </div>
          {data.memory_warning && <p className="rounded-[var(--radius-sm)] bg-[var(--warning-soft)] px-3 py-2 text-[12px] text-[var(--warning)]" role="status">{t('admin.inbound.clamav.memoryWarning')}</p>}
          <dl className="grid gap-2 text-[12px] text-[var(--ink-secondary)] sm:grid-cols-2">
            <div><dt>{t('admin.inbound.clamav.database')}</dt><dd className="font-medium">{data.database ? `${data.database.version} · ${when(data.database.updated_at)}` : '—'}</dd></div>
            <div><dt>{t('admin.inbound.clamav.lastUpdate')}</dt><dd className="font-medium">{data.last_update ? `${data.last_update.status === 'success' ? t('admin.inbound.clamav.success') : t('admin.inbound.clamav.failed')} · ${when(data.last_update.at)}` : '—'}</dd></div>
            <div><dt>{t('admin.inbound.clamav.scanned')}</dt><dd className="font-medium tabular-nums">{data.scanned}</dd></div>
            <div><dt>{t('admin.inbound.clamav.blocked')}</dt><dd className="font-medium tabular-nums">{data.blocked}</dd></div>
          </dl>
          <Button type="button" variant="outline" size="sm" disabled={!data.enabled || data.updating || update.isPending || toggle.isPending} onClick={() => update.mutate()}>
            {t('admin.inbound.clamav.update')}
          </Button>
        </>
      )}
    </section>
  )
}
