import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  Folder,
  Inbox,
  PenSquare,
  Send,
  Settings,
  Star,
  X,
} from 'lucide-react'
import {
  authApi,
  SYSTEM_GROUP_IDS,
  userApi,
  type EmailDetail,
  type EmailListItem,
} from '@/api'
import type { SupportedLanguage } from '@/i18n'
import { getStorageItem, setStorageItem } from '@/lib/storage'
import { usePreferences } from '@/lib/preferences'
import { cn } from '@/lib/cn'
import { useViewportBreakpoint } from '@/hooks/useMediaQuery'
import {
  queryKeys,
  useAddressesQuery,
  useCreateGroupMutation,
  useDeleteEmailsMutation,
  useDeleteGroupMutation,
  useEmailDetailQuery,
  useEmailInfiniteList,
  useGroupOptionsQuery,
  useMarkReadMutation,
  useMoveEmailsMutation,
  usePrefetchEmailDetail,
  useSidebarGroupsQuery,
  useStarEmailsMutation,
  useUserMeQuery,
} from '@/hooks/useMailQueries'
import { useLocalDrafts } from '@/hooks/useDrafts'
import { useKeyboardShortcuts } from '@/hooks/useKeyboardShortcuts'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { MailSidebar } from './MailSidebar'
import { EmailListPane } from './EmailListPane'
import { ReadingPane } from './ReadingPane'
import { ShortcutsDialog } from './ShortcutsDialog'
import { ErrorBoundary } from '@/components/common/ErrorBoundary'
import type { ContactSuggestion } from './RecipientChipInput'

const ComposeModal = lazy(() =>
  import('./ComposeModal').then((m) => ({ default: m.ComposeModal })),
)
import {
  folderToRouteSegment,
  parseFolderSegment,
  type ComposeContext,
  type FolderSelection,
  type LocalDraft,
  type MailFilterMode,
  type SystemFolderKey,
} from './types'

const MIN_SIDEBAR_W = 190
const MAX_SIDEBAR_W = 300
const COLLAPSED_SIDEBAR_W = 60
const MIN_LIST_W = 310
const MAX_LIST_W = 540

export function MailWorkspace() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { folder: folderParam } = useParams<{ folder?: string }>()
  const { isDesktop, isTablet, isMobile } = useViewportBreakpoint()
  const { listDensity, readingPanePosition } = usePreferences()

  // Resizable column widths persisted in localStorage
  const [sidebarWidth, setSidebarWidth] = useState<number>(() =>
    getStorageItem('sidebar_width', 232),
  )
  const [sidebarCollapsed, setSidebarCollapsed] = useState<boolean>(() =>
    getStorageItem('sidebar_collapsed', false),
  )
  const [listWidth, setListWidth] = useState<number>(() =>
    getStorageItem('list_width', 372),
  )

  const [drawerOpen, setDrawerOpen] = useState(false)
  const [activeEmailId, setActiveEmailId] = useState<number | null>(null)
  const [checkedIds, setCheckedIds] = useState<Set<number>>(new Set())
  const [searchInput, setSearchInput] = useState('')
  const [debouncedSearch, setDebouncedSearch] = useState('')
  const [filterMode, setFilterMode] = useState<MailFilterMode>('all')

  const [composeContext, setComposeContext] = useState<ComposeContext | null>(null)
  const [shortcutsOpen, setShortcutsOpen] = useState(false)
  const [quickMoveIds, setQuickMoveIds] = useState<number[] | null>(null)
  const [confirmForcedDeleteIds, setConfirmForcedDeleteIds] = useState<number[] | null>(null)

  const searchInputRef = useRef<HTMLInputElement | null>(null)

  // Debounce search input
  useEffect(() => {
    const timer = window.setTimeout(() => setDebouncedSearch(searchInput), 220)
    return () => window.clearTimeout(timer)
  }, [searchInput])

  // Queries
  const { data: user } = useUserMeQuery(true)
  const { data: addresses = [] } = useAddressesQuery(true)
  const { data: sidebarGroups = [] } = useSidebarGroupsQuery(true)
  const { data: groupOptions = [] } = useGroupOptionsQuery(true)

  const customGroups = useMemo(
    () => sidebarGroups.filter((g) => g.id > 0),
    [sidebarGroups],
  )

  const activeFolder: FolderSelection = useMemo(() => {
    const parsed = parseFolderSegment(folderParam)
    if (parsed.kind === 'group') {
      const matched = customGroups.find((g) => g.id === parsed.id)
      if (matched) {
        return { kind: 'group', id: matched.id, name: matched.label, tag: matched.tag }
      }
    }
    return parsed
  }, [folderParam, customGroups])

  const {
    data: emailPages,
    isLoading: isListLoading,
    isFetching: isListFetching,
    isFetchingNextPage,
    hasNextPage,
    isError: isListError,
    fetchNextPage,
    refetch: refetchList,
  } = useEmailInfiniteList(activeFolder, debouncedSearch)

  const { data: activeEmailDetail } = useEmailDetailQuery(activeEmailId)
  const { prefetch: prefetchEmail, cancel: cancelPrefetch } = usePrefetchEmailDetail()

  const { drafts: localDrafts, upsertDraft, removeDraft } = useLocalDrafts()

  const starMutation = useStarEmailsMutation()
  const markReadMutation = useMarkReadMutation()
  const deleteMutation = useDeleteEmailsMutation()
  const moveMutation = useMoveEmailsMutation()
  const createGroupMutation = useCreateGroupMutation()
  const deleteGroupMutation = useDeleteGroupMutation()

  // Flatten paginated emails + apply client-side filter pills / starred filter
  const rawEmails = useMemo(() => {
    if (!emailPages) return []
    const seen = new Set<number>()
    const out: EmailListItem[] = []
    for (const page of emailPages.pages) {
      for (const item of page.list ?? []) {
        if (!seen.has(item.id)) {
          seen.add(item.id)
          out.push(item)
        }
      }
    }
    return out
  }, [emailPages])

  const emails = useMemo(() => {
    let list = rawEmails
    if (filterMode === 'unread') {
      list = list.filter((e) => !e.is_read)
    } else if (filterMode === 'attachments') {
      list = list.filter(
        (e) =>
          Boolean(e.has_attachments) ||
          Boolean(e.attachments_list && e.attachments_list.some((a) => !a.inline)),
      )
    }
    return list
  }, [rawEmails, filterMode])

  const totalCount = emailPages?.pages[0]?.total
  const activeListItem = useMemo(
    () => emails.find((e) => e.id === activeEmailId) ?? null,
    [emails, activeEmailId],
  )

  // Auto-select first email on desktop when folder loads if reading pane is visible
  useEffect(() => {
    if (
      isDesktop &&
      readingPanePosition !== 'off' &&
      activeEmailId === null &&
      emails.length > 0 &&
      emails[0]
    ) {
      setActiveEmailId(emails[0].id)
    }
  }, [isDesktop, readingPanePosition, activeEmailId, emails])

  // Clear selection when changing folder
  useEffect(() => {
    setCheckedIds(new Set())
    if (isMobile) {
      setActiveEmailId(null)
    }
  }, [folderParam, isMobile])

  // Compute unread counts from sidebarGroups + inbox items
  const unreadCounts = useMemo<Partial<Record<SystemFolderKey, number>>>(() => {
    const counts: Partial<Record<SystemFolderKey, number>> = {}
    for (const g of sidebarGroups) {
      if (typeof g.unread !== 'number') continue
      if (g.tag.includes('"starred":true')) {
        counts.starred = g.unread
      } else if (g.tag.includes('"type":0') && g.tag.includes('"group_id":0')) {
        counts.inbox = g.unread
      } else if (g.tag.includes('"status":4')) {
        counts.drafts = g.unread
      } else if (g.tag.includes('"status":5')) {
        counts.junk = g.unread
      } else if (g.tag.includes('"status":3')) {
        counts.trash = g.unread
      }
    }
    if (
      counts.inbox === undefined &&
      activeFolder.kind === 'system' &&
      activeFolder.key === 'inbox'
    ) {
      counts.inbox = rawEmails.filter((e) => !e.is_read).length
    }
    return counts
  }, [sidebarGroups, activeFolder, rawEmails])

  // Contact suggestions mined from recent emails & user addresses
  const contactSuggestions = useMemo<ContactSuggestion[]>(() => {
    const map = new Map<string, ContactSuggestion>()
    for (const addr of addresses) {
      map.set(addr.address.toLowerCase(), {
        name: user?.display_name || user?.name || '',
        email: addr.address,
      })
    }
    for (const item of rawEmails.slice(0, 100)) {
      if (item.sender?.EmailAddress) {
        const key = item.sender.EmailAddress.toLowerCase()
        if (!map.has(key)) {
          map.set(key, { name: item.sender.Name || '', email: item.sender.EmailAddress })
        }
      }
      for (const rec of item.to ?? []) {
        if (rec.EmailAddress) {
          const key = rec.EmailAddress.toLowerCase()
          if (!map.has(key)) {
            map.set(key, { name: rec.Name || '', email: rec.EmailAddress })
          }
        }
      }
    }
    return Array.from(map.values())
  }, [addresses, rawEmails, user])

  const sendableAddresses = useMemo(
    () => addresses.filter((a) => a.can_send),
    [addresses],
  )

  const folderTitle = useMemo(() => {
    if (activeFolder.kind === 'group') {
      return activeFolder.name || t('nav.groups')
    }
    const map: Record<SystemFolderKey, string> = {
      inbox: t('nav.inbox'),
      starred: t('nav.starred'),
      sent: t('nav.sent'),
      drafts: t('nav.drafts'),
      junk: t('nav.junk'),
      trash: t('nav.trash'),
    }
    return map[activeFolder.key]
  }, [activeFolder, t])

  const currentRestoreGroupId = useMemo(() => {
    if (activeFolder.kind === 'group') return activeFolder.id
    if (activeFolder.key === 'junk') return SYSTEM_GROUP_IDS.JUNK
    if (activeFolder.key === 'sent') return SYSTEM_GROUP_IDS.SENT
    if (activeFolder.key === 'drafts') return SYSTEM_GROUP_IDS.DRAFTS
    return SYSTEM_GROUP_IDS.INBOX
  }, [activeFolder])

  // Handlers
  const handleSelectFolder = useCallback(
    (folder: FolderSelection) => {
      const seg = folderToRouteSegment(folder)
      navigate(`/mail/${seg}`)
      setActiveEmailId(null)
      setDrawerOpen(false)
    },
    [navigate],
  )

  const handleSelectEmail = useCallback(
    (email: EmailListItem) => {
      setActiveEmailId(email.id)
      if (!email.is_read) {
        markReadMutation.mutate({ ids: [email.id], isRead: true, silent: true })
      }
    },
    [markReadMutation],
  )

  const handleToggleCheck = useCallback((id: number) => {
    setCheckedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }, [])

  const handleSelectAll = useCallback(() => {
    setCheckedIds(new Set(emails.map((e) => e.id)))
  }, [emails])

  const handleClearSelection = useCallback(() => {
    setCheckedIds(new Set())
  }, [])

  const advanceSelectionAfterRemoval = useCallback(
    (removedIds: number[]) => {
      const removedSet = new Set(removedIds)
      if (activeEmailId && removedSet.has(activeEmailId)) {
        const currentIdx = emails.findIndex((e) => e.id === activeEmailId)
        const nextEmail =
          emails.slice(currentIdx + 1).find((e) => !removedSet.has(e.id)) ??
          emails
            .slice(0, Math.max(0, currentIdx))
            .reverse()
            .find((e) => !removedSet.has(e.id))
        setActiveEmailId(isMobile ? null : (nextEmail?.id ?? null))
      }
      setCheckedIds((prev) => {
        const next = new Set(prev)
        removedIds.forEach((id) => next.delete(id))
        return next
      })
    },
    [activeEmailId, emails, isMobile],
  )

  const handleDeleteEmails = useCallback(
    (ids: number[], forced: boolean) => {
      if (ids.length === 0) return
      if (forced) {
        setConfirmForcedDeleteIds(ids)
        return
      }
      advanceSelectionAfterRemoval(ids)
      deleteMutation.mutate({
        ids,
        forcedDel: false,
        restoreGroupId: currentRestoreGroupId,
      })
    },
    [advanceSelectionAfterRemoval, currentRestoreGroupId, deleteMutation],
  )

  const handleConfirmForcedDelete = () => {
    if (!confirmForcedDeleteIds) return
    const ids = confirmForcedDeleteIds
    setConfirmForcedDeleteIds(null)
    advanceSelectionAfterRemoval(ids)
    deleteMutation.mutate({ ids, forcedDel: true })
  }

  const handleMoveEmails = useCallback(
    (ids: number[], groupId: number, label: string) => {
      if (ids.length === 0) return
      advanceSelectionAfterRemoval(ids)
      moveMutation.mutate({
        ids,
        groupId,
        targetLabel: label,
        restoreGroupId: currentRestoreGroupId,
      })
    },
    [advanceSelectionAfterRemoval, currentRestoreGroupId, moveMutation],
  )

  const handleToggleStar = useCallback(
    (ids: number[], nextStarred: boolean) => {
      starMutation.mutate({ ids, starred: nextStarred })
    },
    [starMutation],
  )

  const handleToggleRead = useCallback(
    (ids: number[], nextRead: boolean) => {
      markReadMutation.mutate({ ids, isRead: nextRead })
    },
    [markReadMutation],
  )

  const handleRefresh = useCallback(() => {
    void refetchList().then(() => {
      toast.success(t('mail.toast.refreshed'))
    })
    void queryClient.invalidateQueries({ queryKey: queryKeys.sidebarGroups })
  }, [refetchList, queryClient, t])

  const handleOpenCompose = useCallback((ctx: ComposeContext) => {
    setComposeContext(ctx)
  }, [])

  const handleResumeDraft = useCallback((draft: LocalDraft) => {
    setComposeContext({ mode: 'draft', draft })
  }, [])

  const handleDiscardDraft = useCallback(
    (draftId: string) => {
      removeDraft(draftId)
      toast(t('mail.draft.discarded'))
    },
    [removeDraft, t],
  )

  const handleLogout = useCallback(async () => {
    try {
      await authApi.logout()
    } catch {
      // Proceed to clear client state regardless
    }
    queryClient.clear()
    toast.success(t('auth.logoutSuccess'))
    navigate('/login', { replace: true })
  }, [navigate, queryClient, t])

  const handleLanguageChange = useCallback((lang: SupportedLanguage) => {
    void userApi.me({ lang }).catch(() => {
      // Ignore if backend doesn't yet support lang update (C2)
    })
  }, [])

  // Resizable column pointer drag handlers (Desktop)
  const startResize = useCallback(
    (target: 'sidebar' | 'list', startX: number) => {
      const startW = target === 'sidebar' ? sidebarWidth : listWidth
      document.body.style.cursor = 'col-resize'
      document.body.style.userSelect = 'none'

      const onMove = (e: MouseEvent) => {
        const delta = e.clientX - startX
        if (target === 'sidebar') {
          const next = Math.max(MIN_SIDEBAR_W, Math.min(MAX_SIDEBAR_W, startW + delta))
          setSidebarWidth(next)
          setStorageItem('sidebar_width', next)
        } else {
          const next = Math.max(MIN_LIST_W, Math.min(MAX_LIST_W, startW + delta))
          setListWidth(next)
          setStorageItem('list_width', next)
        }
      }

      const onUp = () => {
        document.body.style.cursor = ''
        document.body.style.userSelect = ''
        window.removeEventListener('mousemove', onMove)
        window.removeEventListener('mouseup', onUp)
      }

      window.addEventListener('mousemove', onMove)
      window.addEventListener('mouseup', onUp)
    },
    [sidebarWidth, listWidth],
  )

  const toggleSidebarCollapsed = useCallback(() => {
    setSidebarCollapsed((prev) => {
      const next = !prev
      setStorageItem('sidebar_collapsed', next)
      return next
    })
  }, [])

  // Keyboard shortcuts
  const shortcutHandlers = useMemo(
    () => ({
      onNext: () => {
        if (emails.length === 0) return
        const idx = emails.findIndex((e) => e.id === activeEmailId)
        const nextItem = emails[Math.min(emails.length - 1, idx + 1)] ?? emails[0]
        if (nextItem) handleSelectEmail(nextItem)
      },
      onPrev: () => {
        if (emails.length === 0) return
        const idx = emails.findIndex((e) => e.id === activeEmailId)
        const prevItem = emails[Math.max(0, idx <= 0 ? 0 : idx - 1)] ?? emails[0]
        if (prevItem) handleSelectEmail(prevItem)
      },
      onOpen: () => {
        if (activeEmailId) {
          const current = emails.find((e) => e.id === activeEmailId)
          if (current) handleSelectEmail(current)
        } else if (emails[0]) {
          handleSelectEmail(emails[0])
        }
      },
      onArchive: () => {
        const targets = checkedIds.size > 0 ? Array.from(checkedIds) : activeEmailId ? [activeEmailId] : []
        if (targets.length > 0) setQuickMoveIds(targets)
      },
      onDelete: () => {
        const targets = checkedIds.size > 0 ? Array.from(checkedIds) : activeEmailId ? [activeEmailId] : []
        const isInTrash = activeFolder.kind === 'system' && activeFolder.key === 'trash'
        if (targets.length > 0) handleDeleteEmails(targets, isInTrash)
      },
      onStar: () => {
        if (activeEmailId) {
          const currentStarred = Boolean(
            activeEmailDetail?.is_starred ?? activeListItem?.is_starred,
          )
          handleToggleStar([activeEmailId], !currentStarred)
        }
      },
      onReply: () => {
        if (activeEmailDetail) handleOpenCompose({ mode: 'reply', sourceEmail: activeEmailDetail })
      },
      onReplyAll: () => {
        if (activeEmailDetail) handleOpenCompose({ mode: 'replyAll', sourceEmail: activeEmailDetail })
      },
      onForward: () => {
        if (activeEmailDetail) handleOpenCompose({ mode: 'forward', sourceEmail: activeEmailDetail })
      },
      onCompose: () => {
        handleOpenCompose({ mode: 'new' })
      },
      onSearch: () => {
        searchInputRef.current?.focus()
        searchInputRef.current?.select()
      },
      onEscape: () => {
        if (shortcutsOpen) {
          setShortcutsOpen(false)
          return
        }
        if (quickMoveIds) {
          setQuickMoveIds(null)
          return
        }
        if (composeContext) {
          setComposeContext(null)
          return
        }
        if (drawerOpen) {
          setDrawerOpen(false)
          return
        }
        if (checkedIds.size > 0) {
          setCheckedIds(new Set())
          return
        }
        if (searchInput) {
          setSearchInput('')
          return
        }
        if (isMobile && activeEmailId) {
          setActiveEmailId(null)
        }
      },
      onHelp: () => {
        setShortcutsOpen((v) => !v)
      },
    }),
    [
      emails,
      activeEmailId,
      activeEmailDetail,
      checkedIds,
      activeFolder,
      shortcutsOpen,
      quickMoveIds,
      composeContext,
      drawerOpen,
      searchInput,
      isMobile,
      handleSelectEmail,
      handleDeleteEmails,
      handleToggleStar,
      activeListItem,
      handleOpenCompose,
    ],
  )

  useKeyboardShortcuts(
    shortcutHandlers,
    Boolean(composeContext) || shortcutsOpen || Boolean(quickMoveIds) || Boolean(confirmForcedDeleteIds),
  )

  const sidebarNode = (isDrawer: boolean) => (
    <MailSidebar
      activeFolder={activeFolder}
      collapsed={!isDrawer && sidebarCollapsed}
      user={user}
      customGroups={customGroups}
      unreadCounts={unreadCounts}
      starredCount={unreadCounts.starred ?? 0}
      draftsCount={(unreadCounts.drafts ?? 0) + localDrafts.length}
      isMobileDrawer={isDrawer}
      onToggleCollapse={isDrawer ? undefined : toggleSidebarCollapsed}
      onSelectFolder={handleSelectFolder}
      onCompose={() => {
        setDrawerOpen(false)
        handleOpenCompose({ mode: 'new' })
      }}
      onCreateGroup={async (name) => {
        await createGroupMutation.mutateAsync({ name })
      }}
      onDeleteGroup={async (id, name) => {
        await deleteGroupMutation.mutateAsync({ id, name })
        if (activeFolder.kind === 'group' && activeFolder.id === id) {
          handleSelectFolder({ kind: 'system', key: 'inbox' })
        }
      }}
      onOpenShortcuts={() => {
        setDrawerOpen(false)
        setShortcutsOpen(true)
      }}
      onLogout={() => void handleLogout()}
      onLanguageChange={handleLanguageChange}
    />
  )

  const listPaneNode = (
    <EmailListPane
      folder={activeFolder}
      folderTitle={folderTitle}
      emails={emails}
      totalCount={totalCount}
      localDrafts={localDrafts}
      activeEmailId={activeEmailId}
      checkedIds={checkedIds}
      searchQuery={searchInput}
      filterMode={filterMode}
      density={listDensity}
      isLoading={isListLoading}
      isFetching={isListFetching}
      isFetchingNextPage={isFetchingNextPage}
      hasNextPage={Boolean(hasNextPage)}
      isError={isListError}
      groupOptions={groupOptions}
      isMobile={isMobile}
      isTablet={isTablet}
      searchInputRef={searchInputRef}
      onOpenMobileDrawer={() => setDrawerOpen(true)}
      onSearchChange={setSearchInput}
      onFilterChange={setFilterMode}
      onRefresh={handleRefresh}
      onLoadMore={() => void fetchNextPage()}
      onSelectEmail={handleSelectEmail}
      onToggleCheck={handleToggleCheck}
      onSelectAll={handleSelectAll}
      onClearSelection={handleClearSelection}
      onToggleStar={(id, nextStarred) => handleToggleStar([id], nextStarred)}
      onBulkMarkRead={handleToggleRead}
      onBulkStar={(ids) => handleToggleStar(ids, true)}
      onBulkDelete={handleDeleteEmails}
      onBulkMove={handleMoveEmails}
      onPrefetchEmail={prefetchEmail}
      onCancelPrefetch={cancelPrefetch}
      onResumeDraft={handleResumeDraft}
      onDiscardDraft={handleDiscardDraft}
    />
  )

  const readingPaneNode = (
    <ErrorBoundary inline resetKey={activeEmailId}>
      <ReadingPane
        emailId={activeEmailId}
        folder={activeFolder}
        unreadInboxCount={unreadCounts.inbox ?? 0}
        isStarred={Boolean(activeListItem?.is_starred)}
        isReadFallback={activeListItem ? activeListItem.is_read : true}
        groupOptions={groupOptions}
        isMobile={isMobile || (isDesktop && readingPanePosition === 'off')}
        onBack={() => setActiveEmailId(null)}
        onReply={(email: EmailDetail, allowExt?: boolean) =>
          handleOpenCompose({
            mode: 'reply',
            sourceEmail: email,
            allowExternalImages: Boolean(allowExt),
          })
        }
        onReplyAll={(email: EmailDetail, allowExt?: boolean) =>
          handleOpenCompose({
            mode: 'replyAll',
            sourceEmail: email,
            allowExternalImages: Boolean(allowExt),
          })
        }
        onForward={(email: EmailDetail, allowExt?: boolean) =>
          handleOpenCompose({
            mode: 'forward',
            sourceEmail: email,
            allowExternalImages: Boolean(allowExt),
          })
        }
        onEditDraft={(email: EmailDetail) => handleOpenCompose({ mode: 'serverDraft', sourceEmail: email })}
        onToggleStar={(id, nextStarred) => handleToggleStar([id], nextStarred)}
        onToggleRead={(id, nextRead) => handleToggleRead([id], nextRead)}
        onDelete={(id, forced) => handleDeleteEmails([id], forced)}
        onMove={(id, groupId, label) => handleMoveEmails([id], groupId, label)}
      />
    </ErrorBoundary>
  )

  const effectiveSidebarW = sidebarCollapsed ? COLLAPSED_SIDEBAR_W : sidebarWidth

  return (
    <div className="relative flex h-full w-full overflow-hidden bg-[var(--surface-canvas)]">
      {/* Desktop (>=1024px): 3-Column / Bottom Split / List-Only based on readingPanePosition */}
      {isDesktop && (
        <>
          <div
            style={{ width: `${effectiveSidebarW}px` }}
            className="h-full shrink-0 border-r border-[var(--border-hairline)] transition-[width] duration-[var(--duration-fast)]"
          >
            {sidebarNode(false)}
          </div>

          {!sidebarCollapsed && (
            <div
              role="separator"
              aria-orientation="vertical"
              onMouseDown={(e) => startResize('sidebar', e.clientX)}
              className="group relative z-10 -mx-1 w-2 shrink-0 cursor-col-resize"
            >
              <div className="mx-auto h-full w-[2px] bg-transparent transition-colors group-hover:bg-[var(--accent)]/60" />
            </div>
          )}

          {readingPanePosition === 'right' ? (
            <>
              <div
                style={{ width: `${listWidth}px` }}
                className="h-full shrink-0 border-r border-[var(--border-hairline)]"
              >
                {listPaneNode}
              </div>

              <div
                role="separator"
                aria-orientation="vertical"
                onMouseDown={(e) => startResize('list', e.clientX)}
                className="group relative z-10 -mx-1 w-2 shrink-0 cursor-col-resize"
              >
                <div className="mx-auto h-full w-[2px] bg-transparent transition-colors group-hover:bg-[var(--accent)]/60" />
              </div>

              <main className="h-full min-w-0 flex-1">{readingPaneNode}</main>
            </>
          ) : readingPanePosition === 'bottom' ? (
            <div className="flex h-full min-w-0 flex-1 flex-col overflow-hidden">
              <div className="h-[44%] min-h-[220px] shrink-0 border-b border-[var(--border-hairline)]">
                {listPaneNode}
              </div>
              <main className="min-h-0 flex-1">{readingPaneNode}</main>
            </div>
          ) : (
            <div className="relative h-full min-w-0 flex-1 overflow-hidden">
              {activeEmailId !== null ? readingPaneNode : listPaneNode}
            </div>
          )}
        </>
      )}

      {/* Tablet (768px-1023px): 2-Column Layout + Slide-Over Sidebar Drawer */}
      {isTablet && (
        <>
          <div className="h-full w-[340px] shrink-0 border-r border-[var(--border-hairline)]">
            {listPaneNode}
          </div>
          <main className="h-full min-w-0 flex-1">{readingPaneNode}</main>
        </>
      )}

      {/* Mobile (<768px): Single Column + Push Detail + Bottom Nav */}
      {isMobile && (
        <div className="relative flex h-full w-full flex-col overflow-hidden">
          <main className="min-h-0 flex-1 overflow-hidden">{listPaneNode}</main>

          {/* Mobile Bottom Navigation */}
          {!activeEmailId && (
            <nav
              aria-label={t('nav.menu')}
              className="shrink-0 border-t border-[var(--border-hairline)] bg-[var(--surface-sidebar)]/95 backdrop-blur-md pb-safe"
            >
              <div className="grid h-14 grid-cols-5 items-center px-1">
                <MobileNavBtn
                  active={activeFolder.kind === 'system' && activeFolder.key === 'inbox'}
                  icon={Inbox}
                  label={t('nav.inbox')}
                  badge={unreadCounts.inbox}
                  onClick={() => handleSelectFolder({ kind: 'system', key: 'inbox' })}
                />
                <MobileNavBtn
                  active={activeFolder.kind === 'system' && activeFolder.key === 'starred'}
                  icon={Star}
                  label={t('nav.starred')}
                  onClick={() => handleSelectFolder({ kind: 'system', key: 'starred' })}
                />
                <div className="flex justify-center">
                  <button
                    type="button"
                    onClick={() => handleOpenCompose({ mode: 'new' })}
                    aria-label={t('nav.compose')}
                    className="flex h-10 w-10 items-center justify-center rounded-full bg-[var(--accent)] text-white shadow-[var(--shadow-sm)] active:scale-95 transition-transform"
                  >
                    <PenSquare className="h-4 w-4" />
                  </button>
                </div>
                <MobileNavBtn
                  active={activeFolder.kind === 'system' && activeFolder.key === 'sent'}
                  icon={Send}
                  label={t('nav.sent')}
                  onClick={() => handleSelectFolder({ kind: 'system', key: 'sent' })}
                />
                <MobileNavBtn
                  active={false}
                  icon={Settings}
                  label={t('nav.settings')}
                  onClick={() => navigate('/settings')}
                />
              </div>
            </nav>
          )}

          {/* Fullscreen push-in reading view on mobile */}
          {activeEmailId !== null && (
            <div className="fixed inset-0 z-30 bg-[var(--surface-reading)]">
              {readingPaneNode}
            </div>
          )}
        </div>
      )}

      {/* Left Slide-Over Sidebar Drawer (Mobile & Tablet) */}
      {(isMobile || isTablet) && drawerOpen && (
        <div className="fixed inset-0 z-40 flex">
          <div
            aria-hidden="true"
            onClick={() => setDrawerOpen(false)}
            className="fixed inset-0 bg-slate-950/45 backdrop-blur-[1px]"
          />
          <div className="relative z-10 h-full w-[276px] max-w-[82vw] shadow-[var(--shadow-lg)]">
            <button
              type="button"
              onClick={() => setDrawerOpen(false)}
              aria-label={t('common.close')}
              className="absolute right-2.5 top-2.5 z-20 inline-flex h-8 w-8 items-center justify-center rounded-[var(--radius-sm)] text-[var(--ink-muted)] hover:bg-[var(--surface-hover)] hover:text-[var(--ink-primary)]"
            >
              <X className="h-4 w-4" />
            </button>
            {sidebarNode(true)}
          </div>
        </div>
      )}

      {/* Compose Window (lazy-loaded with TipTap) */}
      {composeContext && (
        <Suspense fallback={null}>
          <ComposeModal
            context={composeContext}
            sendableAddresses={sendableAddresses}
            userDisplayName={user?.display_name || user?.name || user?.account || ''}
            suggestions={contactSuggestions}
            isMobile={isMobile}
            onSaveDraft={upsertDraft}
            onDeleteDraft={removeDraft}
            onSentSuccess={() => {
              void queryClient.invalidateQueries({ queryKey: queryKeys.emailListsRoot })
            }}
            onClose={() => setComposeContext(null)}
          />
        </Suspense>
      )}

      {/* Keyboard Shortcuts Help Modal */}
      <ShortcutsDialog open={shortcutsOpen} onOpenChange={setShortcutsOpen} />

      {/* Quick Move Dialog (triggered by `e` shortcut) */}
      <Dialog open={quickMoveIds !== null} onOpenChange={(open) => !open && setQuickMoveIds(null)}>
        <DialogContent className="max-w-xs">
          <DialogHeader>
            <DialogTitle>{t('mail.actions.move')}</DialogTitle>
          </DialogHeader>
          <div className="mt-3 space-y-1">
            {[
              { id: SYSTEM_GROUP_IDS.INBOX, name: t('nav.inbox') },
              { id: SYSTEM_GROUP_IDS.JUNK, name: t('nav.junk') },
              ...groupOptions
                .filter((g) => g.id < SYSTEM_GROUP_IDS.INBOX)
                .map((g) => ({ id: g.id, name: g.name })),
            ].map((dest) => (
              <button
                key={dest.id}
                type="button"
                onClick={() => {
                  if (quickMoveIds) {
                    handleMoveEmails(quickMoveIds, dest.id, dest.name)
                  }
                  setQuickMoveIds(null)
                }}
                className="flex w-full items-center gap-2.5 rounded-[var(--radius-sm)] px-3 py-2 text-[13.5px] text-[var(--ink-primary)] transition-colors hover:bg-[var(--surface-hover)]"
              >
                <Folder className="h-4 w-4 text-[var(--ink-muted)]" />
                <span>{dest.name}</span>
              </button>
            ))}
          </div>
        </DialogContent>
      </Dialog>

      {/* Confirm Permanent Delete Dialog */}
      <Dialog
        open={confirmForcedDeleteIds !== null}
        onOpenChange={(open) => !open && setConfirmForcedDeleteIds(null)}
      >
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>{t('mail.actions.deleteForever')}</DialogTitle>
            <DialogDescription>
              {confirmForcedDeleteIds
                ? t('mail.confirmDeleteForever', { count: confirmForcedDeleteIds.length })
                : ''}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setConfirmForcedDeleteIds(null)}
            >
              {t('common.cancel')}
            </Button>
            <Button variant="danger" size="sm" onClick={handleConfirmForcedDelete}>
              {t('mail.actions.deleteForever')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function MobileNavBtn({
  active,
  icon: Icon,
  label,
  badge,
  onClick,
}: {
  active: boolean
  icon: typeof Inbox
  label: string
  badge?: number
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'relative flex flex-col items-center justify-center gap-0.5 py-1 text-[10.5px] transition-colors',
        active ? 'font-medium text-[var(--accent)]' : 'text-[var(--ink-muted)]',
      )}
    >
      <div className="relative">
        <Icon className="h-[19px] w-[19px]" />
        {badge !== undefined && badge > 0 && (
          <span className="absolute -right-2 -top-1 flex h-3.5 min-w-[14px] items-center justify-center rounded-full bg-[var(--accent)] px-1 text-[9px] font-semibold text-white">
            {badge > 99 ? '99+' : badge}
          </span>
        )}
      </div>
      <span className="truncate max-w-[64px]">{label}</span>
    </button>
  )
}
