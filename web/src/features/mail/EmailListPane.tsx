import { useEffect, useRef, useState, type RefObject } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useTranslation } from 'react-i18next'
import {
  FileEdit,
  FolderInput,
  Inbox,
  Loader2,
  MailOpen,
  Menu,
  RefreshCw,
  RotateCcw,
  Search,
  ShieldBan,
  ShieldCheck,
  Star,
  Trash2,
  X,
} from 'lucide-react'
import { SYSTEM_GROUP_IDS, type EmailListItem as EmailListItemModel, type GroupOption } from '@/api'
import { formatMailListTime } from '@/lib/date'
import { cn } from '@/lib/cn'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { ThemeToggle } from '@/components/common/ThemeToggle'
import type { ListDensity } from '@/lib/preferences'
import { EmailListItem } from './EmailListItem'
import type { FolderSelection, LocalDraft, MailFilterMode } from './types'

interface EmailListPaneProps {
  folder: FolderSelection
  folderTitle: string
  emails: EmailListItemModel[]
  totalCount?: number
  localDrafts: LocalDraft[]
  showAddressLabels?: boolean
  activeEmailId: number | null
  checkedIds: Set<number>
  searchQuery: string
  filterMode: MailFilterMode
  density: ListDensity
  isLoading: boolean
  isFetching: boolean
  isFetchingNextPage: boolean
  hasNextPage: boolean
  isError: boolean
  groupOptions: GroupOption[]
  isMobile: boolean
  isTablet: boolean
  searchInputRef: RefObject<HTMLInputElement | null>
  onOpenMobileDrawer?: () => void
  onSearchChange: (q: string) => void
  onFilterChange: (mode: MailFilterMode) => void
  onRefresh: () => void
  onLoadMore: () => void
  onSelectEmail: (email: EmailListItemModel) => void
  onToggleCheck: (id: number) => void
  onSelectAll: () => void
  onClearSelection: () => void
  onToggleStar: (id: number, nextStarred: boolean) => void
  onBulkMarkRead: (ids: number[], isRead: boolean) => void
  onBulkStar: (ids: number[]) => void
  onBulkDelete: (ids: number[], forced: boolean) => void
  onBulkRestore?: (ids: number[]) => void
  onEmptyTrash?: () => void
  emptyingTrash?: boolean
  trashRetentionDays?: number
  onBulkMove: (ids: number[], groupId: number, label: string) => void
  onBulkMarkSpam: (ids: number[], isSpam: boolean) => void
  onPrefetchEmail: (id: number) => void
  onCancelPrefetch: () => void
  onResumeDraft: (draft: LocalDraft) => void
  onDiscardDraft: (draftId: string) => void
}

export function EmailListPane({
  folder,
  folderTitle,
  emails,
  totalCount,
  localDrafts,
  showAddressLabels,
  activeEmailId,
  checkedIds,
  searchQuery,
  filterMode,
  density,
  isLoading,
  isFetching,
  isFetchingNextPage,
  hasNextPage,
  isError,
  groupOptions,
  isMobile,
  isTablet,
  searchInputRef,
  onOpenMobileDrawer,
  onSearchChange,
  onFilterChange,
  onRefresh,
  onLoadMore,
  onSelectEmail,
  onToggleCheck,
  onSelectAll,
  onClearSelection,
  onToggleStar,
  onBulkMarkRead,
  onBulkStar,
  onBulkDelete,
  onBulkRestore,
  onEmptyTrash,
  emptyingTrash = false,
  trashRetentionDays,
  onBulkMove,
  onBulkMarkSpam,
  onPrefetchEmail,
  onCancelPrefetch,
  onResumeDraft,
  onDiscardDraft,
}: EmailListPaneProps) {
  const { t, i18n } = useTranslation()
  const [emptyTrashOpen, setEmptyTrashOpen] = useState(false)
  const [confirmHardDelIds, setConfirmHardDelIds] = useState<number[] | null>(null)
  const scrollRef = useRef<HTMLDivElement | null>(null)
  const [pullDistance, setPullDistance] = useState(0)
  const pullStartY = useRef<number | null>(null)

  const isDraftsFolder = folder.kind === 'system' && folder.key === 'drafts'
  const isDraftOrSentFolder =
    folder.kind === 'system' && (folder.key === 'drafts' || folder.key === 'sent')
  const isInTrash = folder.kind === 'system' && folder.key === 'trash'
  const isInJunk = folder.kind === 'system' && folder.key === 'junk'
  const selectionMode = checkedIds.size > 0
  const selectedIdsArray = Array.from(checkedIds)

  const estimatedRowHeight =
    density === 'compact' ? 72 : density === 'spacious' ? 96 : 84

  const virtualizer = useVirtualizer({
    count: hasNextPage ? emails.length + 1 : emails.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => estimatedRowHeight,
    overscan: 10,
  })

  const virtualItems = virtualizer.getVirtualItems()

  // Trigger infinite loading & prefetch next page cleanly before reaching the end
  useEffect(() => {
    const lastItem = virtualItems[virtualItems.length - 1]
    if (!lastItem) return
    if (
      lastItem.index >= emails.length - 15 &&
      hasNextPage &&
      !isFetchingNextPage
    ) {
      onLoadMore()
    }
  }, [virtualItems, emails.length, hasNextPage, isFetchingNextPage, onLoadMore])

  // Keep keyboard-selected active email scrolled smoothly into view
  useEffect(() => {
    if (!activeEmailId) return
    const idx = emails.findIndex((e) => e.id === activeEmailId)
    if (idx >= 0) {
      virtualizer.scrollToIndex(idx, { align: 'auto' })
    }
  }, [activeEmailId, emails, virtualizer])

  // Mobile pull-to-refresh handlers
  const handlePullStart = (e: React.TouchEvent) => {
    if (!isMobile || e.touches.length !== 1) return
    if ((scrollRef.current?.scrollTop ?? 0) <= 0) {
      pullStartY.current = e.touches[0]?.clientY ?? null
    }
  }

  const handlePullMove = (e: React.TouchEvent) => {
    if (!isMobile || pullStartY.current === null || e.touches.length !== 1) return
    const currentY = e.touches[0]?.clientY ?? pullStartY.current
    const delta = currentY - pullStartY.current
    if (delta > 0 && (scrollRef.current?.scrollTop ?? 0) <= 0) {
      setPullDistance(Math.min(72, delta * 0.5))
    }
  }

  const handlePullEnd = () => {
    if (!isMobile) return
    if (pullDistance >= 52) {
      onRefresh()
    }
    setPullDistance(0)
    pullStartY.current = null
  }

  const allChecked = emails.length > 0 && checkedIds.size === emails.length
  const someChecked = checkedIds.size > 0 && checkedIds.size < emails.length

  const filters: { key: MailFilterMode; label: string }[] = [
    { key: 'all', label: t('mail.filterAll') },
    { key: 'unread', label: t('mail.filterUnread') },
    { key: 'attachments', label: t('mail.filterAttachments') },
  ]

  return (
    <section className="flex h-full flex-col bg-[var(--surface-list)]">
      {/* Header: Folder Title + Search + Filter Pills */}
      <div className="shrink-0 border-b border-[var(--border-hairline)] bg-[var(--surface-list)] pt-safe">
        <div className="flex h-12 items-center justify-between gap-2 px-3.5">
          <div className="flex min-w-0 items-center gap-2">
            {(isMobile || isTablet) && onOpenMobileDrawer && (
              <Button
                variant="ghost"
                size="iconSm"
                onClick={onOpenMobileDrawer}
                aria-label={t('nav.menu')}
              >
                <Menu className="h-4 w-4" />
              </Button>
            )}
            <h1 className="truncate text-[15px] font-semibold tracking-tight text-[var(--ink-primary)]">
              {folderTitle}
            </h1>
            {!isLoading && (
              <span className="shrink-0 rounded-full bg-[var(--surface-subtle)] px-2 py-0.5 text-[11px] font-medium tabular-nums text-[var(--ink-muted)]">
                {totalCount && totalCount > emails.length
                  ? totalCount.toLocaleString()
                  : emails.length.toLocaleString()}
              </span>
            )}
          </div>

          <div className="flex items-center gap-1">
            <Button
              variant="ghost"
              size="iconSm"
              onClick={onRefresh}
              disabled={isFetching}
              aria-label={t('common.refresh')}
              title={t('common.refresh')}
            >
              <RefreshCw className={cn('h-3.5 w-3.5', isFetching && 'animate-spin')} />
            </Button>
            {isMobile && <ThemeToggle />}
          </div>
        </div>

        {/* Search Input */}
        <div className="px-3.5 pb-2.5">
          <div className="relative">
            <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-[var(--ink-muted)]" />
            <input
              ref={searchInputRef}
              type="search"
              value={searchQuery}
              onChange={(e) => onSearchChange(e.target.value)}
              placeholder={t('mail.searchPlaceholder')}
              className="h-8 w-full rounded-[var(--radius-sm)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)] pl-8 pr-7 text-[13px] text-[var(--ink-primary)] placeholder:text-[var(--ink-subtle)] transition-colors focus:border-[var(--accent)] focus:bg-[var(--surface-canvas)] focus:outline-none focus:ring-2 focus:ring-[var(--accent-ring)]"
            />
            {searchQuery && (
              <button
                type="button"
                onClick={() => onSearchChange('')}
                aria-label={t('common.clear')}
                className="absolute right-2 top-1/2 -translate-y-1/2 rounded p-0.5 text-[var(--ink-muted)] hover:text-[var(--ink-primary)]"
              >
                <X className="h-3.5 w-3.5" />
              </button>
            )}
          </div>

          {/* Filter Pills OR Bulk Action Bar */}
          {selectionMode ? (
            <div className="mt-2 flex items-center justify-between gap-2 rounded-[var(--radius-sm)] bg-[var(--accent-soft)] px-2.5 py-1.5">
              <div className="flex items-center gap-2">
                <Checkbox
                  checked={allChecked ? true : someChecked ? 'indeterminate' : false}
                  onCheckedChange={() => (allChecked ? onClearSelection() : onSelectAll())}
                  aria-label={t('mail.selectAll')}
                />
                <span className="text-[12px] font-medium text-[var(--accent)]">
                  {t('mail.selectedCount', { count: checkedIds.size })}
                </span>
              </div>
              <div className="flex items-center gap-0.5">
                <Button
                  variant="ghost"
                  size="iconSm"
                  onClick={() => onBulkMarkRead(selectedIdsArray, true)}
                  aria-label={t('mail.actions.markRead')}
                  title={t('mail.actions.markRead')}
                >
                  <MailOpen className="h-3.5 w-3.5" />
                </Button>
                <Button
                  variant="ghost"
                  size="iconSm"
                  onClick={() => onBulkStar(selectedIdsArray)}
                  aria-label={t('mail.actions.star')}
                  title={t('mail.actions.star')}
                >
                  <Star className="h-3.5 w-3.5" />
                </Button>
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button
                      variant="ghost"
                      size="iconSm"
                      aria-label={t('mail.actions.move')}
                      title={t('mail.actions.move')}
                    >
                      <FolderInput className="h-3.5 w-3.5" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    <DropdownMenuLabel>{t('mail.actions.move')}</DropdownMenuLabel>
                    <DropdownMenuItem
                      onClick={() =>
                        onBulkMove(selectedIdsArray, SYSTEM_GROUP_IDS.INBOX, t('nav.inbox'))
                      }
                    >
                      {t('nav.inbox')}
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      onClick={() =>
                        onBulkMove(selectedIdsArray, SYSTEM_GROUP_IDS.JUNK, t('nav.junk'))
                      }
                    >
                      {t('nav.junk')}
                    </DropdownMenuItem>
                    {groupOptions
                      .filter((g) => g.id < SYSTEM_GROUP_IDS.INBOX)
                      .map((g) => (
                        <DropdownMenuItem
                          key={g.id}
                          onClick={() => onBulkMove(selectedIdsArray, g.id, g.name)}
                        >
                          {g.name}
                        </DropdownMenuItem>
                      ))}
                  </DropdownMenuContent>
                </DropdownMenu>
                <Button
                  variant="ghost"
                  size="iconSm"
                  onClick={() => onBulkMarkSpam(selectedIdsArray, !isInJunk)}
                  aria-label={isInJunk ? t('mail.actions.notSpam') : t('mail.actions.markSpam')}
                  title={isInJunk ? t('mail.actions.notSpam') : t('mail.actions.markSpam')}
                >
                  {isInJunk ? (
                    <ShieldCheck className="h-3.5 w-3.5" />
                  ) : (
                    <ShieldBan className="h-3.5 w-3.5" />
                  )}
                </Button>
                {isInTrash && onBulkRestore && (
                  <Button
                    variant="ghost"
                    size="xs"
                    onClick={() => onBulkRestore(selectedIdsArray)}
                    aria-label={t('mail.trash.restore')}
                    title={t('mail.trash.restore')}
                  >
                    <RotateCcw className="h-3.5 w-3.5" />
                    <span>{t('mail.trash.restore')}</span>
                  </Button>
                )}
                <Button
                  variant="dangerGhost"
                  size="iconSm"
                  onClick={() => {
                    if (isInTrash) {
                      setConfirmHardDelIds(selectedIdsArray)
                    } else {
                      onBulkDelete(selectedIdsArray, false)
                    }
                  }}
                  aria-label={isInTrash ? t('mail.actions.deleteForever') : t('mail.actions.delete')}
                  title={isInTrash ? t('mail.actions.deleteForever') : t('mail.actions.delete')}
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
                <Button
                  variant="ghost"
                  size="iconSm"
                  onClick={onClearSelection}
                  aria-label={t('mail.clearSelection')}
                  title={t('mail.clearSelection')}
                >
                  <X className="h-3.5 w-3.5" />
                </Button>
              </div>
            </div>
          ) : (
            <div className="mt-2 flex items-center justify-between gap-2">
              <div className="flex items-center gap-1">
                {filters.map((f) => (
                  <button
                    key={f.key}
                    type="button"
                    onClick={() => onFilterChange(f.key)}
                    className={cn(
                      'rounded-full px-2.5 py-0.5 text-[12px] font-medium transition-colors duration-[var(--duration-fast)]',
                      filterMode === f.key
                        ? 'bg-[var(--ink-primary)] text-[var(--ink-inverse)]'
                        : 'text-[var(--ink-secondary)] hover:bg-[var(--surface-hover)] hover:text-[var(--ink-primary)]',
                    )}
                  >
                    {f.label}
                  </button>
                ))}
              </div>
              {emails.length > 0 && (
                <button
                  type="button"
                  onClick={onSelectAll}
                  className="text-[11.5px] text-[var(--ink-muted)] hover:text-[var(--ink-primary)]"
                >
                  {t('mail.selectAll')}
                </button>
              )}
            </div>
          )}
        </div>
      </div>

      {/* Trash Retention & Empty Banner */}
      {isInTrash && (
        <div className="flex shrink-0 flex-wrap items-center justify-between gap-2 border-b border-[var(--border-hairline)] bg-[var(--warning-soft)]/50 px-3.5 py-2 text-[12px] text-[var(--ink-secondary)]">
          <span className="min-w-0 flex-1 leading-snug">
            {trashRetentionDays === 0
              ? t('mail.trash.bannerNoAutoPurge')
              : t('mail.trash.bannerRetention', { days: trashRetentionDays ?? 30 })}
          </span>
          {onEmptyTrash && (
            <Button
              type="button"
              variant="dangerGhost"
              size="xs"
              disabled={emptyingTrash || (totalCount ?? emails.length) === 0}
              onClick={() => setEmptyTrashOpen(true)}
            >
              <Trash2 className="h-3.5 w-3.5" />
              {t('mail.trash.emptyButton')}
            </Button>
          )}
        </div>
      )}

      {/* Pull to refresh indicator (Mobile) */}
      {isMobile && pullDistance > 8 && (
        <div
          className="flex shrink-0 items-center justify-center gap-1.5 overflow-hidden text-[12px] text-[var(--ink-muted)] transition-[height]"
          style={{ height: `${pullDistance}px` }}
        >
          <RefreshCw
            className={cn('h-3.5 w-3.5', pullDistance >= 52 && 'animate-spin text-[var(--accent)]')}
          />
          <span>
            {pullDistance >= 52 ? t('mail.releaseToRefresh') : t('mail.pullToRefresh')}
          </span>
        </div>
      )}

      {/* Virtualized List Body */}
      <div
        ref={scrollRef}
        role="listbox"
        aria-label={folderTitle}
        onTouchStart={handlePullStart}
        onTouchMove={handlePullMove}
        onTouchEnd={handlePullEnd}
        className="mail-scroll flex-1 overflow-y-auto"
      >
        {/* Local auto-saved drafts inside Drafts folder */}
        {isDraftsFolder && localDrafts.length > 0 && (
          <div className="border-b border-[var(--border-subtle)] bg-[var(--surface-subtle)]/40">
            {localDrafts.map((draft) => (
              <div
                key={draft.id}
                onClick={() => onResumeDraft(draft)}
                className="group flex cursor-pointer items-start justify-between gap-3 border-b border-[var(--border-hairline)] px-3.5 py-3 transition-colors hover:bg-[var(--surface-hover)]"
              >
                <div className="flex min-w-0 items-start gap-2.5">
                  <div className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-[var(--warning-soft)] text-[var(--warning)]">
                    <FileEdit className="h-3.5 w-3.5" />
                  </div>
                  <div className="min-w-0">
                    <div className="flex items-center gap-1.5">
                      <span className="rounded bg-[var(--warning-soft)] px-1.5 py-0.5 text-[10.5px] font-medium text-[var(--warning)]">
                        {t('mail.draft.badge')}
                      </span>
                      <span className="truncate text-[13px] font-medium text-[var(--ink-primary)]">
                        {draft.to.length > 0 ? draft.to.join(', ') : t('mail.draft.localHint')}
                      </span>
                    </div>
                    <p className="mt-0.5 truncate text-[13px] text-[var(--ink-secondary)]">
                      {draft.subject || t('mail.noSubject')}
                    </p>
                    <p className="mt-0.5 truncate text-[12px] text-[var(--ink-muted)]">
                      {draft.text || t('mail.noSnippet')}
                    </p>
                  </div>
                </div>
                <div className="flex shrink-0 flex-col items-end gap-1">
                  <span className="text-[11px] tabular-nums text-[var(--ink-muted)]">
                    {formatMailListTime(draft.updatedAt, i18n.language)}
                  </span>
                  <button
                    type="button"
                    onClick={(e) => {
                      e.stopPropagation()
                      onDiscardDraft(draft.id)
                    }}
                    aria-label={t('mail.draft.discard')}
                    className="rounded p-1 text-[var(--ink-muted)] opacity-0 transition-opacity hover:bg-[var(--danger-soft)] hover:text-[var(--danger)] group-hover:opacity-100"
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}

        {/* Loading skeleton */}
        {isLoading ? (
          <div className="divide-y divide-[var(--border-hairline)]">
            {Array.from({ length: 8 }).map((_, i) => (
              <div key={i} className="flex items-start gap-3 px-4 py-3.5">
                <Skeleton className="h-8 w-8 rounded-full shrink-0" />
                <div className="flex-1 space-y-2">
                  <div className="flex justify-between gap-4">
                    <Skeleton className="h-3.5 w-28" />
                    <Skeleton className="h-3 w-12" />
                  </div>
                  <Skeleton className="h-3.5 w-4/5" />
                  <Skeleton className="h-3 w-3/5" />
                </div>
              </div>
            ))}
          </div>
        ) : isError ? (
          <div className="flex flex-col items-center justify-center px-6 py-16 text-center">
            <p className="text-[13.5px] font-medium text-[var(--ink-primary)]">
              {t('mail.loadError')}
            </p>
            <Button variant="secondary" size="sm" onClick={onRefresh} className="mt-3">
              {t('app.retry')}
            </Button>
          </div>
        ) : emails.length === 0 && !(isDraftsFolder && localDrafts.length > 0) ? (
          <div className="flex flex-col items-center justify-center px-6 py-20 text-center">
            <div className="flex h-12 w-12 items-center justify-center rounded-2xl bg-[var(--surface-subtle)] text-[var(--ink-muted)]">
              <Inbox className="h-5 w-5" />
            </div>
            <p className="mt-3.5 text-[14px] font-medium text-[var(--ink-primary)]">
              {searchQuery || filterMode !== 'all'
                ? t('mail.emptySearchTitle')
                : t('mail.emptyFolderTitle')}
            </p>
            <p className="mt-1 text-[12.5px] text-[var(--ink-muted)]">
              {searchQuery || filterMode !== 'all'
                ? t('mail.emptySearchDesc')
                : t('mail.emptyFolderDesc')}
            </p>
          </div>
        ) : (
          <div
            style={{
              height: `${virtualizer.getTotalSize()}px`,
              width: '100%',
              position: 'relative',
            }}
          >
            {virtualItems.map((virtualRow) => {
              const isLoaderRow = virtualRow.index >= emails.length
              const email = emails[virtualRow.index]

              return (
                <div
                  key={virtualRow.key}
                  data-index={virtualRow.index}
                  ref={virtualizer.measureElement}
                  style={{
                    position: 'absolute',
                    top: 0,
                    left: 0,
                    width: '100%',
                    transform: `translateY(${virtualRow.start}px)`,
                  }}
                >
                  {isLoaderRow ? (
                    <div className="flex items-center justify-center gap-2 py-4 text-[12px] text-[var(--ink-muted)]">
                      <Loader2 className="h-3.5 w-3.5 animate-spin" />
                      <span>{t('mail.loadingMore')}</span>
                    </div>
                  ) : email ? (
                    <EmailListItem
                      email={email}
                      showAddressLabels={showAddressLabels}
                      isActive={activeEmailId === email.id}
                      isChecked={checkedIds.has(email.id)}
                      isStarred={Boolean(email.is_starred)}
                      selectionMode={selectionMode}
                      isMobile={isMobile}
                      density={density}
                      suppressAuthWarning={isDraftOrSentFolder}
                      onSelect={onSelectEmail}
                      onToggleCheck={onToggleCheck}
                      onToggleStar={onToggleStar}
                      onSwipeToggleRead={(item) => onBulkMarkRead([item.id], !item.is_read)}
                      onSwipeDelete={(item) => {
                        if (isInTrash) setConfirmHardDelIds([item.id])
                        else onBulkDelete([item.id], false)
                      }}
                      onPrefetch={onPrefetchEmail}
                      onCancelPrefetch={onCancelPrefetch}
                    />
                  ) : null}
                </div>
              )
            })}
          </div>
        )}
      </div>

      {/* Empty Trash Confirm Dialog */}
      <Dialog open={emptyTrashOpen} onOpenChange={setEmptyTrashOpen}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>{t('mail.trash.emptyConfirmTitle')}</DialogTitle>
            <DialogDescription>
              {t('mail.trash.emptyConfirmDesc', { count: totalCount ?? emails.length })}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={emptyingTrash}
              onClick={() => setEmptyTrashOpen(false)}
            >
              {t('common.cancel')}
            </Button>
            <Button
              type="button"
              variant="danger"
              size="sm"
              disabled={emptyingTrash}
              onClick={() => {
                setEmptyTrashOpen(false)
                onEmptyTrash?.()
              }}
            >
              {emptyingTrash && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
              {t('mail.trash.emptyConfirmAction')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Permanent Delete Confirm Dialog */}
      <Dialog
        open={confirmHardDelIds !== null}
        onOpenChange={(open) => !open && setConfirmHardDelIds(null)}
      >
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>{t('mail.trash.deleteForeverTitle')}</DialogTitle>
            <DialogDescription>
              {t('mail.trash.deleteForeverDesc', { count: confirmHardDelIds?.length ?? 0 })}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => setConfirmHardDelIds(null)}
            >
              {t('common.cancel')}
            </Button>
            <Button
              type="button"
              variant="danger"
              size="sm"
              onClick={() => {
                if (confirmHardDelIds && confirmHardDelIds.length > 0) {
                  onBulkDelete(confirmHardDelIds, true)
                }
                setConfirmHardDelIds(null)
              }}
            >
              {t('mail.actions.deleteForever')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  )
}
