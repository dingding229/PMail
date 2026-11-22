import { useEffect, useRef } from 'react'
import { useNavigate, useParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import {
  ArrowLeft,
  AtSign,
  Filter,
  Folder,
  Forward,
  KeyRound,
  MailPlus,
  Palette,
  SendToBack,
  Shield,
  UserCog,
} from 'lucide-react'
import { cn } from '@/lib/cn'
import { useUserMeQuery } from '@/hooks/useMailQueries'
import { Button } from '@/components/ui/button'
import { ThemeToggle } from '@/components/common/ThemeToggle'
import { LanguageToggle } from '@/components/common/LanguageToggle'
import { ProfileSecuritySection } from './sections/ProfileSecuritySection'
import { AppearanceSection } from './sections/AppearanceSection'
import { AddressesSection } from './sections/AddressesSection'
import { ForwardSection } from './sections/ForwardSection'
import { RulesSection } from './sections/RulesSection'
import { GroupsSection } from './sections/GroupsSection'
import { ExternalMailSection } from './sections/ExternalMailSection'
import { TokensSection } from './sections/TokensSection'
import { QueueSection } from './sections/QueueSection'

type SettingsTabKey =
  | 'profile'
  | 'appearance'
  | 'addresses'
  | 'forward'
  | 'rules'
  | 'groups'
  | 'external'
  | 'tokens'
  | 'queue'

export function SettingsPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { tab } = useParams<{ tab?: string }>()
  const { data: me } = useUserMeQuery(true)

  const tabs: { key: SettingsTabKey; label: string; icon: typeof UserCog }[] = [
    { key: 'profile', label: t('settings.tabs.profile'), icon: UserCog },
    { key: 'appearance', label: t('settings.tabs.appearance'), icon: Palette },
    { key: 'addresses', label: t('settings.tabs.addresses'), icon: AtSign },
    { key: 'forward', label: t('settings.tabs.forward'), icon: Forward },
    { key: 'rules', label: t('settings.tabs.rules'), icon: Filter },
    { key: 'groups', label: t('settings.tabs.groups'), icon: Folder },
    { key: 'external', label: t('settings.tabs.external'), icon: MailPlus },
    { key: 'tokens', label: t('settings.tabs.tokens'), icon: KeyRound },
    { key: 'queue', label: t('settings.tabs.queue'), icon: SendToBack },
  ]

  const activeTab: SettingsTabKey = tabs.some((x) => x.key === tab)
    ? (tab as SettingsTabKey)
    : 'profile'

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
            <span className="hidden sm:inline">{t('settings.backToMail')}</span>
          </Button>
          <span className="h-4 w-px bg-[var(--border-subtle)]" />
          <h1 className="truncate text-[15px] font-semibold tracking-tight">
            {t('settings.title')}
          </h1>
        </div>
        <div className="flex items-center gap-1">
          {me?.is_admin === 1 && (
            <Button variant="outline" size="xs" onClick={() => navigate('/admin')}>
              <Shield className="h-3.5 w-3.5" />
              <span className="hidden sm:inline">{t('nav.admin')}</span>
            </Button>
          )}
          <LanguageToggle />
          <ThemeToggle />
        </div>
      </header>

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
                  onClick={() => navigate(`/settings/${key}`)}
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

      {/* Main Content */}
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
                  onClick={() => navigate(`/settings/${key}`)}
                  className={cn(
                    'flex w-full items-center gap-2.5 rounded-[var(--radius-sm)] px-3 py-2 text-[13.5px] transition-colors',
                    active
                      ? 'bg-[var(--surface-active)] font-medium text-[var(--accent-ink)]'
                      : 'text-[var(--ink-secondary)] hover:bg-[var(--surface-hover)] hover:text-[var(--ink-primary)]',
                  )}
                >
                  <Icon
                    className={cn(
                      'h-4 w-4 shrink-0',
                      active ? 'text-[var(--accent-ink)]' : 'text-[var(--ink-muted)]',
                    )}
                  />
                  <span className="truncate">{label}</span>
                </button>
              )
            })}
          </nav>
        </aside>

        {/* Active Panel */}
        <main className="min-w-0 flex-1 pb-10">
          {activeTab === 'profile' && <ProfileSecuritySection />}
          {activeTab === 'appearance' && <AppearanceSection />}
          {activeTab === 'addresses' && <AddressesSection />}
          {activeTab === 'forward' && <ForwardSection />}
          {activeTab === 'rules' && <RulesSection />}
          {activeTab === 'groups' && <GroupsSection />}
          {activeTab === 'external' && <ExternalMailSection />}
          {activeTab === 'tokens' && <TokensSection />}
          {activeTab === 'queue' && <QueueSection scope="mine" />}
        </main>
      </div>
    </div>
  )
}

export default SettingsPage
