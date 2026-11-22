import { useEffect, useRef, type ComponentType, type ReactNode } from 'react'
import { ArrowLeft } from 'lucide-react'
import { cn } from '@/lib/cn'
import { Button } from '@/components/ui/button'
import { ThemeToggle } from '@/components/common/ThemeToggle'
import { LanguageToggle } from '@/components/common/LanguageToggle'
import { ErrorBoundary } from '@/components/common/ErrorBoundary'

export interface PanelTabItem<T extends string = string> {
  key: T
  label: string
  icon: ComponentType<{ className?: string }>
}

interface SectionPanelShellProps<T extends string> {
  title: string
  backLabel: string
  onBack: () => void
  headerActions?: ReactNode
  tabs: PanelTabItem<T>[]
  activeTab: T
  onSelectTab: (key: T) => void
  /** Optional replacement content (e.g. forbidden guard in AdminPage) */
  guardContent?: ReactNode
  children: ReactNode
}

export function SectionPanelShell<T extends string>({
  title,
  backLabel,
  onBack,
  headerActions,
  tabs,
  activeTab,
  onSelectTab,
  guardContent,
  children,
}: SectionPanelShellProps<T>) {
  const activeMobileTabRef = useRef<HTMLButtonElement | null>(null)

  useEffect(() => {
    const prefersReduced =
      typeof window !== 'undefined' &&
      window.matchMedia('(prefers-reduced-motion: reduce)').matches
    activeMobileTabRef.current?.scrollIntoView({
      behavior: prefersReduced ? 'auto' : 'smooth',
      inline: 'center',
      block: 'nearest',
    })
  }, [activeTab])

  return (
    <div className="flex min-h-full flex-col bg-[var(--surface-sidebar)] text-[var(--ink-primary)] pt-safe pb-safe">
      {/* Sticky Top Bar */}
      <header className="sticky top-0 z-20 flex h-14 shrink-0 items-center justify-between gap-2 border-b border-[var(--border-hairline)] bg-[var(--surface-canvas)]/95 px-4 backdrop-blur-md sm:px-6">
        <div className="flex min-w-0 items-center gap-2.5">
          <Button variant="ghost" size="sm" onClick={onBack}>
            <ArrowLeft className="h-4 w-4" />
            <span className="hidden sm:inline">{backLabel}</span>
          </Button>
          <span className="h-4 w-px bg-[var(--border-subtle)]" aria-hidden="true" />
          <h1 className="truncate text-[15px] font-semibold tracking-tight">{title}</h1>
        </div>
        <div className="flex items-center gap-1">
          {headerActions}
          <LanguageToggle />
          <ThemeToggle />
        </div>
      </header>

      {guardContent ? (
        guardContent
      ) : (
        <>
          {/* Mobile horizontal scrollable tab strip (hidden scrollbar + right fade hint + auto scroll active) */}
          <nav
            aria-label={title}
            className="sticky top-14 z-10 border-b border-[var(--border-hairline)] bg-[var(--surface-canvas)] lg:hidden"
          >
            <div className="relative">
              <div className="no-scrollbar flex gap-1.5 overflow-x-auto px-4 py-2 pr-10">
                {tabs.map(({ key, label, icon: Icon }) => {
                  const active = activeTab === key
                  return (
                    <button
                      key={key}
                      ref={active ? activeMobileTabRef : undefined}
                      type="button"
                      aria-current={active ? 'page' : undefined}
                      onClick={() => onSelectTab(key)}
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
          </nav>

          {/* Main Layout */}
          <div className="mx-auto flex w-full max-w-6xl flex-1 gap-6 px-4 py-6 sm:px-6">
            {/* Desktop Sidebar Navigation */}
            <aside className="hidden w-56 shrink-0 lg:block">
              <nav aria-label={title} className="sticky top-20 space-y-0.5">
                {tabs.map(({ key, label, icon: Icon }) => {
                  const active = activeTab === key
                  return (
                    <button
                      key={key}
                      type="button"
                      aria-current={active ? 'page' : undefined}
                      onClick={() => onSelectTab(key)}
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

            {/* Active Panel wrapped in pane-level ErrorBoundary */}
            <main className="min-w-0 flex-1 pb-10">
              <ErrorBoundary inline resetKey={activeTab}>
                {children}
              </ErrorBoundary>
            </main>
          </div>
        </>
      )}
    </div>
  )
}
