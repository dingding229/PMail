import type { ComponentType, ReactNode } from 'react'
import { cn } from '@/lib/cn'

export type SectionIconTone = 'accent' | 'neutral' | 'warning' | 'danger' | 'success'

const TONE_CLASSES: Record<SectionIconTone, string> = {
  accent: 'bg-[var(--accent-soft)] text-[var(--accent)]',
  neutral: 'bg-[var(--surface-subtle)] text-[var(--ink-secondary)]',
  warning: 'bg-[var(--warning-soft)] text-[var(--warning)]',
  danger: 'bg-[var(--danger-soft)] text-[var(--danger)]',
  success: 'bg-[var(--success-soft)] text-[var(--success)]',
}

export interface SectionHeaderProps {
  icon?: ComponentType<{ className?: string }>
  iconTone?: SectionIconTone
  title: ReactNode
  titleId?: string
  description?: ReactNode
  badge?: ReactNode
  action?: ReactNode
  className?: string
}

export function SectionHeader({
  icon: Icon,
  iconTone = 'accent',
  title,
  titleId,
  description,
  badge,
  action,
  className,
}: SectionHeaderProps) {
  return (
    <div className={cn('flex flex-wrap items-start justify-between gap-3 sm:flex-nowrap', className)}>
      <div className="flex min-w-0 flex-1 items-start gap-3">
        {Icon && (
          <div
            className={cn(
              'flex h-9 w-9 shrink-0 items-center justify-center rounded-[var(--radius-md)]',
              TONE_CLASSES[iconTone],
            )}
          >
            <Icon className="h-4.5 w-4.5" />
          </div>
        )}
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h2
              id={titleId}
              className="text-[15px] font-semibold tracking-tight text-[var(--ink-primary)]"
            >
              {title}
            </h2>
            {badge}
          </div>
          {description && (
            <p className="mt-0.5 text-[12.5px] leading-relaxed text-[var(--ink-secondary)]">
              {description}
            </p>
          )}
        </div>
      </div>
      {action && <div className="flex shrink-0 flex-wrap items-center gap-2">{action}</div>}
    </div>
  )
}
