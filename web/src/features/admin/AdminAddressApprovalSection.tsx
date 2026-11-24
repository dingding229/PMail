import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  Check,
  ChevronLeft,
  ChevronRight,
  ClipboardCheck,
  Loader2,
  X,
} from 'lucide-react'
import {
  adminAddressApprovalApi,
  userApi,
  type AddressStatus,
  type AdminAddressRequest,
} from '@/api'
import { queryKeys } from '@/hooks/useMailQueries'
import { formatMailFullTime } from '@/lib/date'
import { cn } from '@/lib/cn'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  DataTable,
  DataTableBody,
  DataTableCell,
  DataTableHead,
  DataTableHeaderCell,
  DataTableRow,
  EmptyState,
  SectionCard,
  SectionErrorState,
  SectionHeader,
  SectionLoadingState,
} from '@/components/settings'

const PAGE_SIZE = 20
const MAX_REJECT_REASON_LENGTH = 200

export function AdminAddressApprovalSection() {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()

  const [statusTab, setStatusTab] = useState<AddressStatus>('pending')
  const [page, setPage] = useState(1)
  const [rejectingItem, setRejectingItem] = useState<AdminAddressRequest | null>(null)
  const [rejectReason, setRejectReason] = useState('')
  const [approvingId, setApprovingId] = useState<number | null>(null)

  const settingsQuery = useQuery({
    queryKey: queryKeys.adminAddressSettings,
    queryFn: () => adminAddressApprovalApi.getSettings(),
  })

  const requestsQuery = useQuery({
    queryKey: queryKeys.adminAddressRequests(statusTab, page, PAGE_SIZE),
    queryFn: () =>
      adminAddressApprovalApi.listRequests({
        status: statusTab,
        page,
        page_size: PAGE_SIZE,
      }),
  })

  // Optional user lookup to display reviewer account/name cleanly
  const usersQuery = useQuery({
    queryKey: queryKeys.users(1, 100),
    queryFn: () => userApi.list({ current_page: 1, page_size: 100 }),
    staleTime: 60_000,
    retry: false,
  })

  const reviewerMap = useMemo(() => {
    const map = new Map<number, string>()
    for (const u of usersQuery.data?.list ?? []) {
      map.set(u.id, u.display_name ? `${u.display_name} (${u.account})` : u.account)
    }
    return map
  }, [usersQuery.data?.list])

  const invalidateApprovalQueries = () => {
    void queryClient.invalidateQueries({ queryKey: queryKeys.adminAddressRequestsRoot })
    void queryClient.invalidateQueries({ queryKey: queryKeys.adminOverview })
    void queryClient.invalidateQueries({ queryKey: queryKeys.addresses })
    void queryClient.invalidateQueries({ queryKey: queryKeys.usersRoot })
  }

  const toggleSettingMutation = useMutation({
    mutationFn: (required: boolean) =>
      adminAddressApprovalApi.updateSettings({ address_approval_required: required }),
    onSuccess: (updated) => {
      queryClient.setQueryData(queryKeys.adminAddressSettings, updated)
      toast.success(t('admin.addressApproval.settingSaved'))
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const approveMutation = useMutation({
    mutationFn: (req: AdminAddressRequest) => {
      setApprovingId(req.id)
      return adminAddressApprovalApi.approve(req.id)
    },
    onSuccess: (_res, req) => {
      toast.success(t('admin.addressApproval.approvedToast', { address: req.address }))
      invalidateApprovalQueries()
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
    onSettled: () => {
      setApprovingId(null)
    },
  })

  const rejectMutation = useMutation({
    mutationFn: ({ req, reason }: { req: AdminAddressRequest; reason: string }) =>
      adminAddressApprovalApi.reject(req.id, reason),
    onSuccess: (_res, { req }) => {
      toast.success(t('admin.addressApproval.rejectedToast', { address: req.address }))
      setRejectingItem(null)
      setRejectReason('')
      invalidateApprovalQueries()
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const data = requestsQuery.data
  const items = data?.items ?? []
  const total = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))
  const pendingCount = data?.pending_address_requests ?? 0
  const reasonCharCount = Array.from(rejectReason).length
  const reasonTooLong = reasonCharCount > MAX_REJECT_REASON_LENGTH

  const handleTabChange = (next: AddressStatus) => {
    setStatusTab(next)
    setPage(1)
  }

  const openRejectDialog = (req: AdminAddressRequest) => {
    setRejectingItem(req)
    setRejectReason('')
  }

  const formatReviewer = (reviewedBy?: number | null) => {
    if (!reviewedBy) return '—'
    return reviewerMap.get(reviewedBy) ?? `#${reviewedBy}`
  }

  const statusTabs: { key: AddressStatus; label: string; count?: number }[] = [
    {
      key: 'pending',
      label: t('admin.addressApproval.tabs.pending'),
      count: pendingCount > 0 ? pendingCount : undefined,
    },
    { key: 'active', label: t('admin.addressApproval.tabs.approved') },
    { key: 'rejected', label: t('admin.addressApproval.tabs.rejected') },
  ]

  return (
    <div className="space-y-5">
      {/* Policy Switch Card */}
      <SectionCard>
        <SectionHeader
          icon={ClipboardCheck}
          title={t('admin.addressApproval.title')}
          description={t('admin.addressApproval.desc')}
        />

        {settingsQuery.isLoading ? (
          <SectionLoadingState rows={1} rowHeight="h-14" />
        ) : settingsQuery.error ? (
          <SectionErrorState
            message={
              settingsQuery.error instanceof Error
                ? settingsQuery.error.message
                : t('common.unknownError')
            }
            retryLabel={t('app.retry')}
            onRetry={() => void settingsQuery.refetch()}
          />
        ) : (
          <div className="flex items-center justify-between gap-4 rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] px-4 py-3.5">
            <div className="space-y-0.5">
              <p className="text-[13.5px] font-medium text-[var(--ink-primary)]">
                {t('admin.addressApproval.requireApprovalLabel')}
              </p>
              <p className="text-[12px] text-[var(--ink-muted)]">
                {t('admin.addressApproval.requireApprovalDesc')}
              </p>
            </div>
            <Switch
              checked={Boolean(settingsQuery.data?.address_approval_required)}
              disabled={toggleSettingMutation.isPending}
              onCheckedChange={(checked) => toggleSettingMutation.mutate(checked)}
            />
          </div>
        )}
      </SectionCard>

      {/* Requests List Card */}
      <SectionCard>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div
            role="tablist"
            aria-label={t('admin.addressApproval.tabsAria')}
            className="inline-flex items-center gap-1 rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)] p-1"
          >
            {statusTabs.map((item) => {
              const active = statusTab === item.key
              return (
                <button
                  key={item.key}
                  type="button"
                  role="tab"
                  aria-selected={active}
                  onClick={() => handleTabChange(item.key)}
                  className={cn(
                    'inline-flex items-center gap-1.5 rounded-[var(--radius-sm)] px-3 py-1.5 text-[12.5px] font-medium transition-colors',
                    active
                      ? 'bg-[var(--surface-canvas)] text-[var(--ink-primary)] shadow-[var(--shadow-xs)]'
                      : 'text-[var(--ink-secondary)] hover:text-[var(--ink-primary)]',
                  )}
                >
                  <span>{item.label}</span>
                  {item.count !== undefined && (
                    <Badge
                      variant="warning"
                      className="h-4.5 min-w-[18px] justify-center px-1 text-[10.5px]"
                    >
                      {item.count}
                    </Badge>
                  )}
                </button>
              )
            })}
          </div>

          {total > 0 && (
            <span className="text-[12px] text-[var(--ink-muted)]">
              {t('admin.addressApproval.totalCount', { count: total })}
            </span>
          )}
        </div>

        {requestsQuery.isLoading ? (
          <SectionLoadingState rows={3} rowHeight="h-12" />
        ) : requestsQuery.error ? (
          <SectionErrorState
            message={
              requestsQuery.error instanceof Error
                ? requestsQuery.error.message
                : t('common.unknownError')
            }
            retryLabel={t('app.retry')}
            onRetry={() => void requestsQuery.refetch()}
          />
        ) : items.length === 0 ? (
          <EmptyState
            icon={ClipboardCheck}
            title={
              statusTab === 'pending'
                ? t('admin.addressApproval.emptyPending')
                : statusTab === 'active'
                  ? t('admin.addressApproval.emptyApproved')
                  : t('admin.addressApproval.emptyRejected')
            }
            compact
          />
        ) : (
          <>
            <DataTable>
              <DataTableHead>
                <tr>
                  <DataTableHeaderCell>{t('admin.addressApproval.colApplicant')}</DataTableHeaderCell>
                  <DataTableHeaderCell>{t('admin.addressApproval.colAddress')}</DataTableHeaderCell>
                  <DataTableHeaderCell>
                    {t('admin.addressApproval.colRequestedAt')}
                  </DataTableHeaderCell>
                  {statusTab !== 'pending' && (
                    <>
                      <DataTableHeaderCell>
                        {t('admin.addressApproval.colReviewer')}
                      </DataTableHeaderCell>
                      <DataTableHeaderCell>
                        {t('admin.addressApproval.colReviewedAt')}
                      </DataTableHeaderCell>
                    </>
                  )}
                  {statusTab === 'pending' && (
                    <DataTableHeaderCell align="right">
                      {t('admin.addressApproval.colActions')}
                    </DataTableHeaderCell>
                  )}
                </tr>
              </DataTableHead>
              <DataTableBody>
                {items.map((req) => (
                  <DataTableRow key={req.id}>
                    <DataTableCell className="whitespace-nowrap font-medium text-[var(--ink-primary)]">
                      {req.applicant}
                    </DataTableCell>
                    <DataTableCell>
                      <div className="space-y-0.5">
                        <span className="break-all font-mono text-[13px] text-[var(--ink-primary)]">
                          {req.address}
                        </span>
                        {statusTab === 'rejected' && req.reject_reason && (
                          <p className="text-[11.5px] text-[var(--danger)]">
                            {t('settings.addresses.rejectReasonPrefix', {
                              reason: req.reject_reason,
                            })}
                          </p>
                        )}
                      </div>
                    </DataTableCell>
                    <DataTableCell className="whitespace-nowrap text-[12.5px] text-[var(--ink-secondary)]">
                      {req.requested_at
                        ? formatMailFullTime(req.requested_at, i18n.language)
                        : '—'}
                    </DataTableCell>
                    {statusTab !== 'pending' && (
                      <>
                        <DataTableCell className="whitespace-nowrap text-[12.5px] text-[var(--ink-secondary)]">
                          {formatReviewer(req.reviewed_by)}
                        </DataTableCell>
                        <DataTableCell className="whitespace-nowrap text-[12.5px] text-[var(--ink-secondary)]">
                          {req.reviewed_at
                            ? formatMailFullTime(req.reviewed_at, i18n.language)
                            : '—'}
                        </DataTableCell>
                      </>
                    )}
                    {statusTab === 'pending' && (
                      <DataTableCell align="right" className="whitespace-nowrap">
                        <div className="inline-flex items-center gap-1.5">
                          <Button
                            size="xs"
                            disabled={approveMutation.isPending || rejectMutation.isPending}
                            onClick={() => approveMutation.mutate(req)}
                          >
                            {approvingId === req.id ? (
                              <Loader2 className="h-3 w-3 animate-spin" />
                            ) : (
                              <Check className="h-3 w-3" />
                            )}
                            {t('admin.addressApproval.approve')}
                          </Button>
                          <Button
                            variant="outline"
                            size="xs"
                            disabled={approveMutation.isPending || rejectMutation.isPending}
                            onClick={() => openRejectDialog(req)}
                            className="text-[var(--danger)] hover:border-[var(--danger)]/40 hover:bg-[var(--danger-soft)] hover:text-[var(--danger)]"
                          >
                            <X className="h-3 w-3" />
                            {t('admin.addressApproval.reject')}
                          </Button>
                        </div>
                      </DataTableCell>
                    )}
                  </DataTableRow>
                ))}
              </DataTableBody>
            </DataTable>

            {totalPages > 1 && (
              <div className="flex items-center justify-between pt-2 text-[12.5px] text-[var(--ink-secondary)]">
                <span>
                  {t('admin.addressApproval.pageIndicator', {
                    page,
                    totalPages,
                  })}
                </span>
                <div className="flex items-center gap-1.5">
                  <Button
                    variant="outline"
                    size="xs"
                    disabled={page <= 1 || requestsQuery.isFetching}
                    onClick={() => setPage((p) => Math.max(1, p - 1))}
                  >
                    <ChevronLeft className="h-3.5 w-3.5" />
                    {t('admin.addressApproval.prevPage')}
                  </Button>
                  <Button
                    variant="outline"
                    size="xs"
                    disabled={page >= totalPages || requestsQuery.isFetching}
                    onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                  >
                    {t('admin.addressApproval.nextPage')}
                    <ChevronRight className="h-3.5 w-3.5" />
                  </Button>
                </div>
              </div>
            )}
          </>
        )}
      </SectionCard>

      {/* Reject Reason Confirm Dialog */}
      <Dialog
        open={rejectingItem !== null}
        onOpenChange={(open) => {
          if (!open) {
            setRejectingItem(null)
            setRejectReason('')
          }
        }}
      >
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>{t('admin.addressApproval.rejectDialogTitle')}</DialogTitle>
            <DialogDescription>
              {rejectingItem
                ? t('admin.addressApproval.rejectDialogDesc', {
                    applicant: rejectingItem.applicant,
                    address: rejectingItem.address,
                  })
                : ''}
            </DialogDescription>
          </DialogHeader>

          <div className="mt-2 space-y-1.5">
            <div className="flex items-center justify-between">
              <Label htmlFor="reject_reason_input">
                {t('admin.addressApproval.rejectReasonLabel')}
              </Label>
              <span
                className={cn(
                  'font-mono text-[11.5px]',
                  reasonTooLong ? 'text-[var(--danger)]' : 'text-[var(--ink-muted)]',
                )}
              >
                {reasonCharCount} / {MAX_REJECT_REASON_LENGTH}
              </span>
            </div>
            <textarea
              id="reject_reason_input"
              rows={3}
              maxLength={MAX_REJECT_REASON_LENGTH}
              value={rejectReason}
              onChange={(e) => setRejectReason(e.target.value)}
              placeholder={t('admin.addressApproval.rejectReasonPlaceholder')}
              className="w-full resize-none rounded-[var(--radius-sm)] border border-[var(--border-subtle)] bg-[var(--surface-canvas)] px-3 py-2 text-[13px] text-[var(--ink-primary)] placeholder:text-[var(--ink-muted)] focus:border-[var(--accent)] focus:outline-none focus:ring-2 focus:ring-[var(--accent-ring)]"
            />
          </div>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => {
                setRejectingItem(null)
                setRejectReason('')
              }}
            >
              {t('common.cancel')}
            </Button>
            <Button
              type="button"
              variant="danger"
              size="sm"
              disabled={rejectMutation.isPending || reasonTooLong}
              onClick={() =>
                rejectingItem &&
                rejectMutation.mutate({ req: rejectingItem, reason: rejectReason })
              }
            >
              {rejectMutation.isPending && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
              {t('admin.addressApproval.confirmReject')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
