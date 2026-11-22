import { useState } from 'react'
import { useNavigate } from 'react-router'
import { useTranslation } from 'react-i18next'
import {
  Check,
  ChevronDown,
  ChevronRight,
  FileEdit,
  Folder,
  Inbox,
  Keyboard,
  Languages,
  LogOut,
  Monitor,
  Moon,
  PanelLeftClose,
  PanelLeftOpen,
  PenSquare,
  Plus,
  Send,
  Settings,
  Shield,
  ShieldAlert,
  Star,
  Sun,
  Trash2,
} from 'lucide-react'
import type { GroupTreeItem, UserProfile } from '@/api'
import { getAppLanguage, setAppLanguage, type SupportedLanguage } from '@/i18n'
import { useTheme, type ThemeMode } from '@/lib/theme'
import { prefetchAdminPage, prefetchComposeModal, prefetchSettingsPage } from '@/lib/prefetch'
import { cn } from '@/lib/cn'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { BrandLogo } from '@/components/common/BrandLogo'
import { SenderAvatar } from '@/components/common/SenderAvatar'
import type { FolderSelection, SystemFolderKey } from './types'

interface MailSidebarProps {
  activeFolder: FolderSelection
  collapsed: boolean
  user?: UserProfile
  customGroups: GroupTreeItem[]
  unreadCounts: Partial<Record<SystemFolderKey, number>>
  starredCount: number
  draftsCount: number
  isMobileDrawer?: boolean
  onToggleCollapse?: () => void
  onSelectFolder: (folder: FolderSelection) => void
  onCompose: () => void
  onCreateGroup: (name: string) => Promise<void>
  onDeleteGroup: (id: number, name: string) => Promise<void>
  onOpenShortcuts: () => void
  onLogout: () => void
  onLanguageChange?: (lang: SupportedLanguage) => void
}

export function MailSidebar({
  activeFolder,
  collapsed,
  user,
  customGroups,
  unreadCounts,
  starredCount,
  draftsCount,
  isMobileDrawer = false,
  onToggleCollapse,
  onSelectFolder,
  onCompose,
  onCreateGroup,
  onDeleteGroup,
  onOpenShortcuts,
  onLogout,
  onLanguageChange,
}: MailSidebarProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { mode: themeMode, setMode: setThemeMode } = useTheme()
  const currentLang = getAppLanguage()

  const [groupsExpanded, setGroupsExpanded] = useState(true)
  const [createDialogOpen, setCreateDialogOpen] = useState(false)
  const [newGroupName, setNewGroupName] = useState('')
  const [creatingGroup, setCreatingGroup] = useState(false)
  const [deletingGroup, setDeletingGroup] = useState<{ id: number; name: string } | null>(null)

  const systemFolders: {
    key: SystemFolderKey
    label: string
    icon: typeof Inbox
    count?: number
  }[] = [
    { key: 'inbox', label: t('nav.inbox'), icon: Inbox, count: unreadCounts.inbox },
    { key: 'starred', label: t('nav.starred'), icon: Star, count: starredCount || undefined },
    { key: 'sent', label: t('nav.sent'), icon: Send },
    { key: 'drafts', label: t('nav.drafts'), icon: FileEdit, count: draftsCount || undefined },
    { key: 'junk', label: t('nav.junk'), icon: ShieldAlert, count: unreadCounts.junk },
    { key: 'trash', label: t('nav.trash'), icon: Trash2, count: unreadCounts.trash },
  ]

  const isSystemActive = (key: SystemFolderKey) =>
    activeFolder.kind === 'system' && activeFolder.key === key

  const isGroupActive = (id: number) =>
    activeFolder.kind === 'group' && activeFolder.id === id

  const handleCreateSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    const trimmed = newGroupName.trim()
    if (!trimmed) return
    setCreatingGroup(true)
    try {
      await onCreateGroup(trimmed)
      setNewGroupName('')
      setCreateDialogOpen(false)
    } finally {
      setCreatingGroup(false)
    }
  }

  const handleConfirmDeleteGroup = async () => {
    if (!deletingGroup) return
    await onDeleteGroup(deletingGroup.id, deletingGroup.name)
    setDeletingGroup(null)
  }

  const themeOptions: { value: ThemeMode; label: string; icon: typeof Sun }[] = [
    { value: 'light', label: t('theme.light'), icon: Sun },
    { value: 'dark', label: t('theme.dark'), icon: Moon },
    { value: 'system', label: t('theme.system'), icon: Monitor },
  ]

  const displayName = user?.display_name || user?.name || user?.account || ''
  const primaryAddress =
    user?.addresses?.find((a) => a.is_primary)?.address ||
    user?.addresses?.[0]?.address ||
    user?.account ||
    ''

  return (
    <aside className="flex h-full flex-col bg-[var(--surface-sidebar)] text-[var(--ink-primary)] select-none pt-safe pb-safe">
      {/* Top Brand + Collapse */}
      <div
        className={cn(
          'flex h-12 shrink-0 items-center px-3',
          collapsed ? 'justify-center' : 'justify-between',
        )}
      >
        <BrandLogo compact={collapsed} />
        {!isMobileDrawer && onToggleCollapse && !collapsed && (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="iconSm"
                onClick={onToggleCollapse}
                aria-label={t('nav.collapseSidebar')}
              >
                <PanelLeftClose className="h-4 w-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent side="right">{t('nav.collapseSidebar')}</TooltipContent>
          </Tooltip>
        )}
      </div>

      {/* Expand button when collapsed */}
      {!isMobileDrawer && onToggleCollapse && collapsed && (
        <div className="mb-1 flex justify-center">
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="iconSm"
                onClick={onToggleCollapse}
                aria-label={t('nav.expandSidebar')}
              >
                <PanelLeftOpen className="h-4 w-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent side="right">{t('nav.expandSidebar')}</TooltipContent>
          </Tooltip>
        </div>
      )}

      {/* Compose Primary CTA */}
      <div className={cn('px-3 pb-3 pt-1', collapsed && 'px-2')}>
        {collapsed ? (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="primary"
                size="icon"
                className="mx-auto flex h-9 w-9 rounded-[var(--radius-md)]"
                onMouseEnter={prefetchComposeModal}
                onFocus={prefetchComposeModal}
                onClick={onCompose}
                aria-label={t('nav.compose')}
              >
                <PenSquare className="h-4 w-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent side="right">{t('nav.compose')} (c)</TooltipContent>
          </Tooltip>
        ) : (
          <Button
            variant="primary"
            size="md"
            className="w-full justify-between rounded-[var(--radius-md)] px-3.5"
            onMouseEnter={prefetchComposeModal}
            onFocus={prefetchComposeModal}
            onClick={onCompose}
          >
            <span className="inline-flex items-center gap-2">
              <PenSquare className="h-4 w-4" />
              {t('nav.compose')}
            </span>
            <kbd className="rounded bg-white/20 px-1.5 py-0.5 font-mono text-[10.5px] leading-none text-white/90">
              C
            </kbd>
          </Button>
        )}
      </div>

      {/* Scrollable Navigation */}
      <nav className="mail-scroll flex-1 overflow-y-auto px-2 space-y-4">
        {/* Built-in folders */}
        <ul className="space-y-0.5">
          {systemFolders.map(({ key, label, icon: Icon, count }) => {
            const active = isSystemActive(key)
            const btn = (
              <button
                type="button"
                aria-current={active ? 'page' : undefined}
                onClick={() => onSelectFolder({ kind: 'system', key })}
                className={cn(
                  'flex w-full items-center rounded-[var(--radius-sm)] text-[13.5px] transition-colors duration-[var(--duration-fast)]',
                  collapsed ? 'justify-center px-0 py-2' : 'justify-between gap-2 px-2.5 py-1.5',
                  active
                    ? 'bg-[var(--surface-active)] font-medium text-[var(--accent-ink)]'
                    : 'text-[var(--ink-secondary)] hover:bg-[var(--surface-hover)] hover:text-[var(--ink-primary)]',
                )}
              >
                <span className="inline-flex min-w-0 items-center gap-2.5">
                  <Icon
                    className={cn(
                      'h-4 w-4 shrink-0',
                      active ? 'text-[var(--accent-ink)]' : 'text-[var(--ink-muted)]',
                    )}
                  />
                  {!collapsed && <span className="truncate">{label}</span>}
                </span>
                {!collapsed && count !== undefined && count > 0 && (
                  <span
                    className={cn(
                      'shrink-0 rounded-full px-1.5 py-0.5 text-[11px] font-medium tabular-nums leading-none',
                      active
                        ? 'bg-[var(--accent)] text-white'
                        : 'bg-[var(--surface-subtle)] text-[var(--ink-secondary)]',
                    )}
                  >
                    {count > 999 ? '999+' : count}
                  </span>
                )}
              </button>
            )

            return (
              <li key={key}>
                {collapsed ? (
                  <Tooltip>
                    <TooltipTrigger asChild>{btn}</TooltipTrigger>
                    <TooltipContent side="right">
                      {label}
                      {count ? ` (${count})` : ''}
                    </TooltipContent>
                  </Tooltip>
                ) : (
                  btn
                )}
              </li>
            )
          })}
        </ul>

        {/* Custom Groups Section */}
        {!collapsed && (
          <div>
            <div className="mb-1 flex items-center justify-between px-2">
              <button
                type="button"
                onClick={() => setGroupsExpanded((v) => !v)}
                className="inline-flex items-center gap-1 text-[11.5px] font-semibold uppercase tracking-wider text-[var(--ink-muted)] hover:text-[var(--ink-primary)]"
              >
                {groupsExpanded ? (
                  <ChevronDown className="h-3 w-3" />
                ) : (
                  <ChevronRight className="h-3 w-3" />
                )}
                <span>{t('nav.groups')}</span>
              </button>
              <button
                type="button"
                onClick={() => setCreateDialogOpen(true)}
                aria-label={t('nav.createGroup')}
                title={t('nav.createGroup')}
                className="rounded p-1 text-[var(--ink-muted)] transition-colors hover:bg-[var(--surface-hover)] hover:text-[var(--ink-primary)]"
              >
                <Plus className="h-3.5 w-3.5" />
              </button>
            </div>

            {groupsExpanded && (
              <ul className="space-y-0.5">
                {customGroups.length === 0 ? (
                  <li className="px-2.5 py-1.5 text-[12px] text-[var(--ink-subtle)]">
                    {t('nav.groupEmpty')}
                  </li>
                ) : (
                  customGroups.map((group) => {
                    const active = isGroupActive(group.id)
                    return (
                      <li key={group.id} className="group/item relative flex items-center">
                        <button
                          type="button"
                          onClick={() =>
                            onSelectFolder({
                              kind: 'group',
                              id: group.id,
                              name: group.label,
                              tag: group.tag,
                            })
                          }
                          className={cn(
                            'flex w-full items-center justify-between gap-2 rounded-[var(--radius-sm)] px-2.5 py-1.5 pr-7 text-[13px] transition-colors duration-[var(--duration-fast)]',
                            active
                              ? 'bg-[var(--surface-active)] font-medium text-[var(--accent-ink)]'
                              : 'text-[var(--ink-secondary)] hover:bg-[var(--surface-hover)] hover:text-[var(--ink-primary)]',
                          )}
                        >
                          <span className="inline-flex min-w-0 items-center gap-2.5">
                            <Folder
                              className={cn(
                                'h-3.5 w-3.5 shrink-0',
                                active ? 'text-[var(--accent-ink)]' : 'text-[var(--ink-muted)]',
                              )}
                            />
                            <span className="truncate">{group.label}</span>
                          </span>
                          {group.unread !== undefined && group.unread > 0 && (
                            <span className="shrink-0 text-[11px] tabular-nums text-[var(--ink-muted)]">
                              {group.unread}
                            </span>
                          )}
                        </button>
                        <button
                          type="button"
                          onClick={() => setDeletingGroup({ id: group.id, name: group.label })}
                          aria-label={t('nav.deleteGroup')}
                          className="absolute right-1.5 rounded p-1 text-[var(--ink-muted)] opacity-0 transition-opacity hover:bg-[var(--danger-soft)] hover:text-[var(--danger)] group-hover/item:opacity-100"
                        >
                          <Trash2 className="h-3 w-3" />
                        </button>
                      </li>
                    )
                  })
                )}
              </ul>
            )}
          </div>
        )}
      </nav>

      {/* Footer: Settings / Admin / User Profile */}
      <div className="shrink-0 border-t border-[var(--border-hairline)] p-2 space-y-0.5">
        <button
          type="button"
          onMouseEnter={prefetchSettingsPage}
          onFocus={prefetchSettingsPage}
          onClick={() => navigate('/settings')}
          title={collapsed ? t('nav.settings') : undefined}
          className={cn(
            'flex w-full items-center rounded-[var(--radius-sm)] text-[13px] text-[var(--ink-secondary)] transition-colors hover:bg-[var(--surface-hover)] hover:text-[var(--ink-primary)]',
            collapsed ? 'justify-center py-2' : 'gap-2.5 px-2.5 py-1.5',
          )}
        >
          <Settings className="h-4 w-4 shrink-0 text-[var(--ink-muted)]" />
          {!collapsed && <span className="truncate">{t('nav.settings')}</span>}
        </button>

        {user?.is_admin === 1 && (
          <button
            type="button"
            onMouseEnter={prefetchAdminPage}
            onFocus={prefetchAdminPage}
            onClick={() => navigate('/admin')}
            title={collapsed ? t('nav.admin') : undefined}
            className={cn(
              'flex w-full items-center rounded-[var(--radius-sm)] text-[13px] text-[var(--ink-secondary)] transition-colors hover:bg-[var(--surface-hover)] hover:text-[var(--ink-primary)]',
              collapsed ? 'justify-center py-2' : 'gap-2.5 px-2.5 py-1.5',
            )}
          >
            <Shield className="h-4 w-4 shrink-0 text-[var(--ink-muted)]" />
            {!collapsed && <span className="truncate">{t('nav.admin')}</span>}
          </button>
        )}

        {/* User Account Menu */}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button
              type="button"
              className={cn(
                'mt-1 flex w-full items-center rounded-[var(--radius-md)] p-1.5 text-left transition-colors hover:bg-[var(--surface-hover)]',
                collapsed ? 'justify-center' : 'gap-2.5',
              )}
            >
              <SenderAvatar name={displayName} email={primaryAddress} size="sm" />
              {!collapsed && (
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-1.5">
                    <span className="truncate text-[13px] font-medium text-[var(--ink-primary)]">
                      {displayName}
                    </span>
                    {user?.is_admin === 1 && (
                      <Badge variant="accent" className="px-1.5 py-0 text-[10px]">
                        {t('nav.adminBadge')}
                      </Badge>
                    )}
                  </div>
                  <p className="truncate text-[11.5px] text-[var(--ink-muted)]">{primaryAddress}</p>
                </div>
              )}
            </button>
          </DropdownMenuTrigger>

          <DropdownMenuContent side={collapsed ? 'right' : 'top'} align="start" className="w-56">
            <DropdownMenuLabel>{t('theme.label')}</DropdownMenuLabel>
            {themeOptions.map(({ value, label, icon: Icon }) => (
              <DropdownMenuItem
                key={value}
                onClick={() => setThemeMode(value)}
                className="justify-between"
              >
                <span className="inline-flex items-center gap-2">
                  <Icon className="h-3.5 w-3.5 text-[var(--ink-muted)]" />
                  {label}
                </span>
                {themeMode === value && <Check className="h-3.5 w-3.5 text-[var(--accent)]" />}
              </DropdownMenuItem>
            ))}

            <DropdownMenuSeparator />
            <DropdownMenuLabel>{t('language.label')}</DropdownMenuLabel>
            <DropdownMenuItem
              onClick={() => {
                setAppLanguage('zh-CN')
                onLanguageChange?.('zh-CN')
              }}
              className="justify-between"
            >
              <span className="inline-flex items-center gap-2">
                <Languages className="h-3.5 w-3.5 text-[var(--ink-muted)]" />
                {t('language.zhCN')}
              </span>
              {currentLang === 'zh-CN' && <Check className="h-3.5 w-3.5 text-[var(--accent)]" />}
            </DropdownMenuItem>
            <DropdownMenuItem
              onClick={() => {
                setAppLanguage('en')
                onLanguageChange?.('en')
              }}
              className="justify-between"
            >
              <span className="inline-flex items-center gap-2">
                <Languages className="h-3.5 w-3.5 text-[var(--ink-muted)]" />
                {t('language.en')}
              </span>
              {currentLang === 'en' && <Check className="h-3.5 w-3.5 text-[var(--accent)]" />}
            </DropdownMenuItem>

            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={onOpenShortcuts}>
              <Keyboard className="h-3.5 w-3.5 text-[var(--ink-muted)]" />
              {t('nav.shortcuts')}
            </DropdownMenuItem>
            <DropdownMenuItem destructive onClick={onLogout}>
              <LogOut className="h-3.5 w-3.5" />
              {t('auth.logout')}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      {/* Create Group Dialog */}
      <Dialog open={createDialogOpen} onOpenChange={setCreateDialogOpen}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>{t('nav.createGroup')}</DialogTitle>
          </DialogHeader>
          <form onSubmit={handleCreateSubmit} className="mt-3 space-y-3">
            <Input
              value={newGroupName}
              onChange={(e) => setNewGroupName(e.target.value)}
              placeholder={t('nav.groupNamePlaceholder')}
              maxLength={10}
              autoFocus
            />
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => setCreateDialogOpen(false)}
              >
                {t('common.cancel')}
              </Button>
              <Button
                type="submit"
                size="sm"
                disabled={!newGroupName.trim() || creatingGroup}
              >
                {t('common.create')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Confirm Delete Group Dialog */}
      <Dialog open={deletingGroup !== null} onOpenChange={(open) => !open && setDeletingGroup(null)}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>{t('nav.deleteGroup')}</DialogTitle>
          </DialogHeader>
          <p className="mt-2 text-[13.5px] leading-relaxed text-[var(--ink-secondary)]">
            {deletingGroup ? t('nav.deleteGroupConfirm', { name: deletingGroup.name }) : ''}
          </p>
          <DialogFooter>
            <Button variant="outline" size="sm" onClick={() => setDeletingGroup(null)}>
              {t('common.cancel')}
            </Button>
            <Button variant="danger" size="sm" onClick={() => void handleConfirmDeleteGroup()}>
              {t('common.delete')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </aside>
  )
}
