import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { BrandLogo } from '@/components/common/BrandLogo'
import { ThemeToggle } from '@/components/common/ThemeToggle'
import { LanguageToggle } from '@/components/common/LanguageToggle'

interface AuthLayoutProps {
  title: string
  subtitle: string
  children: ReactNode
  wide?: boolean
}

export function AuthLayout({ title, subtitle, children, wide = false }: AuthLayoutProps) {
  const { t } = useTranslation()

  return (
    <div className="relative flex min-h-full flex-col bg-[var(--surface-sidebar)] text-[var(--ink-primary)] pt-safe pb-safe">
      {/* Subtle ambient background glow */}
      <div
        aria-hidden="true"
        className="pointer-events-none fixed inset-0 overflow-hidden"
      >
        <div className="absolute -top-32 left-1/2 h-[420px] w-[720px] -translate-x-1/2 rounded-full bg-[var(--accent)] opacity-[0.05] blur-[110px]" />
      </div>

      {/* Top utility bar */}
      <header className="relative z-10 flex items-center justify-between px-5 py-4 sm:px-8">
        <BrandLogo />
        <div className="flex items-center gap-1">
          <LanguageToggle />
          <ThemeToggle />
        </div>
      </header>

      {/* Main centered card */}
      <main className="relative z-10 flex flex-1 items-center justify-center px-4 py-6 sm:px-6">
        <div
          className={
            wide
              ? 'w-full max-w-[460px] rounded-[var(--radius-xl)] border border-[var(--border-subtle)] bg-[var(--surface-elevated)] p-6 shadow-[var(--shadow-md)] sm:p-8'
              : 'w-full max-w-[400px] rounded-[var(--radius-xl)] border border-[var(--border-subtle)] bg-[var(--surface-elevated)] p-6 shadow-[var(--shadow-md)] sm:p-8'
          }
        >
          <div className="mb-6">
            <h1 className="text-[22px] font-semibold tracking-tight text-[var(--ink-primary)]">
              {title}
            </h1>
            <p className="mt-1.5 text-[13.5px] leading-relaxed text-[var(--ink-secondary)]">
              {subtitle}
            </p>
          </div>
          {children}
        </div>
      </main>

      <footer className="relative z-10 py-4 text-center text-[12px] text-[var(--ink-muted)]">
        {t('app.name')} · {t('app.tagline')}
      </footer>
    </div>
  )
}
