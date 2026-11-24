import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  AlertCircle,
  RefreshCw,
  RotateCcw,
  Search,
  SendToBack,
  Trash2,
} from 'lucide-react'
import { queueApi, type QueueItem, type QueueStatus } from '@/api'
import { formatMailFullTime, pickDate } from '@/lib/date'
import { cn } from '@/lib/cn'
import { queryKeys, useUserMeQuery } from '@/hooks/useMailQueries'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
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

interface QueueSectionProps {
  /** 'mine' filters to current user inside Settings; 'global' shows all users inside Admin */
  scope: 'mine' | 'global'
}

export function QueueSection({ scope }: QueueSectionProps) {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  const { data: me } = useUserMeQuery(true)

  const { data: allItems = [], isLoading, isFetching, refetch } = useQuery({
    queryKey: queryKeys.queue,
    queryFn: () => queueApi.list(),
  })

  const [statusFilter, setStatusFilter] = useState<'all' | QueueStatus>('all')
  const [search, setSearch] = useState('')
  const [deleting, setDeleting] = useState<QueueItem | null>(null)

  const items = useMemo(() => {
    const q = search.trim().toLowerCase()
    return allItems
      .filter((item) => {
        if (scope === 'mine' && me && item.user_id !== me.id) return false
        if (statusFilter !== 'all' && item.status !== statusFilter) return false
        if (
          q &&
          !item.mail_from.toLowerCase().includes(q) &&
          !item.rcpt_to.toLowerCase().includes(q) &&
          !item.last_error.toLowerCase().includes(q)
        ) {
          return false
        }
        return true
      })
      .sort((a, b) =>
        pickDate(b.updated_iso, b.updated) > pickDate(a.updated_iso, a.updated) ? 1 : -1,
      )
  }, [allItems, scope, me, statusFilter, search])

  const retryMutation = useMutation({
    mutationFn: (id: number) => queueApi.retry(id),
    onSuccess: () => {
      toast.success(t('settings.queue.retried'))
      void queryClient.invalidateQueries({ queryKey: queryKeys.queue })
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: number) => queueApi.remove(id),
    onSuccess: () => {
      toast.success(t('settings.queue.deleted'))
      setDeleting(null)
      void queryClient.invalidateQueries({ queryKey: queryKeys.queue })
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const statusMeta: Record<
    QueueStatus,
    { label: string; variant: 'default' | 'accent' | 'success' | 'warning' | 'danger' }
  > = {
    pending: { label: t('settings.queue.statusPending'), variant: 'warning' },
    sending: { label: t('settings.queue.statusSending'), variant: 'accent' },
    sent: { label: t('settings.queue.statusSent'), variant: 'success' },
    failed: { label: t('settings.queue.statusFailed'), variant: 'danger' },
  }

  const kindLabel = (kind: QueueItem['kind']) => {
    if (kind === 'forward') return t('settings.queue.kindForward')
    if (kind === 'bounce') return t('settings.queue.kindBounce')
    return t('settings.queue.kindSend')
  }

  const filterTabs: { key: 'all' | QueueStatus; label: string }[] = [
    { key: 'all', label: t('settings.queue.filterAll') },
    { key: 'pending', label: t('settings.queue.statusPending') },
    { key: 'sending', label: t('settings.queue.statusSending') },
    { key: 'failed', label: t('settings.queue.statusFailed') },
    { key: 'sent', label: t('settings.queue.statusSent') },
  ]

  return (
    <SectionCard>
      <SectionHeader
        icon={SendToBack}
        title={scope === 'global' ? t('admin.queue.title') : t('settings.queue.title')}
        description={scope === 'global' ? t('admin.queue.desc') : t('settings.queue.desc')}
        action={
          <Button
            variant="outline"
            size="sm"
            disabled={isFetching}
            onClick={() => void refetch()}
          >
            <RefreshCw className={cn('h-3.5 w-3.5', isFetching && 'animate-spin')} />
            {t('common.refresh')}
          </Button>
        }
      />

      {/* Filters + Search */}
      <div className="flex flex-col gap-2.5 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex flex-wrap items-center gap-1">
          {filterTabs.map((tab) => (
            <button
              key={tab.key}
              type="button"
              onClick={() => setStatusFilter(tab.key)}
              className={cn(
                'rounded-full px-2.5 py-1 text-[12px] font-medium transition-colors',
                statusFilter === tab.key
                  ? 'bg-[var(--ink-primary)] text-[var(--ink-inverse)]'
                  : 'text-[var(--ink-secondary)] hover:bg-[var(--surface-hover)]',
              )}
            >
              {tab.label}
            </button>
          ))}
        </div>

        <div className="relative w-full sm:w-64">
          <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-[var(--ink-muted)]" />
          <input
            type="search"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={t('settings.queue.searchPlaceholder')}
            className="h-8 w-full rounded-[var(--radius-sm)] border border-[var(--border-subtle)] bg-[var(--surface-canvas)] pl-8 pr-3 text-[12.5px] text-[var(--ink-primary)] placeholder:text-[var(--ink-subtle)] outline-none focus:border-[var(--accent)] focus:ring-2 focus:ring-[var(--accent-ring)]"
          />
        </div>
      </div>

      {isLoading ? (
        <SectionLoadingState rows={2} rowHeight="h-20" />
      ) : items.length === 0 ? (
        <EmptyState
          icon={SendToBack}
          title={t('settings.queue.empty')}
          description={t('settings.queue.emptyDesc')}
        />
      ) : (
        <div className="space-y-2.5">
          {items.map((item) => {
            const meta = statusMeta[item.status] ?? statusMeta.pending
            return (
              <div
                key={item.id}
                className="rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] p-3.5 space-y-2"
              >
                <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                  <div className="min-w-0 space-y-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <Badge variant={meta.variant}>{meta.label}</Badge>
                      <Badge variant="default">{kindLabel(item.kind)}</Badge>
                      <span className="break-all font-mono text-[13px] font-medium text-[var(--ink-primary)]">
                        {item.mail_from} → {item.rcpt_to}
                      </span>
                    </div>
                    <div className="flex flex-wrap gap-x-4 gap-y-0.5 text-[11.5px] text-[var(--ink-muted)]">
                      <span>
                        {t('settings.queue.attempts', {
                          attempts: item.attempts,
                          max: item.max_attempts,
                        })}
                      </span>
                      {item.status === 'pending' &&
                        pickDate(item.next_retry_at_iso, item.next_retry_at) && (
                          <span>
                            {t('settings.queue.nextRetry')}：
                            {formatMailFullTime(
                              pickDate(item.next_retry_at_iso, item.next_retry_at),
                              i18n.language,
                            )}
                          </span>
                        )}
                      <span>
                        {formatMailFullTime(
                          pickDate(item.created_iso, item.created),
                          i18n.language,
                        )}
                      </span>
                    </div>
                  </div>

                  <div className="flex shrink-0 items-center gap-1.5 self-end sm:self-center">
                    {(item.status === 'failed' || item.status === 'pending') && (
                      <Button
                        variant="outline"
                        size="xs"
                        disabled={retryMutation.isPending}
                        onClick={() => retryMutation.mutate(item.id)}
                      >
                        <RotateCcw className="h-3 w-3" />
                        {t('settings.queue.retry')}
                      </Button>
                    )}
                    <Button
                      variant="dangerGhost"
                      size="iconSm"
                      aria-label={t('common.delete')}
                      onClick={() => setDeleting(item)}
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </Button>
                  </div>
                </div>

                {item.last_error && (
                  <div className="flex items-start gap-2 rounded-[var(--radius-sm)] border border-[var(--danger)]/25 bg-[var(--danger-soft)] px-3 py-2 font-mono text-[11.5px] text-[var(--danger)]">
                    <AlertCircle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                    <span className="break-all">{item.last_error}</span>
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}

      {/* Delete Confirm */}
      <Dialog open={deleting !== null} onOpenChange={(open) => !open && setDeleting(null)}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>{t('settings.queue.deleteTitle')}</DialogTitle>
            <DialogDescription>
              {deleting ? t('settings.queue.deleteConfirm', { rcpt: deleting.rcpt_to }) : ''}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" size="sm" onClick={() => setDeleting(null)}>
              {t('common.cancel')}
            </Button>
            <Button
              variant="danger"
              size="sm"
              disabled={deleteMutation.isPending}
              onClick={() => deleting && deleteMutation.mutate(deleting.id)}
            >
              {t('common.delete')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </SectionCard>
  )
}
