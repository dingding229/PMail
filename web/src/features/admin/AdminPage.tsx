import { useEffect, useRef } from 'react'
import { useNavigate, useParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import {
  ArrowLeft,
  AtSign,
  Globe,
  SendToBack,
  Settings,
  ShieldAlert,
  Users,
} from 'lucide-react'
import { cn } from '@/lib/cn'
import { useUserMeQuery } from '@/hooks/useMailQueries'
import { Button } from '@/components/ui/button'
import { ThemeToggle } from '@/components/common/ThemeToggle'
import { LanguageToggle } from '@/components/common/LanguageToggle'
import { QueueSection } from '@/features/settings/sections/QueueSection'
import { DomainsSection } from './DomainsSection'
import { UsersSection } from './UsersSection'
import { AdminAddressesSection } from './AdminAddressesSection'

type AdminTabKey = 'domains' | 'users' | 'addresses' | 'queue'

export function AdminPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { tab } = useParams<{ tab?: string }>()
  const { data: user } = useUserMeQuery(true)

  const tabs: { key: AdminTabKey; label: string; icon: typeof Globe }[] = [
    { key: 'domains', label: t('admin.tabs.domains'), icon: Globe },
    { key: 'users', label: t('admin.tabs.users'), icon: Users },
    { key: 'addresses', label: t('admin.tabs.addresses'), icon: AtSign },
    { key: 'queue', label: t('admin.tabs.queue'), icon: SendToBack },
  ]

  const activeTab: AdminTabKey = tabs.some((x) => x.key === tab)
    ? (tab as AdminTabKey)
    : 'domains'

  const activeMobileTabRef = useRef<HTMLButtonElement | null>(null)
  useEffect(() => {
    activeMobileTabRef.current?.scrollIntoView({
      behavior: 'smooth',
      inline: 'center',
      block: 'nearest',
    })
  }, [activeTab])

  return (
    <div className="flex min-h-full flex-col bg-[var(--surface-sidebar)] text-[var(--ink-primary)] pt-safe pb-safe">
      {/* Top Bar */}
      <header className="sticky top-0 z-20 flex h-14 shrink-0 items-center justify-between gap-2 border-b border-[var(--border-hairline)] bg-[var(--surface-canvas)]/95 px-4 backdrop-blur-md sm:px-6">
        <div className="flex min-w-0 items-center gap-2.5">
          <Button variant="ghost" size="sm" onClick={() => navigate('/mail/inbox')}>
            <ArrowLeft className="h-4 w-4" />
            <span className="hidden sm:inline">{t('admin.backToMail')}</span>
          </Button>
          <span className="h-4 w-px bg-[var(--border-subtle)]" />
          <h1 className="truncate text-[15px] font-semibold tracking-tight">{t('admin.title')}</h1>
        </div>
        <div className="flex items-center gap-1">
          <Button variant="outline" size="xs" onClick={() => navigate('/settings')}>
            <Settings className="h-3.5 w-3.5" />
            <span className="hidden sm:inline">{t('nav.settings')}</span>
          </Button>
          <LanguageToggle />
          <ThemeToggle />
        </div>
      </header>

      {user && user.is_admin !== 1 ? (
        <main className="mx-auto max-w-xl px-4 py-16 text-center">
          <div className="flex flex-col items-center rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-elevated)] p-10">
            <ShieldAlert className="h-9 w-9 text-[var(--danger)]" />
            <p className="mt-3 text-[14.5px] font-medium">{t('admin.noPermission')}</p>
            <Button
              variant="secondary"
              size="sm"
              className="mt-4"
              onClick={() => navigate('/mail/inbox')}
            >
              {t('admin.backToMail')}
            </Button>
          </div>
        </main>
      ) : (
        <>
          {/* Mobile horizontal scrollable tab strip (hidden scrollbar + right fade hint + auto scroll active) */}
          <div className="sticky top-14 z-10 border-b border-[var(--border-hairline)] bg-[var(--surface-canvas)] lg:hidden">
            <div className="relative">
              <div className="no-scrollbar flex gap-1.5 overflow-x-auto px-4 py-2 pr-10">
                {tabs.map(({ key, label, icon: Icon }) => {
                  const active = activeTab === key
                  return (
                    <button
                      key={key}
                      ref={active ? activeMobileTabRef : undefined}
                      type="button"
                      onClick={() => navigate(`/admin/${key}`)}
                      className={cn(
                        'inline-flex shrink-0 items-center gap-1.5 rounded-full px-3 py-1.5 text-[12.5px] font-medium transition-colors',
                        active
                          ? 'bg-[var(--accent)] text-white'
                          : 'bg-[var(--surface-subtle)] text-[var(--ink-secondary)] hover:text-[var(--ink-primary)]',
                      )}
                    >
                      <Icon className="h-3.5 w-3.5" />
                      <span>{label}</span>
                    </button>
                  )
                })}
              </div>
              <div
                aria-hidden="true"
                className="pointer-events-none absolute inset-y-0 right-0 w-10 bg-gradient-to-l from-[var(--surface-canvas)] to-transparent"
              />
            </div>
          </div>

          <div className="mx-auto flex w-full max-w-6xl flex-1 gap-6 px-4 py-6 sm:px-6">
            {/* Desktop Sidebar Navigation */}
            <aside className="hidden w-56 shrink-0 lg:block">
              <nav className="sticky top-20 space-y-0.5">
                {tabs.map(({ key, label, icon: Icon }) => {
                  const active = activeTab === key
                  return (
                    <button
                      key={key}
                      type="button"
                      onClick={() => navigate(`/admin/${key}`)}
                      className={cn(
                        'flex w-full items-center gap-2.5 rounded-[var(--radius-sm)] px-3 py-2 text-[13.5px] transition-colors',
                        active
                          ? 'bg-[var(--surface-active)] font-medium text-[var(--accent)]'
                          : 'text-[var(--ink-secondary)] hover:bg-[var(--surface-hover)] hover:text-[var(--ink-primary)]',
                      )}
                    >
                      <Icon
                        className={cn(
                          'h-4 w-4 shrink-0',
                          active ? 'text-[var(--accent)]' : 'text-[var(--ink-muted)]',
                        )}
                      />
                      <span className="truncate">{label}</span>
                    </button>
                  )
                })}
              </nav>
            </aside>

            <main className="min-w-0 flex-1 pb-10">
              {activeTab === 'domains' && <DomainsSection />}
              {activeTab === 'users' && <UsersSection />}
              {activeTab === 'addresses' && <AdminAddressesSection />}
              {activeTab === 'queue' && <QueueSection scope="global" />}
            </main>
          </div>
        </>
      )}
    </div>
  )
}

export default AdminPage
