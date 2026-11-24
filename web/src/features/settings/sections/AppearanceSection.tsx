import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  Bell,
  Check,
  Columns2,
  Info,
  List,
  Monitor,
  Moon,
  Palette,
  PanelBottom,
  PanelRight,
  Sun,
} from 'lucide-react'
import { useTheme, type ThemeMode } from '@/lib/theme'
import {
  usePreferences,
  type ListDensity,
  type ReadingPanePosition,
} from '@/lib/preferences'
import { cn } from '@/lib/cn'
import { Switch } from '@/components/ui/switch'
import { SectionCard, SectionHeader } from '@/components/settings'

export function AppearanceSection() {
  const { t } = useTranslation()
  const { mode, setMode } = useTheme()
  const {
    listDensity,
    readingPanePosition,
    autoRefresh,
    desktopNotifications,
    showBrandAvatars,
    setListDensity,
    setReadingPanePosition,
    setAutoRefresh,
    setDesktopNotifications,
    setShowBrandAvatars,
  } = usePreferences()

  const notificationSupported = typeof window !== 'undefined' && 'Notification' in window
  const [notifPermission, setNotifPermission] = useState<NotificationPermission>(() =>
    notificationSupported ? Notification.permission : 'denied',
  )

  const handleToggleDesktopNotifications = async (checked: boolean) => {
    if (!checked) {
      setDesktopNotifications(false)
      return
    }
    if (!notificationSupported) {
      setDesktopNotifications(false)
      toast.error(t('settings.appearance.desktopNotifUnsupported'))
      return
    }
    if (Notification.permission === 'granted') {
      setNotifPermission('granted')
      setDesktopNotifications(true)
      return
    }
    if (Notification.permission === 'denied') {
      setNotifPermission('denied')
      setDesktopNotifications(false)
      return
    }
    try {
      const result = await Notification.requestPermission()
      setNotifPermission(result)
      if (result === 'granted') {
        setDesktopNotifications(true)
      } else {
        setDesktopNotifications(false)
      }
    } catch {
      setDesktopNotifications(false)
    }
  }

  const themeOptions: { value: ThemeMode; label: string; desc: string; icon: typeof Sun }[] = [
    {
      value: 'light',
      label: t('theme.light'),
      desc: t('settings.appearance.themeLightDesc'),
      icon: Sun,
    },
    {
      value: 'dark',
      label: t('theme.dark'),
      desc: t('settings.appearance.themeDarkDesc'),
      icon: Moon,
    },
    {
      value: 'system',
      label: t('theme.system'),
      desc: t('settings.appearance.themeSystemDesc'),
      icon: Monitor,
    },
  ]

  const densityOptions: { value: ListDensity; label: string; desc: string }[] = [
    {
      value: 'compact',
      label: t('settings.appearance.densityCompact'),
      desc: t('settings.appearance.densityCompactDesc'),
    },
    {
      value: 'comfortable',
      label: t('settings.appearance.densityComfortable'),
      desc: t('settings.appearance.densityComfortableDesc'),
    },
    {
      value: 'spacious',
      label: t('settings.appearance.densitySpacious'),
      desc: t('settings.appearance.densitySpaciousDesc'),
    },
  ]

  const positionOptions: {
    value: ReadingPanePosition
    label: string
    desc: string
    icon: typeof PanelRight
  }[] = [
    {
      value: 'right',
      label: t('settings.appearance.readingPosRight'),
      desc: t('settings.appearance.readingPosRightDesc'),
      icon: PanelRight,
    },
    {
      value: 'bottom',
      label: t('settings.appearance.readingPosBottom'),
      desc: t('settings.appearance.readingPosBottomDesc'),
      icon: PanelBottom,
    },
    {
      value: 'off',
      label: t('settings.appearance.readingPosOff'),
      desc: t('settings.appearance.readingPosOffDesc'),
      icon: List,
    },
  ]

  return (
    <div className="space-y-5">
      <SectionCard className="space-y-6">
        <SectionHeader
          icon={Palette}
          title={t('settings.appearance.title')}
          description={t('settings.appearance.desc')}
        />

        {/* Theme Mode */}
        <div className="space-y-2.5">
          <h3 className="text-[13px] font-semibold text-[var(--ink-primary)]">
            {t('settings.appearance.themeTitle')}
          </h3>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            {themeOptions.map(({ value, label, desc, icon: Icon }) => {
              const active = mode === value
              return (
                <button
                  key={value}
                  type="button"
                  onClick={() => setMode(value)}
                  className={cn(
                    'relative flex flex-col items-start gap-2 rounded-[var(--radius-md)] border p-3.5 text-left transition-all',
                    active
                      ? 'border-[var(--accent)] bg-[var(--accent-soft)]/50 ring-1 ring-[var(--accent)]'
                      : 'border-[var(--border-subtle)] bg-[var(--surface-canvas)] hover:bg-[var(--surface-hover)]',
                  )}
                >
                  <div className="flex w-full items-center justify-between">
                    <Icon className={cn('h-4 w-4', active ? 'text-[var(--accent)]' : 'text-[var(--ink-muted)]')} />
                    {active && <Check className="h-4 w-4 text-[var(--accent)]" />}
                  </div>
                  <div>
                    <p className="text-[13.5px] font-medium text-[var(--ink-primary)]">{label}</p>
                    <p className="mt-0.5 text-[12px] text-[var(--ink-muted)]">{desc}</p>
                  </div>
                </button>
              )
            })}
          </div>
        </div>

        {/* List Density */}
        <div className="space-y-2.5">
          <div className="flex items-center gap-2">
            <Columns2 className="h-4 w-4 text-[var(--ink-muted)]" />
            <h3 className="text-[13px] font-semibold text-[var(--ink-primary)]">
              {t('settings.appearance.densityTitle')}
            </h3>
          </div>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            {densityOptions.map(({ value, label, desc }) => {
              const active = listDensity === value
              return (
                <button
                  key={value}
                  type="button"
                  onClick={() => setListDensity(value)}
                  className={cn(
                    'flex flex-col items-start gap-1 rounded-[var(--radius-md)] border p-3.5 text-left transition-all',
                    active
                      ? 'border-[var(--accent)] bg-[var(--accent-soft)]/50 ring-1 ring-[var(--accent)]'
                      : 'border-[var(--border-subtle)] bg-[var(--surface-canvas)] hover:bg-[var(--surface-hover)]',
                  )}
                >
                  <div className="flex w-full items-center justify-between">
                    <span className="text-[13.5px] font-medium text-[var(--ink-primary)]">
                      {label}
                    </span>
                    {active && <Check className="h-4 w-4 text-[var(--accent)]" />}
                  </div>
                  <p className="text-[12px] text-[var(--ink-muted)]">{desc}</p>
                </button>
              )
            })}
          </div>
        </div>

        {/* Reading Pane Position */}
        <div className="space-y-2.5">
          <h3 className="text-[13px] font-semibold text-[var(--ink-primary)]">
            {t('settings.appearance.readingPosTitle')}
          </h3>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            {positionOptions.map(({ value, label, desc, icon: Icon }) => {
              const active = readingPanePosition === value
              return (
                <button
                  key={value}
                  type="button"
                  onClick={() => setReadingPanePosition(value)}
                  className={cn(
                    'flex flex-col items-start gap-2 rounded-[var(--radius-md)] border p-3.5 text-left transition-all',
                    active
                      ? 'border-[var(--accent)] bg-[var(--accent-soft)]/50 ring-1 ring-[var(--accent)]'
                      : 'border-[var(--border-subtle)] bg-[var(--surface-canvas)] hover:bg-[var(--surface-hover)]',
                  )}
                >
                  <div className="flex w-full items-center justify-between">
                    <Icon className={cn('h-4 w-4', active ? 'text-[var(--accent)]' : 'text-[var(--ink-muted)]')} />
                    {active && <Check className="h-4 w-4 text-[var(--accent)]" />}
                  </div>
                  <div>
                    <p className="text-[13.5px] font-medium text-[var(--ink-primary)]">{label}</p>
                    <p className="mt-0.5 text-[12px] text-[var(--ink-muted)]">{desc}</p>
                  </div>
                </button>
              )
            })}
          </div>
        </div>
      </SectionCard>

      {/* Live Updates & Desktop Notifications */}
      <SectionCard>
        <SectionHeader
          icon={Bell}
          title={t('settings.appearance.liveTitle')}
          description={t('settings.appearance.liveDesc')}
        />

        <div className="divide-y divide-[var(--border-hairline)] rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)]">
          <div className="flex items-center justify-between gap-4 px-4 py-3.5">
            <div className="space-y-0.5">
              <p className="text-[13.5px] font-medium text-[var(--ink-primary)]">
                {t('settings.appearance.showBrandAvatarsLabel')}
              </p>
              <p className="text-[12px] text-[var(--ink-muted)]">
                {t('settings.appearance.showBrandAvatarsDesc')}
              </p>
            </div>
            <Switch checked={showBrandAvatars} onCheckedChange={setShowBrandAvatars} />
          </div>

          <div className="flex items-center justify-between gap-4 px-4 py-3.5">
            <div className="space-y-0.5">
              <p className="text-[13.5px] font-medium text-[var(--ink-primary)]">
                {t('settings.appearance.autoRefreshLabel')}
              </p>
              <p className="text-[12px] text-[var(--ink-muted)]">
                {t('settings.appearance.autoRefreshDesc')}
              </p>
            </div>
            <Switch checked={autoRefresh} onCheckedChange={setAutoRefresh} />
          </div>

          <div className="space-y-2.5 px-4 py-3.5">
            <div className="flex items-center justify-between gap-4">
              <div className="space-y-0.5">
                <p className="text-[13.5px] font-medium text-[var(--ink-primary)]">
                  {t('settings.appearance.desktopNotifLabel')}
                </p>
                <p className="text-[12px] text-[var(--ink-muted)]">
                  {t('settings.appearance.desktopNotifDesc')}
                </p>
              </div>
              <Switch
                checked={desktopNotifications && notifPermission === 'granted'}
                disabled={!notificationSupported}
                onCheckedChange={(checked) => void handleToggleDesktopNotifications(checked)}
              />
            </div>

            {notificationSupported && notifPermission === 'denied' && (
              <div className="flex items-start gap-2 rounded-[var(--radius-sm)] border border-[var(--warning)]/30 bg-[var(--warning-soft)]/70 px-3 py-2 text-[12px] text-[var(--ink-secondary)]">
                <Info className="mt-0.5 h-3.5 w-3.5 shrink-0 text-[var(--warning)]" />
                <span>{t('settings.appearance.desktopNotifDenied')}</span>
              </div>
            )}
          </div>
        </div>
      </SectionCard>
    </div>
  )
}
